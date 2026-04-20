package src

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// writeTempConfig writes content to a temporary YAML file and returns its path.
// The file is removed automatically when the test finishes.
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "b3tty-*.yaml")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(f.Name()) })
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func TestValidateConfig(t *testing.T) {
	t.Run("valid full config passes", func(t *testing.T) {
		path := writeTempConfig(t, `
server:
  tls: true
  cert-file: "/path/to/cert"
  key-file: "/path/to/key"
  no-auth: false
  no-browser: false
  port: 8443
terminal:
  font-family: "monospace"
  font-size: 14
  cursor-blink: true
  rows: 24
  columns: 80
theme: "my-theme"
themes:
  my-theme:
    foreground: "#dbdbdb"
    background: "#15191e"
    black: "#14181d"
    bright-black: "#404040"
    red: "#eb5a4b"
    bright-red: "#ee837b"
profiles:
  work:
    working-directory: "~/projects"
    title: "Work"
    shell: "/bin/zsh"
    commands:
      - "echo hello"
`)
		assert.NoError(t, ValidateConfig(path))
	})

	t.Run("empty config passes", func(t *testing.T) {
		path := writeTempConfig(t, "")
		assert.NoError(t, ValidateConfig(path))
	})

	t.Run("partial config with only terminal section passes", func(t *testing.T) {
		path := writeTempConfig(t, `
terminal:
  font-size: 16
  rows: 30
`)
		assert.NoError(t, ValidateConfig(path))
	})

	t.Run("partial config with only server section passes", func(t *testing.T) {
		path := writeTempConfig(t, `
server:
  no-auth: true
  port: 9000
`)
		assert.NoError(t, ValidateConfig(path))
	})

	t.Run("unknown top-level key is rejected", func(t *testing.T) {
		path := writeTempConfig(t, `
unknown-key: true
`)
		err := ValidateConfig(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown-key")
	})

	t.Run("misspelled server key is rejected", func(t *testing.T) {
		path := writeTempConfig(t, `
server:
  tls-enabled: true
`)
		err := ValidateConfig(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tls-enabled")
	})

	t.Run("misspelled terminal key is rejected", func(t *testing.T) {
		path := writeTempConfig(t, `
terminal:
  fontsize: 14
`)
		err := ValidateConfig(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "fontsize")
	})

	t.Run("misspelled theme color key is rejected", func(t *testing.T) {
		path := writeTempConfig(t, `
themes:
  my-theme:
    colour: "#ffffff"
`)
		err := ValidateConfig(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "colour")
	})

	t.Run("misspelled profile key is rejected", func(t *testing.T) {
		path := writeTempConfig(t, `
profiles:
  work:
    workingdirectory: "~/projects"
`)
		err := ValidateConfig(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "workingdirectory")
	})

	t.Run("wrong type for font-size is rejected", func(t *testing.T) {
		path := writeTempConfig(t, `
terminal:
  font-size: "big"
`)
		err := ValidateConfig(path)
		assert.Error(t, err)
	})

	t.Run("wrong type for tls bool is rejected", func(t *testing.T) {
		// yaml.v3 coerces strings/numbers to bool, but a mapping never coerces.
		path := writeTempConfig(t, `
server:
  tls:
    enabled: true
`)
		err := ValidateConfig(path)
		assert.Error(t, err)
	})

	t.Run("wrong type for rows is rejected", func(t *testing.T) {
		// yaml.v3 truncates floats to int, but a mapping never coerces to int.
		path := writeTempConfig(t, `
terminal:
  rows:
    count: 24
`)
		err := ValidateConfig(path)
		assert.Error(t, err)
	})

	t.Run("wrong type for commands is rejected", func(t *testing.T) {
		path := writeTempConfig(t, `
profiles:
  work:
    commands: "not-a-list"
`)
		err := ValidateConfig(path)
		assert.Error(t, err)
	})

	t.Run("error message includes the config file path", func(t *testing.T) {
		path := writeTempConfig(t, `
unknown-key: true
`)
		err := ValidateConfig(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), path)
	})

	t.Run("file not found returns error", func(t *testing.T) {
		err := ValidateConfig("/nonexistent/path/b3tty.yaml")
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// buildConfigYAML
// ---------------------------------------------------------------------------

// mustBuildConfigYAML calls buildConfigYAML and fails the test on error.
func mustBuildConfigYAML(t *testing.T, themeName string, colors map[string]any) string {
	t.Helper()
	out, err := buildConfigYAML(themeName, colors)
	require.NoError(t, err)
	return out
}

// parseConfigYAML is a test helper that unmarshals the output of buildConfigYAML
// into a generic map so tests can inspect values without relying on string layout.
func parseConfigYAML(t *testing.T, s string) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(s), &out))
	return out
}

func TestBuildConfigYAML(t *testing.T) {
	t.Run("theme name appears at top level and under themes", func(t *testing.T) {
		out := parseConfigYAML(t, mustBuildConfigYAML(t, "b3tty-dark", map[string]any{
			"foreground": "#ffffff",
		}))
		assert.Equal(t, "b3tty-dark", out["theme"])
		themes, ok := out["themes"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, themes, "b3tty-dark")
	})

	t.Run("color values round-trip correctly", func(t *testing.T) {
		colors := map[string]any{
			"foreground":           "#ffffff",
			"background":           "#000000",
			"bright-red":           "#ff5555",
			"selection-background": "#44475a",
		}
		out := parseConfigYAML(t, mustBuildConfigYAML(t, "my-theme", colors))
		themes := out["themes"].(map[string]any)
		palette := themes["my-theme"].(map[string]any)
		for k, v := range colors {
			assert.Equal(t, v.(string), palette[k], "key %q", k)
		}
	})

	t.Run("hex colors starting with # are valid YAML", func(t *testing.T) {
		// A bare # in YAML begins an inline comment; yaml.v3 must quote it.
		yamlOut := mustBuildConfigYAML(t, "b3tty-dark", map[string]any{"foreground": "#aabbcc"})
		out := parseConfigYAML(t, yamlOut)
		palette := out["themes"].(map[string]any)["b3tty-dark"].(map[string]any)
		assert.Equal(t, "#aabbcc", palette["foreground"])
	})

	t.Run("empty color map produces valid YAML with empty theme block", func(t *testing.T) {
		out := parseConfigYAML(t, mustBuildConfigYAML(t, "b3tty-light", map[string]any{}))
		assert.Equal(t, "b3tty-light", out["theme"])
		themes := out["themes"].(map[string]any)
		assert.Contains(t, themes, "b3tty-light")
	})

	t.Run("output passes ValidateConfig", func(t *testing.T) {
		colors := map[string]any{
			"foreground": "#dbdbdb",
			"background": "#15191e",
			"red":        "#eb5a4b",
			"bright-red": "#ee837b",
		}
		yamlOut := mustBuildConfigYAML(t, "b3tty_dark", colors)
		path := writeTempConfig(t, yamlOut)
		assert.NoError(t, ValidateConfig(path))
	})

	t.Run("different theme names produce distinct sections", func(t *testing.T) {
		outA := parseConfigYAML(t, mustBuildConfigYAML(t, "alpha", map[string]any{"foreground": "#111111"}))
		outB := parseConfigYAML(t, mustBuildConfigYAML(t, "beta", map[string]any{"foreground": "#222222"}))
		assert.Equal(t, "alpha", outA["theme"])
		assert.Equal(t, "beta", outB["theme"])
		assert.Contains(t, outA["themes"].(map[string]any), "alpha")
		assert.Contains(t, outB["themes"].(map[string]any), "beta")
	})

	t.Run("non-string values are silently dropped", func(t *testing.T) {
		colors := map[string]any{
			"foreground": "#ffffff",
			"count":      float64(3),
			"flag":       true,
		}
		out := parseConfigYAML(t, mustBuildConfigYAML(t, "b3tty-dark", colors))
		palette := out["themes"].(map[string]any)["b3tty-dark"].(map[string]any)
		assert.Equal(t, "#ffffff", palette["foreground"])
		assert.NotContains(t, palette, "count")
		assert.NotContains(t, palette, "flag")
	})

	t.Run("invalid color strings are silently dropped", func(t *testing.T) {
		colors := map[string]any{
			"foreground": "#ffffff",
			"background": "rgb(0,0,0)",
			"red":        "not#valid",
		}
		out := parseConfigYAML(t, mustBuildConfigYAML(t, "b3tty-dark", colors))
		palette := out["themes"].(map[string]any)["b3tty-dark"].(map[string]any)
		assert.Equal(t, "#ffffff", palette["foreground"])
		assert.NotContains(t, palette, "background")
		assert.NotContains(t, palette, "red")
	})

	t.Run("mix of valid, invalid, and non-string values keeps only valid colors", func(t *testing.T) {
		colors := map[string]any{
			"foreground": "#aabbcc", // valid hex — kept
			"background": "white",   // valid named color — kept
			"red":        "#gggggg", // invalid hex chars — dropped
			"green":      42,        // non-string — dropped
		}
		out := parseConfigYAML(t, mustBuildConfigYAML(t, "mixed", colors))
		palette := out["themes"].(map[string]any)["mixed"].(map[string]any)
		assert.Equal(t, "#aabbcc", palette["foreground"])
		assert.Equal(t, "white", palette["background"])
		assert.NotContains(t, palette, "red")
		assert.NotContains(t, palette, "green")
	})
}

// ---------------------------------------------------------------------------
// UpdateThemeInConfig
// ---------------------------------------------------------------------------

// setupUpdateThemeTest creates a temp HOME directory, points HOME at it, and
// returns a helper that reads the resulting conf.yaml back as a generic map.
func setupUpdateThemeTest(t *testing.T) func() map[string]any {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	configPath := tmpHome + "/.config/b3tty/conf.yaml"
	return func() map[string]any {
		t.Helper()
		data, err := os.ReadFile(configPath)
		require.NoError(t, err)
		var out map[string]any
		require.NoError(t, yaml.Unmarshal(data, &out))
		return out
	}
}

// writeInitialConfig pre-populates the conf.yaml under the current HOME.
func writeInitialConfig(t *testing.T, content string) {
	t.Helper()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	dir := home + "/.config/b3tty"
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(dir+"/conf.yaml", []byte(content), 0644))
}

func TestUpdateThemeInConfig(t *testing.T) {
	t.Run("creates config file when none exists", func(t *testing.T) {
		readConfig := setupUpdateThemeTest(t)
		require.NoError(t, UpdateThemeInConfig("dracula", map[string]any{"foreground": "#f8f8f2"}))
		out := readConfig()
		assert.Equal(t, "dracula", out["theme"])
	})

	t.Run("sets the theme key at the top level", func(t *testing.T) {
		readConfig := setupUpdateThemeTest(t)
		require.NoError(t, UpdateThemeInConfig("catppuccin-mocha", map[string]any{"foreground": "#cdd6f4"}))
		assert.Equal(t, "catppuccin-mocha", readConfig()["theme"])
	})

	t.Run("adds color entries under the themes section", func(t *testing.T) {
		readConfig := setupUpdateThemeTest(t)
		require.NoError(t, UpdateThemeInConfig("dracula", map[string]any{
			"foreground": "#f8f8f2",
			"background": "#282a36",
		}))
		out := readConfig()
		themes := out["themes"].(map[string]any)
		palette := themes["dracula"].(map[string]any)
		assert.Equal(t, "#f8f8f2", palette["foreground"])
		assert.Equal(t, "#282a36", palette["background"])
	})

	t.Run("preserves existing settings in the config file", func(t *testing.T) {
		readConfig := setupUpdateThemeTest(t)
		writeInitialConfig(t, `
server:
  no-auth: true
  port: 9000
theme: b3tty-dark
themes:
  b3tty-dark:
    foreground: "#dbdbdb"
`)
		require.NoError(t, UpdateThemeInConfig("dracula", map[string]any{"foreground": "#f8f8f2"}))
		out := readConfig()
		server := out["server"].(map[string]any)
		assert.Equal(t, true, server["no-auth"])
		assert.Equal(t, 9000, server["port"])
	})

	t.Run("updates the theme key without touching other existing themes", func(t *testing.T) {
		readConfig := setupUpdateThemeTest(t)
		writeInitialConfig(t, `
theme: b3tty-dark
themes:
  b3tty-dark:
    foreground: "#dbdbdb"
`)
		require.NoError(t, UpdateThemeInConfig("dracula", map[string]any{"foreground": "#f8f8f2"}))
		out := readConfig()
		themes := out["themes"].(map[string]any)
		assert.Equal(t, "dracula", out["theme"])
		assert.Contains(t, themes, "b3tty-dark")
		assert.Contains(t, themes, "dracula")
	})

	t.Run("does not overwrite colors when the theme already exists in the themes section", func(t *testing.T) {
		readConfig := setupUpdateThemeTest(t)
		writeInitialConfig(t, `
theme: dracula
themes:
  dracula:
    foreground: "#original"
`)
		require.NoError(t, UpdateThemeInConfig("dracula", map[string]any{"foreground": "#new"}))
		out := readConfig()
		palette := out["themes"].(map[string]any)["dracula"].(map[string]any)
		assert.Equal(t, "#original", palette["foreground"])
	})

	t.Run("creates the themes section when the config has none", func(t *testing.T) {
		readConfig := setupUpdateThemeTest(t)
		writeInitialConfig(t, `
server:
  port: 8080
`)
		require.NoError(t, UpdateThemeInConfig("dracula", map[string]any{"foreground": "#f8f8f2"}))
		out := readConfig()
		themes, ok := out["themes"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, themes, "dracula")
	})

	t.Run("silently drops non-string color values", func(t *testing.T) {
		readConfig := setupUpdateThemeTest(t)
		require.NoError(t, UpdateThemeInConfig("dracula", map[string]any{
			"foreground": "#f8f8f2",
			"count":      42,
			"flag":       true,
		}))
		palette := readConfig()["themes"].(map[string]any)["dracula"].(map[string]any)
		assert.Equal(t, "#f8f8f2", palette["foreground"])
		assert.NotContains(t, palette, "count")
		assert.NotContains(t, palette, "flag")
	})

	t.Run("silently drops invalid color strings", func(t *testing.T) {
		readConfig := setupUpdateThemeTest(t)
		require.NoError(t, UpdateThemeInConfig("dracula", map[string]any{
			"foreground": "#f8f8f2",
			"background": "rgb(40,42,54)",
		}))
		palette := readConfig()["themes"].(map[string]any)["dracula"].(map[string]any)
		assert.Equal(t, "#f8f8f2", palette["foreground"])
		assert.NotContains(t, palette, "background")
	})

	t.Run("output passes ValidateConfig", func(t *testing.T) {
		readConfig := setupUpdateThemeTest(t)
		home, _ := os.UserHomeDir()
		configPath := home + "/.config/b3tty/conf.yaml"
		require.NoError(t, UpdateThemeInConfig("dracula", map[string]any{
			"foreground":  "#f8f8f2",
			"background":  "#282a36",
			"cursor":      "#f8f8f2",
			"red":         "#ff5555",
			"bright-red":  "#ff6e6e",
		}))
		_ = readConfig() // ensure file exists
		assert.NoError(t, ValidateConfig(configPath))
	})
}
