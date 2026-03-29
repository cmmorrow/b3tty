package src

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServerAddr(t *testing.T) {
	testCases := []struct {
		name     string
		server   Server
		expected string
	}{
		{
			name:     "Standard case",
			server:   Server{Uri: "example.com", Port: 8080},
			expected: "example.com:8080",
		},
		{
			name:     "Localhost",
			server:   Server{Uri: "localhost", Port: 3000},
			expected: "localhost:3000",
		},
		{
			name:     "IP address",
			server:   Server{Uri: "192.168.1.1", Port: 443},
			expected: "192.168.1.1:443",
		},
		{
			name:     "No port",
			server:   Server{Uri: "localhost"},
			expected: "localhost:0", // TODO: Handle this case
		},
		{
			name:     "No Uri",
			server:   Server{Port: 8080},
			expected: ":8080",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.server.Addr()
			assert.Equal(t, tc.expected, result.Host)
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

func TestMapToTheme(t *testing.T) {
	assert := assert.New(t)

	// Test case 1: Empty map
	theme := &Theme{}
	emptyMap := map[string]any{}
	theme.MapToTheme(emptyMap)
	assert.Equal(Theme{}, *theme)

	// Test case 2: Map with valid keys
	theme = &Theme{}
	validMap := map[string]any{
		"foreground":           "white",
		"background":           "black",
		"selection-foreground": "yellow",
		"selection-background": "blue",
	}
	theme.MapToTheme(validMap)
	assert.Equal("white", theme.Foreground)
	assert.Equal("black", theme.Background)
	assert.Equal("yellow", theme.SelectionForeground)
	assert.Equal("blue", theme.SelectionBackground)

	// Test case 3: Map with invalid keys
	theme = &Theme{Foreground: "red"}
	invalidMap := map[string]any{
		"invalid_key":     "value",
		"another_invalid": 123,
	}
	theme.MapToTheme(invalidMap)
	assert.Equal("red", theme.Foreground)
	assert.Empty(theme.Background)

	// Test case 4: Map with mixed valid and invalid keys
	theme = &Theme{}
	mixedMap := map[string]any{
		"foreground":      "white",
		"invalid_key":     "value",
		"background":      "black",
		"another_invalid": 123,
	}
	theme.MapToTheme(mixedMap)
	assert.Equal("white", theme.Foreground)
	assert.Equal("black", theme.Background)
	assert.Empty(theme.SelectionForeground)
	assert.Empty(theme.SelectionBackground)

	// Test case 5: Valid field name with a non-string value does not panic
	theme = &Theme{Foreground: "red"}
	nonStringMap := map[string]any{
		"foreground": 42,
		"background": true,
	}
	assert.NotPanics(func() { theme.MapToTheme(nonStringMap) })
	assert.Equal("red", theme.Foreground)
	assert.Empty(theme.Background)
}
