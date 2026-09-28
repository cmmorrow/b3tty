package src

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
func buildConfigJSON(srv *Server, clnt *TerminalClient, thm *Theme, themeNames []string, allThemeNames []string, builtinThemeNames []string, profileNames []string, activeTheme string, showMenubar string) ([]byte, error) {
	cfg := NewTermConfig(srv, clnt, thm, themeNames, allThemeNames, builtinThemeNames, profileNames, activeTheme, showMenubar)
	return json.Marshal(cfg)
}

// requireToken wraps an API handler so it only runs when the request carries
// the server's auth token as an "Authorization: Bearer <token>" header,
// answering 403 otherwise. The same-origin check (requireSameOrigin) only stops
// other websites; without the token, any program able to reach the port could
// call the API — e.g. set a profile's shell to an arbitrary command, or turn
// off authentication for the next start via POST /settings. In no-auth mode
// ts.Token is empty and every request passes.
func (ts *TerminalServer) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, isBearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !isBearer {
			token = ""
		}
		if !validateToken(token, ts.Token) {
			Warnf("%s %s: forbidden: invalid or missing token", r.Method, r.URL.Path)
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		next(w, r)
	}
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
	Debugf(" %s -> %s %s %s", r.RemoteAddr, r.Host, r.Method, redactedURL(r.URL))
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

	thm := ts.Theme
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

// backgroundHandler serves the background image of the theme named by the
// "theme" query parameter, or of the active theme when it is absent. Naming the
// theme gives each one its own URL: browsers reuse an image already loaded by
// the page for the same URL without making a request at all, whatever the cache
// headers say, so a single shared URL made every theme show whichever image the
// page loaded first. Like the page itself, the request must carry the server token. The configured path has a
// leading "~" expanded and must then be absolute; it is re-checked on every
// request, since the file can change while the server runs. Every non-200
// response is logged at warning level so a misconfigured image is never silent:
//   - 403 when the token is missing or wrong
//   - 404 when no image is configured, or the path is relative, missing, a
//     directory, or unreadable
//   - 415 when the file is not an allowed image type (see validateBackgroundImage)
//
// A served image is sent with "Cache-Control: no-cache" and an ETag identifying
// the exact file (see backgroundImageETag), so the browser revalidates on every
// load and reuses its cached copy only while the same file is configured.
// Changing the path, switching to a theme with a different image, or editing the
// file changes the ETag; a path that has since gone bad gets its 404/415 instead
// of the stale cached image.
func (ts *TerminalServer) backgroundHandler(w http.ResponseWriter, r *http.Request) {
	if !validateToken(r.URL.Query().Get("token"), ts.Token) {
		rejectBackground(w, r, http.StatusForbidden, "invalid or missing token")
		return
	}

	themeName := r.URL.Query().Get("theme")
	ts.StateMu.RLock()
	configured := ts.Theme.BackgroundImage
	if themeName != "" {
		// Built-in themes never carry a background image, so a theme missing
		// from ts.Themes has none either.
		configured = ts.Themes[themeName].BackgroundImage
	}
	ts.StateMu.RUnlock()
	if configured == "" {
		reason := "the active theme has no background image configured"
		if themeName != "" {
			reason = fmt.Sprintf("theme %q has no background image configured", themeName)
		}
		rejectBackground(w, r, http.StatusNotFound, reason)
		return
	}

	imagePath, err := resolveBackgroundImage(configured)
	if err != nil {
		status := http.StatusNotFound
		if errors.Is(err, errUnsupportedBackgroundImage) {
			status = http.StatusUnsupportedMediaType
		}
		rejectBackground(w, r, status, err.Error())
		return
	}

	f, err := os.Open(imagePath)
	if err != nil {
		rejectBackground(w, r, http.StatusNotFound, fmt.Sprintf("background image: %v", err))
		return
	}
	defer f.Close()
	// The ETag describes the opened file itself, so it matches the bytes served.
	info, err := f.Stat()
	if err != nil {
		rejectBackground(w, r, http.StatusNotFound, fmt.Sprintf("background image: %v", err))
		return
	}

	Debugf("Serving background image %s", imagePath)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", backgroundImageETag(imagePath, info))
	// A zero modtime keeps ServeContent from sending Last-Modified, leaving the
	// ETag as the only validator. Otherwise a browser's If-Modified-Since for a
	// previously configured, newer file could draw a 304 for a different image.
	http.ServeContent(w, r, filepath.Base(imagePath), time.Time{}, f)
}

// backgroundImageETag returns a strong ETag identifying the file at path by its
// resolved path, size, and modification time, so it changes whenever a different
// file is configured or the configured file is rewritten.
func backgroundImageETag(path string, info fs.FileInfo) string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s|%d|%d", path, info.Size(), info.ModTime().UnixNano())
	return fmt.Sprintf(`"%x"`, h.Sum64())
}

// rejectBackground logs why a /background request failed at warning level and
// writes status in place of the image. r.URL.Path excludes the query string, so
// the token is never logged.
func rejectBackground(w http.ResponseWriter, r *http.Request, status int, reason string) {
	Warnf("%s %s: %d %s: %s", r.Method, r.URL.Path, status, http.StatusText(status), reason)
	http.Error(w, http.StatusText(status), status)
}
