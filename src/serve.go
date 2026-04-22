package src

import (
	"context"
	"embed"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed assets
var assets embed.FS

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

// Serve wires up the HTTP mux and starts the server.
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
