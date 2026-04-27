package src

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"

	"github.com/google/shlex"
)

type Client struct {
	Rows        int
	Columns     int
	CursorBlink bool
	FontFamily  string
	FontSize    int
	Theme       Theme
}

func NewClient(rows *int, columns *int, blink *bool, fontFamily *string, fontSize *int, theme *Theme) *Client {
	return &Client{
		Rows:        *rows,
		Columns:     *columns,
		CursorBlink: *blink,
		FontFamily:  *fontFamily,
		FontSize:    *fontSize,
		Theme:       *theme,
	}
}

type Server struct {
	Uri      string
	Port     int
	NoAuth   bool
	FirstRun bool
	TLS
}

func NewServer(uri *string, port *int, noAuth *bool, tls *TLS) *Server {
	return &Server{
		Uri:    *uri,
		Port:   *port,
		NoAuth: *noAuth,
		TLS:    *tls,
	}
}

func (s *Server) Addr() url.URL {
	return url.URL{
		Host: s.Uri + ":" + strconv.Itoa(s.Port),
	}
}

type TLS struct {
	Enabled      bool
	CertFilePath string
	KeyFilePath  string
}

type Profile struct {
	Root             string
	WorkingDirectory string
	Shell            string
	Title            string
	Commands         []string
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

type Theme struct {
	Foreground          string `json:"foreground,omitempty"`
	Background          string `json:"background,omitempty"`
	Cursor              string `json:"cursor,omitempty"`
	CursorAccent        string `json:"cursorAccent,omitempty"`
	SelectionForeground string `json:"selectionForeground,omitempty"`
	SelectionBackground string `json:"selectionBackground,omitempty"`
	Black               string `json:"black,omitempty"`
	BrightBlack         string `json:"brightBlack,omitempty"`
	Red                 string `json:"red,omitempty"`
	BrightRed           string `json:"brightRed,omitempty"`
	Yellow              string `json:"yellow,omitempty"`
	BrightYellow        string `json:"brightYellow,omitempty"`
	Green               string `json:"green,omitempty"`
	BrightGreen         string `json:"brightGreen,omitempty"`
	Blue                string `json:"blue,omitempty"`
	BrightBlue          string `json:"brightBlue,omitempty"`
	Magenta             string `json:"magenta,omitempty"`
	BrightMagenta       string `json:"brightMagenta,omitempty"`
	Cyan                string `json:"cyan,omitempty"`
	BrightCyan          string `json:"brightCyan,omitempty"`
	White               string `json:"white,omitempty"`
	BrightWhite         string `json:"brightWhite,omitempty"`
	// BackgroundImage is a server-side file path and is intentionally excluded
	// from JSON serialization to avoid exposing local paths to the browser.
	BackgroundImage string `json:"-"`
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
// names expected by MapToTheme and buildConfigYAML. Empty fields are omitted.
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
	CursorBlink        bool     `json:"cursorBlink"`
	FontFamily         string   `json:"fontFamily"`
	FontSize           int      `json:"fontSize"`
	Rows               int      `json:"rows"`
	Columns            int      `json:"columns"`
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
}

func NewTermConfig(srv *Server, clnt *Client, thm *Theme, themeNames []string, allThemeNames []string, builtinThemeNames []string, profileNames []string, activeTheme string) *TermConfig {
	return &TermConfig{
		TLS:                srv.TLS.Enabled,
		CursorBlink:        clnt.CursorBlink,
		FontFamily:         clnt.FontFamily,
		FontSize:           clnt.FontSize,
		Rows:               clnt.Rows,
		Columns:            clnt.Columns,
		Theme:              *thm,
		Uri:                srv.Uri,
		Port:               srv.Port,
		Debug:              debugEnabled,
		HasBackgroundImage: thm.BackgroundImage != "",
		ThemeNames:         themeNames,
		AllThemeNames:      allThemeNames,
		BuiltinThemeNames:  builtinThemeNames,
		ProfileNames:       profileNames,
		ActiveTheme:        activeTheme,
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

// profileConfigResponse is the JSON shape returned by GET /profile-config.
type profileConfigResponse struct {
	Shell            string   `json:"shell"`
	WorkingDirectory string   `json:"workingDirectory"`
	Title            string   `json:"title"`
	Root             string   `json:"root"`
	Commands         []string `json:"commands"`
}

// editProfileResponse is returned by POST /edit-profile and POST /delete-profile.
// ProfileNames is the sorted list of all non-default profile names after the operation.
type editProfileResponse struct {
	ProfileNames []string `json:"profileNames"`
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
