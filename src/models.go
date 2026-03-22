package src

import (
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
	Uri    string
	Port   int
	NoAuth bool
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

type TermConfig struct {
	TLS               bool   `json:"tls"`
	CursorBlink       bool   `json:"cursorBlink"`
	FontFamily        string `json:"fontFamily"`
	FontSize          int    `json:"fontSize"`
	Rows              int    `json:"rows"`
	Columns           int    `json:"columns"`
	Theme             Theme  `json:"theme"`
	Uri               string `json:"uri"`
	Port              int    `json:"port"`
	Debug             bool   `json:"debug"`
	HasBackgroundImage bool  `json:"backgroundImage"`
}

func NewTermConfig(srv *Server, clnt *Client, thm *Theme) *TermConfig {
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
	}
}
