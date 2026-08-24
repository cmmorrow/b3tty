package src

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// broadcastTheme sends a theme control message over every open WebSocket session
// so all open browser sessions apply the new theme immediately without a page
// reload. Failed writes are silently discarded.
func (ts *TerminalServer) broadcastTheme(name string, resp themeConfigResponse) {
	msg := struct {
		Type  string              `json:"type"`
		Name  string              `json:"name"`
		Theme themeConfigResponse `json:"theme"`
	}{
		Type:  "theme",
		Name:  name,
		Theme: resp,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	ts.WSClientsMu.Lock()
	clients := make([]*wsClient, 0, len(ts.WSClients))
	for _, c := range ts.WSClients {
		clients = append(clients, c)
	}
	ts.WSClientsMu.Unlock()

	for _, c := range clients {
		c.mu.Lock()
		c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_ = c.conn.WriteMessage(websocket.TextMessage, data)
		c.mu.Unlock()
	}
}

// broadcastSettings sends a settings control message over every open WebSocket
// session so the browser can apply live-applicable fields (fontFamily, fontSize,
// cursorBlink) without a page reload. Failed writes are silently discarded.
func (ts *TerminalServer) broadcastSettings(terminal SettingsTerminalConfig) {
	msg := struct {
		Type     string                `json:"type"`
		Terminal SettingsTerminalConfig `json:"terminal"`
	}{
		Type:     "settings",
		Terminal: terminal,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	ts.WSClientsMu.Lock()
	clients := make([]*wsClient, 0, len(ts.WSClients))
	for _, c := range ts.WSClients {
		clients = append(clients, c)
	}
	ts.WSClientsMu.Unlock()

	for _, c := range clients {
		c.mu.Lock()
		c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_ = c.conn.WriteMessage(websocket.TextMessage, data)
		c.mu.Unlock()
	}
}

// settingsHandler returns or updates the server and terminal settings.
//
// GET /settings returns the current in-memory state from ts.Client and ts.Server.
// No CSRF check is required since this is a read-only request.
//
// POST /settings accepts a settingsConfigResponse body, applies terminal fields
// (fontFamily, fontSize, cursorBlink, rows, columns) to ts.Client in memory, and
// persists all settings to the config file via SaveSettingsToConfig. Server fields
// (port, noAuth, noBrowser) are persisted but NOT applied to the running server —
// they take effect only after a restart. Requires Sec-Fetch-Site: same-origin.
func (ts *TerminalServer) settingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "POST" {
		Warnf("%s %s: method not allowed", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if r.Method == "GET" {
		writeJSON(w, settingsConfigResponse{
			Server: SettingsServerConfig{
				Port:      ts.Server.Port,
				NoAuth:    ts.Server.NoAuth,
				NoBrowser: ts.NoBrowser,
			},
			Terminal: SettingsTerminalConfig{
				FontFamily:  ts.Client.FontFamily,
				FontSize:    ts.Client.FontSize,
				CursorBlink: ts.Client.CursorBlink,
				AutoResize:  ts.Client.AutoResize,
				Rows:        ts.Client.Rows,
				Columns:     ts.Client.Columns,
			},
		}, "settings")
		return
	}

	// POST
	if requireSameOrigin(w, r) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQUEST_BODY_SIZE)
	var req settingsConfigResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Warnf("%s %s: bad request: %v", r.Method, r.URL.Path, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !ValidatePortNumber(req.Server.Port) {
		Warnf("%s %s: bad request: invalid port %d", r.Method, r.URL.Path, req.Server.Port)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if req.Terminal.FontSize < 1 {
		Warnf("%s %s: bad request: invalid font-size %d", r.Method, r.URL.Path, req.Terminal.FontSize)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if req.Terminal.Rows < 10 {
		Warnf("%s %s: bad request: invalid rows %d (minimum 10)", r.Method, r.URL.Path, req.Terminal.Rows)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if req.Terminal.Columns < 10 {
		Warnf("%s %s: bad request: invalid columns %d (minimum 10)", r.Method, r.URL.Path, req.Terminal.Columns)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ts.Client.FontFamily = req.Terminal.FontFamily
	ts.Client.FontSize = req.Terminal.FontSize
	ts.Client.CursorBlink = req.Terminal.CursorBlink
	ts.Client.AutoResize = req.Terminal.AutoResize
	ts.Client.Rows = req.Terminal.Rows
	ts.Client.Columns = req.Terminal.Columns

	ts.broadcastSettings(req.Terminal)

	if err := SaveSettingsToConfig(ts.ConfigFile, req.Server, req.Terminal); err != nil {
		Errorf("settings: failed to save config: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	Debugf("saved settings: server.port=%d noAuth=%v noBrowser=%v terminal.fontSize=%d",
		req.Server.Port, req.Server.NoAuth, req.Server.NoBrowser, req.Terminal.FontSize)

	writeJSON(w, settingsConfigResponse{
		Server: SettingsServerConfig{
			Port:      ts.Server.Port,
			NoAuth:    ts.Server.NoAuth,
			NoBrowser: ts.NoBrowser,
		},
		Terminal: SettingsTerminalConfig{
			FontFamily:  ts.Client.FontFamily,
			FontSize:    ts.Client.FontSize,
			CursorBlink: ts.Client.CursorBlink,
			AutoResize:  ts.Client.AutoResize,
			Rows:        ts.Client.Rows,
			Columns:     ts.Client.Columns,
		},
	}, "settings")
}
