package src

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"

	"github.com/google/shlex"
)

// TerminalClient holds the terminal-display settings shared across in-memory
// runtime state (TerminalServer.Client), the YAML config file's terminal
// section (see Config in config.go), and the /settings JSON API.
type TerminalClient struct {
	FontFamily string `yaml:"font-family" json:"fontFamily"`
	FontSize   int    `yaml:"font-size" json:"fontSize"`
	AutoResize bool   `yaml:"auto-resize" json:"autoResize"`
	Rows       int    `yaml:"rows" json:"rows"`
	Columns    int    `yaml:"columns" json:"columns"`
}

func NewTerminalClient(rows *int, columns *int, autoResize *bool, fontFamily *string, fontSize *int) *TerminalClient {
	return &TerminalClient{
		Rows:       *rows,
		Columns:    *columns,
		AutoResize: *autoResize,
		FontFamily: *fontFamily,
		FontSize:   *fontSize,
	}
}

type Server struct {
	URL    url.URL
	NoAuth bool
	TLS
}

func NewServer(uri *string, port *int, noAuth *bool, tls *TLS) *Server {
	scheme := "http"
	if tls.Enabled {
		scheme = "https"
	}
	return &Server{
		URL: url.URL{
			Scheme: scheme,
			Host:   net.JoinHostPort(*uri, strconv.Itoa(*port)),
		},
		NoAuth: *noAuth,
		TLS:    *tls,
	}
}

// Port returns the server's port as an int, parsed from URL.Host. url.URL has
// no dedicated Port field — net/url convention encodes host:port together in
// Host, with Port() as the accessor that splits it back out as a string.
func (s *Server) Port() int {
	port, _ := strconv.Atoi(s.URL.Port())
	return port
}

type TLS struct {
	Enabled      bool   `yaml:"tls"`
	CertFilePath string `yaml:"cert-file"`
	KeyFilePath  string `yaml:"key-file"`
}

// Profile holds a terminal profile's settings, shared across in-memory runtime
// state (TerminalServer.Profiles), the YAML config file's profiles: section
// (Config.Profiles in src/config.go), and the /profile-config JSON API.
type Profile struct {
	Root             string   `yaml:"root" json:"root"`
	WorkingDirectory string   `yaml:"working-directory" json:"workingDirectory"`
	Shell            string   `yaml:"shell" json:"shell"`
	Title            string   `yaml:"title" json:"title"`
	Commands         []string `yaml:"commands" json:"commands"`
}

// ParseCommands processes the Profile Commands and returns a slice of string slices.
// Each command in the Commands slice is trimmed of whitespace and split into arguments.
//
// Returns:
//   - [][]string: A slice of string slices, where each inner slice represents a parsed command with its arguments.
//   - error: An error if any occurs during the parsing process, nil otherwise.
func (p *Profile) ParseCommands() ([][]string, error) {
	commands := [][]string{}
	for _, cmd := range p.Commands {
		proto, err := shlex.Split(strings.TrimSpace(cmd))
		if err != nil {
			return commands, err
		}
		commands = append(commands, proto)
	}
	return commands, nil
}

// ApplyToCommand applies the Profile's settings to the given exec.Cmd.
// It sets the working directory based on the Profile's WorkingDirectory field,
// expanding $HOME and ~ to the user's home directory.
// If a custom shell is specified in the Profile, it replaces the last argument
// of the command with the custom shell.
// Returns the modified exec.Cmd and any error encountered.
func (p *Profile) ApplyToCommand(cmd *exec.Cmd) (*exec.Cmd, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	if p.WorkingDirectory == "" || p.WorkingDirectory == "$HOME" {
		cmd.Dir = home
	} else {
		cmd.Dir = p.WorkingDirectory
		if strings.HasPrefix(p.WorkingDirectory, "~/") {
			cmd.Dir = strings.Replace(p.WorkingDirectory, "~", home, 1)
		}
	}

	if p.Shell != "" && p.Shell != "$SHELL" && strings.Contains(p.Shell, " ") == false {
		cmd.Args[len(cmd.Args)-1] = p.Shell
	}
	return cmd, nil
}

func NewProfile(shell string, wd string, root string, title string, commands []string) Profile {
	return Profile{
		Shell:            shell,
		WorkingDirectory: wd,
		Root:             root,
		Title:            title,
		Commands:         commands,
	}
}

// Theme holds a color scheme's settings, shared across in-memory runtime
// state (TerminalServer.Theme, TerminalServer.Themes), the YAML config file's
// themes: section (Config.Themes in src/config.go), and the
// /theme-config JSON API (via themeConfigResponse, which embeds Theme).
// MapToTheme/toColorMap remain the conversion path to/from map[string]any —
// used by built-in JSON themes and POST request bodies — neither of which
// goes through these yaml/json struct tags directly. The config file's themes:
// section does go through them, via Config.Themes.
type Theme struct {
	Foreground          string `yaml:"foreground" json:"foreground,omitempty"`
	Background          string `yaml:"background" json:"background,omitempty"`
	Cursor              string `yaml:"cursor" json:"cursor,omitempty"`
	CursorAccent        string `yaml:"cursor-accent" json:"cursorAccent,omitempty"`
	SelectionForeground string `yaml:"selection-foreground" json:"selectionForeground,omitempty"`
	SelectionBackground string `yaml:"selection-background" json:"selectionBackground,omitempty"`
	Black               string `yaml:"black" json:"black,omitempty"`
	BrightBlack         string `yaml:"bright-black" json:"brightBlack,omitempty"`
	Red                 string `yaml:"red" json:"red,omitempty"`
	BrightRed           string `yaml:"bright-red" json:"brightRed,omitempty"`
	Yellow              string `yaml:"yellow" json:"yellow,omitempty"`
	BrightYellow        string `yaml:"bright-yellow" json:"brightYellow,omitempty"`
	Green               string `yaml:"green" json:"green,omitempty"`
	BrightGreen         string `yaml:"bright-green" json:"brightGreen,omitempty"`
	Blue                string `yaml:"blue" json:"blue,omitempty"`
	BrightBlue          string `yaml:"bright-blue" json:"brightBlue,omitempty"`
	Magenta             string `yaml:"magenta" json:"magenta,omitempty"`
	BrightMagenta       string `yaml:"bright-magenta" json:"brightMagenta,omitempty"`
	Cyan                string `yaml:"cyan" json:"cyan,omitempty"`
	BrightCyan          string `yaml:"bright-cyan" json:"brightCyan,omitempty"`
	White               string `yaml:"white" json:"white,omitempty"`
	BrightWhite         string `yaml:"bright-white" json:"brightWhite,omitempty"`
	// BackgroundImage is a server-side file path and is intentionally excluded
	// from JSON serialization to avoid exposing local paths to the browser.
	BackgroundImage string `yaml:"background-image" json:"-"`
}

// HasBackgroundImage reports whether the theme has a background image
// configured, i.e. whether BackgroundImage is a non-empty file path.
func (tm *Theme) HasBackgroundImage() bool {
	return len(tm.BackgroundImage) > 0
}

// MapToTheme maps the key-value pairs from the given map to the corresponding
// fields of the Theme struct. It uses reflection to set the values of the
// struct fields based on the map keys. The map keys are expected to be in a
// format that can be converted to the struct field names. Only string values
// from the map are set to the corresponding struct fields.
//
// Parameters:
//   - m: A map[string]any containing the theme properties to be set.
//
// Note: This method modifies the Theme struct in-place.
func (tm *Theme) MapToTheme(m map[string]any) {
	val := reflect.ValueOf(tm).Elem()
	for k, v := range m {
		// Convert the map key to the struct field name
		fieldName := convertToFieldName(k)
		field := val.FieldByName(fieldName)
		if s, ok := v.(string); ok && field.IsValid() && field.CanSet() {
			field.SetString(s)
		}
	}
}

// toColorMap converts the Theme to a map[string]any using the hyphenated key
// names expected by MapToTheme and the config.go read-modify-write helpers.
// Empty fields are omitted.
// BackgroundImage is intentionally excluded since it holds a file path, not a color.
func (tm Theme) toColorMap() map[string]any {
	m := make(map[string]any)
	set := func(k, v string) {
		if v != "" {
			m[k] = v
		}
	}
	set("foreground", tm.Foreground)
	set("background", tm.Background)
	set("cursor", tm.Cursor)
	set("cursor-accent", tm.CursorAccent)
	set("selection-foreground", tm.SelectionForeground)
	set("selection-background", tm.SelectionBackground)
	set("black", tm.Black)
	set("bright-black", tm.BrightBlack)
	set("red", tm.Red)
	set("bright-red", tm.BrightRed)
	set("yellow", tm.Yellow)
	set("bright-yellow", tm.BrightYellow)
	set("green", tm.Green)
	set("bright-green", tm.BrightGreen)
	set("blue", tm.Blue)
	set("bright-blue", tm.BrightBlue)
	set("magenta", tm.Magenta)
	set("bright-magenta", tm.BrightMagenta)
	set("cyan", tm.Cyan)
	set("bright-cyan", tm.BrightCyan)
	set("white", tm.White)
	set("bright-white", tm.BrightWhite)
	return m
}

type TermConfig struct {
	TLS                bool     `json:"tls"`
	FontFamily         string   `json:"fontFamily"`
	FontSize           int      `json:"fontSize"`
	Rows               int      `json:"rows"`
	Columns            int      `json:"columns"`
	AutoResize         bool     `json:"autoResize"`
	Theme              Theme    `json:"theme"`
	Uri                string   `json:"uri"`
	Port               int      `json:"port"`
	Debug              bool     `json:"debug"`
	HasBackgroundImage bool     `json:"backgroundImage"`
	ThemeNames         []string `json:"themeNames"`
	AllThemeNames      []string `json:"allThemeNames"`
	BuiltinThemeNames  []string `json:"builtinThemeNames"`
	ProfileNames       []string `json:"profileNames"`
	ActiveTheme        string   `json:"activeTheme"`
	ShowMenubar        string   `json:"showMenubar"`
}

func NewTermConfig(srv *Server, clnt *TerminalClient, thm *Theme, themeNames []string, allThemeNames []string, builtinThemeNames []string, profileNames []string, activeTheme string, showMenubar string) *TermConfig {
	return &TermConfig{
		TLS:                srv.TLS.Enabled,
		FontFamily:         clnt.FontFamily,
		FontSize:           clnt.FontSize,
		Rows:               clnt.Rows,
		Columns:            clnt.Columns,
		AutoResize:         clnt.AutoResize,
		Theme:              *thm,
		Uri:                srv.URL.Hostname(),
		Port:               srv.Port(),
		Debug:              debugEnabled.Load(),
		HasBackgroundImage: thm.HasBackgroundImage(),
		ThemeNames:         themeNames,
		AllThemeNames:      allThemeNames,
		BuiltinThemeNames:  builtinThemeNames,
		ProfileNames:       profileNames,
		ActiveTheme:        activeTheme,
		ShowMenubar:        showMenubar,
	}
}

// themePaletteResponse is the JSON shape returned by themePaletteHandler and consumed
// by the B3ttyThemeSelector component to build palette preview cards.
type themePaletteResponse struct {
	Bg     string   `json:"bg"`
	Fg     string   `json:"fg"`
	SelBg  string   `json:"selBg"`
	Cursor string   `json:"cursor"`
	Normal []string `json:"normal"`
	Bright []string `json:"bright"`
}

// themeConfigResponse is the JSON shape returned by themeConfigHandler. It embeds
// all Theme color fields (BackgroundImage is excluded via json:"-") and adds a
// HasBackgroundImage boolean so the client knows whether to enable background-image
// mode without receiving the server-side file path.
type themeConfigResponse struct {
	Theme
	HasBackgroundImage bool     `json:"hasBackgroundImage"`
	ThemeNames         []string `json:"themeNames,omitempty"`
}

// editProfileResponse is returned by POST /edit-profile and POST /delete-profile.
// ProfileNames is the sorted list of all non-default profile names after the operation.
type editProfileResponse struct {
	ProfileNames []string `json:"profileNames"`
}

// SettingsServerConfig holds the subset of server settings exposed via the
// Settings overlay. These fields require a server restart to take effect.
// It also composes into ServerConfig (src/config.go) for YAML config file
// validation, hence the yaml tags alongside the existing json tags.
type SettingsServerConfig struct {
	Port        int    `yaml:"port" json:"port"`
	NoAuth      bool   `yaml:"no-auth" json:"noAuth"`
	NoBrowser   bool   `yaml:"no-browser" json:"noBrowser"`
	ShowMenubar string `yaml:"show-menubar" json:"showMenubar"`
}

// NewSettingsServerConfig constructs a SettingsServerConfig from plain values.
// Unlike NewServer/NewTerminalClient, this takes non-pointer params: every
// call site already holds plain values (a method-call result, dereferenced
// struct fields, or local vars), not addresses of long-lived cobra-flag
// package vars, so pointer params would only force throwaway locals.
func NewSettingsServerConfig(port int, noAuth bool, noBrowser bool, showMenubar string) SettingsServerConfig {
	return SettingsServerConfig{
		Port:        port,
		NoAuth:      noAuth,
		NoBrowser:   noBrowser,
		ShowMenubar: showMenubar,
	}
}

// settingsConfigResponse is the JSON shape for GET and POST /settings.
type settingsConfigResponse struct {
	Server   SettingsServerConfig `json:"server"`
	Terminal TerminalClient       `json:"terminal"`
}

// CSPHeader represents a single Content-Security-Policy directive, consisting of
// a directive name (e.g. "script-src") and one or more source values
// (e.g. "self", "nonce-abc123"). Values are rendered without surrounding quotes
// in Add/Set but wrapped in single quotes by String() to produce valid CSP syntax.
type CSPHeader struct {
	Name   string
	Values []string
}

// Set replaces all source values for this directive with the provided values,
// discarding any previously assigned values. Returns the receiver for chaining.
func (ch *CSPHeader) Set(values ...string) *CSPHeader {
	var vals []string
	for _, value := range values {
		vals = append(vals, value)
	}
	ch.Values = vals
	return ch
}

// Add appends a single source value to this directive. Returns the receiver for
// chaining. Mutations are reflected in any CSPHeaders map that holds a pointer
// to this CSPHeader.
func (ch *CSPHeader) Add(value string) *CSPHeader {
	ch.Values = append(ch.Values, value)
	return ch
}

// String renders the directive as a CSP-formatted string, e.g.
// "script-src 'self' 'nonce-abc123';". Each value is wrapped in single quotes.
func (ch CSPHeader) String() string {
	var vals []string
	for _, value := range ch.Values {
		vals = append(vals, fmt.Sprintf("'%s'", value))
	}
	return fmt.Sprintf("%s %s;", ch.Name, strings.Join(vals, " "))
}

// NewCSPHeader constructs a CSPHeader with the given directive name and initial
// source values.
func NewCSPHeader(name string, values ...string) *CSPHeader {
	return &CSPHeader{
		Name:   name,
		Values: values,
	}
}

// CSPHeaders is a collection of CSPHeader directives keyed by directive name.
// Directive pointers are stored in the map, so mutations via Get().Add() or
// Get().Set() are reflected in the CSPHeaders value without re-inserting the
// directive. Use String() to serialize the full policy for use in a
// Content-Security-Policy response header.
type CSPHeaders struct {
	Headers map[string]*CSPHeader
}

// Get returns a pointer to the CSPHeader for the given directive name, or nil
// if no such directive exists. Mutating the returned pointer updates the entry
// in place, which is reflected in subsequent calls to String().
func (chs CSPHeaders) Get(key string) *CSPHeader {
	return chs.Headers[key]
}

// Add inserts or replaces the directive stored under key. Returns a pointer to
// the (possibly updated) CSPHeaders for chaining.
func (chs CSPHeaders) Add(key string, header *CSPHeader) *CSPHeaders {
	chs.Headers[key] = header
	return &chs
}

// String serializes all directives into a single Content-Security-Policy header
// value, with each directive separated by a space. Directive order is not
// guaranteed because the underlying storage is a map.
func (chs CSPHeaders) String() string {
	var vals []string
	for _, values := range chs.Headers {
		vals = append(vals, values.String())
	}
	return strings.Join(vals, " ")
}

// NewCSPHeders constructs a CSPHeaders collection from the provided directives,
// keyed by each directive's Name field.
func NewCSPHeders(headers ...*CSPHeader) *CSPHeaders {
	cspHeaders := CSPHeaders{}
	cspHeaders.Headers = make(map[string]*CSPHeader)
	for _, csp := range headers {
		cspHeaders.Add(csp.Name, csp)
	}
	return &cspHeaders
}
