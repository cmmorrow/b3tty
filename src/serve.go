package src

import (
	"context"
	"embed"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed assets
var assets embed.FS

// dist holds bun's bundled/code-split client build output (terminal.min.js,
// xterm.min.css, and the dynamically-imported *.chunk.js files), generated
// by `make client` into src/dist — kept separate from assets (hand-authored
// static files like favicon.ico and terminal.css) so that directory can be
// committed cleanly without build-artifact churn. src/dist is gitignored;
// `make client` is a prerequisite of test/test-race/build precisely because
// this directive requires the directory to exist at compile time.
//
//go:embed dist
var dist embed.FS

// TerminalServer bundles all mutable per-session state used by the HTTP handlers,
// making them independent of package-level globals and straightforward to test.
type TerminalServer struct {
	Client         *TerminalClient
	Theme          Theme
	Server         *Server
	Profiles       map[string]Profile
	Themes         map[string]Theme
	Token          string
	ProfileName    string
	StartupProfile string
	ActiveTheme    string
	ConfigFile     string
	FailedAttempts int
	FirstRun       bool
	NoBrowser      bool
	ShowMenubar    string
	BackoffMu      sync.Mutex
	WSClients      map[*websocket.Conn]*wsClient
	WSClientsMu    sync.Mutex
	// StateMu guards every field above that is read or written by more than one
	// HTTP handler after startup: Client, Theme, Profiles, Themes, ActiveTheme,
	// and ProfileName. Server, ConfigFile, StartupProfile, NoBrowser, and
	// ShowMenubar are set once before Serve() starts accepting requests and
	// never mutated afterward, so they do not need to be guarded. Callers
	// should hold StateMu only long enough to read or mutate state into local
	// variables — never across template execution, JSON encoding, or other
	// response I/O.
	StateMu sync.RWMutex
	// AuthSleep is the function used to pause on auth failures. It defaults to
	// time.Sleep and can be replaced in tests with a no-op to avoid real delays.
	AuthSleep func(time.Duration)
	// CommandSleep is the function used to pause before and between writing a
	// profile's startup commands to the pty. It defaults to time.Sleep and can
	// be replaced in tests with a no-op to avoid real delays.
	CommandSleep func(time.Duration)
	// StartTime is captured as close to process start as achievable (see the
	// package-level startTime var in cmd/root.go) and set once before Serve()
	// starts accepting requests; never mutated afterward.
	StartTime time.Time
	// WSEstablishedOnce guards the one-time debug log of how long the server
	// took, from StartTime, to reach its first established WebSocket+pty
	// session — see terminalHandler.
	WSEstablishedOnce sync.Once
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
// URL, shell, and working directory. Each profile's URL is built fresh via
// srv.buildUIUrl so it always carries exactly that profile's query parameter,
// regardless of what ts.StartupProfile was at server startup.
func logProfileURLs(srv *Server, profiles map[string]Profile, token string) {
	Info("Configured profiles:")

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
		url := srv.buildUIUrl(token, prf)

		// Pad using the plain name length so ANSI codes in BoldGreen don't
		// inflate the width and break column alignment.
		padding := strings.Repeat(" ", maxLen-len(prf))
		Infof("  %s%s  %s  (shell: %s | dir: %s)", BoldGreen(prf), padding, Bold(url), profile.Shell, profile.WorkingDirectory)
	}
}

// buildUIUrl assembles the URL printed at startup and optionally opened in the browser.
// token is the auth token, or "" in no-auth mode. When startupProfile differs from
// DEFAULT_PROFILE_NAME the profile query parameter is also included.
func (s *Server) buildUIUrl(token, startupProfile string) string {
	u := s.URL
	u.Path = "/"
	q := url.Values{}
	if token != "" {
		q.Set("token", token)
	}
	if startupProfile != DEFAULT_PROFILE_NAME {
		q.Set("profile", startupProfile)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// apiRoutes lists the JSON API endpoints, all of which require the auth token
// as an "Authorization: Bearer" header (see requireToken). Kept as data so a
// test can check that every one of them is protected.
func (ts *TerminalServer) apiRoutes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/theme":          ts.themePaletteHandler,
		"/theme-config":   ts.themeConfigHandler,
		"/add-theme":      ts.addThemeHandler,
		"/edit-theme":     ts.editThemeHandler,
		"/save-config":    ts.saveConfigHandler,
		"/profile-config": ts.profileConfigHandler,
		"/edit-profile":   ts.editProfileHandler,
		"/delete-profile": ts.deleteProfileHandler,
		"/settings":       ts.settingsHandler,
	}
}

// newMux registers every route. The token is required everywhere except the
// static bundles under /assets/ and /dist/, which are the same public client
// code for every user: the page (/), the WebSocket (/ws), and the background
// image (/background) take it as a ?token= query parameter, since an <img>,
// CSS url(), or WebSocket can't set headers, and check it themselves; the API
// routes take it as a header via requireToken.
func (ts *TerminalServer) newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", ts.displayTermHandler)
	mux.HandleFunc("/assets/", assetsHandler)
	mux.HandleFunc("/dist/", distHandler)
	mux.HandleFunc("/ws", ts.terminalHandler)
	mux.HandleFunc("/background", ts.backgroundHandler)
	for path, handler := range ts.apiRoutes() {
		mux.HandleFunc(path, ts.requireToken(handler))
	}
	return mux
}

// Serve wires up the HTTP mux and starts the server.
func Serve(ts *TerminalServer, shouldOpenBrowser bool, useTLS bool) {
	ts.WSClients = make(map[*websocket.Conn]*wsClient)
	Debug("starting b3tty server....")

	if existing, err := ReadLockFile(); err == nil && existing != nil {
		Warnf("lock file found: another b3tty instance may already be running on port %d (pid %d)", existing.Port, existing.PID)
	}

	protocol := ts.Server.URL.Scheme

	Debugf("no-auth mode: %v", ts.Server.NoAuth)
	if !ts.Server.NoAuth {
		var err error
		ts.Token, err = generateToken(TOKEN_LENGTH)
		if err != nil {
			Fatalf("error generating token: %v", err)
		}
	}

	if err := WriteLockFile(ts.Server.Port(), os.Getpid(), ts.Token, protocol); err != nil {
		Warnf("could not write lock file: %v", err)
	}

	uiUrl := ts.Server.buildUIUrl(ts.Token, ts.StartupProfile)

	Debugf("open-browser on start up: %v", shouldOpenBrowser)
	if shouldOpenBrowser {
		if err := OpenBrowser(uiUrl); err != nil {
			Fatal("failed to open default browser")
		}
	}

	Infof("%s server started on %s", protocol, Bold(uiUrl))

	// Display the available profiles in the config file
	if len(ts.Profiles) > 1 {
		logProfileURLs(ts.Server, ts.Profiles, ts.Token)
	}

	httpServer := &http.Server{
		Addr:         ts.Server.URL.Host,
		Handler:      ts.newMux(),
		ErrorLog:     NewWarnLogger(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	var err error
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
		if removeErr := RemoveLockFile(); removeErr != nil {
			Warnf("could not remove lock file: %v", removeErr)
		}
		if err != nil && err != http.ErrServerClosed {
			Fatalf("%s server error: %v", protocol, err)
		}
	case sig := <-quit:
		Infof("received signal %v, shutting down...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err = httpServer.Shutdown(ctx); err != nil {
			if removeErr := RemoveLockFile(); removeErr != nil {
				Warnf("could not remove lock file: %v", removeErr)
			}
			Fatalf("server shutdown error: %v", err)
		}
		if removeErr := RemoveLockFile(); removeErr != nil {
			Warnf("could not remove lock file: %v", removeErr)
		}
	}
}
