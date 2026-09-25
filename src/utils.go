package src

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os/exec"
	"reflect"
	"regexp"
	"runtime"
	"strings"
)

var (
	reHexColor   = regexp.MustCompile(`^#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?$`)
	reNamedColor = regexp.MustCompile(`^[a-zA-Z]+$`)
)

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

// ValidateTheme checks every color field in t against validateThemeColor.
// It returns an error naming the first invalid field and value, or nil when
// all fields are valid. Fields are identified by their JSON tag name.
func ValidateTheme(t *Theme) error {
	val := reflect.ValueOf(t).Elem()
	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		// BackgroundImage is a file path, not a color — skip it.
		if typ.Field(i).Name == "BackgroundImage" {
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

// validateToken reports whether the token query parameter matches the expected server
// token.
func validateToken(q string, serverToken string) bool {
	return q == serverToken
}
