package src

import (
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"text/template"
)

//go:embed templates/setup.tmpl
var setupTempl string

//go:embed templates/theme-select.tmpl
var themeSelectTempl string

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
		// Register the selected theme in ts.Themes so it appears in the Themes
		// menu after the browser reloads into the normal terminal flow.
		if ts.Themes == nil {
			ts.Themes = make(map[string]Theme)
		}
		ts.Themes[req.Theme] = ts.Client.Theme
		ts.ActiveTheme = req.Theme
		Infof("created default %s theme config", req.Theme)
	}

	ts.FirstRun = false
	w.WriteHeader(http.StatusOK)
}
