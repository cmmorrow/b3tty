package src

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// wsClient wraps a WebSocket connection with a write mutex so that the PTY
// output goroutine and broadcastSettings can both write without data races.
type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

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

// logFirstSessionTiming logs, in debug mode only and at most once per server
// lifetime, the time elapsed between process start (ts.StartTime) and the
// first WebSocket session's pty becoming ready — a one-time "time to first
// interactive session" startup metric, not a per-connection one, so later
// sessions (reconnects, additional tabs) are deliberately not logged again.
func (ts *TerminalServer) logFirstSessionTiming() {
	ts.WSEstablishedOnce.Do(func() {
		Debugf("time from server start to websocket+pty established: %dms", time.Since(ts.StartTime).Milliseconds())
	})
}

// wsOutput is a Session's output target: it sends each chunk to the
// WebSocket as a binary message. failed is closed on the first write error,
// after which the session detaches it.
type wsOutput struct {
	client   *wsClient
	failed   chan struct{}
	failOnce sync.Once
}

func (o *wsOutput) Write(p []byte) (int, error) {
	o.client.mu.Lock()
	o.client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := o.client.conn.WriteMessage(websocket.BinaryMessage, p)
	o.client.mu.Unlock()
	if err != nil {
		Errorf("write from pty: %v", err)
		o.failOnce.Do(func() { close(o.failed) })
		return 0, err
	}
	return len(p), nil
}

// terminalHandler upgrades the HTTP connection to a WebSocket and bridges it
// to a new quick Session running the active profile's shell, under a pty
// sized to the "cols"/"rows" query parameters of the /ws request (falling
// back to DEFAULT_COLS/DEFAULT_ROWS via parseSizeParams when absent).
// Session output goes to the WebSocket through wsOutput; a goroutine
// forwards WebSocket input and resizes to the session. Closing the WebSocket
// kills the session; the session ending closes the WebSocket.
func (ts *TerminalServer) terminalHandler(w http.ResponseWriter, r *http.Request) {
	Debugf(" %s -> %s %s %s", r.RemoteAddr, r.Host, r.Method, redactedURL(r.URL))
	Debugf("content length: %d", r.ContentLength)
	// Checked before the upgrade: the upgrader's Origin check only stops
	// browsers on other sites, and lets through any client that sends no
	// Origin header at all, so without the token anyone who could reach the
	// port could open a shell.
	if !validateToken(r.URL.Query().Get("token"), ts.Token) {
		Warnf("%s %s: forbidden: invalid or missing token", r.Method, r.URL.Path)
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		Errorf("upgrader error: %v", err)
		return
	}
	defer ws.Close()
	connectedAt := time.Now()

	client := &wsClient{conn: ws}
	ts.WSClientsMu.Lock()
	ts.WSClients[ws] = client
	ts.WSClientsMu.Unlock()
	defer func() {
		ts.WSClientsMu.Lock()
		delete(ts.WSClients, ws)
		ts.WSClientsMu.Unlock()
	}()

	orgCols, orgRows := parseSizeParams(r.URL.Query())

	ts.StateMu.RLock()
	profile := ts.Profiles[ts.ProfileName]
	ts.StateMu.RUnlock()

	var sess Session
	sess, err = newLocalSession(SessionOptions{
		Mode:         SessionQuick,
		Profile:      profile,
		Cols:         orgCols,
		Rows:         orgRows,
		CommandSleep: ts.CommandSleep,
	})
	if err != nil {
		Errorf("%v", err)
		return
	}
	ts.logFirstSessionTiming()
	// Every session is quick for now, so it ends with its WebSocket.
	defer func() { _ = sess.Kill() }()

	out := &wsOutput{client: client, failed: make(chan struct{})}
	replay, err := sess.Attach(out)
	if err != nil {
		Errorf("attach session: %v", err)
		return
	}
	if len(replay) > 0 {
		_, _ = out.Write(replay)
	}

	// done is closed just before the handler closes the WebSocket on its
	// own (the session ended, or writing to the WebSocket failed). This
	// lets the input goroutine distinguish a planned shutdown from a
	// genuine unexpected error.
	done := make(chan struct{})

	// Handle input from the WebSocket
	go func() {
		for {
			msgType, message, err := ws.ReadMessage()
			if err != nil {
				select {
				case <-done:
					// ws.Close() was called by us after the session ended — not an error.
					Warn("websocket closed after terminal session ended")
				default:
					switch err.(type) {
					case *websocket.CloseError:
						// Connections dropped in under 3 seconds are almost always
						// browser speculative loads (e.g. Safari paste-to-address-bar
						// prefetch) rather than genuine session failures.
						if time.Since(connectedAt) < 3*time.Second {
							Debugf("websocket closed after %s (likely speculative browser load)", time.Since(connectedAt).Round(time.Millisecond))
						} else {
							Warn("cannot read from websocket: unexpectedly closed")
						}
					default:
						Errorf("websocket read: %v", err)
					}
				}
				_ = sess.Kill()
				return
			}
			if msgType == websocket.TextMessage {
				if cols, rows, ok := parseResizeMessage(message); ok {
					if cols == 0 || rows == 0 {
						Warnf("ignoring resize to invalid dimensions: cols=%d rows=%d", cols, rows)
						continue
					}
					Debugf("resizing to %d, %d", cols, rows)
					if err := sess.Resize(cols, rows); err != nil {
						Errorf("error calling pty resize: %v", err)
					}
					continue
				}
			}
			if _, err := sess.Write(message); err != nil {
				if errors.Is(err, os.ErrClosed) {
					Debugf("write to pty: %v", err)
				} else {
					Errorf("write to pty: %v", err)
				}
				_ = sess.Kill()
				return
			}
		}
	}()

	// Wait for the session to end (the shell exited, or the input goroutine
	// killed it after the WebSocket closed), or for a write to the
	// WebSocket to fail. Returning runs the deferred Kill and ws.Close.
	select {
	case <-sess.Done():
	case <-out.failed:
		_ = sess.Kill()
	}
	close(done)
}
