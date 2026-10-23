package src

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newManageTestServer returns a test server with a temp HOME holding
// conf.yaml (with the given content) and a plugin store, as b3tty start
// would set it up.
func newManageTestServer(t *testing.T, config string) *TerminalServer {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeInitialConfig(t, config)
	ts := newTestTerminalServer()
	ts.ConfigFile = filepath.Join(home, DOT_CONFIG_PATH, B3TTY_CONFIG_PATH, CONFIG_FILE_NAME)
	store, err := DefaultPluginStore()
	require.NoError(t, err)
	ts.Plugins = store
	ts.InstalledThemes = map[string]InstalledTheme{}
	ts.WSClients = map[*websocket.Conn]*wsClient{}
	return ts
}

// watchBroadcasts registers a WebSocket session with ts, as terminalHandler
// does, and returns a channel of the control messages broadcast to it.
func watchBroadcasts(t *testing.T, ts *TerminalServer) <-chan map[string]any {
	t.Helper()
	registered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		ts.WSClientsMu.Lock()
		ts.WSClients[conn] = &wsClient{conn: conn}
		ts.WSClientsMu.Unlock()
		close(registered)
	}))
	t.Cleanup(server.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	<-registered

	ch := make(chan map[string]any, 16)
	go func() {
		defer close(ch)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg map[string]any
			if json.Unmarshal(data, &msg) == nil {
				ch <- msg
			}
		}
	}()
	return ch
}

// nextBroadcast returns the next message of type typ on ch, failing the test
// when none arrives in time.
func nextBroadcast(t *testing.T, ch <-chan map[string]any, typ string) map[string]any {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case msg, ok := <-ch:
			require.True(t, ok, "connection closed before a %q message", typ)
			if msg["type"] == typ {
				return msg
			}
		case <-timeout:
			require.FailNow(t, "no broadcast", "no %q message", typ)
		}
	}
}

// broadcastThemeNames returns the names in a "themes" broadcast's themes list.
func broadcastThemeNames(msg map[string]any) []string {
	var names []string
	list, _ := msg["themes"].([]any)
	for _, e := range list {
		if entry, ok := e.(map[string]any); ok {
			names = append(names, entry["name"].(string))
		}
	}
	return names
}

// entryNames returns the names of entries, in order.
func entryNames(entries []ThemeEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}

// postJSON calls handler with a POST of body and returns the response.
func postJSON(handler http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

// writePackageFile writes a package to a temp file and returns its path.
func writePackageFile(t *testing.T, data []byte) string {
	t.Helper()
	return writeTempFile(t, "theme.b3theme", data)
}

func importBody(path string, replace bool) string {
	b, _ := json.Marshal(map[string]any{"path": path, "replace": replace})
	return string(b)
}

func TestImportThemeHandler(t *testing.T) {
	t.Run("installs a package without activating it or adding it to the menu", func(t *testing.T) {
		ts := newManageTestServer(t, "available-themes: [dracula]\n")
		ts.AvailableThemes = []string{"dracula"}
		messages := watchBroadcasts(t, ts)
		path := writePackageFile(t, testPackage(t, "midnight", testThemeID, "1.0", true))

		w := postJSON(ts.importThemeHandler, "/import-theme", importBody(path, false))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp importThemeResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "midnight", resp.Name)
		assert.True(t, resp.Theme.HasBackgroundImage)
		assert.Equal(t, []string{"dracula"}, resp.ThemeNames)
		assert.False(t, resp.Upgraded)

		assert.Contains(t, ts.InstalledThemes, "midnight")
		assert.Equal(t, "", ts.ActiveTheme)
		msg := nextBroadcast(t, messages, "themes")
		assert.Contains(t, broadcastThemeNames(msg), "midnight")
		assert.Equal(t, []string{"dracula"}, resp.ThemeNames)
		assert.Contains(t, entryNames(resp.Themes), "midnight")
	})

	t.Run("asks before upgrading, then applies the new version to the active theme", func(t *testing.T) {
		ts := newManageTestServer(t, "theme: midnight\n")
		messages := watchBroadcasts(t, ts)
		w := postJSON(ts.importThemeHandler, "/import-theme", importBody(writePackageFile(t, testPackage(t, "midnight", testThemeID, "1.0", false)), false))
		require.Equal(t, http.StatusOK, w.Code)
		ts.ActiveTheme = "midnight"

		v2 := writePackageFile(t, testPackage(t, "midnight", testThemeID, "2.0", false))
		w = postJSON(ts.importThemeHandler, "/import-theme", importBody(v2, false))
		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), "this package is version 2.0")

		w = postJSON(ts.importThemeHandler, "/import-theme", importBody(v2, true))
		require.Equal(t, http.StatusOK, w.Code)
		var resp importThemeResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.True(t, resp.Upgraded)
		assert.Equal(t, "2.0", ts.InstalledThemes["midnight"].Manifest.ThemeVersion)
		assert.Equal(t, "midnight", nextBroadcast(t, messages, "theme")["name"])
	})

	t.Run("rejects bad packages and paths with the reason", func(t *testing.T) {
		ts := newManageTestServer(t, "themes:\n  mine:\n    foreground: \"#ffffff\"\n")
		ts.Themes["mine"] = Theme{Foreground: "#ffffff"}
		cases := map[string]struct{ path, want string }{
			"user theme name": {writePackageFile(t, testPackage(t, "mine", testThemeID, "1.0", false)), "rename your theme"},
			"not a package":   {writePackageFile(t, []byte("hello")), "not a valid .b3theme package"},
			"relative path":   {"theme.b3theme", "must be an absolute path"},
			"missing file":    {filepath.Join(t.TempDir(), "nope.b3theme"), "no such file"},
		}
		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				w := postJSON(ts.importThemeHandler, "/import-theme", importBody(tc.path, false))
				assert.Equal(t, http.StatusBadRequest, w.Code)
				assert.Contains(t, w.Body.String(), tc.want)
			})
		}
		assert.Empty(t, ts.InstalledThemes)
	})

	t.Run("requires a same-origin POST with a path", func(t *testing.T) {
		ts := newManageTestServer(t, "")
		req := httptest.NewRequest(http.MethodPost, "/import-theme", strings.NewReader(`{"path":"/x"}`))
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		w := httptest.NewRecorder()
		ts.importThemeHandler(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)

		w = httptest.NewRecorder()
		ts.importThemeHandler(w, httptest.NewRequest(http.MethodGet, "/import-theme", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

		assert.Equal(t, http.StatusBadRequest, postJSON(ts.importThemeHandler, "/import-theme", `{}`).Code)
	})
}

func TestCopyThemeHandler(t *testing.T) {
	copyBody := func(name, newName string) string {
		b, _ := json.Marshal(map[string]string{"name": name, "newName": newName})
		return string(b)
	}

	t.Run("copies an installed theme, extracting its image", func(t *testing.T) {
		ts := newManageTestServer(t, "theme: dracula\navailable-themes: [dracula]\n")
		ts.ActiveTheme = "dracula"
		ts.AvailableThemes = []string{"dracula"}
		ts.InstalledThemes["midnight"] = installTestTheme(t, ts.Plugins, "midnight", testThemeID, true)

		w := postJSON(ts.copyThemeHandler, "/copy-theme", copyBody("midnight", "my midnight"))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp themeConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, []string{"dracula", "my midnight"}, resp.ThemeNames)
		assert.Contains(t, resp.Themes, ThemeEntry{Name: "my midnight", Source: ThemeSourceUser, InMenu: true})
		assert.True(t, resp.HasBackgroundImage)

		home, _ := os.UserHomeDir()
		image := filepath.Join(home, ".config", "b3tty", "backgrounds", "my midnight.png")
		assert.Equal(t, image, ts.Themes["my midnight"].BackgroundImage)
		data, err := os.ReadFile(image)
		require.NoError(t, err)
		assert.Equal(t, pngMagic, data)
		assert.Nil(t, ts.Themes["my midnight"].Package)
		assert.Equal(t, "dracula", ts.ActiveTheme, "a copy is not activated")

		out := readTestConfig(t)
		assert.Equal(t, "dracula", out["theme"])
		entry := out["themes"].(map[string]any)["my midnight"].(map[string]any)
		assert.Equal(t, image, entry["background-image"])
		assert.Equal(t, "#ffffff", entry["foreground"])
	})

	t.Run("never overwrites an existing image file", func(t *testing.T) {
		ts := newManageTestServer(t, "")
		ts.InstalledThemes["midnight"] = installTestTheme(t, ts.Plugins, "midnight", testThemeID, true)
		home, _ := os.UserHomeDir()
		dir := filepath.Join(home, ".config", "b3tty", "backgrounds")
		require.NoError(t, os.MkdirAll(dir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "copy.png"), []byte("mine"), 0600))

		w := postJSON(ts.copyThemeHandler, "/copy-theme", copyBody("midnight", "copy"))
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, filepath.Join(dir, "copy-2.png"), ts.Themes["copy"].BackgroundImage)
		kept, _ := os.ReadFile(filepath.Join(dir, "copy.png"))
		assert.Equal(t, "mine", string(kept))
	})

	t.Run("copies a built-in theme and drops a user theme's package details", func(t *testing.T) {
		ts := newManageTestServer(t, "")
		w := postJSON(ts.copyThemeHandler, "/copy-theme", copyBody("dracula", "my-dracula"))
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, builtinThemes["dracula"].Background, ts.Themes["my-dracula"].Background)

		details := testPackageDetails
		ts.Themes["shared"] = Theme{Foreground: "#ffffff", Package: &details}
		w = postJSON(ts.copyThemeHandler, "/copy-theme", copyBody("shared", "shared-copy"))
		require.Equal(t, http.StatusOK, w.Code)
		assert.Nil(t, ts.Themes["shared-copy"].Package)
		assert.NotContains(t, readTestConfig(t)["themes"].(map[string]any)["shared-copy"], "package")
	})

	t.Run("rejects unknown sources and taken or invalid names", func(t *testing.T) {
		ts := newManageTestServer(t, "")
		assert.Equal(t, http.StatusNotFound, postJSON(ts.copyThemeHandler, "/copy-theme", copyBody("nope", "x")).Code)
		w := postJSON(ts.copyThemeHandler, "/copy-theme", copyBody("dracula", "one-light"))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "already exists")
		w = postJSON(ts.copyThemeHandler, "/copy-theme", copyBody("dracula", " spaced"))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "leading or trailing spaces")
	})
}

func TestDeleteThemeHandler(t *testing.T) {
	deleteBody := func(name string) string { return `{"name":"` + name + `"}` }

	t.Run("deleting the active user theme falls back to the default theme", func(t *testing.T) {
		ts := newManageTestServer(t, `theme: mine
default-theme: one-light
available-themes: [one-light, mine]
themes:
  mine:
    foreground: "#ffffff"
`)
		ts.Themes["mine"] = Theme{Foreground: "#ffffff"}
		ts.AvailableThemes = []string{"one-light", "mine"}
		ts.ActiveTheme, ts.Theme, ts.DefaultTheme = "mine", ts.Themes["mine"], "one-light"
		messages := watchBroadcasts(t, ts)

		w := postJSON(ts.deleteThemeHandler, "/delete-theme", deleteBody("mine"))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp themeNamesResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, []string{"one-light"}, resp.ThemeNames)
		assert.NotContains(t, entryNames(resp.Themes), "mine")
		assert.NotContains(t, ts.Themes, "mine")
		assert.Equal(t, "one-light", ts.ActiveTheme)
		assert.Equal(t, "one-light", nextBroadcast(t, messages, "theme")["name"])
		assert.NotContains(t, broadcastThemeNames(nextBroadcast(t, messages, "themes")), "mine")

		out := readTestConfig(t)
		assert.NotContains(t, out, "theme")
		assert.NotContains(t, out["themes"], "mine")
		assert.Equal(t, "one-light", out["default-theme"])
	})

	t.Run("uninstalls an installed theme", func(t *testing.T) {
		ts := newManageTestServer(t, "default-theme: midnight\navailable-themes: [midnight]\n")
		result, err := ts.Plugins.Install(testPackage(t, "midnight", testThemeID, "1.0", false), noUserThemes, false)
		require.NoError(t, err)
		ts.InstalledThemes["midnight"] = result.Theme
		ts.AvailableThemes = []string{"midnight"}
		ts.DefaultTheme = "midnight"

		w := postJSON(ts.deleteThemeHandler, "/delete-theme", deleteBody("midnight"))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Empty(t, ts.InstalledThemes)
		assert.Empty(t, ts.AvailableThemes)
		assert.Equal(t, "", ts.DefaultTheme)
		entries, err := ts.Plugins.ThemeEntries()
		require.NoError(t, err)
		assert.Empty(t, entries)
		assert.NotContains(t, readTestConfig(t), "default-theme")
	})

	t.Run("a built-in theme is only removed from the menu", func(t *testing.T) {
		ts := newManageTestServer(t, "theme: dracula\navailable-themes: [dracula]\n")
		ts.ActiveTheme, ts.Theme = "dracula", builtinThemes["dracula"]
		ts.AvailableThemes = []string{"dracula"}

		w := postJSON(ts.deleteThemeHandler, "/delete-theme", deleteBody("dracula"))
		require.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, ts.AvailableThemes)
		assert.Equal(t, "dracula", ts.ActiveTheme)
		assert.Equal(t, "dracula", readTestConfig(t)["theme"])

		w = postJSON(ts.deleteThemeHandler, "/delete-theme", deleteBody("dracula"))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "isn't in the Themes menu")
	})

	t.Run("an unknown theme is 404", func(t *testing.T) {
		ts := newManageTestServer(t, "")
		assert.Equal(t, http.StatusNotFound, postJSON(ts.deleteThemeHandler, "/delete-theme", deleteBody("nope")).Code)
	})
}

func TestReloadThemesHandler(t *testing.T) {
	t.Run("picks up themes changed on disk and applies the active one", func(t *testing.T) {
		ts := newManageTestServer(t, "theme: dracula\n")
		ts.ActiveTheme, ts.Theme = "dracula", builtinThemes["dracula"]
		messages := watchBroadcasts(t, ts)

		// What `b3tty theme import` and `b3tty theme set` do from another process.
		_, err := ts.Plugins.Install(testPackage(t, "midnight", testThemeID, "1.0", true), noUserThemes, false)
		require.NoError(t, err)
		require.NoError(t, ActivateThemeInConfig(ts.ConfigFile, "midnight"))

		w := postJSON(ts.reloadThemesHandler, "/reload-themes", "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp themeNamesResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, []string{"midnight"}, resp.ThemeNames)
		assert.Contains(t, resp.Themes, ThemeEntry{
			Name: "midnight", Source: ThemeSourceInstalled, InMenu: true,
			Author: "Chris", ThemeVersion: "1.0", Repo: "https://github.com/example/themes",
		})
		assert.Contains(t, ts.InstalledThemes, "midnight")
		assert.Equal(t, "midnight", ts.ActiveTheme)

		msg := nextBroadcast(t, messages, "theme")
		assert.Equal(t, "midnight", msg["name"])
		assert.Equal(t, true, msg["theme"].(map[string]any)["hasBackgroundImage"])
		assert.Contains(t, broadcastThemeNames(nextBroadcast(t, messages, "themes")), "midnight")
	})

	t.Run("doesn't re-send an unchanged active theme", func(t *testing.T) {
		ts := newManageTestServer(t, "theme: dracula\n")
		ts.ActiveTheme, ts.Theme = "dracula", builtinThemes["dracula"]
		messages := watchBroadcasts(t, ts)
		require.Equal(t, http.StatusOK, postJSON(ts.reloadThemesHandler, "/reload-themes", "").Code)
		assert.Equal(t, "themes", (<-messages)["type"])
	})

	t.Run("keeps the current state when the config file is broken", func(t *testing.T) {
		ts := newManageTestServer(t, "theme: [\n")
		ts.ActiveTheme = "dracula"
		w := postJSON(ts.reloadThemesHandler, "/reload-themes", "")
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, "dracula", ts.ActiveTheme)
	})
}

func TestInstalledThemesAreReadOnly(t *testing.T) {
	ts := newManageTestServer(t, "")
	ts.InstalledThemes["midnight"] = InstalledTheme{Theme: Theme{Foreground: "#ffffff"}}

	w := postJSON(ts.editThemeHandler, "/edit-theme", `{"name":"midnight","theme":{"foreground":"#000000"}}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "an installed theme and can't be edited")

	w = postJSON(ts.exportThemeHandler, "/export-theme", `{"name":"midnight","path":"/tmp/x"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "can't be exported; copy it")

	rec := httptest.NewRecorder()
	ts.themePackageHandler(rec, httptest.NewRequest(http.MethodGet, "/theme-package?name=midnight", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestBackgroundHandlerInstalledTheme(t *testing.T) {
	ts := newManageTestServer(t, "")
	midnight := installTestTheme(t, ts.Plugins, "midnight", testThemeID, true)
	ts.InstalledThemes["midnight"] = midnight
	ts.InstalledThemes["plain"] = installTestTheme(t, ts.Plugins, "plain", otherThemeID, false)
	get := func(query, ifNoneMatch string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/background?token=test-token-1234"+query, nil)
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		ts.backgroundHandler(w, req)
		return w
	}

	w := get("&theme=midnight", "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, pngMagic, w.Body.Bytes())
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.Equal(t, `"`+midnight.Digest+`"`, w.Header().Get("ETag"))

	ts.ActiveTheme = "midnight"
	assert.Equal(t, http.StatusOK, get("", "").Code, "the active installed theme")

	assert.Equal(t, http.StatusNotFound, get("&theme=plain", "").Code)

	t.Run("answers a current cached copy with 304, without reading the package", func(t *testing.T) {
		w := get("&theme=midnight", `W/"other", "`+midnight.Digest+`"`)
		assert.Equal(t, http.StatusNotModified, w.Code)
		assert.Empty(t, w.Body.Bytes())
	})

	t.Run("refuses a package that changed after it was loaded", func(t *testing.T) {
		later := time.Now().Add(time.Hour)
		require.NoError(t, os.Chtimes(midnight.PackagePath, later, later))
		var w *httptest.ResponseRecorder
		logged := captureLog(func() { w = get("&theme=midnight", "") })
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, logged, "changed since it was loaded")
	})
}

func TestETagMatches(t *testing.T) {
	assert.True(t, etagMatches(`"abc"`, `"abc"`))
	assert.True(t, etagMatches(`"x", W/"abc"`, `"abc"`))
	assert.True(t, etagMatches(`*`, `"abc"`))
	assert.False(t, etagMatches(``, `"abc"`))
	assert.False(t, etagMatches(`"abcd"`, `"abc"`))
}

func TestDisplayTermHandlerInstalledTheme(t *testing.T) {
	ts := newTestTerminalServer()
	ts.InstalledThemes = map[string]InstalledTheme{"midnight": {ImageFile: "assets/background.png"}}
	ts.ActiveTheme = "midnight"
	w := httptest.NewRecorder()
	ts.displayTermHandler(w, httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil))
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"backgroundImage":true`)
	assert.Contains(t, body, `{"name":"midnight","source":"installed","inMenu":false}`)
	assert.NotContains(t, body, "allThemeNames")
	assert.NotContains(t, body, "builtinThemeNames")
}
