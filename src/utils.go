package src

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	reHexColor   = regexp.MustCompile(`^#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?$`)
	reNamedColor = regexp.MustCompile(`^[a-zA-Z]+$`)
	reThemeID    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// backgroundImageExts and backgroundImageTypes are the allowlists a theme's
// background-image must pass: the file extension (compared case-insensitively)
// and the MIME type http.DetectContentType reports for the file's contents.
// SVG is deliberately absent: DetectContentType reports it as text, and SVG
// documents can carry script.
var (
	backgroundImageExts  = []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}
	backgroundImageTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}
)

// errUnsupportedBackgroundImage is wrapped by validateBackgroundImage when a
// file's extension or contents are not an allowed image type, so callers can
// tell a bad file type apart from a failure to open or read the file.
var errUnsupportedBackgroundImage = errors.New("unsupported background image type")

// mustUnmarshalTheme decodes an embedded JSON theme file into a Theme. The
// files use the camelCase key names from Theme's own json tags, so they decode
// directly with no intermediate map or key translation. It panics on error
// since theme files are embedded at compile time and must always be valid.
func mustUnmarshalTheme(data []byte) Theme {
	var t Theme
	if err := json.Unmarshal(data, &t); err != nil {
		panic("failed to parse embedded theme JSON: " + err.Error())
	}
	return t
}

// validateThemeColor reports whether s is a valid theme color value. Valid
// values are an empty string (field not set), a CSS hex color with 3 or 6
// hex digits (e.g. "#fff" or "#14181d"), or a string of only ASCII letters
// representing a named CSS color (e.g. "red" or "cornflowerblue").
func ValidateThemeColor(s string) bool {
	if s == "" {
		return true
	}
	return reHexColor.MatchString(s) || reNamedColor.MatchString(s)
}

// ValidateTheme checks every color field in t against validateThemeColor, and
// that BackgroundImageTransparency, when set, is within 0–100. It returns an
// error naming the first invalid field and value, or nil when all fields are
// valid. Fields are identified by their JSON tag name. The background image
// path is not checked here: whether it resolves to a valid image depends on
// the filesystem, not the config (see resolveBackgroundImage).
func ValidateTheme(t *Theme) error {
	if tr := t.BackgroundImageTransparency; tr != nil && (*tr < 0 || *tr > 100) {
		return fmt.Errorf("invalid backgroundImageTransparency: %d (must be between 0 and 100)", *tr)
	}
	val := reflect.ValueOf(t).Elem()
	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		// Only string fields hold colors; BackgroundImage is a file path, not a color.
		if val.Field(i).Kind() != reflect.String || typ.Field(i).Name == "BackgroundImage" {
			continue
		}
		color := val.Field(i).String()
		if !ValidateThemeColor(color) {
			tag := typ.Field(i).Tag.Get("json")
			name := strings.Split(tag, ",")[0]
			return fmt.Errorf("invalid theme color for %s: %q", name, color)
		}
	}
	return nil
}

// validateBackgroundImage checks that the file at path is an allowed background
// image: its extension must be in backgroundImageExts, and http.DetectContentType
// must identify its first 512 bytes as one of backgroundImageTypes. Type failures
// wrap errUnsupportedBackgroundImage; failures to open or read the file are
// returned as-is. It does not check that path is absolute or a regular file.
func validateBackgroundImage(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	if err := checkBackgroundImageExt(ext); err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// DetectContentType considers at most the first 512 bytes. A shorter file
	// yields io.ErrUnexpectedEOF (or io.EOF when empty), which is not an error here.
	buf := make([]byte, 512)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return err
	}
	return checkBackgroundImageContent(buf[:n])
}

// validateBackgroundImageData is validateBackgroundImage for an image held in
// memory, such as one read from a theme package: ext is its lowercased file
// extension, with the dot.
func validateBackgroundImageData(ext string, data []byte) error {
	if err := checkBackgroundImageExt(ext); err != nil {
		return err
	}
	return checkBackgroundImageContent(data)
}

// checkBackgroundImageExt checks a lowercased extension against
// backgroundImageExts.
func checkBackgroundImageExt(ext string) error {
	if !slices.Contains(backgroundImageExts, ext) {
		return fmt.Errorf("%w: extension %q is not one of %s", errUnsupportedBackgroundImage, ext, strings.Join(backgroundImageExts, ", "))
	}
	return nil
}

// checkBackgroundImageContent checks that http.DetectContentType, which looks
// at no more than the first 512 bytes, finds an allowed image type in data.
func checkBackgroundImageContent(data []byte) error {
	contentType := http.DetectContentType(data)
	if !slices.Contains(backgroundImageTypes, contentType) {
		return fmt.Errorf("%w: contents detected as %q", errUnsupportedBackgroundImage, contentType)
	}
	return nil
}

// validateThemeName checks a theme name that b3tty didn't get from its own
// config file: the name in a theme package's manifest, or the name of a copy.
// It must be non-empty, at most MAX_THEME_NAME_LENGTH characters, without
// surrounding whitespace, a "/", or control characters. Its errors are
// user-facing.
func validateThemeName(name string) error {
	switch {
	case name == "":
		return errors.New("a theme name is required")
	case strings.TrimSpace(name) != name:
		return fmt.Errorf("theme name %q has leading or trailing spaces", name)
	case utf8.RuneCountInString(name) > MAX_THEME_NAME_LENGTH:
		return fmt.Errorf("theme name %q is longer than %d characters", name, MAX_THEME_NAME_LENGTH)
	case strings.Contains(name, "/"):
		return fmt.Errorf("theme name %q can't contain a /", name)
	case strings.IndexFunc(name, unicode.IsControl) >= 0:
		return fmt.Errorf("theme name %q can't contain control characters", name)
	}
	return nil
}

// resolveBackgroundImage resolves a theme's configured background-image path to
// the file it names and checks that the file is an allowed image: a leading "~"
// is expanded, the result must be absolute, and it must be an existing regular
// file that passes validateBackgroundImage. It returns the resolved path, or an
// error whose text explains the problem; type failures wrap
// errUnsupportedBackgroundImage so callers can tell them apart.
func resolveBackgroundImage(configured string) (string, error) {
	imagePath, err := expandHomePath(configured)
	if err != nil {
		return "", fmt.Errorf("background image %q: %w", configured, err)
	}
	if !filepath.IsAbs(imagePath) {
		return "", fmt.Errorf("background image %q must be an absolute path", configured)
	}
	info, err := os.Stat(imagePath)
	if err != nil {
		return "", fmt.Errorf("background image: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("background image %q is a directory", imagePath)
	}
	if err := validateBackgroundImage(imagePath); err != nil {
		return "", fmt.Errorf("background image %q: %w", imagePath, err)
	}
	return imagePath, nil
}

// expandHomePath replaces a leading "~" or "~/" in path with the current user's
// home directory. Any other path, including the "~user/..." form, is returned
// unchanged.
func expandHomePath(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[1:]), nil
}

// validateTerminalDimension reports whether dim is a valid terminal dimension.
// A valid dimension is a non-negative integer that fits within a uint16 (0–65535),
// matching the range accepted by the pty resize API.
func validateTerminalDimension(dim int) bool {
	if dim < 0 || dim > math.MaxUint16 {
		return false
	}
	return true
}

// ValidatePortNumber reports whether port is a valid TCP/UDP port number (1–65535).
func ValidatePortNumber(port int) bool {
	if port < 1 || port > 65535 {
		return false
	}
	return true
}

// ValidateShowMenubar reports whether v is one of the allowed show-menubar
// values: "hover", "visible", or "disable".
func ValidateShowMenubar(v string) bool {
	switch v {
	case SHOW_MENUBAR_HOVER, SHOW_MENUBAR_VISIBLE, SHOW_MENUBAR_DISABLE:
		return true
	}
	return false
}

// OpenBrowser attempts to open url in the system default browser using the
// appropriate OS command. It returns an error if the command fails or the
// platform is unsupported.
func OpenBrowser(url string) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		return err
	}
	return nil
}

// generateToken returns a cryptographically random alphanumeric string of the
// given length. It returns an error if length is negative or if the underlying
// random number generator fails.
func generateToken(length int) (string, error) {
	if length < 0 {
		return "", errors.New("generate token: length is less than zero")
	}
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	charsetLength := big.NewInt(int64(len(charset)))
	for i := range result {
		randomInt, err := rand.Int(rand.Reader, charsetLength)
		if err != nil {
			return "", err
		}
		result[i] = charset[randomInt.Int64()]
	}
	return string(result), nil
}

// newThemeID returns a random (version 4) UUID in its lowercase canonical form,
// used as a theme package's id: its identity across exported versions.
func newThemeID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate theme id: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// isThemeID reports whether s is a UUID in lowercase canonical form, the shape
// newThemeID produces.
func isThemeID(s string) bool {
	return reThemeID.MatchString(s)
}

// validateRepoURL checks that s is an absolute https URL with a host, the form
// a theme package's repo must take. Only https is accepted so the link the UI
// renders from it can never be a javascript: or other non-web URL.
func validateRepoURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("repo must be an https URL, got %q", s)
	}
	return nil
}

// redactedURL returns u as a string with the value of any "token" query
// parameter replaced, for request logging: the page and WebSocket URLs carry
// the auth token, which must not end up in logs that get shared.
func redactedURL(u *url.URL) string {
	q := u.Query()
	if !q.Has("token") {
		return u.String()
	}
	q.Set("token", "REDACTED")
	redacted := *u
	redacted.RawQuery = q.Encode()
	return redacted.String()
}

// validateToken reports whether the token sent with a request matches the
// expected server token. The comparison is constant-time so response timing
// can't reveal how much of a guess was right.
func validateToken(q string, serverToken string) bool {
	return subtle.ConstantTimeCompare([]byte(q), []byte(serverToken)) == 1
}
