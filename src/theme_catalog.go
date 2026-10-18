package src

import (
	"slices"
	"sort"
)

// ThemeSource says where a theme's definition comes from. Built-in themes are
// embedded in the binary and user themes are defined in the config file's
// themes section. Neither kind is ever copied into the other.
type ThemeSource string

const (
	ThemeSourceBuiltin ThemeSource = "builtin"
	ThemeSourceUser    ThemeSource = "user"
)

// resolveTheme looks name up among every theme b3tty knows: built-in themes
// first, then user themes, so a built-in always wins a name clash (startup
// drops a user theme that a built-in shadows; see LoadThemeState). Callers
// must hold ts.StateMu (read or write), since it reads ts.Themes.
func (ts *TerminalServer) resolveTheme(name string) (Theme, ThemeSource, bool) {
	if t, ok := builtinThemes[name]; ok {
		return t, ThemeSourceBuiltin, true
	}
	if t, ok := ts.Themes[name]; ok {
		return t, ThemeSourceUser, true
	}
	return Theme{}, "", false
}

// allThemeNames returns the names of every built-in and user theme, sorted —
// the themes the theme picker offers. Callers must hold ts.StateMu.
func (ts *TerminalServer) allThemeNames() []string {
	names := GetBuiltinThemeNames()
	for name := range ts.Themes {
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

// ThemeState is the theme-related part of the server's startup state, derived
// from the config file by LoadThemeState.
type ThemeState struct {
	// Themes holds the user-defined themes, keyed by name.
	Themes map[string]Theme
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

// LoadThemeState derives the server's theme state from a decoded config file.
// keys reports which keys the file contained. Problems a hand-edited or older
// config file can have are logged as warnings rather than failing startup:
//
//   - A user theme named after a built-in theme is dropped: the built-in wins.
//     Configs written before available-themes existed hold such copies of
//     every built-in theme the user selected.
//   - With no available-themes key at all, the Themes menu is every name in
//     the themes section, sorted, which is what older versions showed; a
//     dropped copy of a built-in stays in it, now meaning the built-in itself.
//     An empty list means an empty menu.
//   - A menu entry that names no theme is skipped.
//   - When theme names no theme, default-theme is used instead, and when that
//     names none either, no theme is applied. An absent theme or default-theme
//     key is not a problem; it just means no theme.
func LoadThemeState(cfg Config, keys ConfigKeys) ThemeState {
	state := ThemeState{Themes: make(map[string]Theme), DefaultTheme: cfg.DefaultTheme}

	for name, t := range cfg.Themes {
		if IsBuiltinThemeName(name) {
			Warnf("themes.%s is ignored: %q is a built-in theme, which can't be redefined. Delete the entry, or rename it to keep it as your own theme.", name, name)
			continue
		}
		state.Themes[name] = t
	}

	resolve := func(name string) (Theme, bool) {
		if t, ok := builtinThemes[name]; ok {
			return t, true
		}
		t, ok := state.Themes[name]
		return t, ok
	}

	if keys.Has("available-themes") {
		for _, name := range cfg.AvailableThemes {
			if _, ok := resolve(name); !ok {
				Warnf("available-themes: skipping %q, which is not a built-in or user theme", name)
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
		Warnf("%s: %q is not a built-in or user theme; %s", c.key, c.name, next)
	}
	return state
}
