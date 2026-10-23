package src

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"reflect"
	"slices"
)

// themeNamesResponse is returned by the endpoints that change which themes
// exist without returning one: themeNames is the updated Themes menu, and
// themes every theme of every source (see themeEntries).
type themeNamesResponse struct {
	ThemeNames []string     `json:"themeNames"`
	Themes     []ThemeEntry `json:"themes"`
}

// importThemeResponse is returned by POST /import-theme.
type importThemeResponse struct {
	Name       string              `json:"name"`
	Theme      themeConfigResponse `json:"theme"`
	ThemeNames []string            `json:"themeNames"`
	Themes     []ThemeEntry        `json:"themes"`
	// Upgraded is true when the package replaced an installed version of the
	// theme, and Unchanged when that exact package was already installed.
	Upgraded  bool `json:"upgraded"`
	Unchanged bool `json:"unchanged"`
}

// requireSameOriginPost answers requests other than a same-origin POST, which
// every theme management endpoint requires, and reports whether it did.
func requireSameOriginPost(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "POST" {
		Warnf("%s %s: method not allowed: %s", r.Method, r.URL.Path, r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return true
	}
	return requireSameOrigin(w, r)
}

// importThemeHandler handles POST /import-theme. It accepts
// { "path": "<file>", "replace": bool } and installs the .b3theme package at
// path, a file on the server's filesystem, via ImportThemePackage. The theme is
// installed but not activated or added to the Themes menu. A new version of an
// installed theme replaces it only with "replace": true; without it the answer
// is 409 with the reason, which names any change of author or repository, so
// the caller can ask the user. Any other failure is 400 with the reason as a
// plain-text body. Upgrading the active theme applies the new version to every
// open session.
func (ts *TerminalServer) importThemeHandler(w http.ResponseWriter, r *http.Request) {
	if requireSameOriginPost(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQUEST_BODY_SIZE)
	var req struct {
		Path    string `json:"path"`
		Replace bool   `json:"replace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		Warnf("%s %s: bad request: missing or invalid path", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	isUserTheme := func(name string) bool {
		ts.StateMu.RLock()
		defer ts.StateMu.RUnlock()
		_, ok := ts.Themes[name]
		return ok
	}
	result, err := ImportThemePackage(ts.Plugins, req.Path, isUserTheme, req.Replace)
	if err != nil {
		status := http.StatusBadRequest
		var upgrade *UpgradeRequiredError
		if errors.As(err, &upgrade) {
			status = http.StatusConflict
		}
		Warnf("%s %s: import failed: %v", r.Method, r.URL.Path, err)
		http.Error(w, err.Error(), status)
		return
	}

	installed := result.Theme
	name := installed.Manifest.Name
	themeResp := themeConfigResponse{Theme: installed.Theme, HasBackgroundImage: installed.HasImage()}
	ts.StateMu.Lock()
	if ts.InstalledThemes == nil {
		ts.InstalledThemes = make(map[string]InstalledTheme)
	}
	ts.InstalledThemes[name] = installed
	isActive := ts.ActiveTheme == name
	if isActive {
		ts.Theme = installed.Theme
	}
	themeNames := ts.menuThemeNames()
	themes := ts.themeEntries()
	ts.StateMu.Unlock()

	if !result.Unchanged {
		if isActive {
			ts.broadcastTheme(name, themeResp)
		}
		ts.broadcastThemeLists()
		Infof("installed theme %q (version %s by %s)", name, installed.Manifest.ThemeVersion, installed.Manifest.Author)
	}
	writeJSON(w, importThemeResponse{
		Name:       name,
		Theme:      themeResp,
		ThemeNames: themeNames,
		Themes:     themes,
		Upgraded:   result.Upgraded,
		Unchanged:  result.Unchanged,
	}, "import-theme")
}

// copyThemeHandler handles POST /copy-theme. It accepts
// { "name": "<theme>", "newName": "<name>" } and saves a copy of the theme
// name, of any source, as the new user theme newName via CopyTheme, adding it
// to the Themes menu without activating it. It returns the copy as a
// themeConfigResponse with themeNames. An unknown name is 404; a newName that
// isn't valid or already names a theme is 400, with the reason as a
// plain-text body.
func (ts *TerminalServer) copyThemeHandler(w http.ResponseWriter, r *http.Request) {
	if requireSameOriginPost(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQUEST_BODY_SIZE)
	var req struct {
		Name    string `json:"name"`
		NewName string `json:"newName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		Warnf("%s %s: bad request: missing or invalid name", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := validateThemeName(req.NewName); err != nil {
		Warnf("%s %s: bad request: %v", r.Method, r.URL.Path, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ts.StateMu.RLock()
	source, sourceKind, ok := ts.resolveTheme(req.Name)
	_, _, taken := ts.resolveTheme(req.NewName)
	installed := ts.InstalledThemes[req.Name]
	ts.StateMu.RUnlock()
	if !ok {
		Warnf("%s %s: theme %q not found", r.Method, r.URL.Path, req.Name)
		http.Error(w, fmt.Sprintf("theme %q not found", req.Name), http.StatusNotFound)
		return
	}
	if taken {
		Warnf("%s %s: bad request: theme %q already exists", r.Method, r.URL.Path, req.NewName)
		http.Error(w, fmt.Sprintf("a theme named %q already exists", req.NewName), http.StatusBadRequest)
		return
	}

	// An installed theme's image is read from its package now, outside the
	// lock, rather than kept in memory.
	var image []byte
	if sourceKind == ThemeSourceInstalled && installed.HasImage() {
		var err error
		if image, err = installed.ReadImage(); err != nil {
			Errorf("copy-theme: reading the background image of %q: %v", req.Name, err)
			http.Error(w, fmt.Sprintf("reading the background image of %q: %v", req.Name, err), http.StatusInternalServerError)
			return
		}
	}
	theme, err := CopyTheme(ts.ConfigFile, req.NewName, source, image, installed.ImageExt)
	if err != nil {
		Errorf("copy-theme: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ts.StateMu.Lock()
	ts.Themes[req.NewName] = theme
	ts.addMenuTheme(req.NewName)
	themeNames := ts.menuThemeNames()
	themes := ts.themeEntries()
	ts.StateMu.Unlock()

	ts.broadcastThemeLists()
	Infof("copied theme %q to %q", req.Name, req.NewName)
	writeJSON(w, themeConfigResponse{
		Theme:              theme,
		HasBackgroundImage: theme.HasBackgroundImage(),
		ThemeNames:         themeNames,
		Themes:             themes,
	}, "copy-theme")
}

// deleteThemeHandler handles POST /delete-theme. It accepts { "name": "<theme>" }
// and permanently deletes the theme via DeleteTheme: a user theme is removed
// from the config file, an installed one is uninstalled, and a built-in one is
// only removed from the Themes menu (400 when it isn't in the menu). There is
// no undo; callers confirm first. When a deleted theme was the active theme,
// every open session switches to the default theme, or to no theme. It returns
// themeNames, the updated menu. An unknown name is 404.
func (ts *TerminalServer) deleteThemeHandler(w http.ResponseWriter, r *http.Request) {
	if requireSameOriginPost(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQUEST_BODY_SIZE)
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		Warnf("%s %s: bad request: missing or invalid name", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ts.StateMu.RLock()
	_, source, ok := ts.resolveTheme(req.Name)
	inMenu := slices.Contains(ts.AvailableThemes, req.Name)
	ts.StateMu.RUnlock()
	if !ok {
		Warnf("%s %s: theme %q not found", r.Method, r.URL.Path, req.Name)
		http.Error(w, fmt.Sprintf("theme %q not found", req.Name), http.StatusNotFound)
		return
	}
	if source == ThemeSourceBuiltin && !inMenu {
		Warnf("%s %s: bad request: built-in theme %q isn't in the menu", r.Method, r.URL.Path, req.Name)
		http.Error(w, fmt.Sprintf("%q is a built-in theme and isn't in the Themes menu; built-in themes can't be deleted", req.Name), http.StatusBadRequest)
		return
	}

	if err := DeleteTheme(ts.ConfigFile, ts.Plugins, req.Name, source); err != nil {
		Errorf("delete-theme: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var fellBack bool
	var activeName string
	var activeResp themeConfigResponse
	ts.StateMu.Lock()
	ts.removeMenuTheme(req.Name)
	if source != ThemeSourceBuiltin {
		delete(ts.Themes, req.Name)
		delete(ts.InstalledThemes, req.Name)
		if ts.DefaultTheme == req.Name {
			ts.DefaultTheme = ""
		}
		if ts.ActiveTheme == req.Name {
			activeName, activeResp = ts.fallBackToDefaultTheme()
			fellBack = true
		}
	}
	themeNames := ts.menuThemeNames()
	themes := ts.themeEntries()
	ts.StateMu.Unlock()

	if fellBack {
		ts.broadcastTheme(activeName, activeResp)
	}
	ts.broadcastThemeLists()
	Infof("deleted %s theme %q", source, req.Name)
	writeJSON(w, themeNamesResponse{ThemeNames: themeNames, Themes: themes}, "delete-theme")
}

// reloadThemesHandler handles POST /reload-themes, which the b3tty theme CLI
// calls after changing the config file or the plugin store so the running
// server picks the change up. It re-reads the config file and the installed
// themes, re-verifying each package, and replaces the server's theme state
// with what they hold (see LoadThemeState): the user and installed themes, the
// Themes menu, the default theme, and the active theme. Open sessions get the
// new theme lists, and the active theme too when it changed. It returns
// themeNames, the updated menu. A config file that can't be decoded is 500,
// and nothing changes.
func (ts *TerminalServer) reloadThemesHandler(w http.ResponseWriter, r *http.Request) {
	if requireSameOriginPost(w, r) {
		return
	}

	configPath, err := resolveConfigPath(ts.ConfigFile)
	if err != nil {
		Errorf("reload-themes: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var cfg Config
	keys, err := LoadConfig(configPath, &cfg)
	if errors.Is(err, fs.ErrNotExist) {
		keys, err = ConfigKeys{}, nil
	}
	if err != nil {
		Errorf("reload-themes: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state := LoadThemeState(cfg, keys, ts.Plugins.LoadThemes(true))

	ts.StateMu.Lock()
	prevName, prevTheme := ts.ActiveTheme, ts.Theme
	prevDigest := ts.InstalledThemes[prevName].Digest
	ts.Themes = state.Themes
	ts.InstalledThemes = state.Installed
	ts.AvailableThemes = state.AvailableThemes
	ts.DefaultTheme = state.DefaultTheme
	ts.ActiveTheme, ts.Theme = state.ActiveName, state.Active
	changed := ts.ActiveTheme != prevName ||
		!sameThemeDefinition(ts.Theme, prevTheme) ||
		ts.InstalledThemes[ts.ActiveTheme].Digest != prevDigest
	activeName := ts.ActiveTheme
	activeResp := themeConfigResponse{Theme: ts.Theme, HasBackgroundImage: ts.hasBackgroundImage(ts.ActiveTheme, ts.Theme)}
	themeNames := ts.menuThemeNames()
	themes := ts.themeEntries()
	ts.StateMu.Unlock()

	if changed {
		ts.broadcastTheme(activeName, activeResp)
	}
	ts.broadcastThemeLists()
	Debugf("reloaded themes")
	writeJSON(w, themeNamesResponse{ThemeNames: themeNames, Themes: themes}, "reload-themes")
}

// sameThemeDefinition reports whether a and b look the same, ignoring the
// package details a user theme gains when it is exported.
func sameThemeDefinition(a, b Theme) bool {
	a.Package, b.Package = nil, nil
	return reflect.DeepEqual(a, b)
}
