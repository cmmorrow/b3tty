package src

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os/exec"
	"runtime"
	"strings"
)

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
