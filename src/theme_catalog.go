package src

import (
	"slices"
	"sort"
)

// ThemeSource says where a theme's definition comes from. Built-in themes are
// embedded in the binary, installed themes are packages in the plugin store
// (see PluginStore), and user themes are defined in the config file's themes
// section. None is ever copied into another; copying a theme makes a new user
// theme.
type ThemeSource string

const (
	ThemeSourceBuiltin   ThemeSource = "builtin"
	ThemeSourceInstalled ThemeSource = "installed"
	ThemeSourceUser      ThemeSource = "user"
)

// ResolveTheme looks name up among every theme source, in precedence order:
// built-in, then installed, then user. Startup drops a theme that a
// higher-precedence one shadows (see LoadThemeState), so in practice a name
// belongs to one source. Either map may be nil.
func ResolveTheme(name string, installed map[string]InstalledTheme, user map[string]Theme) (Theme, ThemeSource, bool) {
	if t, ok := builtinThemes[name]; ok {
		return t, ThemeSourceBuiltin, true
	}
	if t, ok := installed[name]; ok {
		return t.Theme, ThemeSourceInstalled, true
	}
	if t, ok := user[name]; ok {
		return t, ThemeSourceUser, true
	}
	return Theme{}, "", false
}

// resolveTheme is ResolveTheme over the server's themes. Callers must hold
// ts.StateMu (read or write).
func (ts *TerminalServer) resolveTheme(name string) (Theme, ThemeSource, bool) {
	return ResolveTheme(name, ts.InstalledThemes, ts.Themes)
}

// hasBackgroundImage reports whether the theme name, whose definition is t,
// has a background image for /background to serve: an installed theme's comes
// from its package, any other theme's from its BackgroundImage path. Callers
// must hold ts.StateMu.
func (ts *TerminalServer) hasBackgroundImage(name string, t Theme) bool {
	if it, ok := ts.InstalledThemes[name]; ok && !IsBuiltinThemeName(name) {
		return it.HasImage()
	}
	return t.HasBackgroundImage()
}

// allThemeNames returns the names of every built-in, installed, and user
// theme, sorted — the themes the theme picker offers. Callers must hold
// ts.StateMu.
func (ts *TerminalServer) allThemeNames() []string {
	names := GetBuiltinThemeNames()
	for name := range ts.InstalledThemes {
		if !IsBuiltinThemeName(name) {
			names = append(names, name)
		}
	}
	for name := range ts.Themes {
		if _, source, _ := ts.resolveTheme(name); source == ThemeSourceUser {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// readOnlyThemeNames returns the names of the built-in and installed themes,
// sorted: the themes that can't be edited or exported. Callers must hold
// ts.StateMu.
func (ts *TerminalServer) readOnlyThemeNames() []string {
	names := GetBuiltinThemeNames()
	for name := range ts.InstalledThemes {
		if !IsBuiltinThemeName(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// menuThemeNames returns a copy of the Themes menu (ts.AvailableThemes), in
// menu order. Callers must hold ts.StateMu.
func (ts *TerminalServer) menuThemeNames() []string {
	return slices.Clone(ts.AvailableThemes)
}

// addMenuTheme appends name to the Themes menu unless it is already there,
// mirroring what ActivateThemeInConfig and SaveThemeToConfig do on disk.
// Callers must hold ts.StateMu for writing.
func (ts *TerminalServer) addMenuTheme(name string) {
	if !slices.Contains(ts.AvailableThemes, name) {
		ts.AvailableThemes = append(ts.AvailableThemes, name)
	}
}

// removeMenuTheme removes name from the Themes menu. Callers must hold
// ts.StateMu for writing.
func (ts *TerminalServer) removeMenuTheme(name string) {
	ts.AvailableThemes = slices.DeleteFunc(ts.AvailableThemes, func(n string) bool { return n == name })
}

// fallBackToDefaultTheme replaces an active theme that no longer exists with
// the default theme (ts.DefaultTheme), or with no theme when that doesn't
// exist either, and returns the new active theme's name and broadcast
// response. Callers must hold ts.StateMu for writing.
func (ts *TerminalServer) fallBackToDefaultTheme() (string, themeConfigResponse) {
	ts.ActiveTheme, ts.Theme = "", Theme{}
	if ts.DefaultTheme != "" {
		if t, _, ok := ts.resolveTheme(ts.DefaultTheme); ok {
			ts.ActiveTheme, ts.Theme = ts.DefaultTheme, t
		}
	}
	return ts.ActiveTheme, themeConfigResponse{
		Theme:              ts.Theme,
		HasBackgroundImage: ts.hasBackgroundImage(ts.ActiveTheme, ts.Theme),
	}
}

// ThemeState is the theme-related part of the server's startup state, derived
// from the config file by LoadThemeState.
type ThemeState struct {
	// Themes holds the user-defined themes, keyed by name.
	Themes map[string]Theme
	// Installed holds the installed themes, keyed by name.
	Installed map[string]InstalledTheme
	// AvailableThemes is the Themes menu, in order.
	AvailableThemes []string
	// DefaultTheme is the config file's default-theme, as written.
	DefaultTheme string
	// ActiveName and Active are the theme to apply. ActiveName is "" and
	// Active the zero Theme when no theme applies, in which case xterm.js
	// uses its own colors.
	ActiveName string
	Active     Theme
}

// LoadThemeState derives the server's theme state from a decoded config file
// and the installed themes (see PluginStore.LoadThemes). keys reports which
// keys the file contained. Problems a hand-edited or older config file can
// have are logged as warnings rather than failing startup:
//
//   - A user theme named after a built-in or installed theme is dropped, and
//     an installed theme named after a built-in one (which a newer b3tty can
//     add) is hidden: precedence is built-in > installed > user. Configs
//     written before available-themes existed hold copies of every built-in
//     theme the user selected.
//   - With no available-themes key at all, the Themes menu is every name in
//     the themes section, sorted, which is what older versions showed; a
//     dropped copy of a built-in stays in it, now meaning the built-in itself.
//     An empty list means an empty menu.
//   - A menu entry that names no theme is skipped.
//   - When theme names no theme, default-theme is used instead, and when that
//     names none either, no theme is applied. An absent theme or default-theme
//     key is not a problem; it just means no theme.
func LoadThemeState(cfg Config, keys ConfigKeys, installed map[string]InstalledTheme) ThemeState {
	state := ThemeState{
		Themes:       make(map[string]Theme),
		Installed:    make(map[string]InstalledTheme),
		DefaultTheme: cfg.DefaultTheme,
	}

	for name, t := range installed {
		if IsBuiltinThemeName(name) {
			Warnf("installed theme %q is hidden: a built-in theme has the same name. Uninstall it with 'b3tty theme delete'.", name)
			continue
		}
		state.Installed[name] = t
	}
	for name, t := range cfg.Themes {
		if IsBuiltinThemeName(name) {
			Warnf("themes.%s is ignored: %q is a built-in theme, which can't be redefined. Delete the entry, or rename it to keep it as your own theme.", name, name)
			continue
		}
		if _, ok := state.Installed[name]; ok {
			Warnf("themes.%s is ignored: an installed theme is named %q. Rename your theme to keep it.", name, name)
			continue
		}
		state.Themes[name] = t
	}

	resolve := func(name string) (Theme, bool) {
		t, _, ok := ResolveTheme(name, state.Installed, state.Themes)
		return t, ok
	}

	if keys.Has("available-themes") {
		for _, name := range cfg.AvailableThemes {
			if _, ok := resolve(name); !ok {
				Warnf("available-themes: skipping %q, which is not a built-in, installed, or user theme", name)
				continue
			}
			if !slices.Contains(state.AvailableThemes, name) {
				state.AvailableThemes = append(state.AvailableThemes, name)
			}
		}
	} else {
		for name := range cfg.Themes {
			state.AvailableThemes = append(state.AvailableThemes, name)
		}
		sort.Strings(state.AvailableThemes)
	}
	if state.AvailableThemes == nil {
		state.AvailableThemes = []string{}
	}

	candidates := []struct{ key, name string }{{"theme", cfg.Theme}, {"default-theme", cfg.DefaultTheme}}
	for i, c := range candidates {
		if c.name == "" {
			continue
		}
		if t, ok := resolve(c.name); ok {
			state.ActiveName, state.Active = c.name, t
			break
		}
		next := "no theme will be applied"
		if i == 0 && cfg.DefaultTheme != "" {
			next = "using default-theme"
		}
		Warnf("%s: %q is not a built-in, installed, or user theme; %s", c.key, c.name, next)
	}
	return state
}
