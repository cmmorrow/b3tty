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
// field ("b3tty-dark", "b3tty-light", or "skip"). For b3tty-dark/b3tty-light, it writes a default config
// file to $HOME/.config/b3tty/conf.yaml. Sets firstRun to false on success.
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

	var themeColors map[string]any
	switch req.Theme {
	case "b3tty-dark":
		themeColors = defaultDarkTheme
	case "b3tty-light":
		themeColors = defaultLightTheme
	}

	if themeColors != nil {
		Debug("writing config file....")
		if err := saveDefaultThemeConfig(ts.ConfigFile, req.Theme, themeColors); err != nil {
			Errorf("failed to write config: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		ts.StateMu.Lock()
		ts.Client.Theme.MapToTheme(themeColors)
		// Register the selected theme in ts.Themes so it appears in the Themes
		// menu after the browser reloads into the normal terminal flow.
		ts.Themes[req.Theme] = ts.Client.Theme
		ts.ActiveTheme = req.Theme
		ts.StateMu.Unlock()
		Infof("created default %s theme config", req.Theme)
	}

	ts.StateMu.Lock()
	ts.FirstRun = false
	ts.StateMu.Unlock()
	w.WriteHeader(http.StatusOK)
}
