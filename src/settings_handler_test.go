package src

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSettingsTestServer() *TerminalServer {
	ts := newTestTerminalServer()
	ts.NoBrowser = false
	return ts
}

// ---------------------------------------------------------------------------
// GET /settings
// ---------------------------------------------------------------------------

func TestSettingsHandlerGet(t *testing.T) {
	t.Run("DELETE is rejected with 405", func(t *testing.T) {
		ts := newSettingsTestServer()
		req := httptest.NewRequest(http.MethodDelete, "/settings", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.settingsHandler(w, req) })
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Contains(t, logged, "method not allowed")
	})

	t.Run("GET returns 200 with application/json", func(t *testing.T) {
		ts := newSettingsTestServer()
		req := httptest.NewRequest(http.MethodGet, "/settings", nil)
		w := httptest.NewRecorder()
		ts.settingsHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	})

	t.Run("GET returns live server and terminal state", func(t *testing.T) {
		ts := newSettingsTestServer()
		ts.Client.FontSize = 18
		ts.Client.FontFamily = "Fira Code"
		ts.Client.AutoResize = false
		ts.Client.Rows = 30
		ts.Client.Columns = 100
		ts.Server.Port = 9090
		ts.Server.NoAuth = true
		ts.NoBrowser = true
		ts.ShowMenubar = "visible"

		req := httptest.NewRequest(http.MethodGet, "/settings", nil)
		w := httptest.NewRecorder()
		ts.settingsHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp settingsConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 9090, resp.Server.Port)
		assert.True(t, resp.Server.NoAuth)
		assert.True(t, resp.Server.NoBrowser)
		assert.Equal(t, "visible", resp.Server.ShowMenubar)
		assert.Equal(t, 18, resp.Terminal.FontSize)
		assert.Equal(t, "Fira Code", resp.Terminal.FontFamily)
		assert.False(t, resp.Terminal.AutoResize)
		assert.Equal(t, 30, resp.Terminal.Rows)
		assert.Equal(t, 100, resp.Terminal.Columns)
	})
}

// ---------------------------------------------------------------------------
// POST /settings
// ---------------------------------------------------------------------------

func makeBodyWithShowMenubar(port, fontSize, rows, cols int, fontFamily, showMenubar string, autoResize, noAuth, noBrowser bool) *bytes.Buffer {
	req := settingsConfigResponse{
		Server:   SettingsServerConfig{Port: port, NoAuth: noAuth, NoBrowser: noBrowser, ShowMenubar: showMenubar},
		Terminal: SettingsTerminalConfig{FontFamily: fontFamily, FontSize: fontSize, AutoResize: autoResize, Rows: rows, Columns: cols},
	}
	b, _ := json.Marshal(req)
	return bytes.NewBuffer(b)
}

func TestSettingsHandlerPost(t *testing.T) {
	makeBody := func(port, fontSize, rows, cols int, fontFamily string, autoResize, noAuth, noBrowser bool) *bytes.Buffer {
		return makeBodyWithShowMenubar(port, fontSize, rows, cols, fontFamily, "hover", autoResize, noAuth, noBrowser)
	}

	t.Run("cross-origin POST is rejected with 403", func(t *testing.T) {
		ts := newSettingsTestServer()
		req := httptest.NewRequest(http.MethodPost, "/settings", makeBody(8080, 14, 24, 80, "mono", true, false, false))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.settingsHandler(w, req) })
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, logged, "cross-origin")
	})

	t.Run("POST with invalid port returns 400", func(t *testing.T) {
		ts := newSettingsTestServer()
		req := httptest.NewRequest(http.MethodPost, "/settings", makeBody(0, 14, 24, 80, "mono", true, false, false))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.settingsHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "invalid port")
	})

	t.Run("POST with invalid font-size returns 400", func(t *testing.T) {
		ts := newSettingsTestServer()
		req := httptest.NewRequest(http.MethodPost, "/settings", makeBody(8080, 0, 24, 80, "mono", true, false, false))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.settingsHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "invalid font-size")
	})

	t.Run("POST with rows below minimum returns 400", func(t *testing.T) {
		ts := newSettingsTestServer()
		req := httptest.NewRequest(http.MethodPost, "/settings", makeBody(8080, 14, 9, 80, "mono", true, false, false))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.settingsHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "invalid rows")
	})

	t.Run("POST with columns below minimum returns 400", func(t *testing.T) {
		ts := newSettingsTestServer()
		req := httptest.NewRequest(http.MethodPost, "/settings", makeBody(8080, 14, 24, 9, "mono", true, false, false))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.settingsHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "invalid columns")
	})

	t.Run("POST with invalid show-menubar returns 400", func(t *testing.T) {
		ts := newSettingsTestServer()
		req := httptest.NewRequest(http.MethodPost, "/settings",
			makeBodyWithShowMenubar(8080, 14, 24, 80, "mono", "always", true, false, false))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.settingsHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "invalid show-menubar")
	})

	t.Run("POST with malformed JSON returns 400", func(t *testing.T) {
		ts := newSettingsTestServer()
		req := httptest.NewRequest(http.MethodPost, "/settings", bytes.NewBufferString("{bad json}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		ts.settingsHandler(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("POST updates ts.Client terminal fields in memory", func(t *testing.T) {
		ts := newSettingsTestServer()
		dir := t.TempDir()
		ts.ConfigFile = filepath.Join(dir, "conf.yaml")
		// Write an initial config so the file exists
		require.NoError(t, os.WriteFile(ts.ConfigFile, []byte("theme: b3tty-dark\n"), 0644))

		req := httptest.NewRequest(http.MethodPost, "/settings",
			makeBody(8080, 20, 30, 120, "JetBrains Mono", false, false, false))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		ts.settingsHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		assert.Equal(t, 20, ts.Client.FontSize)
		assert.Equal(t, "JetBrains Mono", ts.Client.FontFamily)
		assert.False(t, ts.Client.AutoResize)
		assert.Equal(t, 30, ts.Client.Rows)
		assert.Equal(t, 120, ts.Client.Columns)
	})

	t.Run("POST does not update ts.Server fields in memory", func(t *testing.T) {
		ts := newSettingsTestServer()
		dir := t.TempDir()
		ts.ConfigFile = filepath.Join(dir, "conf.yaml")
		require.NoError(t, os.WriteFile(ts.ConfigFile, []byte("theme: b3tty-dark\n"), 0644))

		originalPort := ts.Server.Port
		originalShowMenubar := ts.ShowMenubar

		req := httptest.NewRequest(http.MethodPost, "/settings",
			makeBodyWithShowMenubar(9999, 14, 24, 80, "mono", "disable", true, true, true))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		ts.settingsHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		assert.Equal(t, originalPort, ts.Server.Port, "server port should not change in memory")
		assert.Equal(t, originalShowMenubar, ts.ShowMenubar, "show-menubar should not change in memory")
	})

	t.Run("POST returns live server state in response", func(t *testing.T) {
		ts := newSettingsTestServer()
		dir := t.TempDir()
		ts.ConfigFile = filepath.Join(dir, "conf.yaml")
		require.NoError(t, os.WriteFile(ts.ConfigFile, []byte("theme: b3tty-dark\n"), 0644))

		req := httptest.NewRequest(http.MethodPost, "/settings",
			makeBody(9999, 16, 24, 80, "mono", true, false, false))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		ts.settingsHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp settingsConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		// Response server port is the live port (8080), not the requested 9999
		assert.Equal(t, ts.Server.Port, resp.Server.Port)
		assert.Equal(t, 16, resp.Terminal.FontSize)
	})

	t.Run("absent Sec-Fetch-Site header is allowed (non-browser clients)", func(t *testing.T) {
		ts := newSettingsTestServer()
		dir := t.TempDir()
		ts.ConfigFile = filepath.Join(dir, "conf.yaml")
		require.NoError(t, os.WriteFile(ts.ConfigFile, []byte("theme: b3tty-dark\n"), 0644))

		req := httptest.NewRequest(http.MethodPost, "/settings",
			makeBody(8080, 14, 24, 80, "mono", true, false, false))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.settingsHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("POST with empty fontFamily defaults to DEFAULT_FONT_FAMILY", func(t *testing.T) {
		ts := newSettingsTestServer()
		dir := t.TempDir()
		ts.ConfigFile = filepath.Join(dir, "conf.yaml")
		require.NoError(t, os.WriteFile(ts.ConfigFile, []byte("theme: b3tty-dark\n"), 0644))

		req := httptest.NewRequest(http.MethodPost, "/settings",
			makeBody(8080, 14, 24, 80, "", true, false, false))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		ts.settingsHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		assert.Equal(t, DEFAULT_FONT_FAMILY, ts.Client.FontFamily)

		var resp settingsConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, DEFAULT_FONT_FAMILY, resp.Terminal.FontFamily)

		data, err := os.ReadFile(ts.ConfigFile)
		require.NoError(t, err)
		assert.Contains(t, string(data), DEFAULT_FONT_FAMILY)
	})

	t.Run("POST persists settings to config file", func(t *testing.T) {
		ts := newSettingsTestServer()
		dir := t.TempDir()
		ts.ConfigFile = filepath.Join(dir, "conf.yaml")
		require.NoError(t, os.WriteFile(ts.ConfigFile, []byte("theme: b3tty-dark\n"), 0644))

		req := httptest.NewRequest(http.MethodPost, "/settings",
			makeBodyWithShowMenubar(8080, 16, 25, 90, "Fira Code", "visible", true, false, true))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		ts.settingsHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		data, err := os.ReadFile(ts.ConfigFile)
		require.NoError(t, err)
		content := string(data)
		assert.Contains(t, content, "font-size: 16")
		assert.Contains(t, content, "Fira Code")
		assert.Contains(t, content, "no-browser: true")
		assert.Contains(t, content, "show-menubar: visible")
	})
}
