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
			expected: &exec.Cmd{Path: "/bin/test", Dir: homeDir, Args: []string{"test"}},
		},
		{
			name: "$HOME WorkingDirectory",
			profile: Profile{
				WorkingDirectory: "$HOME",
				Shell:            "",
			},
			cmd:      exec.Command("test"),
			expected: &exec.Cmd{Path: "/bin/test", Dir: homeDir, Args: []string{"test"}},
		},
		{
			name: "Custom WorkingDirectory",
			profile: Profile{
				WorkingDirectory: "/custom/dir",
				Shell:            "",
			},
			cmd:      exec.Command("test"),
			expected: &exec.Cmd{Path: "/bin/test", Dir: "/custom/dir", Args: []string{"test"}},
		},
		{
			name: "WorkingDirectory with ~",
			profile: Profile{
				WorkingDirectory: "~/custom",
				Shell:            "",
			},
			cmd:      exec.Command("test"),
			expected: &exec.Cmd{Path: "/bin/test", Dir: filepath.Join(homeDir, "custom"), Args: []string{"test"}},
		},
		{
			name: "Custom Shell",
			profile: Profile{
				WorkingDirectory: "",
				Shell:            "/bin/customsh",
			},
			cmd:      exec.Command("test", "-c", "echo"),
			expected: &exec.Cmd{Path: "/bin/test", Args: []string{"test", "-c", "/bin/customsh"}, Dir: homeDir},
		},
		{
			name: "Shell with $SHELL",
			profile: Profile{
				WorkingDirectory: "",
				Shell:            "$SHELL",
			},
			cmd:      exec.Command("test", "-c", "echo"),
			expected: &exec.Cmd{Path: "/bin/test", Args: []string{"test", "-c", "echo"}, Dir: homeDir},
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
}
