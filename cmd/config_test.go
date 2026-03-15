package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		assert.NoError(t, validateConfig(path))
	})

	t.Run("empty config passes", func(t *testing.T) {
		path := writeTempConfig(t, "")
		assert.NoError(t, validateConfig(path))
	})

	t.Run("partial config with only terminal section passes", func(t *testing.T) {
		path := writeTempConfig(t, `
terminal:
  font-size: 16
  rows: 30
`)
		assert.NoError(t, validateConfig(path))
	})

	t.Run("partial config with only server section passes", func(t *testing.T) {
		path := writeTempConfig(t, `
server:
  no-auth: true
  port: 9000
`)
		assert.NoError(t, validateConfig(path))
	})

	t.Run("unknown top-level key is rejected", func(t *testing.T) {
		path := writeTempConfig(t, `
unknown-key: true
`)
		err := validateConfig(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown-key")
	})

	t.Run("misspelled server key is rejected", func(t *testing.T) {
		path := writeTempConfig(t, `
server:
  tls-enabled: true
`)
		err := validateConfig(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tls-enabled")
	})

	t.Run("misspelled terminal key is rejected", func(t *testing.T) {
		path := writeTempConfig(t, `
terminal:
  fontsize: 14
`)
		err := validateConfig(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "fontsize")
	})

	t.Run("misspelled theme color key is rejected", func(t *testing.T) {
		path := writeTempConfig(t, `
themes:
  my-theme:
    colour: "#ffffff"
`)
		err := validateConfig(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "colour")
	})

	t.Run("misspelled profile key is rejected", func(t *testing.T) {
		path := writeTempConfig(t, `
profiles:
  work:
    workingdirectory: "~/projects"
`)
		err := validateConfig(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "workingdirectory")
	})

	t.Run("wrong type for font-size is rejected", func(t *testing.T) {
		path := writeTempConfig(t, `
terminal:
  font-size: "big"
`)
		err := validateConfig(path)
		assert.Error(t, err)
	})

	t.Run("wrong type for tls bool is rejected", func(t *testing.T) {
		// yaml.v3 coerces strings/numbers to bool, but a mapping never coerces.
		path := writeTempConfig(t, `
server:
  tls:
    enabled: true
`)
		err := validateConfig(path)
		assert.Error(t, err)
	})

	t.Run("wrong type for rows is rejected", func(t *testing.T) {
		// yaml.v3 truncates floats to int, but a mapping never coerces to int.
		path := writeTempConfig(t, `
terminal:
  rows:
    count: 24
`)
		err := validateConfig(path)
		assert.Error(t, err)
	})

	t.Run("wrong type for commands is rejected", func(t *testing.T) {
		path := writeTempConfig(t, `
profiles:
  work:
    commands: "not-a-list"
`)
		err := validateConfig(path)
		assert.Error(t, err)
	})

	t.Run("error message includes the config file path", func(t *testing.T) {
		path := writeTempConfig(t, `
unknown-key: true
`)
		err := validateConfig(path)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), path)
	})

	t.Run("file not found returns error", func(t *testing.T) {
		err := validateConfig("/nonexistent/path/b3tty.yaml")
		assert.Error(t, err)
	})
}
