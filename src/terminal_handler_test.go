package src

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newWSTestServer starts a real HTTP server exposing only ts.terminalHandler
// at /ws and returns the ws:// URL to dial. The server is closed automatically
// when the test ends.
func newWSTestServer(t *testing.T, ts *TerminalServer) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", ts.terminalHandler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
}

// dialAndRead dials wsURL, optionally with the given request header (e.g. to
// set a custom Origin), and — on success — starts a single background
// goroutine that continuously reads messages and publishes their payloads on
// the returned channel until the connection errors or closes, at which point
// the channel is closed. This matches gorilla/websocket's contract that
// ReadMessage must be driven by exactly one goroutine and that any error it
// returns (including a deadline timeout) is permanent: callers must never
// call ReadMessage again afterward, so all reading is centralized here
// instead of polled with short per-call deadlines. The connection is closed
// automatically when the test ends.
func dialAndRead(t *testing.T, wsURL string, header http.Header) (*websocket.Conn, <-chan []byte, *http.Response, error) {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		return nil, nil, resp, err
	}
	t.Cleanup(func() { conn.Close() })

	ch := make(chan []byte, 64)
	go func() {
		defer close(ch)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			ch <- data
		}
	}()
	return conn, ch, resp, nil
}

// readUntilContains accumulates payloads from ch until the accumulated text
// contains want, the channel closes, or timeout elapses — whichever comes
// first — and returns whatever was accumulated. Callers can assert on
// partial output or on the absence of a substring.
func readUntilContains(ch <-chan []byte, want string, timeout time.Duration) string {
	var out strings.Builder
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return out.String()
			}
			out.Write(data)
			if strings.Contains(out.String(), want) {
				return out.String()
			}
		case <-deadline.C:
			return out.String()
		}
	}
}

// newCatTestServer returns a TerminalServer whose default profile runs `cat`
// under the pty instead of an interactive shell — cat echoes back whatever
// it reads, making output deterministic and dependency-free for assertions.
func newCatTestServer() *TerminalServer {
	ts := newTestTerminalServer()
	ts.Profiles["default"] = Profile{Shell: "cat"}
	return ts
}

// ---------------------------------------------------------------------------
// terminalHandler
// ---------------------------------------------------------------------------

func TestTerminalHandler(t *testing.T) {
	t.Run("echoes typed input back over the websocket", func(t *testing.T) {
		ts := newCatTestServer()
		wsURL := newWSTestServer(t, ts)
		conn, ch, _, err := dialAndRead(t, wsURL, nil)
		require.NoError(t, err)

		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("marco\n")))
		out := readUntilContains(ch, "marco", 2*time.Second)
		assert.Contains(t, out, "marco")
	})

	t.Run("resize message is not leaked into the pty as literal input", func(t *testing.T) {
		ts := newCatTestServer()
		wsURL := newWSTestServer(t, ts)
		conn, ch, _, err := dialAndRead(t, wsURL, nil)
		require.NoError(t, err)

		resizeMsg := `{"type":"resize","cols":100,"rows":40}`
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(resizeMsg)))
		// A successful resize writes nothing to the pty, so a short read
		// window should come back empty.
		out := readUntilContains(ch, "unreachable-sentinel", 300*time.Millisecond)
		assert.Empty(t, out, "resize message must not be echoed as literal pty input")

		// The connection must still be usable afterward.
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("marco\n")))
		out = readUntilContains(ch, "marco", 2*time.Second)
		assert.Contains(t, out, "marco")
	})

	t.Run("resize to a zero dimension is ignored but the connection stays usable", func(t *testing.T) {
		// terminalHandler logs "ignoring resize to invalid dimensions" for
		// this case (see parseResizeMessage/terminalHandler), but that log
		// call happens on the server's goroutine with no synchronization
		// this test could safely observe without racing captureLog's shared
		// buffer, so this test only asserts the resulting behavior: the
		// message is dropped rather than applied or written to the pty, and
		// the connection keeps working afterward.
		ts := newCatTestServer()
		wsURL := newWSTestServer(t, ts)
		conn, ch, _, err := dialAndRead(t, wsURL, nil)
		require.NoError(t, err)

		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":0,"rows":24}`)))
		out := readUntilContains(ch, "unreachable-sentinel", 300*time.Millisecond)
		assert.Empty(t, out, "an ignored resize must not be echoed as literal pty input")

		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("marco\n")))
		out = readUntilContains(ch, "marco", 2*time.Second)
		assert.Contains(t, out, "marco", "connection must still accept input after an ignored resize")
	})

	t.Run("a non-resize JSON text message is written directly to the pty", func(t *testing.T) {
		// parseResizeMessage only special-cases {"type":"resize",...}; any
		// other text message — including JSON that merely resembles a
		// control message — falls through to being written to the pty as
		// literal keystrokes. This documents that existing behavior.
		ts := newCatTestServer()
		wsURL := newWSTestServer(t, ts)
		conn, ch, _, err := dialAndRead(t, wsURL, nil)
		require.NoError(t, err)

		msg := `{"type":"ping"}` + "\n"
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(msg)))
		out := readUntilContains(ch, `"type":"ping"`, 2*time.Second)
		assert.Contains(t, out, `"type":"ping"`)
	})

	t.Run("malformed JSON text message is written directly to the pty", func(t *testing.T) {
		ts := newCatTestServer()
		wsURL := newWSTestServer(t, ts)
		conn, ch, _, err := dialAndRead(t, wsURL, nil)
		require.NoError(t, err)

		msg := "{not valid json\n"
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(msg)))
		out := readUntilContains(ch, "not valid json", 2*time.Second)
		assert.Contains(t, out, "not valid json")
	})

	t.Run("client disconnect cleans up ts.WSClients", func(t *testing.T) {
		ts := newCatTestServer()
		wsURL := newWSTestServer(t, ts)
		conn, _, _, err := dialAndRead(t, wsURL, nil)
		require.NoError(t, err)

		// Wait for the server to register the connection.
		require.Eventually(t, func() bool {
			ts.WSClientsMu.Lock()
			defer ts.WSClientsMu.Unlock()
			return len(ts.WSClients) == 1
		}, 2*time.Second, 20*time.Millisecond, "connection was never registered in ts.WSClients")

		require.NoError(t, conn.Close())

		require.Eventually(t, func() bool {
			ts.WSClientsMu.Lock()
			defer ts.WSClientsMu.Unlock()
			return len(ts.WSClients) == 0
		}, 2*time.Second, 20*time.Millisecond, "ts.WSClients was not cleaned up after the client disconnected")
	})

	t.Run("profile commands are written to the pty after connecting", func(t *testing.T) {
		ts := newTestTerminalServer()
		ts.Profiles["default"] = Profile{Shell: "cat", Commands: []string{"echo readymarker"}}
		wsURL := newWSTestServer(t, ts)
		_, ch, _, err := dialAndRead(t, wsURL, nil)
		require.NoError(t, err)

		// terminalHandler sleeps 1s before writing the first queued command.
		out := readUntilContains(ch, "readymarker", 3*time.Second)
		assert.Contains(t, out, "echo readymarker")
	})

	t.Run("cross-origin upgrade request is rejected", func(t *testing.T) {
		ts := newCatTestServer()
		wsURL := newWSTestServer(t, ts)
		_, _, resp, err := dialAndRead(t, wsURL, http.Header{"Origin": {"http://evil.example.com"}})
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("same-origin upgrade request is accepted", func(t *testing.T) {
		ts := newCatTestServer()
		wsURL := newWSTestServer(t, ts)
		httpURL := "http" + strings.TrimPrefix(wsURL, "ws")
		origin := httpURL[:strings.LastIndex(httpURL, "/ws")]
		_, _, resp, err := dialAndRead(t, wsURL, http.Header{"Origin": {origin}})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	})
}
