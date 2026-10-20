package src

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const otherThemeID = "0b6f9a2e-8c1d-4e57-9f3a-2d4c6e8a1b30"

func newTestStore(t *testing.T) *PluginStore {
	t.Helper()
	return &PluginStore{Dir: filepath.Join(t.TempDir(), "plugins")}
}

func noUserThemes(string) bool { return false }

func TestPluginStoreInstall(t *testing.T) {
	t.Run("stores the package unchanged and registers it", func(t *testing.T) {
		store := newTestStore(t)
		data := testPackage(t, "midnight", testThemeID, "1.0", true)
		result, err := store.Install(data, noUserThemes, false)
		require.NoError(t, err)
		assert.False(t, result.Upgraded)
		assert.False(t, result.Unchanged)
		assert.Equal(t, "midnight", result.Theme.Manifest.Name)

		digest := sha256Hex(data)
		stored, err := os.ReadFile(filepath.Join(store.Dir, "themes", digest, "package.b3theme"))
		require.NoError(t, err)
		assert.Equal(t, data, stored)

		entries, err := store.ThemeEntries()
		require.NoError(t, err)
		require.Len(t, entries, 1)
		e := entries[0]
		assert.Equal(t, "theme", e.Kind)
		assert.Equal(t, testThemeID, e.ID)
		assert.Equal(t, "midnight", e.Name)
		assert.Equal(t, "1.0", e.ThemeVersion)
		assert.Equal(t, "sha256:"+digest, e.Digest)
		assert.True(t, e.Enabled)
		assert.NotEmpty(t, e.InstalledAt)
	})

	t.Run("reinstalling the same package does nothing", func(t *testing.T) {
		store := newTestStore(t)
		data := testPackage(t, "midnight", testThemeID, "1.0", false)
		_, err := store.Install(data, noUserThemes, false)
		require.NoError(t, err)
		result, err := store.Install(data, noUserThemes, false)
		require.NoError(t, err)
		assert.True(t, result.Unchanged)
		entries, _ := store.ThemeEntries()
		assert.Len(t, entries, 1)
	})

	t.Run("rejects name clashes", func(t *testing.T) {
		store := newTestStore(t)
		_, err := store.Install(testPackage(t, "dracula", testThemeID, "1.0", false), noUserThemes, false)
		require.ErrorContains(t, err, "built-in theme")

		_, err = store.Install(testPackage(t, "mine", testThemeID, "1.0", false), func(n string) bool { return n == "mine" }, false)
		require.ErrorContains(t, err, "rename your theme")

		_, err = store.Install(testPackage(t, "midnight", testThemeID, "1.0", false), noUserThemes, false)
		require.NoError(t, err)
		_, err = store.Install(testPackage(t, "midnight", otherThemeID, "1.0", false), noUserThemes, true)
		require.ErrorContains(t, err, `a different theme named "midnight"`)
	})

	t.Run("a new version needs approval, then replaces the old one", func(t *testing.T) {
		store := newTestStore(t)
		v1 := testPackage(t, "midnight", testThemeID, "1.0", false)
		_, err := store.Install(v1, noUserThemes, false)
		require.NoError(t, err)

		v2 := testPackage(t, "midnight", testThemeID, "2.0", false)
		_, err = store.Install(v2, noUserThemes, false)
		var upgrade *UpgradeRequiredError
		require.True(t, errors.As(err, &upgrade))
		assert.Contains(t, err.Error(), "midnight 1.0 is installed; this package is version 2.0")
		assert.NotContains(t, err.Error(), "claims to be")

		result, err := store.Install(v2, noUserThemes, true)
		require.NoError(t, err)
		assert.True(t, result.Upgraded)
		entries, _ := store.ThemeEntries()
		require.Len(t, entries, 1)
		assert.Equal(t, "2.0", entries[0].ThemeVersion)
		assert.NoDirExists(t, filepath.Join(store.Dir, "themes", sha256Hex(v1)))
		assert.FileExists(t, store.packagePath(sha256Hex(v2)))
	})

	t.Run("warns when a new version has another author or repository", func(t *testing.T) {
		store := newTestStore(t)
		_, err := store.Install(testPackage(t, "midnight", testThemeID, "1.0", false), noUserThemes, false)
		require.NoError(t, err)

		details := testPackageDetails
		details.ThemeVersion = "2.0"
		details.Author = "Mallory"
		details.Repo = "https://github.com/mallory/themes"
		data, err := buildThemePackage("midnight", Theme{}, details, nil, "", "", time.Now())
		require.NoError(t, err)
		_, err = store.Install(data, noUserThemes, false)
		require.ErrorContains(t, err, "This claims to be a new version of midnight by Chris, but it's by Mallory")
		require.ErrorContains(t, err, "It comes from https://github.com/mallory/themes, not https://github.com/example/themes")
	})

	t.Run("an upgrade can't rename a theme", func(t *testing.T) {
		store := newTestStore(t)
		_, err := store.Install(testPackage(t, "midnight", testThemeID, "1.0", false), noUserThemes, false)
		require.NoError(t, err)
		_, err = store.Install(testPackage(t, "midnight-2", testThemeID, "2.0", false), noUserThemes, true)
		require.ErrorContains(t, err, "an upgrade can't rename a theme")
		entries, _ := store.ThemeEntries()
		assert.Equal(t, "midnight", entries[0].Name)
	})

	t.Run("rejects an invalid package without writing anything", func(t *testing.T) {
		store := newTestStore(t)
		_, err := store.Install([]byte("not a zip"), noUserThemes, false)
		require.Error(t, err)
		assert.NoDirExists(t, store.Dir)
	})
}

func TestPluginStoreLoadThemes(t *testing.T) {
	t.Run("a nil store has nothing installed", func(t *testing.T) {
		var store *PluginStore
		assert.Empty(t, store.LoadThemes(true))
	})

	t.Run("loads installed themes with their images", func(t *testing.T) {
		store := newTestStore(t)
		_, err := store.Install(testPackage(t, "midnight", testThemeID, "1.0", true), noUserThemes, false)
		require.NoError(t, err)
		var themes map[string]InstalledTheme
		logged := captureLog(func() { themes = store.LoadThemes(true) })
		assert.Empty(t, logged)
		require.Contains(t, themes, "midnight")
		assert.Equal(t, "#ffffff", themes["midnight"].Theme.Foreground)
		assert.Equal(t, "assets/background.png", themes["midnight"].ImageFile)
		assert.False(t, themes["midnight"].Modified)
		image, err := themes["midnight"].ReadImage()
		require.NoError(t, err)
		assert.Equal(t, pngMagic, image)
	})

	t.Run("flags a modified package but still loads it", func(t *testing.T) {
		store := newTestStore(t)
		data := testPackage(t, "midnight", testThemeID, "1.0", false)
		_, err := store.Install(data, noUserThemes, false)
		require.NoError(t, err)
		// A different but valid package in the same place.
		other := testPackage(t, "midnight", testThemeID, "9.9", false)
		require.NoError(t, os.WriteFile(store.packagePath(sha256Hex(data)), other, 0644))

		var themes map[string]InstalledTheme
		logged := captureLog(func() { themes = store.LoadThemes(true) })
		assert.Contains(t, logged, `installed theme "midnight" has been modified`)
		assert.True(t, themes["midnight"].Modified)
	})

	t.Run("skips a missing or broken package with a warning", func(t *testing.T) {
		store := newTestStore(t)
		a := testPackage(t, "aaa", testThemeID, "1.0", false)
		b := testPackage(t, "bbb", otherThemeID, "1.0", false)
		_, err := store.Install(a, noUserThemes, false)
		require.NoError(t, err)
		_, err = store.Install(b, noUserThemes, false)
		require.NoError(t, err)
		require.NoError(t, os.RemoveAll(filepath.Dir(store.packagePath(sha256Hex(a)))))
		require.NoError(t, os.WriteFile(store.packagePath(sha256Hex(b)), []byte("junk"), 0644))

		var themes map[string]InstalledTheme
		logged := captureLog(func() { themes = store.LoadThemes(true) })
		assert.Empty(t, themes)
		assert.Contains(t, logged, `installed theme "aaa" is skipped`)
		assert.Contains(t, logged, `installed theme "bbb" is skipped`)
	})

	t.Run("checks the image while loading, without keeping it", func(t *testing.T) {
		store := newTestStore(t)
		data := zipOf(t,
			zipEntry{"manifest.json", testManifest(t, func(m map[string]any) {
				m["files"] = map[string]any{"themeFile": "theme.json", "backgroundImage": "assets/background.png"}
			})},
			zipEntry{"theme.json", []byte(`{}`)},
			zipEntry{"assets/background.png", []byte("<html>")},
		)
		digest := sha256Hex(data)
		require.NoError(t, store.writePackage(digest, data))
		require.NoError(t, store.writeRegistry(PluginRegistry{SchemaVersion: 1, Plugins: []PluginEntry{
			{Kind: "theme", Name: "midnight", Digest: "sha256:" + digest},
		}}))
		var themes map[string]InstalledTheme
		logged := captureLog(func() { themes = store.LoadThemes(true) })
		assert.Empty(t, themes)
		assert.Contains(t, logged, "unsupported background image type")
	})

	t.Run("without verify, skips hashing and the image", func(t *testing.T) {
		store := newTestStore(t)
		data := testPackage(t, "midnight", testThemeID, "1.0", true)
		_, err := store.Install(data, noUserThemes, false)
		require.NoError(t, err)
		// A different but valid package in the same place.
		other := testPackage(t, "midnight", testThemeID, "9.9", true)
		require.NoError(t, os.WriteFile(store.packagePath(sha256Hex(data)), other, 0644))

		var themes map[string]InstalledTheme
		logged := captureLog(func() { themes = store.LoadThemes(false) })
		assert.Empty(t, logged, "no hash, so no modified warning")
		it := themes["midnight"]
		assert.False(t, it.Modified)
		assert.Equal(t, "assets/background.png", it.ImageFile)
		image, err := it.ReadImage()
		require.NoError(t, err)
		assert.Equal(t, pngMagic, image)

		modified, err := it.CheckModified()
		require.NoError(t, err)
		assert.True(t, modified)
	})

	t.Run("without verify, a bad image goes unnoticed until it's read", func(t *testing.T) {
		store := newTestStore(t)
		data := zipOf(t,
			zipEntry{"manifest.json", testManifest(t, func(m map[string]any) {
				m["files"] = map[string]any{"themeFile": "theme.json", "backgroundImage": "assets/background.png"}
			})},
			zipEntry{"theme.json", []byte(`{}`)},
			zipEntry{"assets/background.png", []byte("<html>")},
		)
		digest := sha256Hex(data)
		require.NoError(t, store.writePackage(digest, data))
		require.NoError(t, store.writeRegistry(PluginRegistry{SchemaVersion: 1, Plugins: []PluginEntry{
			{Kind: "theme", Name: "midnight", Digest: "sha256:" + digest},
		}}))
		themes := store.LoadThemes(false)
		require.Contains(t, themes, "midnight")
		_, err := themes["midnight"].ReadImage()
		assert.ErrorIs(t, err, errUnsupportedBackgroundImage)
	})

	t.Run("never reads outside the store for a hand-edited digest", func(t *testing.T) {
		store := newTestStore(t)
		reg := PluginRegistry{SchemaVersion: 1, Plugins: []PluginEntry{{Kind: "theme", Name: "evil", Digest: "sha256:../../etc"}}}
		require.NoError(t, store.writeRegistry(reg))
		var themes map[string]InstalledTheme
		logged := captureLog(func() { themes = store.LoadThemes(true) })
		assert.Empty(t, themes)
		assert.Contains(t, logged, "invalid digest")
	})

	t.Run("ignores other plugin kinds and unregistered directories", func(t *testing.T) {
		store := newTestStore(t)
		data := testPackage(t, "midnight", testThemeID, "1.0", false)
		require.NoError(t, store.writePackage(sha256Hex(data), data))
		reg := PluginRegistry{SchemaVersion: 1, Plugins: []PluginEntry{{Kind: "font", Name: "x", Digest: "sha256:" + sha256Hex(data)}}}
		require.NoError(t, store.writeRegistry(reg))
		assert.Empty(t, store.LoadThemes(true))
	})

	t.Run("an unreadable registry loads nothing, with a warning", func(t *testing.T) {
		store := newTestStore(t)
		require.NoError(t, os.MkdirAll(store.Dir, 0755))
		require.NoError(t, os.WriteFile(store.registryPath(), []byte("{"), 0644))
		var themes map[string]InstalledTheme
		logged := captureLog(func() { themes = store.LoadThemes(true) })
		assert.Empty(t, themes)
		assert.Contains(t, logged, "installed themes are not loaded")
	})
}

func TestPluginStoreUninstallVerifyGC(t *testing.T) {
	store := newTestStore(t)
	a := testPackage(t, "aaa", testThemeID, "1.0", false)
	b := testPackage(t, "bbb", otherThemeID, "1.0", false)
	for _, data := range [][]byte{a, b} {
		_, err := store.Install(data, noUserThemes, false)
		require.NoError(t, err)
	}

	report, err := store.Verify()
	require.NoError(t, err)
	assert.True(t, report.OK())
	assert.Equal(t, 2, report.Checked)

	// Modify a, leave an orphan directory and a stray temp file behind.
	require.NoError(t, os.WriteFile(store.packagePath(sha256Hex(a)), testPackage(t, "aaa", testThemeID, "1.1", false), 0644))
	orphan := filepath.Join(store.themesDir(), "0000")
	require.NoError(t, os.MkdirAll(orphan, 0755))
	stray := filepath.Join(store.themesDir(), ".package-123.tmp")
	require.NoError(t, os.WriteFile(stray, nil, 0644))

	report, err = store.Verify()
	require.NoError(t, err)
	assert.False(t, report.OK())
	assert.Equal(t, []string{"aaa"}, report.Modified)
	assert.ElementsMatch(t, []string{orphan, stray}, report.Orphans)

	require.NoError(t, store.Uninstall("bbb"))
	assert.NoDirExists(t, filepath.Dir(store.packagePath(sha256Hex(b))))
	require.ErrorIs(t, store.Uninstall("bbb"), ErrThemeNotInstalled)

	require.NoError(t, os.RemoveAll(filepath.Dir(store.packagePath(sha256Hex(a)))))
	report, err = store.Verify()
	require.NoError(t, err)
	assert.Equal(t, []string{"aaa"}, report.Missing)

	removed, err := store.GC()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{orphan, stray}, removed)
	assert.NoDirExists(t, orphan)

	var reg PluginRegistry
	data, err := os.ReadFile(store.registryPath())
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &reg))
	require.Len(t, reg.Plugins, 1)
	assert.Equal(t, "aaa", reg.Plugins[0].Name)
}

// installTestTheme installs a test package into store and returns the
// installed theme, as loaded.
func installTestTheme(t *testing.T, store *PluginStore, name, id string, withImage bool) InstalledTheme {
	t.Helper()
	result, err := store.Install(testPackage(t, name, id, "1.0", withImage), noUserThemes, false)
	require.NoError(t, err)
	return result.Theme
}

func TestInstalledThemeReadImage(t *testing.T) {
	store := newTestStore(t)
	it := installTestTheme(t, store, "midnight", testThemeID, true)
	assert.True(t, it.HasImage())
	image, err := it.ReadImage()
	require.NoError(t, err)
	assert.Equal(t, pngMagic, image)
	assert.Equal(t, `"`+it.Digest+`"`, it.ImageETag())

	t.Run("refuses a package that changed after it was loaded", func(t *testing.T) {
		later := time.Now().Add(time.Hour)
		require.NoError(t, os.Chtimes(it.PackagePath, later, later))
		assert.ErrorIs(t, it.CheckUnchanged(), ErrPackageChanged)
		_, err := it.ReadImage()
		assert.ErrorIs(t, err, ErrPackageChanged)
	})

	t.Run("a theme without an image has none to read", func(t *testing.T) {
		plain := installTestTheme(t, store, "plain", otherThemeID, false)
		assert.False(t, plain.HasImage())
		_, err := plain.ReadImage()
		assert.Error(t, err)
	})
}

func TestNormalizeRepo(t *testing.T) {
	same := []string{
		"https://github.com/cmmorrow/themes",
		"https://github.com/cmmorrow/themes/",
		"https://github.com/cmmorrow/themes.git",
		"http://GitHub.com/cmmorrow/themes",
	}
	for _, r := range same {
		assert.Equal(t, "https://github.com/cmmorrow/themes", normalizeRepo(r), r)
	}
	assert.NotEqual(t, normalizeRepo("https://github.com/cmmorrow/themes"), normalizeRepo("https://github.com/CMMorrow/themes"))
}
