package src

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:    BUFFER_SIZE,
	WriteBufferSize:   BUFFER_SIZE,
	EnableCompression: false,
	// CheckOrigin rejects cross-origin WebSocket upgrade requests. An absent
	// Origin header (non-browser clients) is allowed; any browser-sent Origin
	// must match the Host the browser used to reach this server, preventing
	// third-party pages from silently opening terminal connections.
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return u.Host == r.Host
	},
}

// parseResizeMessage tries to unmarshal message as a JSON resize command of the form
// {"type":"resize","cols":N,"rows":N}. On success it returns (cols, rows, true).
// Any parse failure or a non-"resize" type returns (0, 0, false).
func parseResizeMessage(message []byte) (uint16, uint16, bool) {
	var msg struct {
		Type string `json:"type"`
		Cols uint16 `json:"cols"`
		Rows uint16 `json:"rows"`
	}
	if err := json.Unmarshal(message, &msg); err == nil && msg.Type == "resize" {
		return msg.Cols, msg.Rows, true
	}
	return 0, 0, false
}

// formatCommand trims surrounding whitespace from command and appends a newline,
// producing bytes ready to write directly to a pty.
func formatCommand(command string) []byte {
	return []byte(strings.TrimSpace(command) + "\n")
}

// terminalHandler upgrades the HTTP connection to a WebSocket, starts the
// active profile's shell under a pty sized to the dimensions stored by
// setSizeHandler, then runs two goroutines bridging pty output → WebSocket
// and WebSocket input → pty. A done channel coordinated with sync.Once lets
// the input goroutine distinguish a clean PTY-initiated shutdown from an
// unexpected WebSocket error.
func (ts *TerminalServer) terminalHandler(w http.ResponseWriter, r *http.Request) {
	Debugf(" %s -> %s %s %s", r.RemoteAddr, r.Host, r.Method, r.URL)
	Debugf("content length: %d", r.ContentLength)
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		Errorf("upgrader error: %v", err)
		return
	}
	defer ws.Close()

	profile := ts.Profiles[ts.ProfileName]

	// Start the active profile's shell via /bin/sh -c so that shell flags and
	// paths are handled uniformly regardless of the configured shell binary.
	c := exec.Command("/bin/sh", "-c", profile.Shell)
	c, err = profile.ApplyToCommand(c)
	if err != nil {
		Errorf("apply profile to command: %v", err)
		return
	}

	windowSize := &pty.Winsize{
		Cols: ts.OrgCols,
		Rows: ts.OrgRows,
	}

	Debugf("cols: %d", windowSize.Cols)
	Debugf("rows: %d", windowSize.Rows)
	Debug("starting pty....")
	ptmx, err := pty.StartWithSize(c, windowSize)
	if err != nil {
		Errorf("start pty: %v", err)
		return
	}

	defer func() { _ = ptmx.Close() }() // Best effort.

	// done is closed by the PTY output goroutine just before it calls
	// ws.Close(). This lets the WebSocket input goroutine distinguish a
	// planned shutdown (PTY exited) from a genuine unexpected error.
	done := make(chan struct{})
	var closeOnce sync.Once
	signalDone := func() { closeOnce.Do(func() { close(done) }) }

	// Handle input from the WebSocket
	go func() {
		for {
			msgType, message, err := ws.ReadMessage()
			if err != nil {
				select {
				case <-done:
					// ws.Close() was called by us after the PTY exited — not an error.
					Warn("websocket closed after terminal session ended")
				default:
					switch err.(type) {
					case *websocket.CloseError:
						Warn("cannot read from websocket: unexpectedly closed")
					default:
						Errorf("websocket read: %v", err)
					}
				}
				ptmx.Close()
				break
			}
			if msgType == websocket.TextMessage {
				if cols, rows, ok := parseResizeMessage(message); ok {
					if cols == 0 || rows == 0 {
						Warnf("ignoring resize to invalid dimensions: cols=%d rows=%d", cols, rows)
						continue
					}
					Debugf("resizing to %d, %d", cols, rows)
					err = pty.Setsize(ptmx, &pty.Winsize{Cols: cols, Rows: rows})
					if err != nil {
						Errorf("error calling pty resize: %v", err)
					}
					continue
				}
			}
			_, err = ptmx.Write(message)
			if err != nil {
				Errorf("write to pty: %v", err)
				ptmx.Close()
				break
			}
		}
	}()

	// Handle output from the pty
	go func() {
		buf := make([]byte, BUFFER_SIZE)
		for {
			n, err := ptmx.Read(buf)
			Debugf("bytes read from buffer: %d", n)
			if err != nil {
				switch err {
				case io.EOF:
					Info("terminal session closed")
				default:
					Errorf("pty read: %v", err)
				}
				ptmx.Close()
				signalDone()
				ws.Close()
				return
			}
			ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err = ws.WriteMessage(websocket.BinaryMessage, buf[:n])
			if err != nil {
				Errorf("write from pty: %v", err)
				ptmx.Close()
				signalDone()
				break
			}
		}
	}()

	if len(profile.Commands) > 0 {
		time.Sleep(time.Second * 1)
		for _, command := range profile.Commands {
			_, err = ptmx.Write(formatCommand(command))
			if err != nil {
				Errorf("write to pty: %v", err)
				ptmx.Close()
				return
			}
			time.Sleep(time.Millisecond * 200)
		}
	}

	// Wait for the PTY session to end. The output goroutine closes done
	// when the PTY exits; waiting here lets the deferred ws.Close() and
	// ptmx.Close() run on exit rather than leaking this goroutine forever.
	<-done
}
