package src

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateThemeColor(t *testing.T) {
	valid := []string{
		"",               // unset field
		"#14181d",        // 6-digit hex
		"#FFFFFF",        // 6-digit hex uppercase
		"#aB3f9C",        // 6-digit hex mixed case
		"#fff",           // 3-digit shorthand
		"#ABC",           // 3-digit uppercase
		"red",            // named color
		"cornflowerblue", // named color
	}
	for _, c := range valid {
		assert.True(t, ValidateThemeColor(c), "expected valid: %q", c)
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
		assert.False(t, ValidateThemeColor(c), "expected invalid: %q", c)
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

func TestValidatePortNumber(t *testing.T) {
	valid := []int{1, 80, 443, 8080, 8443, 65535}
	for _, p := range valid {
		assert.True(t, ValidatePortNumber(p), "expected valid: %d", p)
	}

	invalid := []int{0, -1, -1000, 65536, 100000}
	for _, p := range invalid {
		assert.False(t, ValidatePortNumber(p), "expected invalid: %d", p)
	}
}

func TestValidateShowMenubar(t *testing.T) {
	valid := []string{"hover", "visible", "disable"}
	for _, v := range valid {
		assert.True(t, ValidateShowMenubar(v), "expected valid: %q", v)
	}

	invalid := []string{"", "Hover", "always", "true"}
	for _, v := range invalid {
		assert.False(t, ValidateShowMenubar(v), "expected invalid: %q", v)
	}
}

func TestValidateTerminalDimension(t *testing.T) {
	assert.True(t, validateTerminalDimension(0))
	assert.True(t, validateTerminalDimension(80))
	assert.True(t, validateTerminalDimension(65535))
	assert.False(t, validateTerminalDimension(-1))
	assert.False(t, validateTerminalDimension(65536))
	assert.False(t, validateTerminalDimension(-1000))
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

// ---------------------------------------------------------------------------
// mustUnmarshalTheme
// ---------------------------------------------------------------------------

func TestMustUnmarshalTheme(t *testing.T) {
	t.Run("valid JSON returns a populated Theme", func(t *testing.T) {
		data := []byte(`{"foreground":"#ffffff","background":"#000000"}`)
		th := mustUnmarshalTheme(data)
		assert.Equal(t, "#ffffff", th.Foreground)
		assert.Equal(t, "#000000", th.Background)
	})

	t.Run("camelCase keys map to their struct fields", func(t *testing.T) {
		data := []byte(`{"brightRed":"#ee837b","selectionBackground":"#404040","cursorAccent":"#15191e"}`)
		th := mustUnmarshalTheme(data)
		assert.Equal(t, "#ee837b", th.BrightRed)
		assert.Equal(t, "#404040", th.SelectionBackground)
		assert.Equal(t, "#15191e", th.CursorAccent)
	})

	t.Run("hyphenated keys are not recognised", func(t *testing.T) {
		// The embedded files use Theme's json tags; the hyphenated form is the
		// YAML config spelling and is deliberately not accepted here.
		th := mustUnmarshalTheme([]byte(`{"bright-red":"#ee837b"}`))
		assert.Empty(t, th.BrightRed)
	})

	t.Run("empty JSON object returns the zero Theme", func(t *testing.T) {
		th := mustUnmarshalTheme([]byte(`{}`))
		assert.Equal(t, Theme{}, th)
	})

	t.Run("background image fields decode through their camelCase json tags", func(t *testing.T) {
		th := mustUnmarshalTheme([]byte(`{"backgroundImage":"/tmp/x.png","backgroundImageTransparency":30}`))
		assert.Equal(t, "/tmp/x.png", th.BackgroundImage)
		require.NotNil(t, th.BackgroundImageTransparency)
		assert.Equal(t, 30, *th.BackgroundImageTransparency)
	})

	t.Run("invalid JSON panics with descriptive message", func(t *testing.T) {
		assert.PanicsWithValue(t,
			"failed to parse embedded theme JSON: invalid character 'b' looking for beginning of object key string",
			func() { mustUnmarshalTheme([]byte(`{bad}`)) },
		)
	})

	t.Run("empty input panics", func(t *testing.T) {
		assert.Panics(t, func() { mustUnmarshalTheme([]byte{}) })
	})
}

// ---------------------------------------------------------------------------
// validateToken
// ---------------------------------------------------------------------------

func TestValidateToken(t *testing.T) {
	tests := []struct {
		name        string
		queryToken  string
		serverToken string
		expected    bool
	}{
		{
			name:        "matching tokens pass",
			queryToken:  "abc123",
			serverToken: "abc123",
			expected:    true,
		},
		{
			name:        "mismatched tokens are rejected",
			queryToken:  "wrong",
			serverToken: "abc123",
			expected:    false,
		},
		{
			name:        "no-auth mode: both empty strings match",
			queryToken:  "",
			serverToken: "",
			expected:    true,
		},
		{
			name:        "token present but server has no-auth empty token",
			queryToken:  "sometoken",
			serverToken: "",
			expected:    false,
		},
		{
			name:        "token absent but server expects a token",
			queryToken:  "",
			serverToken: "expected",
			expected:    false,
		},
		{
			name:        "case-sensitive: differing case is rejected",
			queryToken:  "ABC123",
			serverToken: "abc123",
			expected:    false,
		},
		{
			name:        "long token matches correctly",
			queryToken:  strings.Repeat("x", 256),
			serverToken: strings.Repeat("x", 256),
			expected:    true,
		},
		{
			name:        "token with special characters matches",
			queryToken:  "t0k!@#$%",
			serverToken: "t0k!@#$%",
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := url.Values{}
			if tt.queryToken != "" {
				q.Set("token", tt.queryToken)
			}
			assert.Equal(t, tt.expected, validateToken(q.Get("token"), tt.serverToken))
		})
	}
}

// ---------------------------------------------------------------------------
// validateBackgroundImage
// ---------------------------------------------------------------------------

// Minimal leading bytes http.DetectContentType recognizes for each allowed
// image type. They are not complete, decodable images — the validator only
// sniffs the file's contents, it never decodes them.
var (
	pngMagic  = []byte("\x89PNG\r\n\x1a\n")
	jpegMagic = []byte("\xff\xd8\xff\xe0")
	gifMagic  = []byte("GIF89a")
	webpMagic = []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")
)

// writeTempFile writes data to name inside a fresh temp directory and returns
// the file's absolute path.
func writeTempFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestValidateBackgroundImage(t *testing.T) {
	valid := []struct {
		name string
		file string
		data []byte
	}{
		{"png", "bg.png", pngMagic},
		{"jpg", "bg.jpg", jpegMagic},
		{"jpeg", "bg.jpeg", jpegMagic},
		{"gif", "bg.gif", gifMagic},
		{"webp", "bg.webp", webpMagic},
		{"extension is matched case-insensitively", "BG.PNG", pngMagic},
		{"extension and contents need not name the same type", "bg.jpg", pngMagic},
	}
	for _, tt := range valid {
		t.Run("accepts "+tt.name, func(t *testing.T) {
			assert.NoError(t, validateBackgroundImage(writeTempFile(t, tt.file, tt.data)))
		})
	}

	unsupported := []struct {
		name string
		file string
		data []byte
	}{
		{"text file with a text extension", "notes.txt", []byte("hello")},
		{"image contents with a disallowed extension", "bg.bmp", pngMagic},
		{"no extension", "background", pngMagic},
		{"svg", "bg.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)},
		{"text contents with an image extension", "bg.png", []byte("not really a png")},
		{"empty file with an image extension", "bg.png", nil},
	}
	for _, tt := range unsupported {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			err := validateBackgroundImage(writeTempFile(t, tt.file, tt.data))
			assert.ErrorIs(t, err, errUnsupportedBackgroundImage)
		})
	}

	t.Run("missing file returns the open error, not a type error", func(t *testing.T) {
		err := validateBackgroundImage(filepath.Join(t.TempDir(), "missing.png"))
		assert.ErrorIs(t, err, os.ErrNotExist)
		assert.NotErrorIs(t, err, errUnsupportedBackgroundImage)
	})
}

// ---------------------------------------------------------------------------
// expandHomePath
// ---------------------------------------------------------------------------

func TestExpandHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"bare tilde expands to the home directory", "~", home},
		{"tilde-slash prefix expands", "~/pics/bg.png", filepath.Join(home, "pics", "bg.png")},
		{"absolute path is unchanged", "/srv/bg.png", "/srv/bg.png"},
		{"relative path is unchanged", "pics/bg.png", "pics/bg.png"},
		{"~user form is unchanged", "~alice/bg.png", "~alice/bg.png"},
		{"tilde not at the start is unchanged", "/srv/~/bg.png", "/srv/~/bg.png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandHomePath(tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ---------------------------------------------------------------------------
// ValidateTheme: background image transparency
// ---------------------------------------------------------------------------

func TestValidateThemeBackgroundImageTransparency(t *testing.T) {
	intPtr := func(v int) *int { return &v }

	t.Run("unset transparency is valid", func(t *testing.T) {
		assert.NoError(t, ValidateTheme(&Theme{Foreground: "#fff"}))
	})

	for _, v := range []int{0, 50, 100} {
		t.Run(fmt.Sprintf("accepts %d", v), func(t *testing.T) {
			assert.NoError(t, ValidateTheme(&Theme{Foreground: "#fff", BackgroundImageTransparency: intPtr(v)}))
		})
	}

	for _, v := range []int{-1, 101} {
		t.Run(fmt.Sprintf("rejects %d", v), func(t *testing.T) {
			err := ValidateTheme(&Theme{BackgroundImageTransparency: intPtr(v)})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "backgroundImageTransparency")
		})
	}

	t.Run("still rejects an invalid color when transparency is set", func(t *testing.T) {
		err := ValidateTheme(&Theme{Foreground: "rgb(1,2,3)", BackgroundImageTransparency: intPtr(50)})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "foreground")
	})

	t.Run("background image path is not validated as a color", func(t *testing.T) {
		assert.NoError(t, ValidateTheme(&Theme{BackgroundImage: "/srv/my bg.png"}))
	})
}

// ---------------------------------------------------------------------------
// resolveBackgroundImage
// ---------------------------------------------------------------------------

func TestResolveBackgroundImage(t *testing.T) {
	t.Run("valid absolute path resolves to itself", func(t *testing.T) {
		path := writeTempFile(t, "bg.png", pngMagic)
		got, err := resolveBackgroundImage(path)
		require.NoError(t, err)
		assert.Equal(t, path, got)
	})

	t.Run("leading ~/ is expanded", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		require.NoError(t, os.WriteFile(filepath.Join(home, "bg.png"), pngMagic, 0o600))
		got, err := resolveBackgroundImage("~/bg.png")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(home, "bg.png"), got)
	})

	failures := []struct {
		name        string
		path        func(t *testing.T) string
		reason      string
		unsupported bool
	}{
		{"relative path", func(t *testing.T) string { return "bg.png" }, "must be an absolute path", false},
		{"missing file", func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing.png") }, "no such file or directory", false},
		{"directory", func(t *testing.T) string { return t.TempDir() }, "is a directory", false},
		{"unsupported type", func(t *testing.T) string { return writeTempFile(t, "notes.txt", []byte("hi")) }, "unsupported background image type", true},
	}
	for _, tt := range failures {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			_, err := resolveBackgroundImage(tt.path(t))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.reason)
			assert.Equal(t, tt.unsupported, errors.Is(err, errUnsupportedBackgroundImage))
		})
	}
}

// ---------------------------------------------------------------------------
// redactedURL
// ---------------------------------------------------------------------------

func TestRedactedURL(t *testing.T) {
	parse := func(raw string) *url.URL {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		return u
	}

	t.Run("replaces the token and keeps the other parameters", func(t *testing.T) {
		got := redactedURL(parse("/ws?cols=80&rows=24&token=s3cret"))
		assert.NotContains(t, got, "s3cret")
		assert.Contains(t, got, "token=REDACTED")
		assert.Contains(t, got, "cols=80")
		assert.Contains(t, got, "rows=24")
	})

	t.Run("leaves a URL without a token unchanged", func(t *testing.T) {
		assert.Equal(t, "/ws?cols=80&rows=24", redactedURL(parse("/ws?cols=80&rows=24")))
	})

	t.Run("does not modify the original URL", func(t *testing.T) {
		u := parse("/?token=s3cret")
		redactedURL(u)
		assert.Equal(t, "token=s3cret", u.RawQuery)
	})
}
