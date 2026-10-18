package src

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Paths of the files inside a .b3theme package. The background image is stored
// as backgroundImageBase plus the source file's lowercased extension, so the
// original file name never leaves the exporter's machine.
const (
	themePackageManifestFile = "manifest.json"
	themePackageThemeFile    = "theme.json"
	themePackageImageBase    = "assets/background"
)

// ErrThemePackageExists is returned by ExportThemePackage when the output file
// already exists. Exports never overwrite a file.
var ErrThemePackageExists = errors.New("file already exists")

// ThemeManifest is the manifest.json at the root of a .b3theme package. The
// ThemeMetadata fields are embedded, so they sit at the top level of the
// manifest alongside the others.
type ThemeManifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	ThemeMetadata
	B3ttyVersion string             `json:"b3ttyVersion"`
	CreatedAt    string             `json:"createdAt"`
	Files        ThemeManifestFiles `json:"files"`
}

// ThemeMetadata is the information an exporter records in a .b3theme package.
// Every field is required. Repo is the https URL of the theme's source
// repository. ThemeVersion is the theme's own version, unrelated to the b3tty
// version, and has no required format. The yaml tags are for the package:
// block of a user theme in conf.yaml (see ThemePackageDetails).
type ThemeMetadata struct {
	Repo         string `json:"repo" yaml:"repo"`
	Author       string `json:"author" yaml:"author"`
	Description  string `json:"description" yaml:"description"`
	ThemeVersion string `json:"themeVersion" yaml:"theme-version"`
}

// trimmed returns m with surrounding whitespace removed from every field.
func (m ThemeMetadata) trimmed() ThemeMetadata {
	return ThemeMetadata{
		Repo:         strings.TrimSpace(m.Repo),
		Author:       strings.TrimSpace(m.Author),
		Description:  strings.TrimSpace(m.Description),
		ThemeVersion: strings.TrimSpace(m.ThemeVersion),
	}
}

// ThemePackageDetails is what a user theme remembers about its last export, as
// the package: block of its entry in conf.yaml: the package id, which every
// later export of the theme reuses so importers see it as an upgrade, and the
// metadata recorded in that export, which fills in the next one.
type ThemePackageDetails struct {
	ID            string `json:"id" yaml:"id"`
	ThemeMetadata `yaml:",inline"`
}

// validate checks that d can be recorded in a package: a UUID id, every
// metadata field non-empty, and an https repo. Its errors are user-facing.
func (d ThemePackageDetails) validate() error {
	if !isThemeID(d.ID) {
		return fmt.Errorf("invalid theme package id %q", d.ID)
	}
	var missing []string
	for _, f := range []struct{ name, value string }{
		{"repo", d.Repo},
		{"author", d.Author},
		{"description", d.Description},
		{"theme version", d.ThemeVersion},
	} {
		if f.value == "" {
			missing = append(missing, f.name)
		}
	}
	switch len(missing) {
	case 0:
	case 1:
		return fmt.Errorf("%s is required", missing[0])
	default:
		return fmt.Errorf("%s are required", strings.Join(missing, ", "))
	}
	return validateRepoURL(d.Repo)
}

// ThemeManifestFiles lists the package-relative paths of the files in a
// .b3theme package. BackgroundImage is omitted when the theme has no image.
type ThemeManifestFiles struct {
	ThemeFile       string `json:"themeFile"`
	BackgroundImage string `json:"backgroundImage,omitempty"`
}

// IsBuiltinThemeName reports whether name is one of the built-in themes.
func IsBuiltinThemeName(name string) bool {
	_, ok := builtinThemes[name]
	return ok
}

// ExportUserTheme exports the user-defined theme name, as ExportThemePackage
// does, and records the export in the theme's package: block in the config file
// at configPath. The package id comes from that block when the theme has been
// exported before and is generated otherwise, so every export of a theme
// carries the same id. meta becomes the block's metadata, but only after the
// package file has been written, so a failed export records nothing. It is
// shared by POST /export-theme and `b3tty theme export`, and returns the path
// written and the details recorded.
func ExportUserTheme(configPath, name string, theme Theme, meta ThemeMetadata, outPath, version string, now time.Time) (string, ThemePackageDetails, error) {
	saved, err := ReadThemePackageDetails(configPath, name)
	if err != nil {
		return "", ThemePackageDetails{}, fmt.Errorf("reading saved package details: %w", err)
	}
	details := ThemePackageDetails{ThemeMetadata: meta.trimmed()}
	if saved != nil && isThemeID(saved.ID) {
		details.ID = saved.ID
	} else if details.ID, err = newThemeID(); err != nil {
		return "", ThemePackageDetails{}, err
	}

	path, err := ExportThemePackage(name, theme, details, outPath, version, now)
	if err != nil {
		return "", ThemePackageDetails{}, err
	}
	if err := SaveThemePackageToConfig(configPath, name, details); err != nil {
		return path, details, fmt.Errorf("exported to %s, but saving the package details failed: %w", path, err)
	}
	return path, details, nil
}

// ExportThemePackage writes theme, under name, as a .b3theme package (a zip of
// manifest.json, theme.json and, when the theme has one, its background image
// under assets/) to outPath, and returns the path written. details, with
// surrounding whitespace trimmed, is recorded in the manifest and must pass
// validation. A leading "~" in outPath is expanded, the result must be
// absolute, and THEME_PACKAGE_EXT is appended when outPath doesn't already end
// with it. The parent directory must already exist — it is never created — and
// an existing file is never overwritten (the error wraps
// ErrThemePackageExists). A configured background image that is missing or not
// an allowed image fails the export. Nothing is written until every check has
// passed.
func ExportThemePackage(name string, theme Theme, details ThemePackageDetails, outPath, version string, now time.Time) (string, error) {
	details.ThemeMetadata = details.trimmed()
	if err := details.validate(); err != nil {
		return "", err
	}
	path, err := resolveThemePackagePath(outPath)
	if err != nil {
		return "", err
	}

	var image []byte
	var imageFile string
	if theme.HasBackgroundImage() {
		imagePath, err := resolveBackgroundImage(theme.BackgroundImage)
		if err != nil {
			return "", err
		}
		if image, err = os.ReadFile(imagePath); err != nil {
			return "", fmt.Errorf("background image: %w", err)
		}
		imageFile = themePackageImageBase + strings.ToLower(filepath.Ext(imagePath))
	}

	pkg, err := buildThemePackage(name, theme, details, image, imageFile, version, now)
	if err != nil {
		return "", err
	}

	// O_EXCL makes "doesn't exist yet" and "create it" one atomic step.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("%w: %s", ErrThemePackageExists, path)
		}
		return "", fmt.Errorf("cannot write %s: %w", path, err)
	}
	_, err = f.Write(pkg)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("cannot write %s: %w", path, err)
	}
	return path, nil
}

// resolveThemePackagePath expands, checks, and adds the package extension to an
// export path, and checks that its parent directory exists.
func resolveThemePackagePath(outPath string) (string, error) {
	if strings.TrimSpace(outPath) == "" {
		return "", errors.New("an output file path is required")
	}
	path, err := expandHomePath(outPath)
	if err != nil {
		return "", fmt.Errorf("output path %q: %w", outPath, err)
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("output path %q must be an absolute path", outPath)
	}
	path = filepath.Clean(path)
	if !strings.HasSuffix(path, THEME_PACKAGE_EXT) {
		path += THEME_PACKAGE_EXT
	}
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("directory %s does not exist", dir)
		}
		return "", fmt.Errorf("directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", dir)
	}
	return path, nil
}

// buildThemePackage returns the zip bytes of a .b3theme package. The theme's
// BackgroundImage path is cleared from theme.json — it is a path on the
// exporter's machine — and the manifest's files.backgroundImage names the
// image inside the package instead. Theme.Package never reaches theme.json
// (its json tag is "-"); the manifest records details instead.
func buildThemePackage(name string, theme Theme, details ThemePackageDetails, image []byte, imageFile, version string, now time.Time) ([]byte, error) {
	theme.BackgroundImage = ""
	themeJSON, err := json.MarshalIndent(theme, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding theme: %w", err)
	}
	manifest := ThemeManifest{
		SchemaVersion: THEME_PACKAGE_SCHEMA_VERSION,
		ID:            details.ID,
		Name:          name,
		ThemeMetadata: details.ThemeMetadata,
		B3ttyVersion:  version,
		CreatedAt:     now.UTC().Format(time.RFC3339),
		Files:         ThemeManifestFiles{ThemeFile: themePackageThemeFile, BackgroundImage: imageFile},
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding manifest: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	type entry struct {
		name string
		data []byte
	}
	entries := []entry{{themePackageManifestFile, manifestJSON}, {themePackageThemeFile, themeJSON}}
	if image != nil {
		entries = append(entries, entry{imageFile, image})
	}
	for _, e := range entries {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: zip.Deflate, Modified: now})
		if err != nil {
			return nil, fmt.Errorf("packaging %s: %w", e.name, err)
		}
		if _, err := w.Write(e.data); err != nil {
			return nil, fmt.Errorf("packaging %s: %w", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("packaging theme: %w", err)
	}
	return buf.Bytes(), nil
}
