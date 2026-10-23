package src

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"
)

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

// defaultDarkTheme and defaultLightTheme are the themes offered by the
// first-run setup page; they are also entries in builtinThemes below.
var defaultDarkTheme = mustUnmarshalTheme(defaultDarkThemeJSON)
var defaultLightTheme = mustUnmarshalTheme(defaultLightThemeJSON)

// builtinThemes maps each built-in theme name to its decoded Theme. They are
// never copied into ts.Themes or the config file; every handler finds them
// through resolveTheme.
var builtinThemes = map[string]Theme{
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

// GetBuiltinThemeNames returns the sorted names of all built-in themes.
func GetBuiltinThemeNames() []string {
	names := make([]string, 0, len(builtinThemes))
	for name := range builtinThemes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
	ts.StateMu.RLock()
	t, _, ok := ts.resolveTheme(name)
	ts.StateMu.RUnlock()
	if !ok {
		Warnf("unknown theme name: %q", name)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// ANSI display order (black, red, yellow, green, cyan, blue, magenta,
	// white) rather than struct order.
	resp := themePaletteResponse{
		Bg:     t.Background,
		Fg:     t.Foreground,
		SelBg:  t.SelectionBackground,
		Cursor: t.Cursor,
		Normal: []string{t.Black, t.Red, t.Yellow, t.Green, t.Cyan, t.Blue, t.Magenta, t.White},
		Bright: []string{t.BrightBlack, t.BrightRed, t.BrightYellow, t.BrightGreen, t.BrightCyan, t.BrightBlue, t.BrightMagenta, t.BrightWhite},
	}
	writeJSON(w, resp, "theme")
}

// themeConfigHandler serves GET and POST /theme-config?name=<themename>.
//
// GET returns the full theme color config and hasBackgroundImage flag without
// side effects.
//
// The name may be a theme of any source (see resolveTheme).
//
// POST additionally activates the named theme on the server (updating
// ts.Theme) so that the /background endpoint immediately serves the correct
// image and subsequent page loads receive the new theme, adds it to the
// Themes menu, and records both in conf.yaml via ActivateThemeInConfig. POST
// requires a same-origin Sec-Fetch-Site header to prevent CSRF.
func (ts *TerminalServer) themeConfigHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET", "POST":
	default:
		Warnf("%s %s: method not allowed: %s", r.Method, r.URL.Path, r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Method == "POST" {
		if requireSameOrigin(w, r) {
			return
		}
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		Warnf("%s %s: bad request: missing name parameter", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ts.StateMu.RLock()
	theme, _, ok := ts.resolveTheme(name)
	hasImage := ts.hasBackgroundImage(name, theme)
	ts.StateMu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method == "POST" {
		ts.StateMu.Lock()
		ts.Theme = theme
		ts.ActiveTheme = name
		ts.addMenuTheme(name)
		ts.StateMu.Unlock()
		if err := ActivateThemeInConfig(ts.ConfigFile, name); err != nil {
			Errorf("theme-config: failed to update config: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	resp := themeConfigResponse{
		Theme:              theme,
		HasBackgroundImage: hasImage,
	}
	if r.Method == "POST" {
		ts.broadcastTheme(name, resp)
	}
	writeJSON(w, resp, "theme-config")
}

// addThemeHandler activates a theme chosen in the theme picker and adds it to
// the Themes menu, in memory and in conf.yaml (ActivateThemeInConfig). The
// theme may be of any source; built-in and installed themes are referenced by
// name, never copied into the themes section or ts.Themes. It returns the
// theme plus themeNames, the updated menu, so the client can rebuild it.
// POST /add-theme  body: {"theme":"<name>"}
func (ts *TerminalServer) addThemeHandler(w http.ResponseWriter, r *http.Request) {
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Theme == "" {
		Warnf("%s %s: bad request: missing or invalid theme", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ts.StateMu.Lock()
	theme, _, ok := ts.resolveTheme(req.Theme)
	if !ok {
		ts.StateMu.Unlock()
		Warnf("%s %s: unknown theme %q", r.Method, r.URL.Path, req.Theme)
		http.NotFound(w, r)
		return
	}
	ts.Theme = theme
	ts.ActiveTheme = req.Theme
	ts.addMenuTheme(req.Theme)
	themeNames := ts.menuThemeNames()
	themes := ts.themeEntries()
	hasImage := ts.hasBackgroundImage(req.Theme, theme)
	ts.StateMu.Unlock()

	if err := ActivateThemeInConfig(ts.ConfigFile, req.Theme); err != nil {
		Errorf("add-theme: failed to update config: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	Debugf("added theme %q", req.Theme)
	resp := themeConfigResponse{
		Theme:              theme,
		HasBackgroundImage: hasImage,
		ThemeNames:         themeNames,
		Themes:             themes,
	}
	writeJSON(w, resp, "add-theme")
}

// editThemeHandler creates or overwrites a user-defined theme, activates it,
// and adds it to the Themes menu; the response's themeNames is the updated
// menu. A built-in or installed theme's name is refused (400), since those
// themes can't be edited.
// POST /edit-theme  body: {"name":"<name>","theme":{...Theme fields...}}
func (ts *TerminalServer) editThemeHandler(w http.ResponseWriter, r *http.Request) {
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
		Name  string `json:"name"`
		Theme Theme  `json:"theme"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Warnf("%s %s: bad request: %v", r.Method, r.URL.Path, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		Warnf("%s %s: bad request: missing name", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Validation errors go back in the response body so the theme editor can
	// show the user what to fix.
	ts.StateMu.RLock()
	_, source, _ := ts.resolveTheme(req.Name)
	ts.StateMu.RUnlock()
	if source == ThemeSourceBuiltin || source == ThemeSourceInstalled {
		Warnf("%s %s: bad request: %s theme %q", r.Method, r.URL.Path, source, req.Name)
		kind := "a built-in"
		if source == ThemeSourceInstalled {
			kind = "an installed"
		}
		http.Error(w, fmt.Sprintf("%q is %s theme and can't be edited; save your changes under a new name", req.Name, kind), http.StatusBadRequest)
		return
	}
	if err := ValidateTheme(&req.Theme); err != nil {
		Warnf("%s %s: bad request: %v", r.Method, r.URL.Path, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// The request owns the background image: an empty path clears it. A
	// non-empty one is stored as typed (e.g. "~/bg.png"), but only once it
	// resolves to a valid image, so a bad path is never saved.
	if req.Theme.BackgroundImage != "" {
		if _, err := resolveBackgroundImage(req.Theme.BackgroundImage); err != nil {
			Warnf("%s %s: bad request: %v", r.Method, r.URL.Path, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	ts.StateMu.Lock()
	ts.Themes[req.Name] = req.Theme
	ts.Theme = req.Theme
	ts.ActiveTheme = req.Name
	ts.addMenuTheme(req.Name)
	themeNames := ts.menuThemeNames()
	themes := ts.themeEntries()
	ts.StateMu.Unlock()

	if err := SaveThemeToConfig(ts.ConfigFile, req.Name, req.Theme); err != nil {
		Errorf("edit-theme: failed to save config: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	Debugf("saved theme %q", req.Name)
	resp := themeConfigResponse{
		Theme:              req.Theme,
		HasBackgroundImage: req.Theme.BackgroundImage != "",
		ThemeNames:         themeNames,
		Themes:             themes,
	}
	writeJSON(w, resp, "edit-theme")
}

// exportThemeHandler handles POST /export-theme. It accepts
// { "name": "<theme>", "path": "<file>" } plus the required ThemeMetadata fields
// (repo, author, description, themeVersion) and writes the saved user-defined
// theme as a .b3theme package to path via ExportUserTheme, which also records
// the export in the theme's package: block in the config file. It returns
// { "path": "<file written>" }. Built-in and installed themes are refused
// (400) — an installed theme is exported only by copying it to a user theme
// first, which gets its own package id — and unknown names are 404; an existing file is 409, any other export failure (including
// missing or invalid metadata) 400, and a written package whose details could
// not be saved 500, each with the reason as a plain-text body for the export
// dialog to show.
func (ts *TerminalServer) exportThemeHandler(w http.ResponseWriter, r *http.Request) {
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
		Name string `json:"name"`
		Path string `json:"path"`
		ThemeMetadata
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Warnf("%s %s: bad request: %v", r.Method, r.URL.Path, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		Warnf("%s %s: bad request: missing name", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if IsBuiltinThemeName(req.Name) {
		Warnf("%s %s: bad request: built-in theme %q", r.Method, r.URL.Path, req.Name)
		http.Error(w, fmt.Sprintf("%q is a built-in theme; only user-defined themes can be exported", req.Name), http.StatusBadRequest)
		return
	}

	ts.StateMu.RLock()
	_, source, _ := ts.resolveTheme(req.Name)
	theme, ok := ts.Themes[req.Name]
	ts.StateMu.RUnlock()
	if source == ThemeSourceInstalled {
		Warnf("%s %s: bad request: installed theme %q", r.Method, r.URL.Path, req.Name)
		http.Error(w, fmt.Sprintf("%q is an installed theme and can't be exported; copy it to make your own", req.Name), http.StatusBadRequest)
		return
	}
	if !ok {
		Warnf("%s %s: theme %q not found", r.Method, r.URL.Path, req.Name)
		http.Error(w, fmt.Sprintf("theme %q not found", req.Name), http.StatusNotFound)
		return
	}

	path, details, err := ExportUserTheme(ts.ConfigFile, req.Name, theme, req.ThemeMetadata, req.Path, ts.Version, time.Now())
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, ErrThemePackageExists):
			status = http.StatusConflict
		case path != "":
			// The package was written but its details weren't saved.
			status = http.StatusInternalServerError
		}
		Warnf("%s %s: export failed: %v", r.Method, r.URL.Path, err)
		http.Error(w, err.Error(), status)
		return
	}

	ts.StateMu.Lock()
	if t, ok := ts.Themes[req.Name]; ok {
		t.Package = &details
		ts.Themes[req.Name] = t
	}
	ts.StateMu.Unlock()

	Infof("exported theme %q to %s", req.Name, path)
	writeJSON(w, struct {
		Path string `json:"path"`
	}{path}, "export-theme")
}

// themePackageHandler handles GET /theme-package?name=<theme>. It returns the
// metadata recorded by the user theme's last export, read from the config
// file's package: block, so the export dialog can fill in its fields — or {}
// when the theme has never been exported. The package id is never sent. A
// built-in or installed name is 400.
func (ts *TerminalServer) themePackageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		Warnf("%s %s: method not allowed: %s", r.Method, r.URL.Path, r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	name := r.URL.Query().Get("name")
	ts.StateMu.RLock()
	_, source, _ := ts.resolveTheme(name)
	ts.StateMu.RUnlock()
	if name == "" || source == ThemeSourceBuiltin || source == ThemeSourceInstalled {
		Warnf("%s %s: bad request: missing, built-in, or installed name %q", r.Method, r.URL.Path, name)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	details, err := ReadThemePackageDetails(ts.ConfigFile, name)
	if err != nil {
		Errorf("theme-package: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if details == nil {
		writeJSON(w, struct{}{}, "theme-package")
		return
	}
	writeJSON(w, details.ThemeMetadata, "theme-package")
}
