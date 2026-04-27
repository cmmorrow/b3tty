package src

import (
	"encoding/json"
	"net/http"
	"sort"
)

// profileConfigHandler returns the stored config fields for a named profile.
// GET /profile-config?name=<name>
func (ts *TerminalServer) profileConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		Warnf("%s %s: method not allowed", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" || name == DEFAULT_PROFILE_NAME {
		Warnf("%s %s: bad request: invalid name %q", r.Method, r.URL.Path, name)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	p, ok := ts.Profiles[name]
	if !ok {
		Warnf("%s %s: profile %q not found", r.Method, r.URL.Path, name)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	resp := profileConfigResponse{
		Shell:            p.Shell,
		WorkingDirectory: p.WorkingDirectory,
		Title:            p.Title,
		Root:             p.Root,
		Commands:         p.Commands,
	}
	if resp.Commands == nil {
		resp.Commands = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		Errorf("profile-config response error: %v", err)
	}
}

// editProfileHandler creates or overwrites a user-defined profile and persists it.
// POST /edit-profile  body: {"name":"<name>","profile":{...profileConfigResponse fields...}}
// The profile is saved to config but NOT activated.
func (ts *TerminalServer) editProfileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		Warnf("%s %s: method not allowed", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		Warnf("%s %s: forbidden: cross-origin request from Sec-Fetch-Site %q", r.Method, r.URL.Path, site)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQUEST_BODY_SIZE)
	var req struct {
		Name    string                `json:"name"`
		Profile profileConfigResponse `json:"profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Warnf("%s %s: bad request: %v", r.Method, r.URL.Path, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Name == DEFAULT_PROFILE_NAME {
		Warnf("%s %s: bad request: invalid name %q", r.Method, r.URL.Path, req.Name)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Discard empty command lines.
	filtered := make([]string, 0, len(req.Profile.Commands))
	for _, cmd := range req.Profile.Commands {
		if cmd != "" {
			filtered = append(filtered, cmd)
		}
	}

	shell := req.Profile.Shell
	if shell == "" {
		shell = DEFAULT_SHELL
	}

	p := NewProfile(shell, req.Profile.WorkingDirectory, req.Profile.Root, req.Profile.Title, filtered)
	ts.Profiles[req.Name] = p

	if err := SaveProfileToConfig(ts.ConfigFile, req.Name, p); err != nil {
		Errorf("edit-profile: failed to save config: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	Debugf("saved profile %q", req.Name)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(editProfileResponse{ProfileNames: nonDefaultProfileNames(ts.Profiles)}); err != nil {
		Errorf("edit-profile response error: %v", err)
	}
}

// deleteProfileHandler removes a non-default profile from memory and the config file.
// POST /delete-profile  body: {"name":"<name>"}
func (ts *TerminalServer) deleteProfileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		Warnf("%s %s: method not allowed", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		Warnf("%s %s: forbidden: cross-origin request from Sec-Fetch-Site %q", r.Method, r.URL.Path, site)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MAX_REQUEST_BODY_SIZE)
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Warnf("%s %s: bad request: %v", r.Method, r.URL.Path, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Name == DEFAULT_PROFILE_NAME {
		Warnf("%s %s: bad request: invalid name %q", r.Method, r.URL.Path, req.Name)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	delete(ts.Profiles, req.Name)

	if err := DeleteProfileFromConfig(ts.ConfigFile, req.Name); err != nil {
		Errorf("delete-profile: failed to save config: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	Debugf("deleted profile %q", req.Name)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(editProfileResponse{ProfileNames: nonDefaultProfileNames(ts.Profiles)}); err != nil {
		Errorf("delete-profile response error: %v", err)
	}
}

// nonDefaultProfileNames returns a sorted slice of all profile names except "default".
func nonDefaultProfileNames(profiles map[string]Profile) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		if name != DEFAULT_PROFILE_NAME {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
