package src

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readThemePackage opens the zip at path and returns its entries by name.
func readThemePackage(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	require.NoError(t, err)
	defer zr.Close()
	files := make(map[string][]byte)
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		files[f.Name] = data
	}
	return files
}

// testThemeID and testPackageDetails are valid package details for tests that
// aren't about the details themselves.
const testThemeID = "6c0e9d2a-41f7-4c3b-9a55-0b8e2f1d7a10"

var testPackageDetails = ThemePackageDetails{
	ID: testThemeID,
	ThemeMetadata: ThemeMetadata{
		Repo:         "https://github.com/example/themes",
		Author:       "Chris",
		Description:  "Soft pastels.",
		ThemeVersion: "1.0",
	},
}

func TestExportThemePackage(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.FixedZone("EST", -5*3600))
	transparency := 30

	t.Run("packages manifest, theme, and background image", func(t *testing.T) {
		image := writeTempFile(t, "My Photo.PNG", pngMagic)
		theme := Theme{Foreground: "#ffffff", Background: "#000000", BackgroundImage: image, BackgroundImageTransparency: &transparency}
		out := filepath.Join(t.TempDir(), "midnight.b3theme")

		got, err := ExportThemePackage("midnight", theme, testPackageDetails, out, "1.2.3", now)
		require.NoError(t, err)
		assert.Equal(t, out, got)

		files := readThemePackage(t, got)
		assert.Len(t, files, 3)

		var manifest ThemeManifest
		require.NoError(t, json.Unmarshal(files["manifest.json"], &manifest))
		assert.Equal(t, ThemeManifest{
			SchemaVersion: 1,
			Kind:          "theme",
			ID:            testThemeID,
			Name:          "midnight",
			ThemeMetadata: testPackageDetails.ThemeMetadata,
			B3ttyVersion:  "1.2.3",
			CreatedAt:     "2026-10-04T17:00:00Z",
			Files:         ThemeManifestFiles{ThemeFile: "theme.json", BackgroundImage: "assets/background.png"},
		}, manifest)

		var decoded Theme
		require.NoError(t, json.Unmarshal(files["theme.json"], &decoded))
		assert.Equal(t, "#ffffff", decoded.Foreground)
		assert.Equal(t, "#000000", decoded.Background)
		assert.Empty(t, decoded.BackgroundImage, "the exporter's local path must not be packaged")
		require.NotNil(t, decoded.BackgroundImageTransparency)
		assert.Equal(t, 30, *decoded.BackgroundImageTransparency)
		assert.NotContains(t, string(files["theme.json"]), "backgroundImage\"")

		assert.Equal(t, pngMagic, files["assets/background.png"])
	})

	t.Run("theme without an image has no assets and no backgroundImage key", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "plain.b3theme")
		_, err := ExportThemePackage("plain", Theme{Foreground: "#fff"}, testPackageDetails, out, "1.2.3", now)
		require.NoError(t, err)

		files := readThemePackage(t, out)
		assert.Len(t, files, 2)
		assert.Contains(t, files, "manifest.json")
		assert.Contains(t, files, "theme.json")
		assert.NotContains(t, string(files["manifest.json"]), "backgroundImage")
	})

	t.Run("records trimmed details in the manifest", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "plain.b3theme")
		details := ThemePackageDetails{ID: testThemeID, ThemeMetadata: ThemeMetadata{
			Repo: " https://github.com/example/themes\n", Description: "  Soft pastels.\n", Author: " Chris ", ThemeVersion: "2024-spring ",
		}}
		_, err := ExportThemePackage("plain", Theme{}, details, out, "1.2.3", now)
		require.NoError(t, err)

		var manifest ThemeManifest
		require.NoError(t, json.Unmarshal(readThemePackage(t, out)["manifest.json"], &manifest))
		assert.Equal(t, testThemeID, manifest.ID)
		assert.Equal(t, ThemeMetadata{Repo: "https://github.com/example/themes", Description: "Soft pastels.", Author: "Chris", ThemeVersion: "2024-spring"}, manifest.ThemeMetadata)
	})

	t.Run("fails without writing when the details are invalid", func(t *testing.T) {
		with := func(change func(*ThemePackageDetails)) ThemePackageDetails {
			d := testPackageDetails
			change(&d)
			return d
		}
		cases := map[string]struct {
			details ThemePackageDetails
			want    string
		}{
			"missing repo":       {with(func(d *ThemePackageDetails) { d.Repo = "" }), "repo is required"},
			"blank author":       {with(func(d *ThemePackageDetails) { d.Author = "   " }), "author is required"},
			"missing several":    {with(func(d *ThemePackageDetails) { d.Description, d.ThemeVersion = "", "" }), "description, theme version are required"},
			"http repo":          {with(func(d *ThemePackageDetails) { d.Repo = "http://github.com/x" }), "https URL"},
			"javascript repo":    {with(func(d *ThemePackageDetails) { d.Repo = "javascript:alert(1)" }), "https URL"},
			"repo without host":  {with(func(d *ThemePackageDetails) { d.Repo = "https:///themes" }), "https URL"},
			"missing id":         {with(func(d *ThemePackageDetails) { d.ID = "" }), "invalid theme package id"},
			"id that isn't uuid": {with(func(d *ThemePackageDetails) { d.ID = "midnight" }), "invalid theme package id"},
		}
		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				out := filepath.Join(t.TempDir(), "bad.b3theme")
				_, err := ExportThemePackage("bad", Theme{}, tc.details, out, "", now)
				require.ErrorContains(t, err, tc.want)
				assert.NoFileExists(t, out)
			})
		}
	})

	t.Run("keeps Theme.Package out of theme.json", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "plain.b3theme")
		details := testPackageDetails
		_, err := ExportThemePackage("plain", Theme{Foreground: "#fff", Package: &details}, testPackageDetails, out, "", now)
		require.NoError(t, err)
		assert.NotContains(t, string(readThemePackage(t, out)["theme.json"]), testThemeID)
	})

	t.Run("appends the .b3theme extension", func(t *testing.T) {
		dir := t.TempDir()
		got, err := ExportThemePackage("plain", Theme{}, testPackageDetails, filepath.Join(dir, "plain.zip"), "", now)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, "plain.zip.b3theme"), got)
		assert.FileExists(t, got)
	})

	t.Run("expands a leading ~", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		got, err := ExportThemePackage("plain", Theme{}, testPackageDetails, "~/plain", "", now)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(home, "plain.b3theme"), got)
	})

	t.Run("never overwrites an existing file", func(t *testing.T) {
		out := writeTempFile(t, "taken.b3theme", []byte("original"))
		_, err := ExportThemePackage("plain", Theme{}, testPackageDetails, out, "", now)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrThemePackageExists))
		data, readErr := os.ReadFile(out)
		require.NoError(t, readErr)
		assert.Equal(t, "original", string(data))
	})

	t.Run("does not create a missing parent directory", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nope")
		_, err := ExportThemePackage("plain", Theme{}, testPackageDetails, filepath.Join(missing, "plain"), "", now)
		require.ErrorContains(t, err, "does not exist")
		assert.NoDirExists(t, missing)
	})

	t.Run("rejects a parent that is a file", func(t *testing.T) {
		parent := writeTempFile(t, "file", nil)
		_, err := ExportThemePackage("plain", Theme{}, testPackageDetails, filepath.Join(parent, "plain"), "", now)
		require.ErrorContains(t, err, "is not a directory")
	})

	t.Run("rejects relative and empty paths", func(t *testing.T) {
		_, err := ExportThemePackage("plain", Theme{}, testPackageDetails, "plain.b3theme", "", now)
		require.ErrorContains(t, err, "must be an absolute path")
		_, err = ExportThemePackage("plain", Theme{}, testPackageDetails, "  ", "", now)
		require.ErrorContains(t, err, "required")
	})

	t.Run("refuses a package import would refuse for its size", func(t *testing.T) {
		image := writeTempFile(t, "huge.png", append(pngMagic, make([]byte, MAX_THEME_PACKAGE_SIZE)...))
		out := filepath.Join(t.TempDir(), "huge.b3theme")
		_, err := ExportThemePackage("huge", Theme{BackgroundImage: image}, testPackageDetails, out, "", now)
		require.ErrorContains(t, err, "larger than the 20 MB limit; use a smaller background image")
		assert.NoFileExists(t, out)
	})

	t.Run("fails without writing when the background image is invalid", func(t *testing.T) {
		cases := map[string]string{
			"missing":   filepath.Join(t.TempDir(), "missing.png"),
			"not image": writeTempFile(t, "notes.png", []byte("plain text")),
			"directory": t.TempDir(),
			"relative":  "bg.png",
		}
		for name, image := range cases {
			t.Run(name, func(t *testing.T) {
				out := filepath.Join(t.TempDir(), "bad.b3theme")
				_, err := ExportThemePackage("bad", Theme{BackgroundImage: image}, testPackageDetails, out, "", now)
				require.ErrorContains(t, err, "background image")
				assert.NoFileExists(t, out)
			})
		}
	})
}

func TestExportUserTheme(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	meta := testPackageDetails.ThemeMetadata

	manifestOf := func(t *testing.T, path string) ThemeManifest {
		t.Helper()
		var m ThemeManifest
		require.NoError(t, json.Unmarshal(readThemePackage(t, path)["manifest.json"], &m))
		return m
	}

	t.Run("generates an id on first export and reuses it after", func(t *testing.T) {
		configPath := writeTempFile(t, "conf.yaml", []byte("themes:\n  mine:\n    foreground: \"#ffffff\"\n"))
		dir := t.TempDir()

		first, details, err := ExportUserTheme(configPath, "mine", Theme{Foreground: "#ffffff"}, meta, filepath.Join(dir, "v1"), "", now)
		require.NoError(t, err)
		assert.True(t, isThemeID(details.ID))
		assert.Equal(t, details.ID, manifestOf(t, first).ID)

		saved, err := ReadThemePackageDetails(configPath, "mine")
		require.NoError(t, err)
		require.NotNil(t, saved)
		assert.Equal(t, details, *saved)

		v2 := meta
		v2.ThemeVersion = "2.0"
		second, details2, err := ExportUserTheme(configPath, "mine", Theme{Foreground: "#ffffff"}, v2, filepath.Join(dir, "v2"), "", now)
		require.NoError(t, err)
		assert.Equal(t, details.ID, details2.ID)
		assert.Equal(t, details.ID, manifestOf(t, second).ID)
		assert.Equal(t, "2.0", manifestOf(t, second).ThemeVersion)

		saved, err = ReadThemePackageDetails(configPath, "mine")
		require.NoError(t, err)
		assert.Equal(t, "2.0", saved.ThemeVersion)
	})

	t.Run("records nothing when the export fails", func(t *testing.T) {
		configPath := writeTempFile(t, "conf.yaml", []byte("themes:\n  mine:\n    foreground: \"#ffffff\"\n"))
		bad := meta
		bad.Repo = ""
		_, _, err := ExportUserTheme(configPath, "mine", Theme{}, bad, filepath.Join(t.TempDir(), "x"), "", now)
		require.ErrorContains(t, err, "repo is required")

		saved, err := ReadThemePackageDetails(configPath, "mine")
		require.NoError(t, err)
		assert.Nil(t, saved)
	})

	t.Run("reports a written package whose details could not be saved", func(t *testing.T) {
		configPath := writeTempFile(t, "conf.yaml", []byte("themes: {}\n"))
		path, _, err := ExportUserTheme(configPath, "gone", Theme{}, meta, filepath.Join(t.TempDir(), "x"), "", now)
		require.ErrorContains(t, err, "saving the package details failed")
		assert.FileExists(t, path)
	})
}

func TestIsBuiltinThemeName(t *testing.T) {
	assert.True(t, IsBuiltinThemeName("dracula"))
	assert.True(t, IsBuiltinThemeName("b3tty-dark"))
	assert.False(t, IsBuiltinThemeName("my-theme"))
	assert.False(t, IsBuiltinThemeName(""))
}

// testPackage returns the bytes of a valid package of a theme named name with
// the given id and version, and a PNG background image when withImage is set.
func testPackage(t *testing.T, name, id, version string, withImage bool) []byte {
	t.Helper()
	details := testPackageDetails
	details.ID = id
	details.ThemeVersion = version
	var image []byte
	var imageFile string
	if withImage {
		image, imageFile = pngMagic, "assets/background.png"
	}
	data, err := buildThemePackage(name, Theme{Foreground: "#ffffff", Background: "#101418"}, details, image, imageFile, "1.12.0", time.Now())
	require.NoError(t, err)
	return data
}

// zipEntry is one file of a hand-built zip.
type zipEntry struct {
	name string
	data []byte
}

// zipOf returns a zip holding entries, in order.
func zipOf(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		require.NoError(t, err)
		_, err = w.Write(e.data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// testManifest returns the manifest.json of a valid image-less package, after
// applying change to it.
func testManifest(t *testing.T, change func(m map[string]any)) []byte {
	t.Helper()
	m := map[string]any{
		"schemaVersion": 1,
		"kind":          "theme",
		"id":            testThemeID,
		"name":          "midnight",
		"repo":          "https://github.com/example/themes",
		"author":        "Chris",
		"description":   "Soft pastels.",
		"themeVersion":  "1.0",
		"b3ttyVersion":  "1.12.0",
		"createdAt":     "2026-10-04T17:00:00Z",
		"files":         map[string]any{"themeFile": "theme.json"},
	}
	if change != nil {
		change(m)
	}
	data, err := json.Marshal(m)
	require.NoError(t, err)
	return data
}

func TestReadThemePackage(t *testing.T) {
	themeJSON := []byte(`{"foreground":"#ffffff"}`)

	t.Run("reads what export writes", func(t *testing.T) {
		data := testPackage(t, "midnight", testThemeID, "1.0", true)
		pkg, err := ReadThemePackage(data)
		require.NoError(t, err)
		assert.Equal(t, "midnight", pkg.Manifest.Name)
		assert.Equal(t, "theme", pkg.Manifest.Kind)
		assert.Equal(t, testThemeID, pkg.Manifest.ID)
		assert.Equal(t, testPackageDetails.Author, pkg.Manifest.Author)
		assert.Equal(t, "#ffffff", pkg.Theme.Foreground)
		assert.Equal(t, pngMagic, pkg.Image)
		assert.Equal(t, ".png", pkg.ImageExt)
		assert.Equal(t, sha256Hex(data), pkg.Digest)
	})

	t.Run("accepts a 12 MB background image", func(t *testing.T) {
		image := append(append([]byte{}, pngMagic...), make([]byte, 12<<20)...)
		data, err := buildThemePackage("big", Theme{}, testPackageDetails, image, "assets/background.png", "", time.Now())
		require.NoError(t, err)
		pkg, err := ReadThemePackage(data)
		require.NoError(t, err)
		assert.Len(t, pkg.Image, len(image))
	})

	t.Run("ignores a backgroundImage path in theme.json", func(t *testing.T) {
		data := zipOf(t,
			zipEntry{"manifest.json", testManifest(t, nil)},
			zipEntry{"theme.json", []byte(`{"foreground":"#fff","backgroundImage":"/etc/passwd"}`)},
		)
		pkg, err := ReadThemePackage(data)
		require.NoError(t, err)
		assert.Empty(t, pkg.Theme.BackgroundImage)
	})

	withImage := func(m map[string]any) {
		m["files"] = map[string]any{"themeFile": "theme.json", "backgroundImage": "assets/background.png"}
	}
	cases := map[string]struct {
		data []byte
		want string
	}{
		"not a zip": {[]byte("hello"), "not a valid .b3theme package"},
		"too large": {make([]byte, MAX_THEME_PACKAGE_SIZE+1), "larger than the 20 MB limit"},
		"too many entries": {zipOf(t,
			zipEntry{"manifest.json", nil}, zipEntry{"theme.json", nil}, zipEntry{"a", nil}, zipEntry{"b", nil}, zipEntry{"c", nil},
		), "at most 4"},
		"unexpected entry": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, nil)}, zipEntry{"theme.json", themeJSON}, zipEntry{"../evil.sh", nil},
		), `unexpected file "../evil.sh"`},
		"svg background": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, nil)}, zipEntry{"theme.json", themeJSON}, zipEntry{"assets/background.svg", nil},
		), "unexpected file"},
		"two backgrounds": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, nil)}, zipEntry{"theme.json", themeJSON},
			zipEntry{"assets/background.png", pngMagic}, zipEntry{"assets/background.gif", nil},
		), "more than one background image"},
		"duplicate entry": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, nil)}, zipEntry{"theme.json", themeJSON}, zipEntry{"theme.json", themeJSON},
		), "more than once"},
		"no theme.json": {zipOf(t, zipEntry{"manifest.json", testManifest(t, nil)}), "has no theme.json"},
		"no kind": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, func(m map[string]any) { delete(m, "kind") })}, zipEntry{"theme.json", themeJSON},
		), "exported again"},
		"another kind": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, func(m map[string]any) { m["kind"] = "font" })}, zipEntry{"theme.json", themeJSON},
		), `kind "font" is not a theme`},
		"future schema": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, func(m map[string]any) { m["schemaVersion"] = 2 })}, zipEntry{"theme.json", themeJSON},
		), "unsupported schemaVersion 2"},
		"bad id": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, func(m map[string]any) { m["id"] = "midnight" })}, zipEntry{"theme.json", themeJSON},
		), "invalid theme package id"},
		"missing author": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, func(m map[string]any) { m["author"] = " " })}, zipEntry{"theme.json", themeJSON},
		), "author is required"},
		"javascript repo": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, func(m map[string]any) { m["repo"] = "javascript:alert(1)" })}, zipEntry{"theme.json", themeJSON},
		), "https URL"},
		"name with a slash": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, func(m map[string]any) { m["name"] = "a/b" })}, zipEntry{"theme.json", themeJSON},
		), "can't contain a /"},
		"unknown manifest field": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, func(m map[string]any) { m["script"] = "x" })}, zipEntry{"theme.json", themeJSON},
		), `unknown field "script"`},
		"files names a missing image": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, withImage)}, zipEntry{"theme.json", themeJSON},
		), "doesn't match the package's contents"},
		"files omits a present image": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, nil)}, zipEntry{"theme.json", themeJSON}, zipEntry{"assets/background.png", pngMagic},
		), "doesn't match the package's contents"},
		"invalid color": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, nil)}, zipEntry{"theme.json", []byte(`{"foreground":"url(x)"}`)},
		), "invalid theme color for foreground"},
		"unknown theme field": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, nil)}, zipEntry{"theme.json", []byte(`{"script":"x"}`)},
		), `unknown field "script"`},
		"image that isn't one": {zipOf(t,
			zipEntry{"manifest.json", testManifest(t, withImage)}, zipEntry{"theme.json", themeJSON},
			zipEntry{"assets/background.png", []byte("<html>")},
		), "unsupported background image type"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ReadThemePackage(tc.data)
			require.ErrorContains(t, err, tc.want)
		})
	}

	t.Run("counts uncompressed bytes as it reads", func(t *testing.T) {
		// Compresses to a few KB, but is larger than the limit uncompressed.
		huge := bytes.Repeat([]byte(" "), MAX_THEME_PACKAGE_SIZE+1)
		data := zipOf(t, zipEntry{"manifest.json", huge}, zipEntry{"theme.json", themeJSON})
		require.Less(t, len(data), MAX_THEME_PACKAGE_SIZE)
		_, err := ReadThemePackage(data)
		require.ErrorContains(t, err, "when uncompressed")
	})
}

func TestValidateThemeName(t *testing.T) {
	assert.NoError(t, validateThemeName("midnight-ops"))
	assert.NoError(t, validateThemeName("Ocean Breeze 2"))
	for _, bad := range []string{"", " lead", "trail ", "a/b", "tab\there", strings.Repeat("x", MAX_THEME_NAME_LENGTH+1)} {
		assert.Error(t, validateThemeName(bad), bad)
	}
}
