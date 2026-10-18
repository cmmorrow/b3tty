package src

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"net/http"
)

//go:embed templates/setup.tmpl
var setupTempl string

// setupTemplate is parsed once at package init instead of on every request:
// it is embedded at compile time and never changes at runtime, so
// re-parsing it per-request (as displayTermHandler used to do too) is pure
// repeated overhead. template.Must panics at startup if the embedded template
// is malformed, matching the fail-fast pattern mustUnmarshalTheme uses for
// the other embedded, compile-time-known assets.
var setupTemplate = template.Must(template.New("setup").Parse(setupTempl))

// renderSetupPage renders the theme selection setup page.
func (ts *TerminalServer) renderSetupPage(w http.ResponseWriter) {
	csp := GetCSPHeaders()
	w.Header().Set("Content-Security-Policy", csp.String())

	if err := setupTemplate.Execute(w, nil); err != nil {
		Errorf("setup response error: %v", err)
	}
}

// saveConfigHandler accepts a POST request with a JSON body containing a "theme"
// field ("b3tty-dark", "b3tty-light", or "skip") and writes a new config file
// to $HOME/.config/b3tty/conf.yaml via saveDefaultThemeConfig: the chosen
// built-in becomes the active and default theme and the one Themes menu entry,
// and "skip" writes a config with no theme. Sets firstRun to false on success.
func (ts *TerminalServer) saveConfigHandler(w http.ResponseWriter, r *http.Request) {
	ts.StateMu.RLock()
	firstRun := ts.FirstRun
	ts.StateMu.RUnlock()
	if !firstRun {
		http.NotFound(w, r)
		return
	}
	if r.Method != "POST" {
		Warnf("%s %s: method not allowed: %s", r.Method, r.URL.Path, r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if requireSameOrigin(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQUEST_BODY_SIZE)
	var req struct {
		Theme string `json:"theme"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Warnf("%s %s: bad request: %v", r.Method, r.URL.Path, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// "b3tty-dark" and "b3tty-light" become the active and default theme;
	// anything else ("skip") applies no theme. Either way a config file is
	// written, so the setup page isn't shown on the next start.
	var themeName string
	var selected Theme
	switch req.Theme {
	case "b3tty-dark":
		themeName, selected = req.Theme, defaultDarkTheme
	case "b3tty-light":
		themeName, selected = req.Theme, defaultLightTheme
	}

	Debug("writing config file....")
	if err := saveDefaultThemeConfig(ts.ConfigFile, themeName); err != nil {
		Errorf("failed to write config: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	ts.StateMu.Lock()
	ts.Theme = selected
	ts.ActiveTheme = themeName
	ts.DefaultTheme = themeName
	ts.AvailableThemes = []string{}
	if themeName != "" {
		ts.AvailableThemes = []string{themeName}
	}
	ts.StateMu.Unlock()
	if themeName != "" {
		Infof("created config with the %s theme", themeName)
	} else {
		Info("created config with no theme")
	}

	ts.StateMu.Lock()
	ts.FirstRun = false
	ts.StateMu.Unlock()
	w.WriteHeader(http.StatusOK)
}
