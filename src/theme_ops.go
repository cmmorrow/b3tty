package src

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// The theme operations below are shared by the HTTP handlers and the b3tty
// theme CLI commands, so each rule lives in one place. They change only what
// is on disk — the config file and the plugin store; the handlers then update
// the server's in-memory state, and the CLI asks a running server to reload.

// ImportThemePackage installs the .b3theme file at path (a leading "~" is
// expanded, and the result must be absolute) into store; see
// PluginStore.Install for the rules, isUserTheme, and replace.
func ImportThemePackage(store *PluginStore, path string, isUserTheme func(string) bool, replace bool) (InstallResult, error) {
	if store == nil {
		return InstallResult{}, errors.New("the plugin store is not available")
	}
	resolved, err := expandHomePath(path)
	if err != nil {
		return InstallResult{}, fmt.Errorf("package path %q: %w", path, err)
	}
	if !filepath.IsAbs(resolved) {
		return InstallResult{}, fmt.Errorf("package path %q must be an absolute path", path)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return InstallResult{}, fmt.Errorf("package: %w", err)
	}
	if !info.Mode().IsRegular() {
		return InstallResult{}, fmt.Errorf("%s is not a file", resolved)
	}
	if info.Size() > MAX_THEME_PACKAGE_SIZE {
		return InstallResult{}, fmt.Errorf("%s is larger than the %d MB limit", resolved, MAX_THEME_PACKAGE_SIZE>>20)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return InstallResult{}, fmt.Errorf("package: %w", err)
	}
	return store.Install(data, isUserTheme, replace)
}

// CopyTheme saves source, the definition of a theme of any source, as the new
// user theme newName in the config file at configPath, and adds it to the
// Themes menu without activating it. The copy is a new theme: it has no
// package details, so its first export gets a new package id. image and
// imageExt are an installed theme's background image, which a user theme can
// only reference as a file, so it is first written to a new file under
// ~/.config/b3tty/backgrounds (see extractBackgroundImage). The caller checks
// that newName names no existing theme. It returns the theme saved.
func CopyTheme(configPath, newName string, source Theme, image []byte, imageExt string) (Theme, error) {
	if err := validateThemeName(newName); err != nil {
		return Theme{}, err
	}
	theme := source
	theme.Package = nil
	var extracted string
	if len(image) > 0 {
		var err error
		if extracted, err = extractBackgroundImage(newName, imageExt, image); err != nil {
			return Theme{}, err
		}
		theme.BackgroundImage = extracted
	}
	if err := AddThemeToConfig(configPath, newName, theme); err != nil {
		if extracted != "" {
			_ = os.Remove(extracted)
		}
		return Theme{}, err
	}
	return theme, nil
}

// extractBackgroundImage writes data, an image with extension ext, to a new
// file named after the theme in ~/.config/b3tty/backgrounds (created if
// needed) and returns its path. An existing file is never overwritten: a
// numeric suffix is added instead (name-2.png, name-3.png, …).
func extractBackgroundImage(themeName, ext string, data []byte) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, DOT_CONFIG_PATH, B3TTY_CONFIG_PATH, BACKGROUNDS_DIR_NAME)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("background image: %w", err)
	}
	for i := 1; i <= 100; i++ {
		name := themeName
		if i > 1 {
			name += "-" + strconv.Itoa(i)
		}
		path := filepath.Join(dir, name+ext)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, fs.ErrExist) {
			continue
		} else if err != nil {
			return "", fmt.Errorf("background image: %w", err)
		}
		_, err = f.Write(data)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(path)
			return "", fmt.Errorf("background image: %w", err)
		}
		return path, nil
	}
	return "", fmt.Errorf("background image: too many files named after %q in %s", themeName, dir)
}

// DeleteTheme permanently deletes the theme name, whose source is source:
//
//   - a user theme is removed from the config file
//   - an installed theme is uninstalled from store, then removed from the
//     config file's Themes menu and theme/default-theme keys
//   - a built-in theme can't be deleted, so it is only removed from the
//     Themes menu
//
// See DeleteThemeFromConfig and RemoveThemeFromMenuInConfig.
func DeleteTheme(configPath string, store *PluginStore, name string, source ThemeSource) error {
	switch source {
	case ThemeSourceUser:
		return DeleteThemeFromConfig(configPath, name)
	case ThemeSourceInstalled:
		if store == nil {
			return errors.New("the plugin store is not available")
		}
		if err := store.Uninstall(name); err != nil {
			return err
		}
		if err := DeleteThemeFromConfig(configPath, name); err != nil {
			return fmt.Errorf("uninstalled %q, but updating the config file failed: %w", name, err)
		}
		return nil
	case ThemeSourceBuiltin:
		return RemoveThemeFromMenuInConfig(configPath, name)
	}
	return fmt.Errorf("unknown theme source %q", source)
}
