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
