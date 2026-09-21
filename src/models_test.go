package src

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestNewServer(t *testing.T) {
	testCases := []struct {
		name           string
		uri            string
		port           int
		tls            bool
		expectedHost   string
		expectedScheme string
	}{
		{
			name:           "Standard case",
			uri:            "example.com",
			port:           8080,
			expectedHost:   "example.com:8080",
			expectedScheme: "http",
		},
		{
			name:           "Localhost",
			uri:            "localhost",
			port:           3000,
			expectedHost:   "localhost:3000",
			expectedScheme: "http",
		},
		{
			name:           "IP address",
			uri:            "192.168.1.1",
			port:           443,
			expectedHost:   "192.168.1.1:443",
			expectedScheme: "http",
		},
		{
			name:           "TLS enabled uses https scheme",
			uri:            "example.com",
			port:           8443,
			tls:            true,
			expectedHost:   "example.com:8443",
			expectedScheme: "https",
		},
		{
			name:           "No port",
			uri:            "localhost",
			port:           0,
			expectedHost:   "localhost:0", // TODO: Handle this case
			expectedScheme: "http",
		},
		{
			name:           "No Uri",
			uri:            "",
			port:           8080,
			expectedHost:   ":8080",
			expectedScheme: "http",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			noAuth := false
			server := NewServer(&tc.uri, &tc.port, &noAuth, &TLS{Enabled: tc.tls})
			assert.Equal(t, tc.expectedHost, server.URL.Host)
			assert.Equal(t, tc.expectedScheme, server.URL.Scheme)
			assert.Equal(t, tc.port, server.Port())
		})
	}
}

func TestParseCommands(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		name     string
		commands []string
		expected [][]string
		hasError bool
	}{
		{
			name:     "Single command",
			commands: []string{"echo hello"},
			expected: [][]string{{"echo", "hello"}},
			hasError: false,
		},
		{
			name:     "Multiple commands",
			commands: []string{"echo hello", "ls -l"},
			expected: [][]string{{"echo", "hello"}, {"ls", "-l"}},
			hasError: false,
		},
		{
			name:     "Command with quotes",
			commands: []string{"echo \"hello world\""},
			expected: [][]string{{"echo", "hello world"}},
			hasError: false,
		},
		{
			name:     "Command with unclosed quotes",
			commands: []string{"echo \"hello world"},
			expected: nil,
			hasError: true,
		},
		{
			name:     "Empty command",
			commands: []string{""},
			expected: [][]string{{}},
			hasError: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Profile{Commands: tt.commands}
			result, err := p.ParseCommands()

			if tt.hasError {
				assert.Error(err)
			} else {
				assert.NoError(err)
				assert.Equal(tt.expected, result)
			}
		})
	}
}

func TestApplyToCommand(t *testing.T) {
	// Setup
	homeDir, err := os.UserHomeDir()
	assert.NoError(t, err)

	testPath, err := exec.LookPath("test")
	assert.NoError(t, err)

	testCases := []struct {
		name     string
		profile  Profile
		cmd      *exec.Cmd
		expected *exec.Cmd
	}{
		{
			name: "Empty WorkingDirectory",
			profile: Profile{
				WorkingDirectory: "",
				Shell:            "",
			},
			cmd:      exec.Command("test"),
			expected: &exec.Cmd{Path: testPath, Dir: homeDir, Args: []string{"test"}},
		},
		{
			name: "$HOME WorkingDirectory",
			profile: Profile{
				WorkingDirectory: "$HOME",
				Shell:            "",
			},
			cmd:      exec.Command("test"),
			expected: &exec.Cmd{Path: testPath, Dir: homeDir, Args: []string{"test"}},
		},
		{
			name: "Custom WorkingDirectory",
			profile: Profile{
				WorkingDirectory: "/custom/dir",
				Shell:            "",
			},
			cmd:      exec.Command("test"),
			expected: &exec.Cmd{Path: testPath, Dir: "/custom/dir", Args: []string{"test"}},
		},
		{
			name: "WorkingDirectory with ~",
			profile: Profile{
				WorkingDirectory: "~/custom",
				Shell:            "",
			},
			cmd:      exec.Command("test"),
			expected: &exec.Cmd{Path: testPath, Dir: filepath.Join(homeDir, "custom"), Args: []string{"test"}},
		},
		{
			name: "Custom Shell",
			profile: Profile{
				WorkingDirectory: "",
				Shell:            "/bin/customsh",
			},
			cmd:      exec.Command("test", "-c", "echo"),
			expected: &exec.Cmd{Path: testPath, Args: []string{"test", "-c", "/bin/customsh"}, Dir: homeDir},
		},
		{
			name: "Shell with $SHELL",
			profile: Profile{
				WorkingDirectory: "",
				Shell:            "$SHELL",
			},
			cmd:      exec.Command("test", "-c", "echo"),
			expected: &exec.Cmd{Path: testPath, Args: []string{"test", "-c", "echo"}, Dir: homeDir},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.profile.ApplyToCommand(tc.cmd)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected.Dir, result.Dir)
			assert.Equal(t, tc.expected.Path, result.Path)
			assert.Equal(t, tc.expected.Args, result.Args)
		})
	}
}

func TestNewCSPHeader(t *testing.T) {
	testCases := []struct {
		name           string
		directiveName  string
		values         []string
		expectedName   string
		expectedValues []string
	}{
		{
			name:           "Single value",
			directiveName:  "script-src",
			values:         []string{"self"},
			expectedName:   "script-src",
			expectedValues: []string{"self"},
		},
		{
			name:           "Multiple values",
			directiveName:  "script-src",
			values:         []string{"self", "wasm-unsafe-eval"},
			expectedName:   "script-src",
			expectedValues: []string{"self", "wasm-unsafe-eval"},
		},
		{
			name:           "No values",
			directiveName:  "default-src",
			values:         []string{},
			expectedName:   "default-src",
			expectedValues: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewCSPHeader(tc.directiveName, tc.values...)
			assert.Equal(t, tc.expectedName, h.Name)
			assert.Equal(t, tc.expectedValues, h.Values)
		})
	}
}

func TestCSPHeaderAdd(t *testing.T) {
	t.Run("Add to existing values", func(t *testing.T) {
		h := NewCSPHeader("script-src", "self")
		result := h.Add("wasm-unsafe-eval")
		assert.Equal(t, []string{"self", "wasm-unsafe-eval"}, h.Values)
		assert.Same(t, h, result)
	})

	t.Run("Add to empty values", func(t *testing.T) {
		h := NewCSPHeader("script-src")
		h.Add("self")
		assert.Equal(t, []string{"self"}, h.Values)
	})

	t.Run("Add multiple times chains correctly", func(t *testing.T) {
		h := NewCSPHeader("script-src", "self")
		h.Add("wasm-unsafe-eval").Add("nonce-abc123")
		assert.Equal(t, []string{"self", "wasm-unsafe-eval", "nonce-abc123"}, h.Values)
	})
}

func TestCSPHeaderSet(t *testing.T) {
	t.Run("Replaces existing values", func(t *testing.T) {
		h := NewCSPHeader("script-src", "self", "wasm-unsafe-eval")
		result := h.Set("none")
		assert.Equal(t, []string{"none"}, h.Values)
		assert.Same(t, h, result)
	})

	t.Run("Set with multiple values", func(t *testing.T) {
		h := NewCSPHeader("script-src", "self")
		h.Set("none", "unsafe-inline")
		assert.Equal(t, []string{"none", "unsafe-inline"}, h.Values)
	})

	t.Run("Set with no values clears existing", func(t *testing.T) {
		h := NewCSPHeader("script-src", "self")
		h.Set()
		assert.Empty(t, h.Values)
	})
}

func TestCSPHeaderString(t *testing.T) {
	testCases := []struct {
		name     string
		header   *CSPHeader
		expected string
	}{
		{
			name:     "Single value",
			header:   NewCSPHeader("default-src", "none"),
			expected: "default-src 'none';",
		},
		{
			name:     "Multiple values",
			header:   NewCSPHeader("script-src", "self", "wasm-unsafe-eval"),
			expected: "script-src 'self' 'wasm-unsafe-eval';",
		},
		{
			name:     "No values",
			header:   NewCSPHeader("default-src"),
			expected: "default-src ;",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.header.String())
		})
	}
}

func TestNewCSPHeaders(t *testing.T) {
	t.Run("Creates with multiple headers", func(t *testing.T) {
		chs := NewCSPHeders(
			NewCSPHeader("default-src", "none"),
			NewCSPHeader("script-src", "self"),
		)
		assert.Len(t, chs.Headers, 2)
		assert.NotNil(t, chs.Headers["default-src"])
		assert.NotNil(t, chs.Headers["script-src"])
	})

	t.Run("Creates empty when no headers provided", func(t *testing.T) {
		chs := NewCSPHeders()
		assert.Empty(t, chs.Headers)
	})
}

func TestCSPHeadersGet(t *testing.T) {
	t.Run("Returns pointer to map element", func(t *testing.T) {
		chs := NewCSPHeders(NewCSPHeader("script-src", "self"))
		h := chs.Get("script-src")
		assert.NotNil(t, h)
		// Mutating the returned pointer must be reflected in the map.
		h.Add("wasm-unsafe-eval")
		assert.Equal(t, []string{"self", "wasm-unsafe-eval"}, chs.Headers["script-src"].Values)
	})

	t.Run("Returns nil for missing key", func(t *testing.T) {
		chs := NewCSPHeders()
		assert.Nil(t, chs.Get("script-src"))
	})
}

func TestCSPHeadersAdd(t *testing.T) {
	t.Run("Adds a new header", func(t *testing.T) {
		chs := NewCSPHeders()
		chs.Add("img-src", NewCSPHeader("img-src", "self"))
		assert.NotNil(t, chs.Headers["img-src"])
		assert.Equal(t, []string{"self"}, chs.Headers["img-src"].Values)
	})

	t.Run("Overwrites existing header", func(t *testing.T) {
		chs := NewCSPHeders(NewCSPHeader("img-src", "self"))
		chs.Add("img-src", NewCSPHeader("img-src", "none"))
		assert.Equal(t, []string{"none"}, chs.Headers["img-src"].Values)
	})
}

func TestCSPHeadersString(t *testing.T) {
	t.Run("Contains all directives", func(t *testing.T) {
		chs := NewCSPHeders(
			NewCSPHeader("default-src", "none"),
			NewCSPHeader("script-src", "self"),
			NewCSPHeader("img-src", "self"),
		)
		result := chs.String()
		assert.Contains(t, result, "default-src 'none';")
		assert.Contains(t, result, "script-src 'self';")
		assert.Contains(t, result, "img-src 'self';")
	})

	t.Run("Empty headers produces empty string", func(t *testing.T) {
		chs := NewCSPHeders()
		assert.Empty(t, chs.String())
	})
}

func TestGetCSPHeadersMutationViaGet(t *testing.T) {
	// Regression: Get must return a pointer to the live map entry so that
	// adding a nonce to script-src is reflected in the final CSP string.
	chs := GetCSPHeaders()
	chs.Get("script-src").Add("nonce-abc123")
	result := chs.String()
	assert.Contains(t, result, "'nonce-abc123'")
}

func TestThemeHasBackgroundImage(t *testing.T) {
	t.Run("returns false when BackgroundImage is empty", func(t *testing.T) {
		theme := &Theme{}
		assert.False(t, theme.HasBackgroundImage())
	})

	t.Run("returns true when BackgroundImage is set", func(t *testing.T) {
		theme := &Theme{BackgroundImage: "/path/to/image.png"}
		assert.True(t, theme.HasBackgroundImage())
	})

	t.Run("other fields being populated does not affect the result", func(t *testing.T) {
		theme := &Theme{Foreground: "#ffffff", Background: "#000000"}
		assert.False(t, theme.HasBackgroundImage())
	})
}

func TestThemeToColorMap(t *testing.T) {
	t.Run("empty theme returns empty map", func(t *testing.T) {
		m := Theme{}.toColorMap()
		assert.Empty(t, m)
	})

	t.Run("populated fields appear with hyphenated keys", func(t *testing.T) {
		theme := Theme{
			Foreground:          "#ffffff",
			Background:          "#000000",
			Cursor:              "#cccccc",
			CursorAccent:        "#aaaaaa",
			SelectionForeground: "#ff0000",
			SelectionBackground: "#0000ff",
		}
		m := theme.toColorMap()
		assert.Equal(t, "#ffffff", m["foreground"])
		assert.Equal(t, "#000000", m["background"])
		assert.Equal(t, "#cccccc", m["cursor"])
		assert.Equal(t, "#aaaaaa", m["cursor-accent"])
		assert.Equal(t, "#ff0000", m["selection-foreground"])
		assert.Equal(t, "#0000ff", m["selection-background"])
	})

	t.Run("all 22 color fields are present when fully populated", func(t *testing.T) {
		theme := Theme{
			Foreground: "#fff", Background: "#000",
			Cursor: "#ccc", CursorAccent: "#aaa",
			SelectionForeground: "#111", SelectionBackground: "#222",
			Black: "#000", BrightBlack: "#333",
			Red: "#f00", BrightRed: "#f55",
			Yellow: "#ff0", BrightYellow: "#ff5",
			Green: "#0f0", BrightGreen: "#5f5",
			Blue: "#00f", BrightBlue: "#55f",
			Magenta: "#f0f", BrightMagenta: "#f5f",
			Cyan: "#0ff", BrightCyan: "#5ff",
			White: "#eee", BrightWhite: "#fff",
		}
		assert.Len(t, theme.toColorMap(), 22)
	})

	t.Run("empty fields are omitted from the map", func(t *testing.T) {
		theme := Theme{Foreground: "#ffffff"}
		m := theme.toColorMap()
		assert.Contains(t, m, "foreground")
		assert.NotContains(t, m, "background")
		assert.NotContains(t, m, "cursor")
	})

	t.Run("BackgroundImage is excluded from the map", func(t *testing.T) {
		theme := Theme{Foreground: "#ffffff", BackgroundImage: "/path/to/image.png"}
		m := theme.toColorMap()
		assert.NotContains(t, m, "background-image")
		assert.NotContains(t, m, "BackgroundImage")
	})

	t.Run("round-trips through the YAML config form", func(t *testing.T) {
		// toColorMap's hyphenated keys are what the config writers splice into
		// themes:, and Theme's yaml tags are what LoadConfig reads back out —
		// so this is the exact round trip production performs.
		original := Theme{
			Foreground: "#f8f8f2",
			Background: "#282a36",
			Red:        "#ff5555",
			BrightRed:  "#ff6e6e",
			Cursor:     "#f8f8f2",
		}
		data, err := yaml.Marshal(original.toColorMap())
		require.NoError(t, err)
		var restored Theme
		require.NoError(t, yaml.Unmarshal(data, &restored))
		assert.Equal(t, original, restored)
	})
}
