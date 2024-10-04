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

func (p *Profile) ParseCommands() ([][]string, error) {
	commands := [][]string{}
	var err error
	for i, cmd := range p.Commands {
		commands[i], err = shlex.Split(strings.TrimSpace(cmd))
		if err != nil {
			return commands, err
		}
	}
	return commands, nil
}

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

	// if p.Root != "" && p.Root != "/" {
	// 	cmd.SysProcAttr = &syscall.SysProcAttr{}
	// 	cmd.SysProcAttr.Chroot = p.Root
	// }

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
	Foreground          string
	Background          string
	SelectionForeground string
	SelectionBackground string
	Black               string
	BrightBlack         string
	Red                 string
	BrightRed           string
	Yellow              string
	BrightYellow        string
	Green               string
	BrightGreen         string
	Blue                string
	BrightBlue          string
	Magenta             string
	BrightMagenta       string
	Cyan                string
	BrightCyan          string
	White               string
	BrightWhite         string
}

func (tm *Theme) MapToTheme(m map[string]any) {
	val := reflect.ValueOf(tm).Elem()
	for k, v := range m {
		// Convert the map key to the struct field name
		fieldName := convertToFieldName(k)
		field := val.FieldByName(fieldName)
		if field.IsValid() && field.CanSet() {
			field.SetString(v.(string))
		}
	}
}
