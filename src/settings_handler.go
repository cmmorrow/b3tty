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
	ts.broadcastJSON(msg)
}

// broadcastSettings sends a settings control message over every open WebSocket
// session so the browser can apply live-applicable fields (fontFamily, fontSize)
// without a page reload. Failed writes are silently discarded.
func (ts *TerminalServer) broadcastSettings(terminal TerminalClient) {
	msg := struct {
		Type     string         `json:"type"`
		Terminal TerminalClient `json:"terminal"`
	}{
		Type:     "settings",
		Terminal: terminal,
	}
	ts.broadcastJSON(msg)
}

// settingsHandler returns or updates the server and terminal settings.
//
// GET /settings returns the current in-memory state from ts.Client and ts.Server.
// No CSRF check is required since this is a read-only request.
//
// POST /settings accepts a settingsConfigResponse body, applies terminal fields
// (fontFamily, fontSize, rows, columns) to ts.Client in memory, and
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
		ts.StateMu.RLock()
		terminal := *ts.Client
		ts.StateMu.RUnlock()
		writeJSON(w, settingsConfigResponse{
			Server:   NewSettingsServerConfig(ts.Server.Port(), ts.Server.NoAuth, ts.NoBrowser, ts.ShowMenubar),
			Terminal: terminal,
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

	if !ValidateShowMenubar(req.Server.ShowMenubar) {
		Warnf("%s %s: bad request: invalid show-menubar %q", r.Method, r.URL.Path, req.Server.ShowMenubar)
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

	ts.StateMu.Lock()
	*ts.Client = req.Terminal
	ts.StateMu.Unlock()

	ts.broadcastSettings(req.Terminal)

	if err := SaveSettingsToConfig(ts.ConfigFile, req.Server, req.Terminal); err != nil {
		Errorf("settings: failed to save config: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	Debugf("saved settings: server.port=%d noAuth=%v noBrowser=%v showMenubar=%s terminal.fontSize=%d",
		req.Server.Port, req.Server.NoAuth, req.Server.NoBrowser, req.Server.ShowMenubar, req.Terminal.FontSize)

	writeJSON(w, settingsConfigResponse{
		Server: NewSettingsServerConfig(ts.Server.Port(), ts.Server.NoAuth, ts.NoBrowser, ts.ShowMenubar),
		// Echo back req.Terminal directly rather than re-reading ts.Client: we
		// just wrote these exact values under StateMu above, and re-reading
		// would need another lock acquisition for values we already have.
		Terminal: req.Terminal,
	}, "settings")
}

// broadcastThemeLists pushes the theme name lists to every open session, as a
// { type: "themes", themeNames, allThemeNames, builtinThemeNames } message, so
// the Themes menu and theme dialogs pick up themes added or removed elsewhere
// (another tab, or the b3tty theme CLI through POST /reload-themes). The lists
// have the same meaning as in the page config (see displayTermHandler).
func (ts *TerminalServer) broadcastThemeLists() {
	ts.StateMu.RLock()
	msg := struct {
		Type              string   `json:"type"`
		ThemeNames        []string `json:"themeNames"`
		AllThemeNames     []string `json:"allThemeNames"`
		BuiltinThemeNames []string `json:"builtinThemeNames"`
	}{
		Type:              "themes",
		ThemeNames:        ts.menuThemeNames(),
		AllThemeNames:     ts.allThemeNames(),
		BuiltinThemeNames: ts.readOnlyThemeNames(),
	}
	ts.StateMu.RUnlock()
	ts.broadcastJSON(msg)
}

// broadcastJSON sends msg, encoded as JSON, as a text message to every open
// WebSocket session. The client list is copied under WSClientsMu and written
// to outside it, each write holding that client's own mutex.
func (ts *TerminalServer) broadcastJSON(msg any) {
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
