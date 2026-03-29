package src

import (
	"embed"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

const DEFAULT_PROFILE_NAME = "default"
const DEFAULT_COLS = 80
const DEFAULT_ROWS = 24
const BUFFER_SIZE = 4096

//go:embed assets
var assets embed.FS

//go:embed templates/terminal.tmpl
var templ string

//go:embed templates/setup.tmpl
var setupTempl string

//go:embed default_themes/b3tty_dark.json
var defaultDarkThemeJSON []byte

//go:embed default_themes/b3tty_light.json
var defaultLightThemeJSON []byte

// defaultDarkTheme and defaultLightTheme are the colour maps used both to
// update ts.client.Theme in memory (via MapToTheme) and to write the YAML
// config file. Keys use the hyphenated form expected by MapToTheme.
var defaultDarkTheme = mustUnmarshalTheme(defaultDarkThemeJSON)
var defaultLightTheme = mustUnmarshalTheme(defaultLightThemeJSON)

var InitClient *Client
var InitServer *Server
var Profiles map[string]Profile

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
	client         *Client
	server         *Server
	profiles       map[string]Profile
	token          string
	orgCols        uint16
	orgRows        uint16
	profileName    string
	failedAttempts int
	firstRun       bool
	backoffMu      sync.Mutex
	// authSleep is the function used to pause on auth failures. It defaults to
	// time.Sleep and can be replaced in tests with a no-op to avoid real delays.
	authSleep func(time.Duration)
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

// resolveProfileName returns the value of the "profile" query parameter when present,
// or "default" when the parameter is absent or empty.
func resolveProfileName(q url.Values) string {
	if p := q.Get("profile"); p != "" {
		return p
	}
	return DEFAULT_PROFILE_NAME
}

// buildConfigJSON serialises a TermConfig derived from the given server, client, and
// theme into JSON. The returned bytes are ready to embed in the HTML template.
func buildConfigJSON(srv *Server, clnt *Client, thm *Theme) ([]byte, error) {
	cfg := NewTermConfig(srv, clnt, thm)
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

// Serve wires up the HTTP mux and starts the server. It creates a TerminalServer from
// the package-level InitClient, InitServer, and Profiles variables set by the cmd layer.
func Serve(shouldOpenBrowser bool, useTLS bool) {
	Debug("starting b3tty server....")
	ts := &TerminalServer{
		client:      InitClient,
		server:      InitServer,
		profiles:    Profiles,
		orgCols:     DEFAULT_COLS,
		orgRows:     DEFAULT_ROWS,
		profileName: DEFAULT_PROFILE_NAME,
		firstRun:    InitServer.FirstRun,
		authSleep:   time.Sleep,
	}

	var err error
	var tokenQuery = ""
	var protocol = "http"

	if useTLS {
		protocol = "https"
	}

	Debugf("no-auth mode: %v", ts.server.NoAuth)
	if !ts.server.NoAuth {
		ts.token, err = generateToken(24)
		if err != nil {
			Fatalf("error generating token: %v", err)
		}
		tokenQuery = "?token=" + ts.token
	}

	addr := ts.server.Addr().Host
	uiUrl := protocol + "://" + addr + "/" + tokenQuery

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
	if len(ts.profiles) > 1 {
		Info("Configured profiles:")
		var prfQuery string
		if len(tokenQuery) > 0 {
			prfQuery = "&profile="
		} else {
			prfQuery = "?profile="
		}
		// Collect and sort non-default profile names for consistent output.
		names := make([]string, 0, len(ts.profiles)-1)
		for prf := range ts.profiles {
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
			profile := ts.profiles[prf]
			url := uiUrl + prfQuery + prf
			// Pad using the plain name length so ANSI codes in BoldGreen don't
			// inflate the width and break column alignment.
			padding := strings.Repeat(" ", maxLen-len(prf))
			Infof("  %s%s  %s  (shell: %s | dir: %s)", BoldGreen(prf), padding, Bold(url), profile.Shell, profile.WorkingDirectory)
		}
	}

	mux.HandleFunc("/", ts.displayTermHandler)
	mux.Handle("/assets/", http.StripPrefix("/", http.FileServer(http.FS(assets))))
	mux.HandleFunc("/ws", ts.terminalHandler)
	mux.HandleFunc("/size", ts.setSizeHandler)
	mux.HandleFunc("/background", ts.backgroundHandler)
	mux.HandleFunc("/theme", ts.themeHandler)
	mux.HandleFunc("/save-config", ts.saveConfigHandler)
	httpServer := &http.Server{
		Addr:     addr,
		Handler:  mux,
		ErrorLog: NewWarnLogger(),
	}
	Debugf("use TLS: %v", useTLS)
	if useTLS {
		err = httpServer.ListenAndServeTLS(ts.server.CertFilePath, ts.server.KeyFilePath)
	} else {
		err = httpServer.ListenAndServe()
	}
	if err != nil {
		Fatalf("%s server error: %v", protocol, err)
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
	ts.orgCols, ts.orgRows = parseSizeParams(r.URL.Query())
	Debugf("extracted cols: %d", ts.orgCols)
	Debugf("extracted rows: %d", ts.orgRows)
}

// displayTermHandler validates the auth token, selects the active profile, serialises
// the TermConfig to JSON, and renders the terminal HTML template.
func (ts *TerminalServer) displayTermHandler(w http.ResponseWriter, r *http.Request) {
	type Props struct {
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

	if !validateToken(query.Get("token"), ts.token) {
		// Only apply backoff when auth is enabled (token is non-empty). In no-auth
		// mode ts.token is always "" and validateToken always passes, so this branch
		// is only reachable in auth mode — but the guard makes the intent explicit.
		if ts.token != "" {
			Debug("requesting mutex lock")
			ts.backoffMu.Lock()
			ts.failedAttempts++
			delay := authBackoffDelay(ts.failedAttempts)
			ts.backoffMu.Unlock()
			Warnf("%s %s: forbidden: invalid or missing token (attempt %d, delay %s)", r.Method, r.URL.Path, ts.failedAttempts, delay)
			ts.authSleep(delay)
		} else {
			Warnf("%s %s: forbidden: invalid or missing token", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusForbidden)
		return
	}

	Debug("requesting mutex lock")
	ts.backoffMu.Lock()
	ts.failedAttempts = 0
	Debug("requesting mutex unlock")
	ts.backoffMu.Unlock()

	if ts.firstRun {
		ts.renderSetupPage(w)
		return
	}

	Debug("Parsing HTML template....")
	tmpl, err := template.New("b3tty").Parse(templ)
	if err != nil {
		Fatal(err)
	}

	ts.profileName = resolveProfileName(query)
	Debugf("resolved profile name: %s", ts.profileName)
	profile := ts.profiles[ts.profileName]

	thm := ts.client.Theme
	cfgJSON, err := buildConfigJSON(ts.server, ts.client, &thm)
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

	// Content-Security-Policy is set as an HTTP header (not a <meta> tag) so
	// that it is enforced by the browser before any page content is parsed and
	// cannot be modified by injected content.
	//
	// script-src: allow same-origin module scripts plus the one inline script
	//   that sets window.B3TTY, identified by its per-request nonce.
	//   'wasm-unsafe-eval' is required for xterm.js which uses WebAssembly
	//   internally; it is more targeted than 'unsafe-eval' and does not permit
	//   JS eval().
	// style-src:  allow same-origin stylesheets plus 'unsafe-inline' for the
	//   dynamic <style> element the JS injects for theme background gradients.
	// connect-src 'self': covers same-origin fetch and ws:/wss: connections.
	// frame-ancestors 'none': prevents the terminal from being embedded in an
	//   iframe on any other page.
	// base-uri 'self': blocks <base> tag injection that could redirect relative
	//   URLs to an attacker-controlled origin.
	csp := GetCSPHeaders()
	csp.Get("script-src").Add("nonce-" + nonce)
	w.Header().Set("Content-Security-Policy", csp.String())

	cfgPayload := string(cfgJSON)
	Debugf("config response body: %s", cfgPayload)
	Debugf("title: %s", profile.Title)
	Debugf("nonce: %s", nonce)
	err = tmpl.Execute(w, Props{ConfigJSON: cfgPayload, Title: profile.Title, ProfileName: ts.profileName, Nonce: nonce})
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

// themePaletteResponse is the JSON shape returned by themeHandler and consumed
// by the B3ttyThemeSelector component to build palette preview cards.
type themePaletteResponse struct {
	Bg     string   `json:"bg"`
	Fg     string   `json:"fg"`
	SelBg  string   `json:"selBg"`
	Normal []string `json:"normal"`
	Bright []string `json:"bright"`
}

// themeNormalOrder and themeBrightOrder define the ANSI display order used by
// the palette preview in the theme selector component.
var themeNormalOrder = []string{"black", "red", "yellow", "green", "cyan", "blue", "magenta", "white"}
var themeBrightOrder = []string{"bright-black", "bright-red", "bright-yellow", "bright-green", "bright-cyan", "bright-blue", "bright-magenta", "bright-white"}

// themeHandler serves a GET /theme?name=<dark|light> request and returns a
// themePaletteResponse JSON payload shaped for the B3ttyThemeSelector component.
func (ts *TerminalServer) themeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var colors map[string]any
	switch r.URL.Query().Get("name") {
	case "dark":
		colors = defaultDarkTheme
	case "light":
		colors = defaultLightTheme
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	str := func(key string) string {
		v, _ := colors[key].(string)
		return v
	}
	normal := make([]string, len(themeNormalOrder))
	for i, key := range themeNormalOrder {
		normal[i] = str(key)
	}
	bright := make([]string, len(themeBrightOrder))
	for i, key := range themeBrightOrder {
		bright[i] = str(key)
	}

	resp := themePaletteResponse{
		Bg:     str("background"),
		Fg:     str("foreground"),
		SelBg:  str("selection-background"),
		Normal: normal,
		Bright: bright,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		Errorf("theme response error: %v", err)
	}
}

// saveConfigHandler accepts a POST request with a JSON body containing a "theme"
// field ("dark", "light", or "skip"). For dark/light, it writes a default config
// file to $HOME/.config/b3tty/conf.yaml. Sets firstRun to false on success.
func (ts *TerminalServer) saveConfigHandler(w http.ResponseWriter, r *http.Request) {
	if !ts.firstRun {
		http.NotFound(w, r)
		return
	}
	if r.Method != "POST" {
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var themeColors map[string]any
	switch req.Theme {
	case "dark":
		themeColors = defaultDarkTheme
	case "light":
		themeColors = defaultLightTheme
	}

	if themeColors != nil {
		if err := WriteDefaultConfig(req.Theme, themeColors); err != nil {
			Errorf("failed to write config: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		ts.client.Theme.MapToTheme(themeColors)
		Infof("created default %s theme config", req.Theme)
	}

	ts.firstRun = false
	w.WriteHeader(http.StatusOK)
}

// backgroundHandler serves the configured background image file, if any.
// Returns 404 when no background image is configured or the file cannot be found.
func (ts *TerminalServer) backgroundHandler(w http.ResponseWriter, r *http.Request) {
	imagePath := ts.client.Theme.BackgroundImage
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

	profile := ts.profiles[ts.profileName]

	// Start the active profile's shell via /bin/sh -c so that shell flags and
	// paths are handled uniformly regardless of the configured shell binary.
	c := exec.Command("/bin/sh", "-c", profile.Shell)
	c, err = profile.ApplyToCommand(c)
	if err != nil {
		Errorf("apply profile to command: %v", err)
		return
	}

	windowSize := &pty.Winsize{
		Cols: ts.orgCols,
		Rows: ts.orgRows,
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
				break
			}
			if msgType == websocket.TextMessage {
				if cols, rows, ok := parseResizeMessage(message); ok {
					Debugf("resizing to %d, %d", cols, rows)
					pty.Setsize(ptmx, &pty.Winsize{Cols: cols, Rows: rows})
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
		n := 0
		var err error
		for {
			n, err = ptmx.Read(buf)
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
			err = ws.WriteMessage(websocket.BinaryMessage, buf[:n])
			if err != nil {
				Errorf("write from pty: %v", err)
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

	// Keep the connection open
	select {}
}
