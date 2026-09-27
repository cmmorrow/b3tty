package src

import (
	"bytes"
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

// The following types mirror the YAML config file structure. They are the
// single decode target for the config file: LoadConfig fills them at startup
// and the same decode performs structural and type validation. They are
// intentionally separate from the runtime structs in src/models.go — with the
// exception of the terminal section, which reuses TerminalClient
// (src/models.go) directly since its field set already matches the config
// file's terminal section exactly, the server section, whose ServerConfig type
// composes TLS and SettingsServerConfig (src/models.go) via yaml:",inline",
// since their combined field set already matches the config file's server
// section exactly, the profiles section, which reuses Profile (src/models.go)
// directly as the map value type (map[string]Profile) since its field set
// already matches the config file's profile entry shape exactly — no
// yaml:",inline" is needed here, unlike the server section, because Profile is
// a plain named map value type rather than an anonymously embedded field, and
// the themes section, which reuses Theme (src/models.go) directly as the map
// value type (map[string]Theme) for the same reason.

// Config is the decoded contents of conf.yaml.
type Config struct {
	Server   ServerConfig       `yaml:"server"`
	Terminal TerminalClient     `yaml:"terminal"`
	Theme    string             `yaml:"theme"`
	Themes   map[string]Theme   `yaml:"themes"`
	Profiles map[string]Profile `yaml:"profiles"`
}

// ServerConfig is the config file's server section.
type ServerConfig struct {
	TLS                  `yaml:",inline"`
	SettingsServerConfig `yaml:",inline"`
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
// Keys in colors use the hyphenated form of Theme's yaml tags (e.g. "bright-red"),
// as produced by Theme.toColorMap.
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
// absent), sets the active theme name, and writes theme to the themes section,
// overwriting any existing entry for that theme name. The entry holds theme's
// valid colors plus background-image and background-image-transparency when
// set; an empty or nil value leaves that key out, clearing it.
func SaveThemeToConfig(configPath string, themeName string, theme Theme) error {
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

	entry := filterValidThemeColors(theme.toColorMap())
	if theme.BackgroundImage != "" {
		entry["background-image"] = theme.BackgroundImage
	}
	if theme.BackgroundImageTransparency != nil {
		entry["background-image-transparency"] = *theme.BackgroundImageTransparency
	}
	themesSection[themeName] = entry

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

	// Stored as the Profile struct itself rather than a hand-built map:
	// gopkg.in/yaml.v3 marshals a struct nested inside a map[string]any using
	// its own yaml tags, so the hyphenated key names live in exactly one place
	// (Profile in models.go) and a field added there cannot be silently
	// dropped here.
	profilesSection[name] = p

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
func SaveSettingsToConfig(configPath string, server SettingsServerConfig, terminal TerminalClient) error {
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

// ConfigKeys is the set of dotted key paths present in the config file, e.g.
// "terminal.rows". LoadConfig builds it so callers can distinguish "the user
// wrote this key" from "this field decoded to its zero value" — the one thing
// a typed decode cannot express on its own.
type ConfigKeys map[string]bool

// Has reports whether the config file contained the given dotted key path.
// It is nil-safe, so the zero ConfigKeys returned by a failed load simply
// reports every key as absent.
func (k ConfigKeys) Has(key string) bool { return k[key] }

// LoadConfig reads the YAML config file at path and decodes it into cfg with
// KnownFields(true) enabled, returning a descriptive error (including the line
// number from the YAML parser) if any field has the wrong type or any
// unrecognised key is present. Loading and validating are the same pass.
//
// cfg is decoded into in place rather than returned fresh so that callers can
// pre-seed it with the values to fall back on: yaml.v3 leaves fields whose
// keys are absent from the document untouched, so after Decode every field
// either holds what the file said or the caller's seed value. That gives
// "config file overrides default, absent key keeps default" for free, with no
// pointer fields and no second is-this-key-set lookup per field.
func LoadConfig(path string, cfg *Config) (ConfigKeys, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		// An empty file produces io.EOF from the decoder, which is not an error.
		if errors.Is(err, io.EOF) {
			return ConfigKeys{}, nil
		}
		return nil, fmt.Errorf("config file %s: %w", path, err)
	}

	// A second pass over the same bytes, purely to record which keys the file
	// actually contains. Only the handful of settings whose absence is
	// meaningful need this (see `b3tty settings get`); everything else is
	// served by the pre-seeded decode above.
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("config file %s: %w", path, err)
	}
	keys := ConfigKeys{}
	collectConfigKeys(raw, "", keys)
	return keys, nil
}

// collectConfigKeys walks a decoded YAML mapping and records every dotted key
// path it contains into keys, recursing into nested mappings.
func collectConfigKeys(m map[string]any, prefix string, keys ConfigKeys) {
	for k, v := range m {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		keys[path] = true
		if nested, ok := v.(map[string]any); ok {
			collectConfigKeys(nested, path, keys)
		}
	}
}
