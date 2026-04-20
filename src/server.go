package src

import (
	"context"
	"embed"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// const DEFAULT_COLS = 80
// const DEFAULT_ROWS = 24

//go:embed assets
var assets embed.FS

//go:embed templates/terminal.tmpl
var templ string

//go:embed templates/setup.tmpl
var setupTempl string

//go:embed templates/theme-select.tmpl
var themeSelectTempl string

//go:embed default_themes/b3tty_dark.json
var defaultDarkThemeJSON []byte

//go:embed default_themes/b3tty_light.json
var defaultLightThemeJSON []byte

//go:embed default_themes/catppuccin-mocha.json
var catppuccinMochaThemeJSON []byte

//go:embed default_themes/catppuccin-latte.json
var catppuccinLatteThemeJSON []byte

//go:embed default_themes/solarized-dark.json
var solarizedDarkThemeJSON []byte

//go:embed default_themes/solarized-light.json
var solarizedLightThemeJSON []byte

//go:embed default_themes/tokyo-night.json
var tokyoNightThemeJSON []byte

//go:embed default_themes/dracula.json
var draculaThemeJSON []byte

//go:embed default_themes/one-light.json
var oneLightThemeJSON []byte

//go:embed default_themes/gruvbox-light.json
var gruvboxLightThemeJSON []byte

// defaultDarkTheme and defaultLightTheme are the color maps used both to
// update ts.client.Theme in memory (via MapToTheme) and to write the YAML
// config file. Keys use the hyphenated form expected by MapToTheme.
var defaultDarkTheme = mustUnmarshalTheme(defaultDarkThemeJSON)
var defaultLightTheme = mustUnmarshalTheme(defaultLightThemeJSON)

// builtinThemes maps each built-in theme name to its color map. All entries
// are available via themePaletteHandler and are registered into ts.Themes at
// startup so the menu bar can switch between them without any conf.yaml entry.
var builtinThemes = map[string]map[string]any{
	"b3tty-dark":       defaultDarkTheme,
	"b3tty-light":      defaultLightTheme,
	"catppuccin-mocha": mustUnmarshalTheme(catppuccinMochaThemeJSON),
	"catppuccin-latte": mustUnmarshalTheme(catppuccinLatteThemeJSON),
	"solarized-dark":   mustUnmarshalTheme(solarizedDarkThemeJSON),
	"solarized-light":  mustUnmarshalTheme(solarizedLightThemeJSON),
	"tokyo-night":      mustUnmarshalTheme(tokyoNightThemeJSON),
	"dracula":          mustUnmarshalTheme(draculaThemeJSON),
	"one-light":        mustUnmarshalTheme(oneLightThemeJSON),
	"gruvbox-light":    mustUnmarshalTheme(gruvboxLightThemeJSON),
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:    BUFFER_SIZE,
	WriteBufferSize:   BUFFER_SIZE,
	EnableCompression: false,
	// CheckOrigin rejects cross-origin WebSocket upgrade requests. An absent
	// Origin header (non-browser clients) is allowed; any browser-sent Origin
	// must match the Host the browser used to reach this server, preventing
	// third-party pages from silently opening terminal connections.
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return u.Host == r.Host
	},
}

// TerminalServer bundles all mutable per-session state used by the HTTP handlers,
// making them independent of package-level globals and straightforward to test.
type TerminalServer struct {
	Client         *Client
	Server         *Server
	Profiles       map[string]Profile
	Themes         map[string]Theme
	Token          string
	OrgCols        uint16
	OrgRows        uint16
	ProfileName    string
	StartupProfile string
	ActiveTheme    string
	FailedAttempts int
	FirstRun       bool
	BackoffMu      sync.Mutex
	// AuthSleep is the function used to pause on auth failures. It defaults to
	// time.Sleep and can be replaced in tests with a no-op to avoid real delays.
	AuthSleep func(time.Duration)
}

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
	cols, err := strconv.Atoi(q.Get("cols"))
	if err != nil || !validateTerminalDimension(cols) {
		cols = DEFAULT_COLS
	}
	rows, err := strconv.Atoi(q.Get("rows"))
	if err != nil || !validateTerminalDimension(rows) {
		rows = DEFAULT_ROWS
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
func buildConfigJSON(srv *Server, clnt *Client, thm *Theme, themeNames []string, allThemeNames []string, profileNames []string, activeTheme string) ([]byte, error) {
	cfg := NewTermConfig(srv, clnt, thm, themeNames, allThemeNames, profileNames, activeTheme)
	return json.Marshal(cfg)
}

// parseResizeMessage tries to unmarshal message as a JSON resize command of the form
// {"type":"resize","cols":N,"rows":N}. On success it returns (cols, rows, true).
// Any parse failure or a non-"resize" type returns (0, 0, false).
func parseResizeMessage(message []byte) (uint16, uint16, bool) {
	var msg struct {
		Type string `json:"type"`
		Cols uint16 `json:"cols"`
		Rows uint16 `json:"rows"`
	}
	if err := json.Unmarshal(message, &msg); err == nil && msg.Type == "resize" {
		return msg.Cols, msg.Rows, true
	}
	return 0, 0, false
}

// formatCommand trims surrounding whitespace from command and appends a newline,
// producing bytes ready to write directly to a pty.
func formatCommand(command string) []byte {
	return []byte(strings.TrimSpace(command) + "\n")
}

// GetCSPHeaders returns the baseline Content-Security-Policy directives used by
// b3tty's HTTP handlers. The returned CSPHeaders value contains the following
// directives:
//
//   - default-src 'none'
//   - script-src 'self' 'wasm-unsafe-eval' (callers must add a per-request nonce)
//   - style-src 'self' 'unsafe-inline'
//   - connect-src 'self'
//   - img-src 'self'
//   - frame-ancestors 'none'
//   - base-uri 'self'
//
// script-src: allow same-origin module scripts plus the one inline script
//
//	that sets window.B3TTY, identified by its per-request nonce.
//	'wasm-unsafe-eval' is required for xterm.js which uses WebAssembly
//	internally; it is more targeted than 'unsafe-eval' and does not permit
//	JS eval().
//
// style-src:  allow same-origin stylesheets plus 'unsafe-inline' for the
//
//	dynamic <style> element the JS injects for theme background gradients.
//
// connect-src 'self': covers same-origin fetch and ws:/wss: connections.
// frame-ancestors 'none': prevents the terminal from being embedded in an
//
//	iframe on any other page.
//
// base-uri 'self': blocks <base> tag injection that could redirect relative
//
//	URLs to an attacker-controlled origin.
//
// Callers that render HTML (e.g. displayTermHandler) should call
// csp.Get("script-src").Add("nonce-<value>") on the returned value to inject a
// per-request nonce before writing the CSP header to the response.
func GetCSPHeaders() CSPHeaders {
	header := NewCSPHeders(
		NewCSPHeader("default-src", "none"),
		NewCSPHeader("script-src", "self", "wasm-unsafe-eval"),
		NewCSPHeader("style-src", "self", "unsafe-inline"),
		NewCSPHeader("connect-src", "self"),
		NewCSPHeader("img-src", "self"),
		NewCSPHeader("frame-ancestors", "none"),
		NewCSPHeader("base-uri", "self"),
	)
	return *header
}

// logProfileURLs prints a header and one line per non-default profile, showing its
// URL, shell, and working directory. The query separator is derived from uiUrl itself:
// "&profile=" when uiUrl already contains a "?", "?profile=" otherwise. This correctly
// handles the case where uiUrl already carries a ?profile= from a --profile startup flag.
func logProfileURLs(profiles map[string]Profile, uiUrl string) {
	Info("Configured profiles:")
	var prfQuery string
	if strings.Contains(uiUrl, "?") && !strings.Contains(uiUrl, "?profile") && !strings.Contains(uiUrl, "&profile") {
		prfQuery = "&profile="
	} else if !strings.Contains(uiUrl, "?") {
		prfQuery = "?profile="
	} else {
		prfQuery = ""
	}

	// Collect and sort non-default profile names for consistent output.
	names := make([]string, 0, len(profiles)-1)
	for prf := range profiles {
		if prf != DEFAULT_PROFILE_NAME {
			names = append(names, prf)
		}
	}
	sort.Strings(names)
	// Compute max name width for aligned columns.
	maxLen := 0
	for _, prf := range names {
		if len(prf) > maxLen {
			maxLen = len(prf)
		}
	}
	for _, prf := range names {
		profile := profiles[prf]
		var url string
		if len(prfQuery) > 0 {
			url = uiUrl + prfQuery + prf
		} else {
			url = uiUrl
		}

		// Pad using the plain name length so ANSI codes in BoldGreen don't
		// inflate the width and break column alignment.
		padding := strings.Repeat(" ", maxLen-len(prf))
		Infof("  %s%s  %s  (shell: %s | dir: %s)", BoldGreen(prf), padding, Bold(url), profile.Shell, profile.WorkingDirectory)
	}
}

// buildUIUrl assembles the URL printed at startup and optionally opened in the browser.
// tokenQuery is either "?token=<tok>" (auth enabled) or "" (no-auth mode).
// When startupProfile differs from DEFAULT_PROFILE_NAME the profile query parameter is
// appended using "&" when a token is already present, or "?" otherwise.
func buildUIUrl(protocol, addr, tokenQuery, startupProfile string) string {
	url := protocol + "://" + addr + "/" + tokenQuery
	if startupProfile != DEFAULT_PROFILE_NAME {
		if tokenQuery != "" {
			url += "&profile=" + startupProfile
		} else {
			url += "?profile=" + startupProfile
		}
	}
	return url
}

// Serve wires up the HTTP mux and starts the server. It creates a TerminalServer from
// the package-level InitClient, InitServer, and Profiles variables set by the cmd layer.
func Serve(ts *TerminalServer, shouldOpenBrowser bool, useTLS bool) {
	Debug("starting b3tty server....")

	var err error
	var tokenQuery = ""
	var protocol = "http"

	if useTLS {
		protocol = "https"
	}

	Debugf("no-auth mode: %v", ts.Server.NoAuth)
	if !ts.Server.NoAuth {
		ts.Token, err = generateToken(TOKEN_LENGTH)
		if err != nil {
			Fatalf("error generating token: %v", err)
		}
		tokenQuery = "?token=" + ts.Token
	}

	addr := ts.Server.Addr().Host
	uiUrl := buildUIUrl(protocol, addr, tokenQuery, ts.StartupProfile)

	Debugf("open-browser on start up: %v", shouldOpenBrowser)
	if shouldOpenBrowser {
		err = openBrowser(uiUrl)
		if err != nil {
			Fatal("failed to open default browser")
		}
	}

	mux := http.NewServeMux()
	Infof("%s server started on %s", protocol, Bold(uiUrl))

	// Display the available profiles in the config file
	if len(ts.Profiles) > 1 {
		logProfileURLs(ts.Profiles, uiUrl)
	}

	mux.HandleFunc("/", ts.displayTermHandler)
	mux.Handle("/assets/", http.StripPrefix("/", http.FileServer(http.FS(assets))))
	mux.HandleFunc("/ws", ts.terminalHandler)
	mux.HandleFunc("/size", ts.setSizeHandler)
	mux.HandleFunc("/background", ts.backgroundHandler)
	mux.HandleFunc("/theme", ts.themePaletteHandler)
	mux.HandleFunc("/theme-config", ts.themeConfigHandler)
	mux.HandleFunc("/theme-select", ts.themeSelectHandler)
	mux.HandleFunc("/add-theme", ts.addThemeHandler)
	mux.HandleFunc("/save-config", ts.saveConfigHandler)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ErrorLog:     NewWarnLogger(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		Debugf("use TLS: %v", useTLS)
		if useTLS {
			serverErr <- httpServer.ListenAndServeTLS(ts.Server.CertFilePath, ts.Server.KeyFilePath)
		} else {
			serverErr <- httpServer.ListenAndServe()
		}
	}()

	select {
	case err = <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			Fatalf("%s server error: %v", protocol, err)
		}
	case sig := <-quit:
		Infof("received signal %v, shutting down...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err = httpServer.Shutdown(ctx); err != nil {
			Fatalf("server shutdown error: %v", err)
		}
	}
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

	profileNames := make([]string, 0, len(ts.Profiles))
	for name := range ts.Profiles {
		profileNames = append(profileNames, name)
	}
	sort.Strings(profileNames)
	Debugf("Profile names: %s", strings.Join(profileNames, ", "))

	thm := ts.Client.Theme
	cfgJSON, err := buildConfigJSON(ts.Server, ts.Client, &thm, themeNames, allThemeNames, profileNames, ts.ActiveTheme)
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

// renderSetupPage renders the theme selection setup page.
func (ts *TerminalServer) renderSetupPage(w http.ResponseWriter) {
	csp := GetCSPHeaders()
	w.Header().Set("Content-Security-Policy", csp.String())

	tmpl, err := template.New("setup").Parse(setupTempl)
	if err != nil {
		Fatal(err)
	}
	if err = tmpl.Execute(w, nil); err != nil {
		Errorf("setup response error: %v", err)
	}
}

// themeSelectHandler renders the full-page theme picker for authenticated users.
// GET /theme-select?token=<tok>&profile=<name>
func (ts *TerminalServer) themeSelectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		Warnf("%s %s: method not allowed: %s", r.Method, r.URL.Path, r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	if !validateToken(query.Get("token"), ts.Token) {
		Warnf("%s %s: forbidden: invalid or missing token", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if ts.FirstRun {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Security-Policy", GetCSPHeaders().String())

	tmpl, err := template.New("theme-select").Parse(themeSelectTempl)
	if err != nil {
		Fatal(err)
	}
	Debug("loading theme-select over-panel")
	if err = tmpl.Execute(w, nil); err != nil {
		Errorf("theme-select response error: %v", err)
	}
}

// addThemeHandler applies a chosen theme and persists it to conf.yaml.
// POST /add-theme  body: {"theme":"<name>"}
func (ts *TerminalServer) addThemeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		Warnf("%s %s: method not allowed: %s", r.Method, r.URL.Path, r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		Warnf("%s %s: forbidden: cross-origin request from Sec-Fetch-Site %q", r.Method, r.URL.Path, site)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQUEST_BODY_SIZE)
	var req struct {
		Theme string `json:"theme"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Theme == "" {
		Warnf("%s %s: bad request: missing or invalid theme", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Resolve theme colors and ensure the theme is in ts.Themes.
	var colors map[string]any
	if builtinColors, ok := builtinThemes[req.Theme]; ok {
		colors = builtinColors
		if ts.Themes == nil {
			ts.Themes = make(map[string]Theme)
		}
		if _, exists := ts.Themes[req.Theme]; !exists {
			var t Theme
			t.MapToTheme(colors)
			ts.Themes[req.Theme] = t
		}
	} else if theme, ok := ts.Themes[req.Theme]; ok {
		colors = theme.toColorMap()
	} else {
		Warnf("%s %s: unknown theme %q", r.Method, r.URL.Path, req.Theme)
		http.NotFound(w, r)
		return
	}

	ts.Client.Theme = ts.Themes[req.Theme]
	ts.ActiveTheme = req.Theme

	if err := UpdateThemeInConfig(req.Theme, colors); err != nil {
		Errorf("add-theme: failed to update config: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	Debugf("added theme %q", req.Theme)
	themeNames := make([]string, 0, len(ts.Themes))
	for name := range ts.Themes {
		themeNames = append(themeNames, name)
	}
	sort.Strings(themeNames)
	resp := themeConfigResponse{
		Theme:              ts.Client.Theme,
		HasBackgroundImage: ts.Client.Theme.BackgroundImage != "",
		ThemeNames:         themeNames,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		Errorf("add-theme response error: %v", err)
	}
}

// themePaletteHandler serves a GET /theme?name=<name> request for any built-in or
// user-defined theme and returns a themePaletteResponse JSON payload shaped for the
// theme selector components.
func (ts *TerminalServer) themePaletteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		Warnf("%s %s: method not allowed: %s", r.Method, r.URL.Path, r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	name := r.URL.Query().Get("name")
	colors, ok := builtinThemes[name]
	if !ok {
		theme, ok := ts.Themes[name]
		if !ok {
			Warnf("unknown theme name: %q", name)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		colors = theme.toColorMap()
	}

	normalOrder := []string{"black", "red", "yellow", "green", "cyan", "blue", "magenta", "white"}
	brightOrder := []string{"bright-black", "bright-red", "bright-yellow", "bright-green", "bright-cyan", "bright-blue", "bright-magenta", "bright-white"}

	str := func(key string) string {
		v, _ := colors[key].(string)
		return v
	}
	normal := make([]string, len(normalOrder))
	for i, key := range normalOrder {
		normal[i] = str(key)
	}
	bright := make([]string, len(brightOrder))
	for i, key := range brightOrder {
		bright[i] = str(key)
	}

	resp := themePaletteResponse{
		Bg:     str("background"),
		Fg:     str("foreground"),
		SelBg:  str("selection-background"),
		Cursor: str("cursor"),
		Normal: normal,
		Bright: bright,
	}
	buf, err := json.Marshal(resp)
	if err != nil {
		Errorf("theme response error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(buf)
}

// themeConfigHandler serves GET and POST /theme-config?name=<themename>.
//
// GET returns the full theme color config and hasBackgroundImage flag without
// side effects.
//
// POST additionally activates the named theme on the server (updating
// ts.client.Theme) so that the /background endpoint immediately serves the
// correct image and subsequent page loads receive the new theme. POST requires
// a same-origin Sec-Fetch-Site header to prevent CSRF.
func (ts *TerminalServer) themeConfigHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET", "POST":
	default:
		Warnf("%s %s: method not allowed: %s", r.Method, r.URL.Path, r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Method == "POST" {
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
			Warnf("%s %s: forbidden: cross-origin request from Sec-Fetch-Site %q", r.Method, r.URL.Path, site)
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		Warnf("%s %s: bad request: missing name parameter", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	theme, ok := ts.Themes[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method == "POST" {
		ts.Client.Theme = theme
		ts.ActiveTheme = name
		var colors map[string]any
		if builtinColors, ok := builtinThemes[name]; ok {
			colors = builtinColors
		} else {
			colors = theme.toColorMap()
		}
		if err := UpdateThemeInConfig(name, colors); err != nil {
			Errorf("theme-config: failed to update config: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	resp := themeConfigResponse{
		Theme:              theme,
		HasBackgroundImage: theme.BackgroundImage != "",
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		Errorf("theme-config response error: %v", err)
	}
}

// saveConfigHandler accepts a POST request with a JSON body containing a "theme"
// field ("b3tty-dark", "b3tty-light", or "skip"). For b3tty-dark/b3tty-light, it writes a default config
// file to $HOME/.config/b3tty/conf.yaml. Sets firstRun to false on success.
func (ts *TerminalServer) saveConfigHandler(w http.ResponseWriter, r *http.Request) {
	if !ts.FirstRun {
		http.NotFound(w, r)
		return
	}
	if r.Method != "POST" {
		Warnf("%s %s: method not allowed: %s", r.Method, r.URL.Path, r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		Warnf("%s %s: forbidden: cross-origin request from Sec-Fetch-Site %q", r.Method, r.URL.Path, site)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	var req struct {
		Theme string `json:"theme"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, MAX_REQUEST_BODY_SIZE)).Decode(&req); err != nil {
		Warn("request body size exceeding limit")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var themeColors map[string]any
	switch req.Theme {
	case "b3tty-dark":
		themeColors = defaultDarkTheme
	case "b3tty-light":
		themeColors = defaultLightTheme
	}

	if themeColors != nil {
		Debug("writing config file....")
		if err := WriteDefaultConfig(req.Theme, themeColors); err != nil {
			Errorf("failed to write config: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		ts.Client.Theme.MapToTheme(themeColors)
		Infof("created default %s theme config", req.Theme)
	}

	ts.FirstRun = false
	w.WriteHeader(http.StatusOK)
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

// terminalHandler upgrades the HTTP connection to a WebSocket, starts the
// active profile's shell under a pty sized to the dimensions stored by
// setSizeHandler, then runs two goroutines bridging pty output → WebSocket
// and WebSocket input → pty. A done channel coordinated with sync.Once lets
// the input goroutine distinguish a clean PTY-initiated shutdown from an
// unexpected WebSocket error.
func (ts *TerminalServer) terminalHandler(w http.ResponseWriter, r *http.Request) {
	Debugf(" %s -> %s %s %s", r.RemoteAddr, r.Host, r.Method, r.URL)
	Debugf("content length: %d", r.ContentLength)
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		Errorf("upgrader error: %v", err)
		return
	}
	defer ws.Close()

	profile := ts.Profiles[ts.ProfileName]

	// Start the active profile's shell via /bin/sh -c so that shell flags and
	// paths are handled uniformly regardless of the configured shell binary.
	c := exec.Command("/bin/sh", "-c", profile.Shell)
	c, err = profile.ApplyToCommand(c)
	if err != nil {
		Errorf("apply profile to command: %v", err)
		return
	}

	windowSize := &pty.Winsize{
		Cols: ts.OrgCols,
		Rows: ts.OrgRows,
	}

	Debugf("cols: %d", windowSize.Cols)
	Debugf("rows: %d", windowSize.Rows)
	Debug("starting pty....")
	ptmx, err := pty.StartWithSize(c, windowSize)
	if err != nil {
		Errorf("start pty: %v", err)
		return
	}

	defer func() { _ = ptmx.Close() }() // Best effort.

	// done is closed by the PTY output goroutine just before it calls
	// ws.Close(). This lets the WebSocket input goroutine distinguish a
	// planned shutdown (PTY exited) from a genuine unexpected error.
	done := make(chan struct{})
	var closeOnce sync.Once
	signalDone := func() { closeOnce.Do(func() { close(done) }) }

	// Handle input from the WebSocket
	go func() {
		for {
			msgType, message, err := ws.ReadMessage()
			if err != nil {
				select {
				case <-done:
					// ws.Close() was called by us after the PTY exited — not an error.
					Warn("websocket closed after terminal session ended")
				default:
					switch err.(type) {
					case *websocket.CloseError:
						Warn("cannot read from websocket: unexpectedly closed")
					default:
						Errorf("websocket read: %v", err)
					}
				}
				ptmx.Close()
				break
			}
			if msgType == websocket.TextMessage {
				if cols, rows, ok := parseResizeMessage(message); ok {
					if cols == 0 || rows == 0 {
						Warnf("ignoring resize to invalid dimensions: cols=%d rows=%d", cols, rows)
						continue
					}
					Debugf("resizing to %d, %d", cols, rows)
					err = pty.Setsize(ptmx, &pty.Winsize{Cols: cols, Rows: rows})
					if err != nil {
						Errorf("error calling pty resize: %v", err)
					}
					continue
				}
			}
			_, err = ptmx.Write(message)
			if err != nil {
				Errorf("write to pty: %v", err)
				ptmx.Close()
				break
			}
		}
	}()

	// Handle output from the pty
	go func() {
		buf := make([]byte, BUFFER_SIZE)
		for {
			n, err := ptmx.Read(buf)
			Debugf("bytes read from buffer: %d", n)
			if err != nil {
				switch err {
				case io.EOF:
					Info("terminal session closed")
				default:
					Errorf("pty read: %v", err)
				}
				ptmx.Close()
				signalDone()
				ws.Close()
				return
			}
			ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err = ws.WriteMessage(websocket.BinaryMessage, buf[:n])
			if err != nil {
				Errorf("write from pty: %v", err)
				ptmx.Close()
				signalDone()
				break
			}
		}
	}()

	if len(profile.Commands) > 0 {
		time.Sleep(time.Second * 1)
		for _, command := range profile.Commands {
			_, err = ptmx.Write(formatCommand(command))
			if err != nil {
				Errorf("write to pty: %v", err)
				ptmx.Close()
				return
			}
			time.Sleep(time.Millisecond * 200)
		}
	}

	// Wait for the PTY session to end. The output goroutine closes done
	// when the PTY exits; waiting here lets the deferred ws.Close() and
	// ptmx.Close() run on exit rather than leaking this goroutine forever.
	<-done
}
