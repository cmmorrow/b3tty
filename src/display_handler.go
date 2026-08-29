package src

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed templates/terminal.tmpl
var templ string

// terminalTemplate is parsed once at package init instead of on every
// request. displayTermHandler is the hottest path in the server (every page
// load), so re-parsing this compile-time-embedded, never-changing template
// per-request was pure repeated overhead. template.Must panics at startup
// if the embedded template is malformed, matching the fail-fast pattern
// mustUnmarshalTheme uses for the other embedded, compile-time-known assets.
var terminalTemplate = template.Must(template.New("b3tty").Parse(templ))

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
func buildConfigJSON(srv *Server, clnt *Client, thm *Theme, themeNames []string, allThemeNames []string, builtinThemeNames []string, profileNames []string, activeTheme string, showMenubar string) ([]byte, error) {
	cfg := NewTermConfig(srv, clnt, thm, themeNames, allThemeNames, builtinThemeNames, profileNames, activeTheme, showMenubar)
	return json.Marshal(cfg)
}

// requireSameOrigin writes 403 and returns true when the request carries a
// cross-origin Sec-Fetch-Site header. Callers should return immediately when
// this returns true.
func requireSameOrigin(w http.ResponseWriter, r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		Warnf("%s %s: forbidden: cross-origin request from Sec-Fetch-Site %q", r.Method, r.URL.Path, site)
		w.WriteHeader(http.StatusForbidden)
		return true
	}
	return false
}

// writeJSON sets Content-Type to application/json and JSON-encodes v into w.
// Encode errors are logged at ERROR level prefixed with errContext.
func writeJSON(w http.ResponseWriter, v any, errContext string) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		Errorf("%s response error: %v", errContext, err)
	}
}

// displayTermHandler validates the auth token, selects the active profile, serialises
// the TermConfig to JSON, and renders the terminal HTML template.
func (ts *TerminalServer) displayTermHandler(w http.ResponseWriter, r *http.Request) {
	type TemplateProps struct {
		// ConfigJSON is template.JS, not string: it is a pre-marshaled JSON
		// object literal meant to appear unquoted as raw JS in the page's
		// inline <script> block (window.B3TTY = {{ .ConfigJSON }};). A plain
		// string would be escaped by html/template's JS-context autoescaper
		// as a quoted JS string literal, corrupting the assignment. Every
		// other field here is a plain string specifically so it *does* get
		// autoescaped — Title and ProfileName can contain arbitrary
		// user-supplied text (profile fields set via POST /edit-profile).
		ConfigJSON  template.JS
		Title       string
		ProfileName string
		Nonce       string
		ShowMenubar string
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

	ts.StateMu.RLock()
	firstRun := ts.FirstRun
	ts.StateMu.RUnlock()
	if firstRun {
		Debug("serving first run page....")
		ts.renderSetupPage(w)
		return
	}

	// Gather every piece of shared mutable state this handler needs under a
	// single lock, copying it into local variables before releasing — the rest
	// of the handler (template execution, JSON encoding, response I/O) runs
	// against these local copies only, never against ts fields directly.
	ts.StateMu.Lock()
	ts.ProfileName = resolveProfileName(query, ts.Profiles)
	profileName := ts.ProfileName
	profile := ts.Profiles[profileName]

	themeNames := ts.sortedThemeNames()

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

	thm := ts.Client.Theme
	clientCopy := *ts.Client
	activeTheme := ts.ActiveTheme
	ts.StateMu.Unlock()

	Debugf("resolved profile name: %s", profileName)
	Debugf("Theme names: %s", strings.Join(themeNames, ", "))
	Debugf("All theme names: %s", strings.Join(allThemeNames, ", "))
	Debugf("Profile names: %s", strings.Join(profileNames, ", "))

	cfgJSON, err := buildConfigJSON(ts.Server, &clientCopy, &thm, themeNames, allThemeNames, builtinNames, profileNames, activeTheme, ts.ShowMenubar)
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
	err = terminalTemplate.Execute(w, TemplateProps{ConfigJSON: template.JS(cfgPayload), Title: profile.Title, ProfileName: profileName, Nonce: nonce, ShowMenubar: ts.ShowMenubar})
	if err != nil {
		Errorf("response error: %v", err)
		return
	}
}

// backgroundHandler serves the configured background image file, if any.
// Returns 404 when no background image is configured or the file cannot be found.
func (ts *TerminalServer) backgroundHandler(w http.ResponseWriter, r *http.Request) {
	ts.StateMu.RLock()
	imagePath := ts.Client.Theme.BackgroundImage
	ts.StateMu.RUnlock()
	if imagePath == "" {
		http.NotFound(w, r)
		return
	}
	Debugf("Serving background image %s", imagePath)
	http.ServeFile(w, r, imagePath)
}
