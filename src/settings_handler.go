package src

import (
	"encoding/json"
	"net/http"
)

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
		resp := settingsConfigResponse{
			Server: settingsServerConfig{
				Port:      ts.Server.Port,
				NoAuth:    ts.Server.NoAuth,
				NoBrowser: ts.NoBrowser,
			},
			Terminal: settingsTerminalConfig{
				FontFamily:  ts.Client.FontFamily,
				FontSize:    ts.Client.FontSize,
				CursorBlink: ts.Client.CursorBlink,
				Rows:        ts.Client.Rows,
				Columns:     ts.Client.Columns,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			Errorf("settings response error: %v", err)
		}
		return
	}

	// POST
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		Warnf("%s %s: forbidden: cross-origin request from Sec-Fetch-Site %q", r.Method, r.URL.Path, site)
		w.WriteHeader(http.StatusForbidden)
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

	if req.Terminal.FontFamily == "" {
		req.Terminal.FontFamily = DEFAULT_FONT_FAMILY
	}

	ts.Client.FontFamily = req.Terminal.FontFamily
	ts.Client.FontSize = req.Terminal.FontSize
	ts.Client.CursorBlink = req.Terminal.CursorBlink
	ts.Client.Rows = req.Terminal.Rows
	ts.Client.Columns = req.Terminal.Columns

	if err := SaveSettingsToConfig(ts.ConfigFile, req.Server, req.Terminal); err != nil {
		Errorf("settings: failed to save config: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	Debugf("saved settings: server.port=%d noAuth=%v noBrowser=%v terminal.fontSize=%d",
		req.Server.Port, req.Server.NoAuth, req.Server.NoBrowser, req.Terminal.FontSize)

	resp := settingsConfigResponse{
		Server: settingsServerConfig{
			Port:      ts.Server.Port,
			NoAuth:    ts.Server.NoAuth,
			NoBrowser: ts.NoBrowser,
		},
		Terminal: settingsTerminalConfig{
			FontFamily:  ts.Client.FontFamily,
			FontSize:    ts.Client.FontSize,
			CursorBlink: ts.Client.CursorBlink,
			Rows:        ts.Client.Rows,
			Columns:     ts.Client.Columns,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		Errorf("settings response error: %v", err)
	}
}
