package src

import (
	"crypto/rand"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	// "os"
	"os/exec"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	// "golang.org/x/crypto/ssh/terminal"
)

const indexPath = "src/index.html"
const DEFAULT_COLS = 80
const DEFAULT_ROWS = 24
const BUFFER_SIZE = 1024

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

type TLS struct {
	Enabled      bool
	CertFilePath string
	KeyFilePath  string
}

type Server struct {
	Uri    string
	Port   int
	NoAuth bool
	TLS
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

func convertToFieldName(key string) string {
	parts := strings.Split(key, "-")
	for i, part := range parts {
		parts[i] = strings.Title(part)
	}
	return strings.Join(parts, "")
}

func MapToStruct(m map[string]any, s any) {
	val := reflect.ValueOf(s).Elem()
	for k, v := range m {
		// Convert the map key to the struct field name
		fieldName := convertToFieldName(k)
		field := val.FieldByName(fieldName)
		if field.IsValid() && field.CanSet() {
			field.SetString(v.(string))
		}
	}
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

var InitClient *Client
var InitServer *Server
var orgCols *uint16
var orgRows *uint16
var token string

var upgrader = websocket.Upgrader{
	ReadBufferSize:  512,
	WriteBufferSize: 512,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("origin")
		if strings.Contains(origin, "localhost") || strings.Contains(origin, "127.0.0.1") {
			return true
		}
		return false
	},
}

func getSize() *pty.Winsize {
	var cols = uint16(DEFAULT_COLS)
	var rows = uint16(DEFAULT_ROWS)
	if orgCols == nil {
		orgCols = &cols
	}
	if orgRows == nil {
		orgRows = &rows
	}
	return &pty.Winsize{
		Cols: *orgCols,
		Rows: *orgRows,
	}
}

func Serve(openBrowser bool) {
	var err error
	var tokenQuery = ""
	if !InitServer.NoAuth {
		token, err = generateToken(16)
		if err != nil {
			log.Fatalf("Error generating token: %v", err)
		}
		tokenQuery = "?token=" + token
	}

	addr := InitServer.Addr().Host
	uiUrl := "http://" + addr + "/" + tokenQuery

	if openBrowser {
		err = OpenBrowser(uiUrl)
		if err != nil {
			log.Fatal("failed to open default browser")
		}
	}

	log.Println("http server started on " + uiUrl)

	http.HandleFunc("/ws", handleTerm)
	http.HandleFunc("/", displayTerm)
	http.HandleFunc("/title/*", displayTerm)
	http.HandleFunc("/size", setSize)
	err = http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func ServeTLS(openBrowser bool) {
	var err error
	var tokenQuery = ""
	if !InitServer.NoAuth {
		token, err = generateToken(16)
		if err != nil {
			log.Fatalf("Error generating token: %v", err)
		}
		tokenQuery = "?token=" + token
	}

	addr := InitServer.Addr().Host
	uiUrl := "https://" + addr + "/" + tokenQuery

	if openBrowser {
		err = OpenBrowser(uiUrl)
		if err != nil {
			log.Fatal("failed to open default browser")
		}
	}

	https := http.NewServeMux()
	log.Println("https server started on " + uiUrl)

	https.HandleFunc("/ws", handleTerm)
	https.HandleFunc("/", displayTerm)
	https.HandleFunc("/title/*", displayTerm)
	https.HandleFunc("/size", setSize)
	err = http.ListenAndServeTLS(addr, InitServer.CertFilePath, InitServer.KeyFilePath, https)
	if err != nil {
		log.Fatalf("HTTPS server error: %v", err)
	}
}

func setSize(w http.ResponseWriter, r *http.Request) {
	val := r.URL.Query()
	cols, err := strconv.Atoi(val.Get("cols"))
	if err != nil {
		cols = DEFAULT_COLS
	}
	rows, err := strconv.Atoi(val.Get("rows"))
	if err != nil {
		rows = DEFAULT_ROWS
	}
	x := uint16(cols)
	y := uint16(rows)
	orgCols = &x
	orgRows = &y
}

func displayTerm(w http.ResponseWriter, r *http.Request) {
	type Props struct {
		Client
		Server
		Title *string
	}
	// f, err := os.Open(indexPath)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// content, err := io.ReadAll(f)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// t, err := template.New("b3tty").Parse(string(content))
	t, err := template.New("b3tty").Parse(HtmlTemplate)
	if err != nil {
		log.Fatal(err)
	}

	title := ""
	path := r.URL.Path
	if strings.Contains(path, "title") {
		fullPath := strings.Split(path, "/")
		if fullPath[1] == "title" {
			title = fullPath[2]
		}
	}

	query := r.URL.Query()
	if query.Get("token") != token {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err = t.Execute(w, Props{Client: *InitClient, Server: *InitServer, Title: &title})
	if err != nil {
		log.Fatal(err)
	}
}

func sum(arr []int) int {
	total := 0
	for _, i := range arr {
		total += i
	}
	return total
}

func OpenBrowser(url string) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		return err
	}
	return nil
}

func generateToken(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	charsetLength := big.NewInt(int64(len(charset)))
	for i := range result {
		randomInt, err := rand.Int(rand.Reader, charsetLength)
		if err != nil {
			return "", err
		}
		result[i] = charset[randomInt.Int64()]
	}
	return string(result), nil
}

func handleTerm(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer ws.Close()

	// Start a zsh shell
	c := exec.Command("zsh")
	// ptmx, err := pty.Start(c)
	// size := pty.Winsize{
	// 	Rows: 24,
	// 	Cols: 172,
	// }
	ptmx, err := pty.StartWithSize(c, getSize())
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = ptmx.Close() }() // Best effort.

	// Handle input from the WebSocket
	go func() {
		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				break
			}
			_, err = ptmx.Write(message)
			if err != nil {
				log.Println("write to pty:", err)
				os.Exit(24)
			}
		}
	}()

	// Handle output from the pty
	go func() {
		buf := make([]byte, BUFFER_SIZE)
		n := 0
		var err error
		for {
			n, err = ptmx.Read(buf)
			if err != nil {
				switch err {
				case io.EOF:
					log.Println("Terminal session closed")
				default:
					log.Println(err.Error())
				}
				ptmx.Close()
				ws.Close()
				return
			}
			err = ws.WriteMessage(websocket.TextMessage, buf[:n])
			if err != nil {
				log.Println("write from pty:", err)
			}
		}
	}()

	// Keep the connection open
	select {}
}
