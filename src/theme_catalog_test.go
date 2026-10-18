package src

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTheme(t *testing.T) {
	ts := newTestTerminalServer()
	ts.Themes["mine"] = Theme{Foreground: "#123456"}
	ts.Themes["dracula"] = Theme{Foreground: "#000000"} // a shadowing copy

	t.Run("finds a built-in theme", func(t *testing.T) {
		got, src, ok := ts.resolveTheme("one-light")
		require.True(t, ok)
		assert.Equal(t, ThemeSourceBuiltin, src)
		assert.Equal(t, builtinThemes["one-light"], got)
	})

	t.Run("finds a user theme", func(t *testing.T) {
		got, src, ok := ts.resolveTheme("mine")
		require.True(t, ok)
		assert.Equal(t, ThemeSourceUser, src)
		assert.Equal(t, "#123456", got.Foreground)
	})

	t.Run("a built-in wins over a user theme of the same name", func(t *testing.T) {
		got, src, ok := ts.resolveTheme("dracula")
		require.True(t, ok)
		assert.Equal(t, ThemeSourceBuiltin, src)
		assert.Equal(t, builtinThemes["dracula"], got)
	})

	t.Run("reports an unknown name", func(t *testing.T) {
		_, _, ok := ts.resolveTheme("nope")
		assert.False(t, ok)
	})
}

func TestThemeNameLists(t *testing.T) {
	ts := newTestTerminalServer()
	ts.Themes["zzz-mine"] = Theme{}
	ts.Themes["dracula"] = Theme{}
	ts.AvailableThemes = []string{"zzz-mine", "dracula"}

	t.Run("allThemeNames lists built-in and user themes once, sorted", func(t *testing.T) {
		names := ts.allThemeNames()
		assert.IsIncreasing(t, names)
		assert.Contains(t, names, "zzz-mine")
		assert.Len(t, names, len(builtinThemes)+1)
	})

	t.Run("menuThemeNames is a copy in menu order", func(t *testing.T) {
		names := ts.menuThemeNames()
		assert.Equal(t, []string{"zzz-mine", "dracula"}, names)
		names[0] = "changed"
		assert.Equal(t, "zzz-mine", ts.AvailableThemes[0])
	})

	t.Run("addMenuTheme appends once", func(t *testing.T) {
		ts.addMenuTheme("one-light")
		ts.addMenuTheme("dracula")
		assert.Equal(t, []string{"zzz-mine", "dracula", "one-light"}, ts.AvailableThemes)
	})
}

func TestLoadThemeState(t *testing.T) {
	load := func(t *testing.T, yamlText string) (ThemeState, string) {
		t.Helper()
		path := writeTempConfig(t, yamlText)
		var cfg Config
		keys, err := LoadConfig(path, &cfg)
		require.NoError(t, err)
		var state ThemeState
		logged := captureLog(func() { state = LoadThemeState(cfg, keys) })
		return state, logged
	}

	t.Run("resolves the active theme from either source", func(t *testing.T) {
		state, logged := load(t, `
theme: mine
available-themes: [dracula, mine]
themes:
  mine:
    foreground: "#123456"
`)
		assert.Equal(t, "mine", state.ActiveName)
		assert.Equal(t, "#123456", state.Active.Foreground)
		assert.Equal(t, []string{"dracula", "mine"}, state.AvailableThemes)
		assert.Empty(t, logged)

		state, _ = load(t, "theme: dracula\n")
		assert.Equal(t, "dracula", state.ActiveName)
		assert.Equal(t, builtinThemes["dracula"], state.Active)
	})

	t.Run("seeds the menu from the user themes when available-themes is absent", func(t *testing.T) {
		state, _ := load(t, `
themes:
  zebra:
    foreground: "#ffffff"
  alpha:
    foreground: "#000000"
`)
		assert.Equal(t, []string{"alpha", "zebra"}, state.AvailableThemes)
	})

	t.Run("keeps an explicitly empty menu empty", func(t *testing.T) {
		state, _ := load(t, `
available-themes: []
themes:
  mine:
    foreground: "#ffffff"
`)
		assert.Equal(t, []string{}, state.AvailableThemes)
	})

	t.Run("drops a user theme a built-in shadows, with a warning", func(t *testing.T) {
		state, logged := load(t, `
theme: dracula
themes:
  dracula:
    foreground: "#000000"
`)
		assert.NotContains(t, state.Themes, "dracula")
		assert.Equal(t, builtinThemes["dracula"], state.Active, "the built-in wins")
		assert.Equal(t, []string{"dracula"}, state.AvailableThemes, "the menu keeps the name, now meaning the built-in")
		assert.Contains(t, logged, "themes.dracula is ignored")
	})

	t.Run("skips menu entries that name no theme, and duplicates", func(t *testing.T) {
		state, logged := load(t, "available-themes: [dracula, gone, dracula]\n")
		assert.Equal(t, []string{"dracula"}, state.AvailableThemes)
		assert.Contains(t, logged, `skipping "gone"`)
	})

	t.Run("falls back to default-theme, then to no theme", func(t *testing.T) {
		state, logged := load(t, "theme: gone\ndefault-theme: one-light\n")
		assert.Equal(t, "one-light", state.ActiveName)
		assert.Equal(t, "one-light", state.DefaultTheme)
		assert.Contains(t, logged, `theme: "gone"`)

		state, logged = load(t, "theme: gone\ndefault-theme: also-gone\n")
		assert.Equal(t, "", state.ActiveName)
		assert.Equal(t, Theme{}, state.Active)
		assert.Contains(t, logged, `default-theme: "also-gone"`)
	})

	t.Run("no theme keys means no theme, without warnings", func(t *testing.T) {
		state, logged := load(t, "server:\n  port: 9000\n")
		assert.Equal(t, "", state.ActiveName)
		assert.Equal(t, Theme{}, state.Active)
		assert.Equal(t, []string{}, state.AvailableThemes)
		assert.Empty(t, logged)
	})
}
