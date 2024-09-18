package src

import (
	"net/url"
	"reflect"
	"strconv"
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
