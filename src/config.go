package src

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const CONFIG_FILE_NAME = "conf.yaml"
const DOT_CONFIG_PATH = ".config"
const B3TTY_CONFIG_PATH = "b3tty"

// The following types mirror the YAML config file structure. They exist solely
// for structural and type validation at startup and are intentionally separate
// from the runtime structs in src/models.go.

type configFile struct {
	Server   serverConfig             `yaml:"server"`
	Terminal terminalConfig           `yaml:"terminal"`
	Theme    string                   `yaml:"theme"`
	Themes   map[string]themeConfig   `yaml:"themes"`
	Profiles map[string]profileConfig `yaml:"profiles"`
}

type serverConfig struct {
	TLS       bool   `yaml:"tls"`
	CertFile  string `yaml:"cert-file"`
	KeyFile   string `yaml:"key-file"`
	NoAuth    bool   `yaml:"no-auth"`
	NoBrowser bool   `yaml:"no-browser"`
	Port      int    `yaml:"port"`
}

type terminalConfig struct {
	FontFamily  string `yaml:"font-family"`
	FontSize    int    `yaml:"font-size"`
	CursorBlink bool   `yaml:"cursor-blink"`
	Rows        int    `yaml:"rows"`
	Columns     int    `yaml:"columns"`
}

type themeConfig struct {
	Black               string `yaml:"black"`
	BrightBlack         string `yaml:"bright-black"`
	Red                 string `yaml:"red"`
	BrightRed           string `yaml:"bright-red"`
	Green               string `yaml:"green"`
	BrightGreen         string `yaml:"bright-green"`
	Yellow              string `yaml:"yellow"`
	BrightYellow        string `yaml:"bright-yellow"`
	Blue                string `yaml:"blue"`
	BrightBlue          string `yaml:"bright-blue"`
	Magenta             string `yaml:"magenta"`
	BrightMagenta       string `yaml:"bright-magenta"`
	Cyan                string `yaml:"cyan"`
	BrightCyan          string `yaml:"bright-cyan"`
	White               string `yaml:"white"`
	BrightWhite         string `yaml:"bright-white"`
	Foreground          string `yaml:"foreground"`
	Background          string `yaml:"background"`
	Cursor              string `yaml:"cursor"`
	CursorAccent        string `yaml:"cursor-accent"`
	SelectionForeground string `yaml:"selection-foreground"`
	SelectionBackground string `yaml:"selection-background"`
	BackgroundImage     string `yaml:"background-image"`
}

type profileConfig struct {
	WorkingDirectory string   `yaml:"working-directory"`
	Title            string   `yaml:"title"`
	Shell            string   `yaml:"shell"`
	Commands         []string `yaml:"commands"`
	Root             string   `yaml:"root"`
}

// buildConfigYAML produces a conf.yaml string for the given theme name and color map.
// Keys in colors use the hyphenated form expected by MapToTheme (e.g. "bright-red").
func buildConfigYAML(themeName string, colors map[string]any) string {
	themeColors := make(map[string]string, len(colors))
	for k, v := range colors {
		if s, ok := v.(string); ok && ValidateThemeColor(s) {
			themeColors[k] = s
		}
	}
	cfg := struct {
		Theme  string                       `yaml:"theme"`
		Themes map[string]map[string]string `yaml:"themes"`
	}{
		Theme:  themeName,
		Themes: map[string]map[string]string{themeName: themeColors},
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		panic("buildConfigYAML: " + err.Error())
	}
	return string(out)
}

// WriteDefaultConfig writes a default theme config file to $HOME/.config/b3tty/conf.yaml.
func WriteDefaultConfig(themeName string, colors map[string]any) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configDir := filepath.Join(home, DOT_CONFIG_PATH, B3TTY_CONFIG_PATH)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}
	configPath := filepath.Join(configDir, CONFIG_FILE_NAME)
	return os.WriteFile(configPath, []byte(buildConfigYAML(themeName, colors)), 0644)
}

// ValidateConfig opens the YAML file at path, decodes it into typed structs
// with KnownFields(true) enabled, and returns a descriptive error (including
// the line number from the YAML parser) if any field has the wrong type or any
// unrecognised key is present.
func ValidateConfig(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open config file: %w", err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var cfg configFile
	if err := dec.Decode(&cfg); err != nil {
		// An empty file produces io.EOF from the decoder, which is not an error.
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("config file %s: %w", path, err)
	}
	return nil
}
