package src

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// Paths inside a plugin store. Each installed theme's package is kept,
// unchanged, at themes/<sha256 of its bytes>/package.b3theme, and
// installed.json lists what is installed.
const (
	pluginRegistryFile          = "installed.json"
	pluginThemesDir             = "themes"
	pluginPackageFile           = "package.b3theme"
	pluginRegistrySchemaVersion = 1
	pluginDigestPrefix          = "sha256:"
)

var reDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

// pluginStoreMu is held across every read-modify-write of a store's registry,
// so two requests in the same process can't interleave. Like configFileMu, it
// doesn't protect against a b3tty CLI command, a separate process, writing at
// the same time.
var pluginStoreMu sync.Mutex

// ErrPackageChanged is returned by InstalledTheme.ReadImage when the package
// file is no longer the one that was verified when the theme was loaded.
var ErrPackageChanged = errors.New("the package changed since it was loaded; run 'b3tty theme verify'")

// ErrThemeNotInstalled is returned by PluginStore.Uninstall for a name that no
// installed theme has.
var ErrThemeNotInstalled = errors.New("theme is not installed")

// PluginStore is the directory where b3tty keeps installed plugins: today,
// installed themes. b3tty owns it; nothing in it is meant to be edited by
// hand.
type PluginStore struct {
	Dir string
}

// DefaultPluginStore returns the store at ~/.config/b3tty/plugins. It is in
// the user's config directory whichever config file b3tty loaded, since a
// config under /etc is usually not writable.
func DefaultPluginStore() (*PluginStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &PluginStore{Dir: filepath.Join(home, DOT_CONFIG_PATH, B3TTY_CONFIG_PATH, PLUGINS_DIR_NAME)}, nil
}

// PluginRegistry is installed.json, the list of what is installed. It is the
// only record of what is installed: a package directory it doesn't list is an
// orphan, which is ignored and removed by GC.
type PluginRegistry struct {
	SchemaVersion int           `json:"schemaVersion"`
	Plugins       []PluginEntry `json:"plugins"`
}

// PluginEntry is one installed plugin. Name, Repo, Author and ThemeVersion are
// copied from the package's manifest when it is installed, for listings and
// messages; the manifest inside the package is what b3tty loads. Enabled is
// always true for themes; it is reserved for future plugin kinds.
type PluginEntry struct {
	Kind         string `json:"kind"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	Repo         string `json:"repo"`
	Author       string `json:"author"`
	ThemeVersion string `json:"themeVersion"`
	Digest       string `json:"digest"`
	InstalledAt  string `json:"installedAt"`
	Enabled      bool   `json:"enabled"`
}

// digestHex returns the entry's digest without its "sha256:" prefix, or ""
// when it isn't a well-formed SHA-256, so a hand-edited registry can never
// name a directory outside the store.
func (e PluginEntry) digestHex() string {
	hex := strings.TrimPrefix(e.Digest, pluginDigestPrefix)
	if !strings.HasPrefix(e.Digest, pluginDigestPrefix) || !reDigest.MatchString(hex) {
		return ""
	}
	return hex
}

// InstalledTheme is an installed theme as loaded from its package. Its
// background image is not held in memory: ImageFile says where it is in the
// package, and ReadImage reads it when it is needed.
type InstalledTheme struct {
	Theme    Theme
	Manifest ThemeManifest
	// Digest is the hex SHA-256 the registry recorded when it was installed.
	Digest string
	// Modified is true when the package's bytes no longer match Digest. The
	// theme still loads, but it is not the package its author published.
	Modified bool
	// PackagePath is the package file in the store. ImageFile is the
	// background image's entry in it ("" when there is none), and ImageExt
	// the image's lowercased extension, with the dot. Theme.BackgroundImage
	// is always empty.
	PackagePath string
	ImageFile   string
	ImageExt    string
	// fileDigest is the hex SHA-256 of the package file as loaded (Digest
	// unless Modified; assumed to be Digest when it was loaded without
	// verifying), and fileSize and fileModTime the file's size and
	// modification time when it was loaded: ReadImage refuses to read a file
	// that no longer matches them.
	fileDigest  string
	fileSize    int64
	fileModTime time.Time
}

// HasImage reports whether the theme's package has a background image.
func (t InstalledTheme) HasImage() bool { return t.ImageFile != "" }

// ImageETag returns an HTTP entity tag for the theme's background image: the
// digest of the package file it is read from.
func (t InstalledTheme) ImageETag() string { return `"` + t.fileDigest + `"` }

// CheckUnchanged returns ErrPackageChanged when the package file's size or
// modification time differs from when it was hashed, and the error from
// stat when it can't be checked.
func (t InstalledTheme) CheckUnchanged() error {
	info, err := os.Stat(t.PackagePath)
	if err != nil {
		return err
	}
	if info.Size() != t.fileSize || !info.ModTime().Equal(t.fileModTime) {
		return ErrPackageChanged
	}
	return nil
}

// ReadImage reads the theme's background image out of its package, after
// checking that the package file is still the one that was verified (see
// CheckUnchanged), and checks the image's size and type again.
func (t InstalledTheme) ReadImage() ([]byte, error) {
	if !t.HasImage() {
		return nil, errors.New("the theme has no background image")
	}
	f, err := os.Open(t.PackagePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() != t.fileSize || !info.ModTime().Equal(t.fileModTime) {
		return nil, ErrPackageChanged
	}
	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		return nil, err
	}
	for _, entry := range zr.File {
		if entry.Name != t.ImageFile {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		data, err := io.ReadAll(io.LimitReader(rc, MAX_THEME_PACKAGE_SIZE+1))
		if err != nil {
			return nil, err
		}
		if len(data) > MAX_THEME_PACKAGE_SIZE {
			return nil, errThemePackageOverLimit
		}
		if err := validateBackgroundImageData(t.ImageExt, data); err != nil {
			return nil, err
		}
		return data, nil
	}
	return nil, fmt.Errorf("the package has no %s", t.ImageFile)
}

// CheckModified hashes the theme's package file and reports whether it no
// longer matches the digest it was installed with. It is how a theme loaded
// without verifying (see PluginStore.LoadThemes) finds out.
func (t InstalledTheme) CheckModified() (bool, error) {
	f, err := os.Open(t.PackagePath)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return hex.EncodeToString(h.Sum(nil)) != t.Digest, nil
}

// loadPackageFile loads the package at path, installed with the given digest.
// With verify, it hashes the file as a stream, setting Modified, and
// validates all of it with readThemePackageZip, the image included but never
// held in memory. Without verify, it neither hashes the file nor reads the
// image — the manifest and theme.json are still validated — which is fast
// whatever the image's size, for callers that don't need to know whether the
// package was tampered with.
func loadPackageFile(path, digest string, verify bool) (InstalledTheme, error) {
	f, err := os.Open(path)
	if err != nil {
		return InstalledTheme{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return InstalledTheme{}, err
	}
	if !info.Mode().IsRegular() {
		return InstalledTheme{}, fmt.Errorf("%s is not a file", path)
	}
	if info.Size() > MAX_THEME_PACKAGE_SIZE {
		return InstalledTheme{}, errThemePackageOverLimit
	}
	fileDigest, mode := digest, imageSkip
	if verify {
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return InstalledTheme{}, err
		}
		fileDigest, mode = hex.EncodeToString(h.Sum(nil)), imageCheck
	}
	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		return InstalledTheme{}, fmt.Errorf("not a valid .b3theme package: %w", err)
	}
	pkg, err := readThemePackageZip(zr, mode)
	if err != nil {
		return InstalledTheme{}, err
	}
	return InstalledTheme{
		Theme:       pkg.Theme,
		Manifest:    pkg.Manifest,
		Digest:      digest,
		Modified:    fileDigest != digest,
		PackagePath: path,
		ImageFile:   pkg.ImageFile,
		ImageExt:    pkg.ImageExt,
		fileDigest:  fileDigest,
		fileSize:    info.Size(),
		fileModTime: info.ModTime(),
	}, nil
}

func (s *PluginStore) registryPath() string { return filepath.Join(s.Dir, pluginRegistryFile) }
func (s *PluginStore) themesDir() string    { return filepath.Join(s.Dir, pluginThemesDir) }
func (s *PluginStore) packagePath(digest string) string {
	return filepath.Join(s.themesDir(), digest, pluginPackageFile)
}

// readRegistry reads installed.json; a store with none has nothing installed.
func (s *PluginStore) readRegistry() (PluginRegistry, error) {
	reg := PluginRegistry{SchemaVersion: pluginRegistrySchemaVersion}
	data, err := os.ReadFile(s.registryPath())
	if errors.Is(err, fs.ErrNotExist) {
		return reg, nil
	} else if err != nil {
		return reg, err
	}
	if err := json.Unmarshal(data, &reg); err != nil {
		return reg, fmt.Errorf("%s: %w", s.registryPath(), err)
	}
	if reg.SchemaVersion != pluginRegistrySchemaVersion {
		return reg, fmt.Errorf("%s: unsupported schemaVersion %d", s.registryPath(), reg.SchemaVersion)
	}
	return reg, nil
}

// writeRegistry replaces installed.json by writing a temporary file and
// renaming it into place, so a reader never sees a partly written registry.
func (s *PluginStore) writeRegistry(reg PluginRegistry) error {
	if reg.Plugins == nil {
		reg.Plugins = []PluginEntry{}
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0755); err != nil {
		return err
	}
	return writeFileAtomic(s.Dir, s.registryPath(), ".installed-*.json", data)
}

// writeFileAtomic writes data to a temporary file in tmpDir, which must be on
// the same filesystem as path, then renames it to path.
func writeFileAtomic(tmpDir, path, pattern string, data []byte) error {
	tmp, err := os.CreateTemp(tmpDir, pattern)
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0644)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}

// themeEntries returns the registry's theme entries, skipping other kinds.
func (reg PluginRegistry) themeEntries() []PluginEntry {
	var entries []PluginEntry
	for _, e := range reg.Plugins {
		if e.Kind == THEME_PACKAGE_KIND {
			entries = append(entries, e)
		}
	}
	return entries
}

// ThemeEntries returns the registry's installed theme entries, sorted by name.
func (s *PluginStore) ThemeEntries() ([]PluginEntry, error) {
	reg, err := s.readRegistry()
	if err != nil {
		return nil, err
	}
	entries := reg.themeEntries()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// LoadThemes loads every installed theme, keyed by its manifest's name. With
// verify, each package is re-hashed and fully validated (see loadPackageFile):
// one whose bytes no longer match its digest is loaded with Modified set and a
// warning. Without verify — for CLI commands, which shouldn't pay for
// decompressing every large image — only each package's manifest and theme are
// read, and Modified is never set. Either way, one that is missing or no
// longer a valid package is skipped with a warning, as is every theme when
// the registry can't be read. A nil store has nothing installed.
func (s *PluginStore) LoadThemes(verify bool) map[string]InstalledTheme {
	themes := make(map[string]InstalledTheme)
	if s == nil {
		return themes
	}
	reg, err := s.readRegistry()
	if err != nil {
		Warnf("installed themes are not loaded: %v", err)
		return themes
	}
	for _, e := range reg.themeEntries() {
		t, err := s.loadTheme(e, verify)
		if err != nil {
			Warnf("installed theme %q is skipped: %v", e.Name, err)
			continue
		}
		name := t.Manifest.Name
		if t.Modified {
			Warnf("installed theme %q has been modified since it was installed; run 'b3tty theme verify' for details", name)
		}
		if _, dup := themes[name]; dup {
			Warnf("installed theme %q is skipped: another installed theme has the same name", name)
			continue
		}
		themes[name] = t
	}
	return themes
}

// loadTheme reads and validates the package of one registry entry (see
// loadPackageFile).
func (s *PluginStore) loadTheme(e PluginEntry, verify bool) (InstalledTheme, error) {
	digest := e.digestHex()
	if digest == "" {
		return InstalledTheme{}, fmt.Errorf("invalid digest %q in %s", e.Digest, s.registryPath())
	}
	return loadPackageFile(s.packagePath(digest), digest, verify)
}

// InstallResult describes what PluginStore.Install did.
type InstallResult struct {
	Theme InstalledTheme
	// Upgraded is true when the package replaced an installed version of the
	// same theme, and Unchanged when that exact package was already installed.
	Upgraded  bool
	Unchanged bool
}

// UpgradeRequiredError is returned by PluginStore.Install for a new version of
// an installed theme when replacing it wasn't approved. Its message says what
// would change, and warns when the new version has a different author or
// repository than the installed one: package ids are not verified, so a
// package can claim to be a new version of any theme.
type UpgradeRequiredError struct {
	Installed PluginEntry
	Incoming  ThemeManifest
}

func (e *UpgradeRequiredError) Error() string {
	old, inc := e.Installed, e.Incoming
	msg := fmt.Sprintf("%s %s is installed; this package is version %s", old.Name, old.ThemeVersion, inc.ThemeVersion)
	if old.Author != inc.Author {
		msg += fmt.Sprintf(". This claims to be a new version of %s by %s, but it's by %s", old.Name, old.Author, inc.Author)
	}
	if normalizeRepo(old.Repo) != normalizeRepo(inc.Repo) {
		msg += fmt.Sprintf(". It comes from %s, not %s", inc.Repo, old.Repo)
	}
	return msg + "."
}

// normalizeRepo returns a repository URL in a form where trivially different
// spellings of the same repository compare equal: http is treated as https,
// the host is lowercased, and a trailing "/" or ".git" is dropped.
func normalizeRepo(repo string) string {
	u, err := url.Parse(strings.TrimSpace(repo))
	if err != nil || u.Host == "" {
		return strings.ToLower(strings.TrimSpace(repo))
	}
	path := strings.TrimSuffix(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimSuffix(path, "/")
	return "https://" + strings.ToLower(u.Host) + path
}

// Install installs a .b3theme package from its bytes. The package must pass
// ReadThemePackage, and its name must be unique across every theme source:
// isUserTheme reports whether a user theme has a name. Precedence on a clash
// is built-in > installed > user, and every clash rejects the package, except
// a package with the id of an installed theme:
//
//   - the same bytes as installed: nothing to do, reported as Unchanged
//   - a different name: rejected, since an upgrade can't rename a theme
//   - otherwise a new version, which replaces the installed one only when
//     replace is true and is an *UpgradeRequiredError otherwise
//
// The package is written to a temporary file and renamed into its directory
// before the registry is updated; a replaced version's directory is removed
// afterwards.
func (s *PluginStore) Install(data []byte, isUserTheme func(string) bool, replace bool) (InstallResult, error) {
	pkg, err := ReadThemePackage(data)
	if err != nil {
		return InstallResult{}, err
	}
	name := pkg.Manifest.Name
	if IsBuiltinThemeName(name) {
		return InstallResult{}, fmt.Errorf("%q is the name of a built-in theme, so this package can't be installed", name)
	}

	pluginStoreMu.Lock()
	defer pluginStoreMu.Unlock()

	reg, err := s.readRegistry()
	if err != nil {
		return InstallResult{}, err
	}
	sameID, sameName := -1, -1
	for i, e := range reg.Plugins {
		if e.Kind != THEME_PACKAGE_KIND {
			continue
		}
		if e.ID == pkg.Manifest.ID {
			sameID = i
		}
		if e.Name == name {
			sameName = i
		}
	}

	var result InstallResult
	var oldDigest string
	switch {
	case sameID >= 0:
		old := reg.Plugins[sameID]
		if old.Name != name {
			return InstallResult{}, fmt.Errorf("this package is a version of the installed theme %q but is named %q; an upgrade can't rename a theme", old.Name, name)
		}
		oldDigest = old.digestHex()
		if oldDigest == pkg.Digest {
			// Rewrite the file in case it went missing or was modified.
			if err := s.writePackage(pkg.Digest, data); err != nil {
				return InstallResult{}, err
			}
			result.Unchanged = true
			result.Theme, err = loadPackageFile(s.packagePath(pkg.Digest), pkg.Digest, false)
			return result, err
		}
		if !replace {
			return InstallResult{}, &UpgradeRequiredError{Installed: old, Incoming: pkg.Manifest}
		}
		result.Upgraded = true
	case sameName >= 0:
		return InstallResult{}, fmt.Errorf("a different theme named %q, by %s, is already installed; uninstall it first", name, reg.Plugins[sameName].Author)
	case isUserTheme != nil && isUserTheme(name):
		return InstallResult{}, fmt.Errorf("you already have a theme named %q; rename your theme, then import the package again", name)
	}

	if err := s.writePackage(pkg.Digest, data); err != nil {
		return InstallResult{}, err
	}
	if result.Theme, err = loadPackageFile(s.packagePath(pkg.Digest), pkg.Digest, false); err != nil {
		if sameID < 0 {
			_ = os.RemoveAll(filepath.Dir(s.packagePath(pkg.Digest)))
		}
		return InstallResult{}, err
	}
	m := pkg.Manifest
	entry := PluginEntry{
		Kind:         THEME_PACKAGE_KIND,
		ID:           m.ID,
		Name:         name,
		Repo:         m.Repo,
		Author:       m.Author,
		ThemeVersion: m.ThemeVersion,
		Digest:       pluginDigestPrefix + pkg.Digest,
		InstalledAt:  time.Now().UTC().Format(time.RFC3339),
		Enabled:      true,
	}
	if sameID >= 0 {
		reg.Plugins[sameID] = entry
	} else {
		reg.Plugins = append(reg.Plugins, entry)
	}
	if err := s.writeRegistry(reg); err != nil {
		if sameID < 0 {
			_ = os.RemoveAll(filepath.Dir(s.packagePath(pkg.Digest)))
		}
		return InstallResult{}, err
	}
	if oldDigest != "" {
		if err := os.RemoveAll(filepath.Dir(s.packagePath(oldDigest))); err != nil {
			Warnf("removing the previous version of %q: %v", name, err)
		}
	}
	return result, nil
}

// writePackage stores data as the package with the given digest, unless a
// file with exactly those bytes is already there.
func (s *PluginStore) writePackage(digest string, data []byte) error {
	path := s.packagePath(digest)
	if existing, err := os.ReadFile(path); err == nil && sha256Hex(existing) == digest {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return writeFileAtomic(s.themesDir(), path, ".package-*.tmp", data)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Uninstall removes the installed theme name: its registry entry, then its
// package directory. It returns ErrThemeNotInstalled when no installed theme
// has that name.
func (s *PluginStore) Uninstall(name string) error {
	pluginStoreMu.Lock()
	defer pluginStoreMu.Unlock()

	reg, err := s.readRegistry()
	if err != nil {
		return err
	}
	i := slices.IndexFunc(reg.Plugins, func(e PluginEntry) bool {
		return e.Kind == THEME_PACKAGE_KIND && e.Name == name
	})
	if i < 0 {
		return fmt.Errorf("%w: %q", ErrThemeNotInstalled, name)
	}
	digest := reg.Plugins[i].digestHex()
	reg.Plugins = slices.Delete(reg.Plugins, i, i+1)
	if err := s.writeRegistry(reg); err != nil {
		return err
	}
	if digest != "" {
		if err := os.RemoveAll(filepath.Dir(s.packagePath(digest))); err != nil {
			Warnf("removing the package of %q: %v", name, err)
		}
	}
	return nil
}

// VerifyReport lists what PluginStore.Verify found wrong.
type VerifyReport struct {
	// Modified names installed themes whose package no longer matches its
	// digest, and Missing those whose package is gone.
	Modified []string
	Missing  []string
	// Invalid maps installed theme names to why their package no longer loads.
	Invalid map[string]string
	// Orphans are paths under the store's themes directory that no installed
	// theme uses.
	Orphans []string
	// Checked is how many installed themes were checked.
	Checked int
}

// OK reports whether Verify found nothing wrong.
func (r VerifyReport) OK() bool {
	return len(r.Modified) == 0 && len(r.Missing) == 0 && len(r.Invalid) == 0 && len(r.Orphans) == 0
}

// Verify re-hashes and re-validates every installed theme's package, and finds
// orphaned files in the store.
func (s *PluginStore) Verify() (VerifyReport, error) {
	report := VerifyReport{Invalid: map[string]string{}}
	reg, err := s.readRegistry()
	if err != nil {
		return report, err
	}
	for _, e := range reg.themeEntries() {
		report.Checked++
		digest := e.digestHex()
		if digest == "" {
			report.Invalid[e.Name] = fmt.Sprintf("invalid digest %q", e.Digest)
			continue
		}
		t, err := loadPackageFile(s.packagePath(digest), digest, true)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			report.Missing = append(report.Missing, e.Name)
		case err != nil:
			report.Invalid[e.Name] = err.Error()
		case t.Modified:
			report.Modified = append(report.Modified, e.Name)
		}
	}
	report.Orphans, err = s.orphans(reg)
	return report, err
}

// orphans returns the paths in the themes directory that reg doesn't use:
// package directories left behind and stray temporary files.
func (s *PluginStore) orphans(reg PluginRegistry) ([]string, error) {
	used := map[string]bool{}
	for _, e := range reg.Plugins {
		if d := e.digestHex(); d != "" {
			used[d] = true
		}
	}
	dirEntries, err := os.ReadDir(s.themesDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var orphans []string
	for _, de := range dirEntries {
		if !used[de.Name()] {
			orphans = append(orphans, filepath.Join(s.themesDir(), de.Name()))
		}
	}
	return orphans, nil
}

// GC removes everything in the themes directory that no installed theme uses,
// and returns the paths removed.
func (s *PluginStore) GC() ([]string, error) {
	pluginStoreMu.Lock()
	defer pluginStoreMu.Unlock()

	reg, err := s.readRegistry()
	if err != nil {
		return nil, err
	}
	orphans, err := s.orphans(reg)
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, path := range orphans {
		if err := os.RemoveAll(path); err != nil {
			return removed, err
		}
		removed = append(removed, path)
	}
	return removed, nil
}
