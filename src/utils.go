package src

import (
	"crypto/rand"
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

// validateThemeColor reports whether s is a valid theme color value. Valid
// values are an empty string (field not set), a CSS hex color with 3 or 6
// hex digits (e.g. "#fff" or "#14181d"), or a string of only ASCII letters
// representing a named CSS color (e.g. "red" or "cornflowerblue").
func validateThemeColor(s string) bool {
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
		color := val.Field(i).String()
		if !validateThemeColor(color) {
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

// convertToFieldName converts a hyphenated string to a camelCase field name.
// It splits the input string by hyphens, capitalizes the first letter of each part,
// and then joins the parts together without spaces.
//
// Parameters:
//   - key: The input string to be converted (e.g., "user-first-name")
//
// Returns:
//   - A string representing the converted field name (e.g., "UserFirstName")
func convertToFieldName(key string) string {
	parts := strings.Split(key, "-")
	for i, part := range parts {
		parts[i] = strings.Title(part)
	}
	return strings.Join(parts, "")
}

// sum calculates the sum of all integers in the given array.
//
// Parameters:
//   - arr: A slice of integers to be summed.
//
// Returns:
//   - An integer representing the sum of all elements in the input array.
func sum(arr []int) int {
	total := 0
	for _, i := range arr {
		total += i
	}
	return total
}

// openBrowser attempts to open the specified URL in the default web browser
// based on the operating system.
//
// Parameters:
//   - url: A string representing the URL to be opened in the browser.
//
// Returns:
//   - An error if the browser couldn't be opened or if the platform is unsupported.
//     Returns nil if the browser was successfully opened.
func openBrowser(url string) error {
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

// generateToken generates a random token of the specified length using alphanumeric characters.
//
// Parameters:
//   - length: An integer specifying the desired length of the token.
//
// Returns:
//   - A string containing the generated token.
//   - An error if there was a problem generating random numbers.
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
