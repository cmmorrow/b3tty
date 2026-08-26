package src

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"sort"
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

// GetBuiltinThemeNames returns the sorted names of all built-in themes.
func GetBuiltinThemeNames() []string {
	names := make([]string, 0, len(builtinThemes))
	for name := range builtinThemes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetBuiltinTheme returns the color map for the named built-in theme.
// The second return value is false when no such theme exists.
func GetBuiltinTheme(name string) (map[string]any, bool) {
	colors, ok := builtinThemes[name]
	return colors, ok
}

// sortedThemeNames returns the names of all user-defined themes in ts.Themes,
// sorted alphabetically. Callers must hold ts.StateMu (read or write) before
// calling this method since it reads ts.Themes directly.
func (ts *TerminalServer) sortedThemeNames() []string {
	names := make([]string, 0, len(ts.Themes))
	for name := range ts.Themes {
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
	t, ok := ts.Themes[name]
	ts.StateMu.RUnlock()

	var colors map[string]any
	if ok {
		colors = t.toColorMap()
	} else if builtinColors, ok := builtinThemes[name]; ok {
		colors = builtinColors
	} else {
		Warnf("unknown theme name: %q", name)
		w.WriteHeader(http.StatusBadRequest)
		return
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
	writeJSON(w, resp, "theme")
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
	theme, ok := ts.Themes[name]
	ts.StateMu.RUnlock()
	if !ok {
		if builtinColors, ok := builtinThemes[name]; ok {
			var t Theme
			t.MapToTheme(builtinColors)
			theme = t
		} else {
			http.NotFound(w, r)
			return
		}
	}
	if r.Method == "POST" {
		ts.StateMu.Lock()
		ts.Client.Theme = theme
		ts.ActiveTheme = name
		ts.StateMu.Unlock()
		var colors map[string]any
		if builtinColors, ok := builtinThemes[name]; ok {
			colors = builtinColors
		} else {
			colors = theme.toColorMap()
		}
		if err := UpdateThemeInConfig(ts.ConfigFile, name, colors); err != nil {
			Errorf("theme-config: failed to update config: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	resp := themeConfigResponse{
		Theme:              theme,
		HasBackgroundImage: theme.BackgroundImage != "",
	}
	if r.Method == "POST" {
		ts.broadcastTheme(name, resp)
	}
	writeJSON(w, resp, "theme-config")
}

// addThemeHandler applies a chosen theme and persists it to conf.yaml.
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

	// Resolve theme colors and ensure the theme is in ts.Themes. The whole
	// read-check-then-write sequence runs under a single write lock so two
	// concurrent add-theme requests for the same new theme name can't race.
	ts.StateMu.Lock()
	var colors map[string]any
	if builtinColors, ok := builtinThemes[req.Theme]; ok {
		colors = builtinColors
		if _, exists := ts.Themes[req.Theme]; !exists {
			var t Theme
			t.MapToTheme(colors)
			ts.Themes[req.Theme] = t
		}
	} else if theme, ok := ts.Themes[req.Theme]; ok {
		colors = theme.toColorMap()
	} else {
		ts.StateMu.Unlock()
		Warnf("%s %s: unknown theme %q", r.Method, r.URL.Path, req.Theme)
		http.NotFound(w, r)
		return
	}

	ts.Client.Theme = ts.Themes[req.Theme]
	ts.ActiveTheme = req.Theme
	activeTheme := ts.Client.Theme
	themeNames := ts.sortedThemeNames()
	ts.StateMu.Unlock()

	if err := UpdateThemeInConfig(ts.ConfigFile, req.Theme, colors); err != nil {
		Errorf("add-theme: failed to update config: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	Debugf("added theme %q", req.Theme)
	resp := themeConfigResponse{
		Theme:              activeTheme,
		HasBackgroundImage: activeTheme.BackgroundImage != "",
		ThemeNames:         themeNames,
	}
	writeJSON(w, resp, "add-theme")
}

// editThemeHandler creates or overwrites a user-defined theme and activates it.
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
	if err := ValidateTheme(&req.Theme); err != nil {
		Warnf("%s %s: bad request: %v", r.Method, r.URL.Path, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ts.StateMu.Lock()
	if existing, ok := ts.Themes[req.Name]; ok && existing.BackgroundImage != "" {
		req.Theme.BackgroundImage = existing.BackgroundImage
	}
	ts.Themes[req.Name] = req.Theme
	ts.Client.Theme = req.Theme
	ts.ActiveTheme = req.Name
	themeNames := ts.sortedThemeNames()
	ts.StateMu.Unlock()

	if err := SaveThemeToConfig(ts.ConfigFile, req.Name, req.Theme.toColorMap()); err != nil {
		Errorf("edit-theme: failed to save config: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	Debugf("saved theme %q", req.Name)
	resp := themeConfigResponse{
		Theme:              req.Theme,
		HasBackgroundImage: req.Theme.BackgroundImage != "",
		ThemeNames:         themeNames,
	}
	writeJSON(w, resp, "edit-theme")
}
