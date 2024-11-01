package src

import (
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// const indexPath = "src/index.html"
const DEFAULT_COLS = 80
const DEFAULT_ROWS = 24
const BUFFER_SIZE = 4096

var InitClient *Client
var InitServer *Server
var Profiles map[string]Profile

var token string
var orgCols = uint16(DEFAULT_COLS)
var orgRows = uint16(DEFAULT_ROWS)
var profileName = "default"

var upgrader = websocket.Upgrader{
	ReadBufferSize:    BUFFER_SIZE,
	WriteBufferSize:   BUFFER_SIZE,
	EnableCompression: true,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("origin")
		if strings.Contains(origin, "localhost") || strings.Contains(origin, "127.0.0.1") {
			return true
		}
		return false
	},
}

func Serve(shouldOpenBrowser bool, useTLS bool) {
	var err error
	var tokenQuery = ""
	var protocol = "http"

	if useTLS {
		protocol = "https"
	}

	if !InitServer.NoAuth {
		token, err = generateToken(16)
		if err != nil {
			log.Fatalf("error generating token: %v", err)
		}
		tokenQuery = "?token=" + token
	}

	addr := InitServer.Addr().Host
	uiUrl := protocol + "://" + addr + "/" + tokenQuery

	if shouldOpenBrowser {
		err = openBrowser(uiUrl)
		if err != nil {
			log.Fatal("faild to open default browser")
		}
	}

	mux := http.NewServeMux()
	log.Printf("%s server started on "+uiUrl, protocol)

	// Display the available profiles in the config file
	if len(Profiles) > 1 {
		log.Println("Configured profiles:")
		var prfQuery string
		if len(tokenQuery) > 0 {
			prfQuery = "&profile="
		} else {
			prfQuery = "?profile="
		}
		for prf := range Profiles {
			if prf == "default" {
				continue
			}
			log.Printf("* %s%s%s", uiUrl, prfQuery, prf)
		}
	}

	mux.HandleFunc("/", displayTermHandler)
	mux.HandleFunc("/ws", terminalHandler)
	mux.HandleFunc("/size", setSizeHandler)
	if useTLS {
		err = http.ListenAndServeTLS(addr, InitServer.CertFilePath, InitServer.KeyFilePath, mux)
	} else {
		err = http.ListenAndServe(addr, mux)
	}
	if err != nil {
		log.Fatalf("%s server error: %v", protocol, err)
	}
}

func setSizeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	val := r.URL.Query()
	cols, err := strconv.Atoi(val.Get("cols"))
	if err != nil {
		cols = DEFAULT_COLS
	}
	rows, err := strconv.Atoi(val.Get("rows"))
	if err != nil {
		rows = DEFAULT_ROWS
	}
	orgCols = uint16(cols)
	orgRows = uint16(rows)
}

func displayTermHandler(w http.ResponseWriter, r *http.Request) {
	type Props struct {
		Client
		Server
		Title *string
	}
	t, err := template.New("b3tty").Parse(HtmlTemplate)
	if err != nil {
		log.Fatal(err)
	}

	query := r.URL.Query()

	if query.Get("token") != token {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	p := query.Get("profile")
	if p != "" {
		profileName = p
	} else {
		profileName = "default"
	}
	profile := Profiles[profileName]

	err = t.Execute(w, Props{Client: *InitClient, Server: *InitServer, Title: &profile.Title})
	if err != nil {
		log.Println("response error: ", err)
		return
	}
}

// terminalHandler handles WebSocket connections for terminal sessions.
// It upgrades the HTTP connection to a WebSocket, starts a shell process,
// and manages bidirectional communication between the WebSocket and the
// shell's pseudo-terminal (pty).
//
// The function performs the following tasks:
//   - Upgrades the HTTP connection to a WebSocket
//   - Starts a shell process with a pty
//   - Handles input from the WebSocket and writes it to the pty
//   - Reads output from the pty and sends it to the WebSocket
//   - Manages the lifecycle of the WebSocket and pty connections
//
// Parameters:
//   - w: http.ResponseWriter to write the HTTP response
//   - r: *http.Request containing the HTTP request details
//
// The function runs indefinitely until the WebSocket or pty connection is closed.
func terminalHandler(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrader error: %v", err)
		return
	}
	defer ws.Close()

	profile := Profiles[profileName]

	// Start the default shell
	c := exec.Command("/bin/sh", "-c", profile.Shell)
	c, err = profile.ApplyToCommand(c)
	if err != nil {
		log.Println(err)
		return
	}

	windowSize := &pty.Winsize{
		Cols: orgCols,
		Rows: orgRows,
	}

	ptmx, err := pty.StartWithSize(c, windowSize)
	if err != nil {
		log.Println(err)
		return
	}

	defer func() { _ = ptmx.Close() }() // Best effort.

	// Handle input from the WebSocket
	go func() {
		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				switch err.(type) {
				case *websocket.CloseError:
					log.Println("cannot read from websocket: unexpectedly closed")
				default:
					log.Println("read:", err)
				}
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
			err = ws.WriteMessage(websocket.BinaryMessage, buf[:n])
			if err != nil {
				log.Println("write from pty:", err)
			}
		}
	}()

	if len(profile.Commands) > 0 {
		time.Sleep(time.Second * 1)
		for _, command := range profile.Commands {
			_, err = ptmx.Write([]byte(strings.TrimSpace(command) + "\n"))
			if err != nil {
				log.Println("write to pty:", err)
				os.Exit(24)
			}
			time.Sleep(time.Millisecond * 200)
		}
	}

	// Keep the connection open
	select {}
}
