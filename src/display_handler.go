package src

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

//go:embed templates/terminal.tmpl
var templ string

const (
	backoffBase = time.Second
	backoffMax  = 30 * time.Second
)

// authBackoffDelay returns the delay to impose after n consecutive failed token
// validations. The delay doubles with each failure (1s, 2s, 4s, …) up to backoffMax.
func authBackoffDelay(n int) time.Duration {
	if n <= 0 {
		return 0
	}
	shift := n - 1
	if shift > 30 {
		return backoffMax
	}
	d := backoffBase << uint(shift)
	return min(d, backoffMax)
}

// parseSizeParams reads "cols" and "rows" from q, falling back to DEFAULT_COLS/DEFAULT_ROWS
// when a value is missing, cannot be parsed as an integer, or falls outside the valid
// uint16 range [0, 65535].
func parseSizeParams(q url.Values) (uint16, uint16) {
	cols, err := strconv.ParseUint(q.Get("cols"), 10, 16)
	if err != nil {
		cols = uint64(DEFAULT_COLS)
	}
	rows, err := strconv.ParseUint(q.Get("rows"), 10, 16)
	if err != nil {
		rows = uint64(DEFAULT_ROWS)
	}
	return uint16(cols), uint16(rows)
}

// resolveProfileName returns the value of the "profile" query parameter when present
// and corresponding to a known profile, or fallback otherwise. fallback should be
// set to ts.StartupProfile so that --profile selections persist across page loads
// that carry no explicit ?profile= query parameter.
func resolveProfileName(q url.Values, profiles map[string]Profile) string {
	if p := q.Get("profile"); p != "" {
		if _, ok := profiles[p]; ok {
			return p
		}
		Warnf("profile %s is not a valid profile name; falling back to profile %s", p, DEFAULT_PROFILE_NAME)
	}
	return DEFAULT_PROFILE_NAME
}

// buildConfigJSON serialises a TermConfig derived from the given server, client, theme,
// and available theme/profile name lists into JSON. The returned bytes are ready to
// embed in the HTML template.
func buildConfigJSON(srv *Server, clnt *Client, thm *Theme, themeNames []string, allThemeNames []string, builtinThemeNames []string, profileNames []string, activeTheme string) ([]byte, error) {
	cfg := NewTermConfig(srv, clnt, thm, themeNames, allThemeNames, builtinThemeNames, profileNames, activeTheme)
	return json.Marshal(cfg)
}

// setSizeHandler accepts a POST request whose query string carries "cols" and "rows",
// storing the parsed values for use when the next pty session is started.
func (ts *TerminalServer) setSizeHandler(w http.ResponseWriter, r *http.Request) {
	Debugf(" %s -> %s %s %s", r.RemoteAddr, r.Host, r.Method, r.URL)
	Debugf("content length: %d", r.ContentLength)
	if r.Method != "POST" {
		Warnf("%s %s: method not allowed: %s", r.Method, r.URL.Path, r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// CSRF protection via Fetch Metadata: browsers attach Sec-Fetch-Site
	// automatically and scripts cannot forge it. Only same-origin fetches (the
	// normal case from terminal.mjs) carry "same-origin"; cross-origin CSRF
	// attempts will carry "cross-site" or "same-site" and are rejected.
	// An absent header indicates a non-browser client (e.g. curl), which is
	// allowed because it cannot be issued by a malicious web page.
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		Warnf("%s %s: forbidden: cross-origin request from Sec-Fetch-Site %q", r.Method, r.URL.Path, site)
		w.WriteHeader(http.StatusForbidden)
		return
	}
	ts.OrgCols, ts.OrgRows = parseSizeParams(r.URL.Query())
	Debugf("extracted cols: %d", ts.OrgCols)
	Debugf("extracted rows: %d", ts.OrgRows)
}

// displayTermHandler validates the auth token, selects the active profile, serialises
// the TermConfig to JSON, and renders the terminal HTML template.
func (ts *TerminalServer) displayTermHandler(w http.ResponseWriter, r *http.Request) {
	type TemplateProps struct {
		ConfigJSON  string
		Title       string
		ProfileName string
		Nonce       string
	}
	Debugf(" %s -> %s %s %s", r.RemoteAddr, r.Host, r.Method, r.URL)
	Debugf("content length: %d", r.ContentLength)

	// The terminal is only served at "/". Anything else that falls through the
	// catch-all mux route (e.g. /favicon.ico, /apple-touch-icon.png fetched
	// automatically by browsers) gets a plain 404 with no auth logic applied,
	// so these browser-initiated probes cannot poison the backoff counter.
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	query := r.URL.Query()

	if !validateToken(query.Get("token"), ts.Token) {
		// Only apply backoff when auth is enabled (token is non-empty). In no-auth
		// mode ts.token is always "" and validateToken always passes, so this branch
		// is only reachable in auth mode — but the guard makes the intent explicit.
		if ts.Token != "" {
			Debug("requesting mutex lock")
			ts.BackoffMu.Lock()
			ts.FailedAttempts++
			delay := authBackoffDelay(ts.FailedAttempts)
			ts.BackoffMu.Unlock()
			Debug("mutex unlocked")
			Warnf("%s %s: forbidden: invalid or missing token (attempt %d, delay %s)", r.Method, r.URL.Path, ts.FailedAttempts, delay)
			ts.AuthSleep(delay)
		} else {
			Warnf("%s %s: forbidden: invalid or missing token", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusForbidden)
		return
	}

	Debug("requesting mutex lock")
	ts.BackoffMu.Lock()
	ts.FailedAttempts = 0
	ts.BackoffMu.Unlock()
	Debug("mutex unlocked")

	if ts.FirstRun {
		Debug("serving first run page....")
		ts.renderSetupPage(w)
		return
	}

	Debug("parsing HTML template....")
	tmpl, err := template.New("b3tty").Parse(templ)
	if err != nil {
		Fatal(err)
	}

	ts.ProfileName = resolveProfileName(query, ts.Profiles)
	Debugf("resolved profile name: %s", ts.ProfileName)
	profile := ts.Profiles[ts.ProfileName]

	themeNames := make([]string, 0, len(ts.Themes))
	for name := range ts.Themes {
		themeNames = append(themeNames, name)
	}
	sort.Strings(themeNames)
	Debugf("Theme names: %s", strings.Join(themeNames, ", "))

	// allThemeNames is the union of built-in and user-defined theme names, used
	// to populate the in-page theme picker.
	allNameSet := make(map[string]struct{})
	var allThemeNames []string
	for name := range builtinThemes {
		if _, seen := allNameSet[name]; !seen {
			allNameSet[name] = struct{}{}
			allThemeNames = append(allThemeNames, name)
		}
	}
	for name := range ts.Themes {
		if _, seen := allNameSet[name]; !seen {
			allNameSet[name] = struct{}{}
			allThemeNames = append(allThemeNames, name)
		}
	}
	sort.Strings(allThemeNames)
	Debugf("All theme names: %s", strings.Join(allThemeNames, ", "))

	builtinNames := make([]string, 0, len(builtinThemes))
	for name := range builtinThemes {
		builtinNames = append(builtinNames, name)
	}
	sort.Strings(builtinNames)

	profileNames := make([]string, 0, len(ts.Profiles))
	for name := range ts.Profiles {
		profileNames = append(profileNames, name)
	}
	sort.Strings(profileNames)
	Debugf("Profile names: %s", strings.Join(profileNames, ", "))

	thm := ts.Client.Theme
	cfgJSON, err := buildConfigJSON(ts.Server, ts.Client, &thm, themeNames, allThemeNames, builtinNames, profileNames, ts.ActiveTheme)
	if err != nil {
		Errorf("config serialization error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	nonce, err := generateToken(16)
	if err != nil {
		Errorf("nonce generation error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	csp := GetCSPHeaders()
	csp.Get("script-src").Add("nonce-" + nonce)
	w.Header().Set("Content-Security-Policy", csp.String())

	cfgPayload := string(cfgJSON)
	Debugf("config response body: %s", cfgPayload)
	Debugf("title: %s", profile.Title)
	Debugf("nonce: %s", nonce)
	err = tmpl.Execute(w, TemplateProps{ConfigJSON: cfgPayload, Title: profile.Title, ProfileName: ts.ProfileName, Nonce: nonce})
	if err != nil {
		Errorf("response error: %v", err)
		return
	}
}

// backgroundHandler serves the configured background image file, if any.
// Returns 404 when no background image is configured or the file cannot be found.
func (ts *TerminalServer) backgroundHandler(w http.ResponseWriter, r *http.Request) {
	imagePath := ts.Client.Theme.BackgroundImage
	if imagePath == "" {
		http.NotFound(w, r)
		return
	}
	Debugf("Serving background image %s", imagePath)
	http.ServeFile(w, r, imagePath)
}
