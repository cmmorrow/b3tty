package src

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// configFileMu serializes every read-modify-write access to conf.yaml made
// by this process. UpdateThemeInConfig, SaveThemeToConfig, SaveProfileToConfig,
// DeleteProfileFromConfig, SaveSettingsToConfig, and saveDefaultThemeConfig each
// read the file, mutate a section (or, for saveDefaultThemeConfig, build it from
// scratch), and write it back; without this lock two such calls racing within
// the same process — e.g. two concurrent HTTP handlers on a running b3tty
// server — can interleave and one write silently clobbers the other's
// changes on disk.
//
// This only protects against races within a single process. It does not
// protect against a `b3tty <subcommand>` CLI invocation — a separate process —
// writing to the same file at the same moment as a running server; that would
// require OS-level file locking and is a larger, separate change.
var configFileMu sync.Mutex

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
	TLS         bool   `yaml:"tls"`
	CertFile    string `yaml:"cert-file"`
	KeyFile     string `yaml:"key-file"`
	NoAuth      bool   `yaml:"no-auth"`
	NoBrowser   bool   `yaml:"no-browser"`
	Port        int    `yaml:"port"`
	ShowMenubar string `yaml:"show-menubar"`
}

type terminalConfig struct {
	FontFamily string `yaml:"font-family"`
	FontSize   int    `yaml:"font-size"`
	AutoResize bool   `yaml:"auto-resize"`
	Rows       int    `yaml:"rows"`
	Columns    int    `yaml:"columns"`
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

// resolveConfigPath returns configPath unchanged when non-empty, or the
// default ~/.config/b3tty/conf.yaml location otherwise.
func resolveConfigPath(configPath string) (string, error) {
	if configPath != "" {
		return configPath, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, DOT_CONFIG_PATH, B3TTY_CONFIG_PATH, CONFIG_FILE_NAME), nil
}

// loadConfigMap reads and parses the YAML config file at path into a generic
// map. Any error reading the file (including the file not existing) is
// treated as "start from an empty config" and silently ignored; only
// a parse error on data that was actually read is returned, prefixed with
// errContext.
func loadConfigMap(path string, errContext string) (map[string]any, error) {
	cfg := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("%s: parse existing config: %w", errContext, err)
		}
	}
	return cfg, nil
}

// writeConfigMap marshals cfg to YAML and writes it to path, creating the
// parent directory if it does not exist. A marshal error is prefixed with
// errContext.
func writeConfigMap(path string, cfg map[string]any, errContext string) error {
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("%s: %w", errContext, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// getOrCreateSection returns cfg[name] as a map[string]any. If the key is
// absent, or holds a value that isn't a map[string]any (e.g. a malformed
// config file), a new empty map is stored at cfg[name] and returned instead.
func getOrCreateSection(cfg map[string]any, name string) map[string]any {
	section, ok := cfg[name].(map[string]any)
	if !ok {
		section = map[string]any{}
		cfg[name] = section
	}
	return section
}

// filterValidThemeColors returns a copy of colors containing only the entries
// whose value is a string passing ValidateThemeColor. Used by every function
// that writes a theme's color entries into the themes section, so that
// invalid or non-string values (e.g. from a malformed request body) are
// silently dropped rather than written to disk.
func filterValidThemeColors(colors map[string]any) map[string]any {
	themeColors := make(map[string]any, len(colors))
	for k, v := range colors {
		if s, ok := v.(string); ok && ValidateThemeColor(s) {
			themeColors[k] = s
		}
	}
	return themeColors
}

// saveDefaultThemeConfig writes a fresh conf.yaml at configPath (resolved via
// resolveConfigPath) containing only the given active theme name and its color
// entries under the themes section. Unlike the other config-writing functions
// in this file, it does not read or preserve any existing file content: it is
// used solely for the first-run setup flow, where no config file exists yet.
// Keys in colors use the hyphenated form expected by MapToTheme (e.g. "bright-red").
func saveDefaultThemeConfig(configPath string, themeName string, colors map[string]any) error {
	configFileMu.Lock()
	defer configFileMu.Unlock()

	configPath, err := resolveConfigPath(configPath)
	if err != nil {
		return err
	}

	cfg := map[string]any{
		"theme":  themeName,
		"themes": map[string]any{themeName: filterValidThemeColors(colors)},
	}

	return writeConfigMap(configPath, cfg, "saveDefaultThemeConfig")
}

// UpdateThemeInConfig reads the existing config file at configPath (creating it if
// absent), sets the active theme name, and adds the theme's color entries to the
// themes section if they are not already present. Existing settings are preserved.
func UpdateThemeInConfig(configPath string, themeName string, colors map[string]any) error {
	configFileMu.Lock()
	defer configFileMu.Unlock()

	configPath, err := resolveConfigPath(configPath)
	if err != nil {
		return err
	}
	cfg, err := loadConfigMap(configPath, "UpdateThemeInConfig")
	if err != nil {
		return err
	}

	cfg["theme"] = themeName
	themesSection := getOrCreateSection(cfg, "themes")

	if _, exists := themesSection[themeName]; !exists && len(colors) > 0 {
		themesSection[themeName] = filterValidThemeColors(colors)
	}

	return writeConfigMap(configPath, cfg, "UpdateThemeInConfig")
}

// SaveThemeToConfig reads the existing config file at configPath (creating it if
// absent), sets the active theme name, and writes the theme's color entries to the
// themes section, overwriting any existing entry for that theme name.
func SaveThemeToConfig(configPath string, themeName string, colors map[string]any) error {
	configFileMu.Lock()
	defer configFileMu.Unlock()

	configPath, err := resolveConfigPath(configPath)
	if err != nil {
		return err
	}
	cfg, err := loadConfigMap(configPath, "SaveThemeToConfig")
	if err != nil {
		return err
	}

	cfg["theme"] = themeName
	themesSection := getOrCreateSection(cfg, "themes")

	themeColors := filterValidThemeColors(colors)
	// Preserve background-image from the existing entry: toColorMap() omits it
	// because it is a file path, not a color, so it would be silently dropped
	// by the ValidateThemeColor filter above.
	if existing, ok := themesSection[themeName].(map[string]any); ok {
		if bgImg, ok := existing["background-image"].(string); ok && bgImg != "" {
			themeColors["background-image"] = bgImg
		}
	}
	themesSection[themeName] = themeColors

	return writeConfigMap(configPath, cfg, "SaveThemeToConfig")
}

// ReadThemeNames reads the config file at path and returns the names from the
// themes section, preserving their exact case as written in the YAML.
func ReadThemeNames(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Themes map[string]any `yaml:"themes"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(raw.Themes))
	for name := range raw.Themes {
		names = append(names, name)
	}
	return names, nil
}

// SaveProfileToConfig reads the existing config file at configPath (creating it if
// absent), upserts the named profile in the profiles section, and writes the file back.
func SaveProfileToConfig(configPath string, name string, p Profile) error {
	configFileMu.Lock()
	defer configFileMu.Unlock()

	configPath, err := resolveConfigPath(configPath)
	if err != nil {
		return err
	}
	cfg, err := loadConfigMap(configPath, "SaveProfileToConfig")
	if err != nil {
		return err
	}

	profilesSection := getOrCreateSection(cfg, "profiles")

	entry := map[string]any{
		"shell":             p.Shell,
		"title":             p.Title,
		"working-directory": p.WorkingDirectory,
		"root":              p.Root,
		"commands":          p.Commands,
	}
	profilesSection[name] = entry

	return writeConfigMap(configPath, cfg, "SaveProfileToConfig")
}

// DeleteProfileFromConfig reads the existing config file at configPath, removes the
// named entry from the profiles section, and writes the file back. No-ops if the
// profiles section or the named entry is absent.
func DeleteProfileFromConfig(configPath string, name string) error {
	configFileMu.Lock()
	defer configFileMu.Unlock()

	configPath, err := resolveConfigPath(configPath)
	if err != nil {
		return err
	}
	cfg, err := loadConfigMap(configPath, "DeleteProfileFromConfig")
	if err != nil {
		return err
	}

	if profilesSection, ok := cfg["profiles"].(map[string]any); ok {
		delete(profilesSection, name)
	}

	return writeConfigMap(configPath, cfg, "DeleteProfileFromConfig")
}

// SaveSettingsToConfig reads the existing config file at configPath (creating it if
// absent), updates the server and terminal sections with the provided values, and
// writes the file back. Existing settings not covered by the structs are preserved.
func SaveSettingsToConfig(configPath string, server SettingsServerConfig, terminal SettingsTerminalConfig) error {
	configFileMu.Lock()
	defer configFileMu.Unlock()

	configPath, err := resolveConfigPath(configPath)
	if err != nil {
		return err
	}
	cfg, err := loadConfigMap(configPath, "SaveSettingsToConfig")
	if err != nil {
		return err
	}

	serverSection := getOrCreateSection(cfg, "server")
	serverSection["port"] = server.Port
	serverSection["no-auth"] = server.NoAuth
	serverSection["no-browser"] = server.NoBrowser
	serverSection["show-menubar"] = server.ShowMenubar

	termSection := getOrCreateSection(cfg, "terminal")
	if terminal.FontFamily != "" {
		termSection["font-family"] = terminal.FontFamily
	} else {
		delete(termSection, "font-family")
	}
	termSection["font-size"] = terminal.FontSize
	termSection["auto-resize"] = terminal.AutoResize
	termSection["rows"] = terminal.Rows
	termSection["columns"] = terminal.Columns

	return writeConfigMap(configPath, cfg, "SaveSettingsToConfig")
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
