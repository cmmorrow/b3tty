package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/cmmorrow/b3tty/src"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runningServer starts a test server recording the last request it received,
// and writes a lock file (under a temp HOME) pointing at it with the given
// token, as a running b3tty server would.
func runningServer(t *testing.T, token string) *http.Request {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	got := &http.Request{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = *r
		got.Body = io.NopCloser(nil)
		got.Header = r.Header.Clone()
		got.Form = url.Values{"body": {string(body)}}
	}))
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	require.NoError(t, src.WriteLockFile(port, 1, token, "http"))
	return got
}

func TestPostToRunningServer(t *testing.T) {
	t.Run("posts to the port in the lock file with the token as a bearer header", func(t *testing.T) {
		got := runningServer(t, "lock-token")
		postToRunningServer("/settings", map[string]int{"n": 1})
		assert.Equal(t, http.MethodPost, got.Method)
		assert.Equal(t, "/settings", got.URL.Path)
		assert.Equal(t, "Bearer lock-token", got.Header.Get("Authorization"))
		assert.Equal(t, "application/json", got.Header.Get("Content-Type"))
		var body map[string]int
		require.NoError(t, json.Unmarshal([]byte(got.Form.Get("body")), &body))
		assert.Equal(t, 1, body["n"])
	})

	t.Run("sends no Authorization header when the server runs without auth", func(t *testing.T) {
		got := runningServer(t, "")
		postToRunningServer("/settings", struct{}{})
		assert.Equal(t, "/settings", got.URL.Path)
		assert.Empty(t, got.Header.Get("Authorization"))
	})

	t.Run("does nothing when no server is running", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		assert.NotPanics(t, func() { postToRunningServer("/settings", struct{}{}) })
	})
}
