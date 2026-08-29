package src

import (
	"errors"
	"os"
	"path/filepath"
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
// UpdateThemeInConfig
// ---------------------------------------------------------------------------

// setupUpdateThemeTest creates a temp HOME directory, points HOME at it, and
// returns a helper that reads the resulting conf.yaml back as a generic map,
// along with the explicit config file path to pass to UpdateThemeInConfig /
// SaveThemeToConfig.
func setupUpdateThemeTest(t *testing.T) (func() map[string]any, string) {
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
	}, configPath
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
		readConfig, cfgPath := setupUpdateThemeTest(t)
		require.NoError(t, UpdateThemeInConfig(cfgPath, "dracula", map[string]any{"foreground": "#f8f8f2"}))
		out := readConfig()
		assert.Equal(t, "dracula", out["theme"])
	})

	t.Run("sets the theme key at the top level", func(t *testing.T) {
		readConfig, cfgPath := setupUpdateThemeTest(t)
		require.NoError(t, UpdateThemeInConfig(cfgPath, "catppuccin-mocha", map[string]any{"foreground": "#cdd6f4"}))
		assert.Equal(t, "catppuccin-mocha", readConfig()["theme"])
	})

	t.Run("adds color entries under the themes section", func(t *testing.T) {
		readConfig, cfgPath := setupUpdateThemeTest(t)
		require.NoError(t, UpdateThemeInConfig(cfgPath, "dracula", map[string]any{
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
		readConfig, cfgPath := setupUpdateThemeTest(t)
		writeInitialConfig(t, `
server:
  no-auth: true
  port: 9000
theme: b3tty-dark
themes:
  b3tty-dark:
    foreground: "#dbdbdb"
`)
		require.NoError(t, UpdateThemeInConfig(cfgPath, "dracula", map[string]any{"foreground": "#f8f8f2"}))
		out := readConfig()
		server := out["server"].(map[string]any)
		assert.Equal(t, true, server["no-auth"])
		assert.Equal(t, 9000, server["port"])
	})

	t.Run("updates the theme key without touching other existing themes", func(t *testing.T) {
		readConfig, cfgPath := setupUpdateThemeTest(t)
		writeInitialConfig(t, `
theme: b3tty-dark
themes:
  b3tty-dark:
    foreground: "#dbdbdb"
`)
		require.NoError(t, UpdateThemeInConfig(cfgPath, "dracula", map[string]any{"foreground": "#f8f8f2"}))
		out := readConfig()
		themes := out["themes"].(map[string]any)
		assert.Equal(t, "dracula", out["theme"])
		assert.Contains(t, themes, "b3tty-dark")
		assert.Contains(t, themes, "dracula")
	})

	t.Run("does not overwrite colors when the theme already exists in the themes section", func(t *testing.T) {
		readConfig, cfgPath := setupUpdateThemeTest(t)
		writeInitialConfig(t, `
theme: dracula
themes:
  dracula:
    foreground: "#original"
`)
		require.NoError(t, UpdateThemeInConfig(cfgPath, "dracula", map[string]any{"foreground": "#new"}))
		out := readConfig()
		palette := out["themes"].(map[string]any)["dracula"].(map[string]any)
		assert.Equal(t, "#original", palette["foreground"])
	})

	t.Run("creates the themes section when the config has none", func(t *testing.T) {
		readConfig, cfgPath := setupUpdateThemeTest(t)
		writeInitialConfig(t, `
server:
  port: 8080
`)
		require.NoError(t, UpdateThemeInConfig(cfgPath, "dracula", map[string]any{"foreground": "#f8f8f2"}))
		out := readConfig()
		themes, ok := out["themes"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, themes, "dracula")
	})

	t.Run("silently drops non-string color values", func(t *testing.T) {
		readConfig, cfgPath := setupUpdateThemeTest(t)
		require.NoError(t, UpdateThemeInConfig(cfgPath, "dracula", map[string]any{
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
		readConfig, cfgPath := setupUpdateThemeTest(t)
		require.NoError(t, UpdateThemeInConfig(cfgPath, "dracula", map[string]any{
			"foreground": "#f8f8f2",
			"background": "rgb(40,42,54)",
		}))
		palette := readConfig()["themes"].(map[string]any)["dracula"].(map[string]any)
		assert.Equal(t, "#f8f8f2", palette["foreground"])
		assert.NotContains(t, palette, "background")
	})

	t.Run("output passes ValidateConfig", func(t *testing.T) {
		readConfig, cfgPath := setupUpdateThemeTest(t)
		require.NoError(t, UpdateThemeInConfig(cfgPath, "dracula", map[string]any{
			"foreground": "#f8f8f2",
			"background": "#282a36",
			"cursor":     "#f8f8f2",
			"red":        "#ff5555",
			"bright-red": "#ff6e6e",
		}))
		_ = readConfig() // ensure file exists
		assert.NoError(t, ValidateConfig(cfgPath))
	})
}

// ---------------------------------------------------------------------------
// SaveThemeToConfig
// ---------------------------------------------------------------------------

func TestSaveThemeToConfig(t *testing.T) {
	t.Run("creates config file when none exists", func(t *testing.T) {
		readConfig, cfgPath := setupUpdateThemeTest(t)
		require.NoError(t, SaveThemeToConfig(cfgPath, "dracula", map[string]any{"foreground": "#f8f8f2"}))
		out := readConfig()
		assert.Equal(t, "dracula", out["theme"])
	})

	t.Run("sets the theme key at the top level", func(t *testing.T) {
		readConfig, cfgPath := setupUpdateThemeTest(t)
		require.NoError(t, SaveThemeToConfig(cfgPath, "catppuccin-mocha", map[string]any{"foreground": "#cdd6f4"}))
		assert.Equal(t, "catppuccin-mocha", readConfig()["theme"])
	})

	t.Run("always overwrites existing theme colors", func(t *testing.T) {
		readConfig, cfgPath := setupUpdateThemeTest(t)
		writeInitialConfig(t, `
theme: dracula
themes:
  dracula:
    foreground: "#aaaaaa"
`)
		require.NoError(t, SaveThemeToConfig(cfgPath, "dracula", map[string]any{"foreground": "#ffffff"}))
		out := readConfig()
		palette := out["themes"].(map[string]any)["dracula"].(map[string]any)
		assert.Equal(t, "#ffffff", palette["foreground"])
	})

	t.Run("preserves other existing themes", func(t *testing.T) {
		readConfig, cfgPath := setupUpdateThemeTest(t)
		writeInitialConfig(t, `
theme: b3tty-dark
themes:
  b3tty-dark:
    foreground: "#dbdbdb"
`)
		require.NoError(t, SaveThemeToConfig(cfgPath, "dracula", map[string]any{"foreground": "#f8f8f2"}))
		out := readConfig()
		themes := out["themes"].(map[string]any)
		assert.Contains(t, themes, "b3tty-dark")
		assert.Contains(t, themes, "dracula")
	})

	t.Run("preserves non-theme config sections", func(t *testing.T) {
		readConfig, cfgPath := setupUpdateThemeTest(t)
		writeInitialConfig(t, `
server:
  no-auth: true
  port: 9000
`)
		require.NoError(t, SaveThemeToConfig(cfgPath, "dracula", map[string]any{"foreground": "#f8f8f2"}))
		out := readConfig()
		server := out["server"].(map[string]any)
		assert.Equal(t, true, server["no-auth"])
		assert.Equal(t, 9000, server["port"])
	})

	t.Run("silently drops invalid color strings", func(t *testing.T) {
		readConfig, cfgPath := setupUpdateThemeTest(t)
		require.NoError(t, SaveThemeToConfig(cfgPath, "dracula", map[string]any{
			"foreground": "#f8f8f2",
			"background": "rgb(40,42,54)",
		}))
		palette := readConfig()["themes"].(map[string]any)["dracula"].(map[string]any)
		assert.Equal(t, "#f8f8f2", palette["foreground"])
		assert.NotContains(t, palette, "background")
	})

	t.Run("output passes ValidateConfig", func(t *testing.T) {
		readConfig, cfgPath := setupUpdateThemeTest(t)
		require.NoError(t, SaveThemeToConfig(cfgPath, "dracula", map[string]any{
			"foreground": "#f8f8f2",
			"background": "#282a36",
			"cursor":     "#f8f8f2",
			"red":        "#ff5555",
			"bright-red": "#ff6e6e",
		}))
		_ = readConfig()
		assert.NoError(t, ValidateConfig(cfgPath))
	})

	t.Run("preserves background-image from existing theme entry", func(t *testing.T) {
		readConfig, cfgPath := setupUpdateThemeTest(t)
		writeInitialConfig(t, `
theme: my-theme
themes:
  my-theme:
    foreground: "#aaaaaa"
    background-image: "/home/user/bg.png"
`)
		require.NoError(t, SaveThemeToConfig(cfgPath, "my-theme", map[string]any{"foreground": "#ffffff"}))
		out := readConfig()
		palette := out["themes"].(map[string]any)["my-theme"].(map[string]any)
		assert.Equal(t, "#ffffff", palette["foreground"])
		assert.Equal(t, "/home/user/bg.png", palette["background-image"])
	})
}

// ---------------------------------------------------------------------------
// SaveProfileToConfig
// ---------------------------------------------------------------------------

func TestSaveProfileToConfig(t *testing.T) {
	readConfig := func(path string) map[string]any {
		t.Helper()
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var out map[string]any
		require.NoError(t, yaml.Unmarshal(data, &out))
		return out
	}

	profile := func(shell, title, wd, root string, commands []string) Profile {
		return NewProfile(shell, wd, root, title, commands)
	}

	t.Run("creates profiles section in empty file", func(t *testing.T) {
		path := writeTempConfig(t, "")
		require.NoError(t, SaveProfileToConfig(path, "dev", profile("/bin/bash", "Dev", "~/dev", "/", []string{"echo ready"})))
		out := readConfig(path)
		profiles, ok := out["profiles"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, profiles, "dev")
	})

	t.Run("writes all profile fields", func(t *testing.T) {
		path := writeTempConfig(t, "")
		require.NoError(t, SaveProfileToConfig(path, "dev", profile("/bin/zsh", "Dev", "~/projects", "/opt", []string{"npm start", "echo ready"})))
		out := readConfig(path)
		entry := out["profiles"].(map[string]any)["dev"].(map[string]any)
		assert.Equal(t, "/bin/zsh", entry["shell"])
		assert.Equal(t, "Dev", entry["title"])
		assert.Equal(t, "~/projects", entry["working-directory"])
		assert.Equal(t, "/opt", entry["root"])
		commands, _ := entry["commands"].([]any)
		assert.Equal(t, "npm start", commands[0])
		assert.Equal(t, "echo ready", commands[1])
	})

	t.Run("overwrites existing profile entry", func(t *testing.T) {
		path := writeTempConfig(t, `
profiles:
  dev:
    shell: /bin/bash
    title: Old Title
`)
		require.NoError(t, SaveProfileToConfig(path, "dev", profile("/bin/zsh", "New Title", "~/dev", "/", nil)))
		out := readConfig(path)
		entry := out["profiles"].(map[string]any)["dev"].(map[string]any)
		assert.Equal(t, "/bin/zsh", entry["shell"])
		assert.Equal(t, "New Title", entry["title"])
	})

	t.Run("preserves other profiles", func(t *testing.T) {
		path := writeTempConfig(t, `
profiles:
  existing:
    shell: /bin/sh
`)
		require.NoError(t, SaveProfileToConfig(path, "new", profile("/bin/bash", "", "", "", nil)))
		out := readConfig(path)
		profiles := out["profiles"].(map[string]any)
		assert.Contains(t, profiles, "existing")
		assert.Contains(t, profiles, "new")
	})

	t.Run("preserves other top-level sections", func(t *testing.T) {
		path := writeTempConfig(t, `
theme: b3tty-dark
`)
		require.NoError(t, SaveProfileToConfig(path, "dev", profile("", "", "", "", nil)))
		out := readConfig(path)
		assert.Equal(t, "b3tty-dark", out["theme"])
	})

	t.Run("output passes ValidateConfig", func(t *testing.T) {
		path := writeTempConfig(t, "")
		require.NoError(t, SaveProfileToConfig(path, "dev", profile("/bin/zsh", "Dev", "~/dev", "/", []string{"echo hello"})))
		assert.NoError(t, ValidateConfig(path))
	})
}

// ---------------------------------------------------------------------------
// DeleteProfileFromConfig
// ---------------------------------------------------------------------------

func TestDeleteProfileFromConfig(t *testing.T) {
	readConfig := func(path string) map[string]any {
		t.Helper()
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var out map[string]any
		require.NoError(t, yaml.Unmarshal(data, &out))
		return out
	}

	t.Run("removes named profile entry", func(t *testing.T) {
		path := writeTempConfig(t, `
profiles:
  dev:
    shell: /bin/bash
  work:
    shell: /bin/zsh
`)
		require.NoError(t, DeleteProfileFromConfig(path, "dev"))
		out := readConfig(path)
		profiles := out["profiles"].(map[string]any)
		assert.NotContains(t, profiles, "dev")
		assert.Contains(t, profiles, "work")
	})

	t.Run("preserves other top-level sections", func(t *testing.T) {
		path := writeTempConfig(t, `
theme: b3tty-dark
profiles:
  dev:
    shell: /bin/bash
`)
		require.NoError(t, DeleteProfileFromConfig(path, "dev"))
		out := readConfig(path)
		assert.Equal(t, "b3tty-dark", out["theme"])
	})

	t.Run("no-op when profiles section is absent", func(t *testing.T) {
		path := writeTempConfig(t, `
theme: b3tty-dark
`)
		assert.NoError(t, DeleteProfileFromConfig(path, "dev"))
		out := readConfig(path)
		assert.Equal(t, "b3tty-dark", out["theme"])
	})

	t.Run("no-op when named profile is absent", func(t *testing.T) {
		path := writeTempConfig(t, `
profiles:
  work:
    shell: /bin/zsh
`)
		assert.NoError(t, DeleteProfileFromConfig(path, "nonexistent"))
		out := readConfig(path)
		profiles := out["profiles"].(map[string]any)
		assert.Contains(t, profiles, "work")
	})
}

// ---------------------------------------------------------------------------
// resolveConfigPath
// ---------------------------------------------------------------------------

func TestResolveConfigPath(t *testing.T) {
	t.Run("returns the given path unchanged when non-empty", func(t *testing.T) {
		got, err := resolveConfigPath("/some/explicit/path.yaml")
		require.NoError(t, err)
		assert.Equal(t, "/some/explicit/path.yaml", got)
	})

	t.Run("falls back to the default config path under $HOME when empty", func(t *testing.T) {
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)
		got, err := resolveConfigPath("")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(tmpHome, DOT_CONFIG_PATH, B3TTY_CONFIG_PATH, CONFIG_FILE_NAME), got)
	})

	t.Run("returns an error when the home directory cannot be determined", func(t *testing.T) {
		t.Setenv("HOME", "")
		_, err := resolveConfigPath("")
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// loadConfigMap
// ---------------------------------------------------------------------------

func TestLoadConfigMap(t *testing.T) {
	t.Run("returns an empty map when the file does not exist", func(t *testing.T) {
		cfg, err := loadConfigMap(filepath.Join(t.TempDir(), "does-not-exist.yaml"), "ctx")
		require.NoError(t, err)
		assert.Empty(t, cfg)
	})

	t.Run("returns an empty map when the file is empty", func(t *testing.T) {
		path := writeTempConfig(t, "")
		cfg, err := loadConfigMap(path, "ctx")
		require.NoError(t, err)
		assert.Empty(t, cfg)
	})

	t.Run("parses existing YAML content into a map", func(t *testing.T) {
		path := writeTempConfig(t, `
theme: dracula
server:
  port: 9000
`)
		cfg, err := loadConfigMap(path, "ctx")
		require.NoError(t, err)
		assert.Equal(t, "dracula", cfg["theme"])
		server, ok := cfg["server"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, 9000, server["port"])
	})

	t.Run("returns a parse error prefixed with errContext for invalid YAML", func(t *testing.T) {
		path := writeTempConfig(t, "theme: [unclosed")
		_, err := loadConfigMap(path, "MyCaller")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MyCaller")
		assert.Contains(t, err.Error(), "parse existing config")
	})

	t.Run("treats a read error other than not-exist as an empty config", func(t *testing.T) {
		// Reading a directory as a file fails, but is not os.ErrNotExist —
		// loadConfigMap's contract is to swallow any read error, not just
		// the file-absent case, since every caller relied on that before
		// this helper was extracted.
		cfg, err := loadConfigMap(t.TempDir(), "ctx")
		require.NoError(t, err)
		assert.Empty(t, cfg)
	})
}

// ---------------------------------------------------------------------------
// writeConfigMap
// ---------------------------------------------------------------------------

// erroringYAMLValue implements yaml.Marshaler and always fails to marshal.
// yaml.v3 panics rather than returning an error for genuinely unsupported Go
// types (e.g. chan), so this is the reliable way to exercise writeConfigMap's
// yaml.Marshal error path.
type erroringYAMLValue struct{}

func (erroringYAMLValue) MarshalYAML() (any, error) {
	return nil, errors.New("boom")
}

func TestWriteConfigMap(t *testing.T) {
	t.Run("writes valid YAML to the given path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "conf.yaml")
		require.NoError(t, writeConfigMap(path, map[string]any{"theme": "dracula"}, "ctx"))
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var out map[string]any
		require.NoError(t, yaml.Unmarshal(data, &out))
		assert.Equal(t, "dracula", out["theme"])
	})

	t.Run("creates parent directories that do not exist", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nested", "dirs", "conf.yaml")
		require.NoError(t, writeConfigMap(path, map[string]any{"theme": "dracula"}, "ctx"))
		_, err := os.Stat(path)
		assert.NoError(t, err)
	})

	t.Run("returns a marshal error prefixed with errContext when a value fails to marshal", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "conf.yaml")
		err := writeConfigMap(path, map[string]any{"bad": erroringYAMLValue{}}, "MyCaller")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MyCaller")
		assert.Contains(t, err.Error(), "boom")
		_, statErr := os.Stat(path)
		assert.True(t, os.IsNotExist(statErr), "file must not be written when marshaling fails")
	})

	t.Run("returns an error when the parent directory cannot be created", func(t *testing.T) {
		// blocker is a regular file; MkdirAll must fail when asked to create
		// a directory at or beneath a path component that is already a file.
		blocker := filepath.Join(t.TempDir(), "blocker")
		require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0644))
		path := filepath.Join(blocker, "sub", "conf.yaml")
		err := writeConfigMap(path, map[string]any{"theme": "dracula"}, "ctx")
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// getOrCreateSection
// ---------------------------------------------------------------------------

func TestGetOrCreateSection(t *testing.T) {
	t.Run("returns the existing section when present and of the right type", func(t *testing.T) {
		cfg := map[string]any{"themes": map[string]any{"dracula": map[string]any{"foreground": "#f8f8f2"}}}
		section := getOrCreateSection(cfg, "themes")
		assert.Equal(t, map[string]any{"dracula": map[string]any{"foreground": "#f8f8f2"}}, section)
	})

	t.Run("creates and stores an empty section when the key is absent", func(t *testing.T) {
		cfg := map[string]any{}
		section := getOrCreateSection(cfg, "profiles")
		assert.Equal(t, map[string]any{}, section)
		assert.Contains(t, cfg, "profiles")
		assert.Equal(t, section, cfg["profiles"])
	})

	t.Run("replaces a value of the wrong type with a new empty section", func(t *testing.T) {
		cfg := map[string]any{"server": "not a map"}
		section := getOrCreateSection(cfg, "server")
		assert.Equal(t, map[string]any{}, section)
		assert.Equal(t, section, cfg["server"])
	})

	t.Run("mutations to the returned section are reflected in cfg", func(t *testing.T) {
		cfg := map[string]any{}
		section := getOrCreateSection(cfg, "terminal")
		section["rows"] = 24
		assert.Equal(t, 24, cfg["terminal"].(map[string]any)["rows"])
	})
}

// ---------------------------------------------------------------------------
// filterValidThemeColors
// ---------------------------------------------------------------------------

func TestFilterValidThemeColors(t *testing.T) {
	t.Run("keeps valid hex and named color strings", func(t *testing.T) {
		out := filterValidThemeColors(map[string]any{
			"foreground": "#ffffff",
			"background": "#000",
			"cursor":     "white",
		})
		assert.Equal(t, map[string]any{
			"foreground": "#ffffff",
			"background": "#000",
			"cursor":     "white",
		}, out)
	})

	t.Run("drops invalid color strings", func(t *testing.T) {
		out := filterValidThemeColors(map[string]any{
			"foreground": "#ffffff",
			"background": "rgb(0,0,0)",
			"red":        "not#valid",
			"green":      "#gggggg",
		})
		assert.Equal(t, map[string]any{"foreground": "#ffffff"}, out)
	})

	t.Run("drops non-string values", func(t *testing.T) {
		out := filterValidThemeColors(map[string]any{
			"foreground": "#ffffff",
			"count":      float64(3),
			"flag":       true,
			"nothing":    nil,
		})
		assert.Equal(t, map[string]any{"foreground": "#ffffff"}, out)
	})

	t.Run("returns an empty, non-nil map for empty input", func(t *testing.T) {
		out := filterValidThemeColors(map[string]any{})
		assert.NotNil(t, out)
		assert.Empty(t, out)
	})

	t.Run("returns an empty, non-nil map for nil input", func(t *testing.T) {
		out := filterValidThemeColors(nil)
		assert.NotNil(t, out)
		assert.Empty(t, out)
	})

	t.Run("returns a new map rather than aliasing the input", func(t *testing.T) {
		in := map[string]any{"foreground": "#ffffff"}
		out := filterValidThemeColors(in)
		out["foreground"] = "#000000"
		assert.Equal(t, "#ffffff", in["foreground"], "mutating the result must not affect the input map")
	})
}

// ---------------------------------------------------------------------------
// saveDefaultThemeConfig
// ---------------------------------------------------------------------------

func TestSaveDefaultThemeConfig(t *testing.T) {
	readConfig := func(t *testing.T, home string) map[string]any {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(home, DOT_CONFIG_PATH, B3TTY_CONFIG_PATH, CONFIG_FILE_NAME))
		require.NoError(t, err)
		var out map[string]any
		require.NoError(t, yaml.Unmarshal(data, &out))
		return out
	}

	t.Run("creates the config directory and file under $HOME when neither exists", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		require.NoError(t, saveDefaultThemeConfig("", "dracula", map[string]any{"foreground": "#f8f8f2"}))

		info, err := os.Stat(filepath.Join(home, DOT_CONFIG_PATH, B3TTY_CONFIG_PATH))
		require.NoError(t, err)
		assert.True(t, info.IsDir())

		out := readConfig(t, home)
		assert.Equal(t, "dracula", out["theme"])
	})

	t.Run("writes the theme's color entries under the themes section", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		require.NoError(t, saveDefaultThemeConfig("", "dracula", map[string]any{
			"foreground": "#f8f8f2",
			"background": "#282a36",
		}))

		out := readConfig(t, home)
		themes := out["themes"].(map[string]any)
		palette := themes["dracula"].(map[string]any)
		assert.Equal(t, "#f8f8f2", palette["foreground"])
		assert.Equal(t, "#282a36", palette["background"])
	})

	t.Run("silently drops invalid color strings", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		require.NoError(t, saveDefaultThemeConfig("", "dracula", map[string]any{
			"foreground": "#f8f8f2",
			"background": "rgb(40,42,54)",
		}))

		palette := readConfig(t, home)["themes"].(map[string]any)["dracula"].(map[string]any)
		assert.Equal(t, "#f8f8f2", palette["foreground"])
		assert.NotContains(t, palette, "background")
	})

	t.Run("output passes ValidateConfig", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		require.NoError(t, saveDefaultThemeConfig("", "dracula", map[string]any{
			"foreground": "#f8f8f2",
			"background": "#282a36",
			"red":        "#ff5555",
		}))

		assert.NoError(t, ValidateConfig(filepath.Join(home, DOT_CONFIG_PATH, B3TTY_CONFIG_PATH, CONFIG_FILE_NAME)))
	})

	t.Run("overwrites an existing config file at the same path", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		require.NoError(t, saveDefaultThemeConfig("", "b3tty-dark", map[string]any{"foreground": "#dbdbdb"}))

		require.NoError(t, saveDefaultThemeConfig("", "b3tty-light", map[string]any{"foreground": "#111111"}))

		out := readConfig(t, home)
		assert.Equal(t, "b3tty-light", out["theme"])
	})

	t.Run("returns an error when the home directory cannot be determined", func(t *testing.T) {
		t.Setenv("HOME", "")
		err := saveDefaultThemeConfig("", "dracula", map[string]any{"foreground": "#f8f8f2"})
		assert.Error(t, err)
	})

	t.Run("returns an error when the config directory cannot be created", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		// blocker occupies the path where the .config directory needs to be
		// created, so MkdirAll must fail.
		require.NoError(t, os.WriteFile(filepath.Join(home, DOT_CONFIG_PATH), []byte("not a directory"), 0644))

		err := saveDefaultThemeConfig("", "dracula", map[string]any{"foreground": "#f8f8f2"})
		assert.Error(t, err)
	})

	t.Run("returns an error when the config file cannot be written", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root bypasses permission checks")
		}
		home := t.TempDir()
		t.Setenv("HOME", home)
		configDir := filepath.Join(home, DOT_CONFIG_PATH, B3TTY_CONFIG_PATH)
		require.NoError(t, os.MkdirAll(configDir, 0755))
		require.NoError(t, os.Chmod(configDir, 0555)) // read+execute, no write
		t.Cleanup(func() { _ = os.Chmod(configDir, 0755) })

		err := saveDefaultThemeConfig("", "dracula", map[string]any{"foreground": "#f8f8f2"})
		assert.Error(t, err)
	})
}
