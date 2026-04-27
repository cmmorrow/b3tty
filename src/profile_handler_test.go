package src

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// profileConfigHandler
// ---------------------------------------------------------------------------

func TestProfileConfigHandler(t *testing.T) {
	newTS := func() *TerminalServer {
		ts := newTestTerminalServer()
		ts.Profiles["dev"] = Profile{Shell: "/bin/bash", WorkingDirectory: "~/dev", Title: "Dev", Root: "/", Commands: []string{"echo ready"}}
		return ts
	}

	t.Run("POST is rejected with 405", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodPost, "/profile-config?name=dev", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.profileConfigHandler(w, req) })
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Contains(t, logged, "method not allowed")
	})

	t.Run("GET with missing name returns 400", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodGet, "/profile-config", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.profileConfigHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "invalid name")
	})

	t.Run("GET with 'default' name returns 400", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodGet, "/profile-config?name=default", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.profileConfigHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "invalid name")
	})

	t.Run("GET with unknown name returns 404", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodGet, "/profile-config?name=nonexistent", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.profileConfigHandler(w, req) })
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, logged, "not found")
	})

	t.Run("GET with valid name returns 200 with application/json", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodGet, "/profile-config?name=dev", nil)
		w := httptest.NewRecorder()
		ts.profileConfigHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	})

	t.Run("GET returns correct profile fields", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodGet, "/profile-config?name=dev", nil)
		w := httptest.NewRecorder()
		ts.profileConfigHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp profileConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "/bin/bash", resp.Shell)
		assert.Equal(t, "~/dev", resp.WorkingDirectory)
		assert.Equal(t, "Dev", resp.Title)
		assert.Equal(t, "/", resp.Root)
		assert.Equal(t, []string{"echo ready"}, resp.Commands)
	})

	t.Run("GET profile with nil commands returns empty slice", func(t *testing.T) {
		ts := newTS()
		ts.Profiles["nocommands"] = Profile{Shell: "/bin/zsh"}
		req := httptest.NewRequest(http.MethodGet, "/profile-config?name=nocommands", nil)
		w := httptest.NewRecorder()
		ts.profileConfigHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp profileConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, []string{}, resp.Commands)
	})
}

// ---------------------------------------------------------------------------
// editProfileHandler
// ---------------------------------------------------------------------------

func TestEditProfileHandler(t *testing.T) {
	newTS := func() *TerminalServer {
		ts := newTestTerminalServer()
		ts.ConfigFile = writeTempConfig(t, "")
		return ts
	}

	encodeBody := func(name string, shell, title, wd, root string, commands []string) []byte {
		body, _ := json.Marshal(map[string]any{
			"name": name,
			"profile": map[string]any{
				"shell":            shell,
				"title":            title,
				"workingDirectory": wd,
				"root":             root,
				"commands":         commands,
			},
		})
		return body
	}

	t.Run("GET is rejected with 405", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodGet, "/edit-profile", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.editProfileHandler(w, req) })
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Contains(t, logged, "method not allowed")
	})

	t.Run("cross-origin request is rejected with 403", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodPost, "/edit-profile", bytes.NewReader(encodeBody("myprofile", "", "", "", "", nil)))
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.editProfileHandler(w, req) })
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, logged, "cross-origin")
	})

	t.Run("missing name returns 400", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodPost, "/edit-profile", bytes.NewReader(encodeBody("", "", "", "", "", nil)))
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.editProfileHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "invalid name")
	})

	t.Run("'default' name returns 400", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodPost, "/edit-profile", bytes.NewReader(encodeBody("default", "", "", "", "", nil)))
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.editProfileHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "invalid name")
	})

	t.Run("valid POST saves profile to ts.Profiles", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodPost, "/edit-profile", bytes.NewReader(encodeBody("myprofile", "/bin/zsh", "My Shell", "~/projects", "/", []string{"npm start"})))
		w := httptest.NewRecorder()
		ts.editProfileHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		p, ok := ts.Profiles["myprofile"]
		require.True(t, ok)
		assert.Equal(t, "/bin/zsh", p.Shell)
		assert.Equal(t, "My Shell", p.Title)
		assert.Equal(t, "~/projects", p.WorkingDirectory)
		assert.Equal(t, "/", p.Root)
		assert.Equal(t, []string{"npm start"}, p.Commands)
	})

	t.Run("empty shell defaults to DEFAULT_SHELL", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodPost, "/edit-profile", bytes.NewReader(encodeBody("myprofile", "", "", "", "", nil)))
		w := httptest.NewRecorder()
		ts.editProfileHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		p := ts.Profiles["myprofile"]
		assert.Equal(t, DEFAULT_SHELL, p.Shell)
	})

	t.Run("empty command lines are filtered out", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodPost, "/edit-profile", bytes.NewReader(encodeBody("myprofile", "", "", "", "", []string{"echo hello", "", "npm start", ""})))
		w := httptest.NewRecorder()
		ts.editProfileHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		p := ts.Profiles["myprofile"]
		assert.Equal(t, []string{"echo hello", "npm start"}, p.Commands)
	})

	t.Run("response contains sorted non-default profileNames", func(t *testing.T) {
		ts := newTS()
		ts.Profiles["zebra"] = Profile{Shell: "/bin/sh"}
		req := httptest.NewRequest(http.MethodPost, "/edit-profile", bytes.NewReader(encodeBody("alpha", "", "", "", "", nil)))
		w := httptest.NewRecorder()
		ts.editProfileHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp editProfileResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, []string{"alpha", "work", "zebra"}, resp.ProfileNames)
	})

	t.Run("response does not include 'default' in profileNames", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodPost, "/edit-profile", bytes.NewReader(encodeBody("newprof", "", "", "", "", nil)))
		w := httptest.NewRecorder()
		ts.editProfileHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp editProfileResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotContains(t, resp.ProfileNames, "default")
	})

	t.Run("same-origin Sec-Fetch-Site is allowed", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodPost, "/edit-profile", bytes.NewReader(encodeBody("myprofile", "", "", "", "", nil)))
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		ts.editProfileHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ---------------------------------------------------------------------------
// deleteProfileHandler
// ---------------------------------------------------------------------------

func TestDeleteProfileHandler(t *testing.T) {
	newTS := func() *TerminalServer {
		ts := newTestTerminalServer()
		ts.ConfigFile = writeTempConfig(t, "")
		return ts
	}

	t.Run("GET is rejected with 405", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodGet, "/delete-profile", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.deleteProfileHandler(w, req) })
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Contains(t, logged, "method not allowed")
	})

	t.Run("cross-origin request is rejected with 403", func(t *testing.T) {
		ts := newTS()
		body, _ := json.Marshal(map[string]string{"name": "work"})
		req := httptest.NewRequest(http.MethodPost, "/delete-profile", bytes.NewReader(body))
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.deleteProfileHandler(w, req) })
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, logged, "cross-origin")
	})

	t.Run("missing name returns 400", func(t *testing.T) {
		ts := newTS()
		body, _ := json.Marshal(map[string]string{"name": ""})
		req := httptest.NewRequest(http.MethodPost, "/delete-profile", bytes.NewReader(body))
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.deleteProfileHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "invalid name")
	})

	t.Run("'default' name returns 400", func(t *testing.T) {
		ts := newTS()
		body, _ := json.Marshal(map[string]string{"name": "default"})
		req := httptest.NewRequest(http.MethodPost, "/delete-profile", bytes.NewReader(body))
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.deleteProfileHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "invalid name")
	})

	t.Run("valid POST removes profile from ts.Profiles", func(t *testing.T) {
		ts := newTS()
		require.Contains(t, ts.Profiles, "work")
		body, _ := json.Marshal(map[string]string{"name": "work"})
		req := httptest.NewRequest(http.MethodPost, "/delete-profile", bytes.NewReader(body))
		w := httptest.NewRecorder()
		ts.deleteProfileHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, ts.Profiles, "work")
	})

	t.Run("response contains updated sorted profileNames", func(t *testing.T) {
		ts := newTS()
		ts.Profiles["alpha"] = Profile{Shell: "/bin/sh"}
		body, _ := json.Marshal(map[string]string{"name": "work"})
		req := httptest.NewRequest(http.MethodPost, "/delete-profile", bytes.NewReader(body))
		w := httptest.NewRecorder()
		ts.deleteProfileHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp editProfileResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, []string{"alpha"}, resp.ProfileNames)
	})

	t.Run("deleting non-existent profile is a no-op returning 200", func(t *testing.T) {
		ts := newTS()
		body, _ := json.Marshal(map[string]string{"name": "nonexistent"})
		req := httptest.NewRequest(http.MethodPost, "/delete-profile", bytes.NewReader(body))
		w := httptest.NewRecorder()
		ts.deleteProfileHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("'default' profile is not removed when deleting non-default", func(t *testing.T) {
		ts := newTS()
		body, _ := json.Marshal(map[string]string{"name": "work"})
		req := httptest.NewRequest(http.MethodPost, "/delete-profile", bytes.NewReader(body))
		w := httptest.NewRecorder()
		ts.deleteProfileHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, ts.Profiles, "default")
	})
}
