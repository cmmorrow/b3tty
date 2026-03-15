package src

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateThemeColor(t *testing.T) {
	valid := []string{
		"",             // unset field
		"#14181d",      // 6-digit hex
		"#FFFFFF",      // 6-digit hex uppercase
		"#aB3f9C",      // 6-digit hex mixed case
		"#fff",         // 3-digit shorthand
		"#ABC",         // 3-digit uppercase
		"red",          // named color
		"cornflowerblue", // named color
	}
	for _, c := range valid {
		assert.True(t, validateThemeColor(c), "expected valid: %q", c)
	}

	invalid := []string{
		"#14181",        // 5 hex digits
		"#1418",         // 4 hex digits
		"#14181d1",      // 7 hex digits
		"14181d",        // missing #
		"#gggggg",       // invalid hex chars
		"#",             // bare hash
		"red blue",      // space in named color
		"#ff0000 extra", // trailing content
		"rgb(0,0,0)",    // CSS function notation
	}
	for _, c := range invalid {
		assert.False(t, validateThemeColor(c), "expected invalid: %q", c)
	}
}

func TestValidateTheme(t *testing.T) {
	t.Run("empty theme is valid", func(t *testing.T) {
		assert.NoError(t, ValidateTheme(&Theme{}))
	})

	t.Run("all valid hex colors passes", func(t *testing.T) {
		thm := &Theme{
			Foreground: "#14181d",
			Background: "#ffffff",
			Red:        "#ff0000",
			BrightRed:  "#ff5555",
		}
		assert.NoError(t, ValidateTheme(thm))
	})

	t.Run("named colors are accepted", func(t *testing.T) {
		thm := &Theme{Foreground: "white", Background: "black"}
		assert.NoError(t, ValidateTheme(thm))
	})

	t.Run("invalid color returns error with field name", func(t *testing.T) {
		thm := &Theme{Foreground: "#14181d", Background: "not#valid"}
		err := ValidateTheme(thm)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "background")
		assert.Contains(t, err.Error(), "not#valid")
	})

	t.Run("invalid color in non-first field is caught", func(t *testing.T) {
		thm := &Theme{Foreground: "#ffffff", BrightRed: "12345"}
		err := ValidateTheme(thm)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "brightRed")
	})
}

func TestValidateTerminalDimension(t *testing.T) {
	assert.True(t, validateTerminalDimension(0))
	assert.True(t, validateTerminalDimension(80))
	assert.True(t, validateTerminalDimension(65535))
	assert.False(t, validateTerminalDimension(-1))
	assert.False(t, validateTerminalDimension(65536))
	assert.False(t, validateTerminalDimension(-1000))
}

func TestConvertToFieldName(t *testing.T) {
	assert.Equal(t, "UserFirstName", convertToFieldName("user-first-name"))
	assert.Equal(t, "Id", convertToFieldName("id"))
	assert.Equal(t, "LongHyphenatedString", convertToFieldName("long-hyphenated-string"))
	assert.Equal(t, "", convertToFieldName(""))
}

func TestSum(t *testing.T) {
	assert.Equal(t, 15, sum([]int{1, 2, 3, 4, 5}))
	assert.Equal(t, 0, sum([]int{}))
	assert.Equal(t, -5, sum([]int{-1, -2, -3, 1}))
	assert.Equal(t, 0, sum([]int{-1, 1}))
}

func TestGenerateToken(t *testing.T) {
	token, err := generateToken(10)
	assert.NoError(t, err)
	assert.Len(t, token, 10)

	token2, err := generateToken(10)
	assert.NoError(t, err)
	assert.NotEqual(t, token, token2)

	emptyToken, err := generateToken(0)
	assert.NoError(t, err)
	assert.Len(t, emptyToken, 0)

	_, err = generateToken(-3)
	assert.Error(t, errors.New("generate token: length is less than zero"))

	longToken, err := generateToken(1000)
	assert.NoError(t, err)
	assert.Len(t, longToken, 1000)
}
