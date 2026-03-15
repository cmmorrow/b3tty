package src

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLog redirects the standard logger to a buffer for the duration of f
// and returns everything that was logged.
func captureLog(f func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(nil) // restore default (stderr)
		log.SetFlags(log.LstdFlags)
	}()
	f()
	return buf.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestTerminalServer returns a TerminalServer with a fully populated default
// profile and a known token, suitable for use in handler tests.
func newTestTerminalServer() *TerminalServer {
	client := &Client{
		Rows:        24,
		Columns:     80,
		CursorBlink: true,
		FontFamily:  "monospace",
		FontSize:    14,
		Theme:       Theme{},
	}
	server := &Server{
		Uri:  "localhost",
		Port: 8080,
		TLS:  TLS{Enabled: false},
	}
	return &TerminalServer{
		client: client,
		server: server,
		profiles: map[string]Profile{
			"default": {Title: "b3tty", Shell: "/bin/bash"},
			"work":    {Title: "Work Terminal", Shell: "/bin/zsh"},
		},
		token:       "test-token-1234",
		orgCols:     DEFAULT_COLS,
		orgRows:     DEFAULT_ROWS,
		profileName: "default",
		authSleep:   func(time.Duration) {}, // no-op: avoid real delays in tests
	}
}

// queryWith builds a url.Values map from alternating key/value pairs.
func queryWith(pairs ...string) url.Values {
	q := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		q.Set(pairs[i], pairs[i+1])
	}
	return q
}

// ---------------------------------------------------------------------------
// parseSizeParams
// ---------------------------------------------------------------------------

func TestParseSizeParams(t *testing.T) {
	tests := []struct {
		name         string
		query        url.Values
		expectedCols uint16
		expectedRows uint16
	}{
		{
			name:         "valid cols and rows",
			query:        queryWith("cols", "120", "rows", "40"),
			expectedCols: 120,
			expectedRows: 40,
		},
		{
			name:         "default cols and rows when both missing",
			query:        url.Values{},
			expectedCols: DEFAULT_COLS,
			expectedRows: DEFAULT_ROWS,
		},
		{
			name:         "default cols when cols missing",
			query:        queryWith("rows", "30"),
			expectedCols: DEFAULT_COLS,
			expectedRows: 30,
		},
		{
			name:         "default rows when rows missing",
			query:        queryWith("cols", "100"),
			expectedCols: 100,
			expectedRows: DEFAULT_ROWS,
		},
		{
			name:         "default cols when cols is non-numeric",
			query:        queryWith("cols", "abc", "rows", "24"),
			expectedCols: DEFAULT_COLS,
			expectedRows: 24,
		},
		{
			name:         "default rows when rows is non-numeric",
			query:        queryWith("cols", "80", "rows", "xyz"),
			expectedCols: 80,
			expectedRows: DEFAULT_ROWS,
		},
		{
			name:         "default both when both are non-numeric",
			query:        queryWith("cols", "!", "rows", "?"),
			expectedCols: DEFAULT_COLS,
			expectedRows: DEFAULT_ROWS,
		},
		{
			name:         "zero values are accepted as-is",
			query:        queryWith("cols", "0", "rows", "0"),
			expectedCols: 0,
			expectedRows: 0,
		},
		{
			name:         "typical terminal dimensions",
			query:        queryWith("cols", "80", "rows", "24"),
			expectedCols: 80,
			expectedRows: 24,
		},
		{
			name:         "large values fit within uint16",
			query:        queryWith("cols", "65535", "rows", "65535"),
			expectedCols: 65535,
			expectedRows: 65535,
		},
		{
			name:         "values exceeding uint16 max fall back to defaults",
			query:        queryWith("cols", "65536", "rows", "65536"),
			expectedCols: DEFAULT_COLS,
			expectedRows: DEFAULT_ROWS,
		},
		{
			name:         "negative cols fall back to default cols",
			query:        queryWith("cols", "-1", "rows", "24"),
			expectedCols: DEFAULT_COLS,
			expectedRows: 24,
		},
		{
			name:         "negative rows fall back to default rows",
			query:        queryWith("cols", "80", "rows", "-1"),
			expectedCols: 80,
			expectedRows: DEFAULT_ROWS,
		},
		{
			name:         "both negative fall back to defaults",
			query:        queryWith("cols", "-100", "rows", "-50"),
			expectedCols: DEFAULT_COLS,
			expectedRows: DEFAULT_ROWS,
		},
		{
			name:         "floating-point strings fall back to defaults",
			query:        queryWith("cols", "80.5", "rows", "24.0"),
			expectedCols: DEFAULT_COLS,
			expectedRows: DEFAULT_ROWS,
		},
		{
			name:         "empty string values fall back to defaults",
			query:        queryWith("cols", "", "rows", ""),
			expectedCols: DEFAULT_COLS,
			expectedRows: DEFAULT_ROWS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cols, rows := parseSizeParams(tt.query)
			assert.Equal(t, tt.expectedCols, cols, "cols mismatch")
			assert.Equal(t, tt.expectedRows, rows, "rows mismatch")
		})
	}
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
			assert.Equal(t, tt.expected, validateToken(q, tt.serverToken))
		})
	}
}

// ---------------------------------------------------------------------------
// resolveProfileName
// ---------------------------------------------------------------------------

func TestResolveProfileName(t *testing.T) {
	tests := []struct {
		name     string
		query    url.Values
		expected string
	}{
		{
			name:     "profile param present returns its value",
			query:    queryWith("profile", "work"),
			expected: "work",
		},
		{
			name:     "absent profile param returns 'default'",
			query:    url.Values{},
			expected: "default",
		},
		{
			name:     "empty profile param returns 'default'",
			query:    queryWith("profile", ""),
			expected: "default",
		},
		{
			name:     "profile with whitespace is returned as-is",
			query:    queryWith("profile", " dev "),
			expected: " dev ",
		},
		{
			name:     "profile param with other params present",
			query:    queryWith("token", "abc", "profile", "staging"),
			expected: "staging",
		},
		{
			name:     "profile name with hyphens",
			query:    queryWith("profile", "my-profile"),
			expected: "my-profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, resolveProfileName(tt.query))
		})
	}
}

// ---------------------------------------------------------------------------
// buildConfigJSON
// ---------------------------------------------------------------------------

func TestBuildConfigJSON(t *testing.T) {
	srv := &Server{
		Uri:  "localhost",
		Port: 8080,
		TLS:  TLS{Enabled: false},
	}
	clnt := &Client{
		Rows:        24,
		Columns:     80,
		CursorBlink: true,
		FontFamily:  "monospace",
		FontSize:    14,
	}
	thm := &Theme{Foreground: "#ffffff", Background: "#000000"}

	t.Run("returns valid JSON", func(t *testing.T) {
		data, err := buildConfigJSON(srv, clnt, thm)
		require.NoError(t, err)
		assert.True(t, json.Valid(data))
	})

	t.Run("JSON contains expected scalar fields", func(t *testing.T) {
		data, err := buildConfigJSON(srv, clnt, thm)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))

		assert.Equal(t, "localhost", result["uri"])
		assert.Equal(t, float64(8080), result["port"])
		assert.Equal(t, false, result["tls"])
		assert.Equal(t, true, result["cursorBlink"])
		assert.Equal(t, "monospace", result["fontFamily"])
		assert.Equal(t, float64(14), result["fontSize"])
		assert.Equal(t, float64(24), result["rows"])
		assert.Equal(t, float64(80), result["columns"])
	})

	t.Run("JSON embeds theme colours", func(t *testing.T) {
		data, err := buildConfigJSON(srv, clnt, thm)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))

		theme, ok := result["theme"].(map[string]any)
		require.True(t, ok, "theme should be an object")
		assert.Equal(t, "#ffffff", theme["foreground"])
		assert.Equal(t, "#000000", theme["background"])
	})

	t.Run("TLS enabled is reflected in JSON", func(t *testing.T) {
		tlsSrv := &Server{Uri: "example.com", Port: 8443, TLS: TLS{Enabled: true}}
		data, err := buildConfigJSON(tlsSrv, clnt, thm)
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))
		assert.Equal(t, true, result["tls"])
	})

	t.Run("empty theme produces valid JSON", func(t *testing.T) {
		emptyTheme := &Theme{}
		data, err := buildConfigJSON(srv, clnt, emptyTheme)
		require.NoError(t, err)
		assert.True(t, json.Valid(data))
	})
}

// ---------------------------------------------------------------------------
// parseResizeMessage
// ---------------------------------------------------------------------------

func TestParseResizeMessage(t *testing.T) {
	tests := []struct {
		name         string
		message      []byte
		expectedCols uint16
		expectedRows uint16
		expectedOK   bool
	}{
		{
			name:         "valid resize message",
			message:      []byte(`{"type":"resize","cols":120,"rows":40}`),
			expectedCols: 120,
			expectedRows: 40,
			expectedOK:   true,
		},
		{
			name:         "valid resize with typical dimensions",
			message:      []byte(`{"type":"resize","cols":80,"rows":24}`),
			expectedCols: 80,
			expectedRows: 24,
			expectedOK:   true,
		},
		{
			name:         "wrong type is rejected",
			message:      []byte(`{"type":"keyboard","cols":80,"rows":24}`),
			expectedCols: 0,
			expectedRows: 0,
			expectedOK:   false,
		},
		{
			name:         "empty type is rejected",
			message:      []byte(`{"type":"","cols":80,"rows":24}`),
			expectedCols: 0,
			expectedRows: 0,
			expectedOK:   false,
		},
		{
			name:         "missing type field is rejected",
			message:      []byte(`{"cols":80,"rows":24}`),
			expectedCols: 0,
			expectedRows: 0,
			expectedOK:   false,
		},
		{
			name:         "invalid JSON is rejected",
			message:      []byte(`not json at all`),
			expectedCols: 0,
			expectedRows: 0,
			expectedOK:   false,
		},
		{
			name:         "empty message is rejected",
			message:      []byte{},
			expectedCols: 0,
			expectedRows: 0,
			expectedOK:   false,
		},
		{
			name:         "plain keyboard input is rejected",
			message:      []byte("ls -la\r"),
			expectedCols: 0,
			expectedRows: 0,
			expectedOK:   false,
		},
		{
			name:         "resize with zero dimensions is accepted",
			message:      []byte(`{"type":"resize","cols":0,"rows":0}`),
			expectedCols: 0,
			expectedRows: 0,
			expectedOK:   true,
		},
		{
			name:         "resize missing cols and rows defaults to zero",
			message:      []byte(`{"type":"resize"}`),
			expectedCols: 0,
			expectedRows: 0,
			expectedOK:   true,
		},
		{
			name:         "resize with maximum uint16 dimensions",
			message:      []byte(`{"type":"resize","cols":65535,"rows":65535}`),
			expectedCols: 65535,
			expectedRows: 65535,
			expectedOK:   true,
		},
		{
			name:         "extra JSON fields are ignored",
			message:      []byte(`{"type":"resize","cols":100,"rows":50,"extra":"ignored"}`),
			expectedCols: 100,
			expectedRows: 50,
			expectedOK:   true,
		},
		{
			name:         "cols exceeding uint16 max is rejected",
			message:      []byte(`{"type":"resize","cols":65536,"rows":24}`),
			expectedCols: 0,
			expectedRows: 0,
			expectedOK:   false,
		},
		{
			name:         "negative cols is rejected",
			message:      []byte(`{"type":"resize","cols":-1,"rows":24}`),
			expectedCols: 0,
			expectedRows: 0,
			expectedOK:   false,
		},
		{
			name:         "empty JSON object is rejected",
			message:      []byte(`{}`),
			expectedCols: 0,
			expectedRows: 0,
			expectedOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cols, rows, ok := parseResizeMessage(tt.message)
			assert.Equal(t, tt.expectedOK, ok, "ok flag mismatch")
			assert.Equal(t, tt.expectedCols, cols, "cols mismatch")
			assert.Equal(t, tt.expectedRows, rows, "rows mismatch")
		})
	}
}

// ---------------------------------------------------------------------------
// formatCommand
// ---------------------------------------------------------------------------

func TestFormatCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []byte
	}{
		{
			name:     "plain command gets newline appended",
			input:    "ls -la",
			expected: []byte("ls -la\n"),
		},
		{
			name:     "leading whitespace is trimmed",
			input:    "   ls -la",
			expected: []byte("ls -la\n"),
		},
		{
			name:     "trailing whitespace is trimmed",
			input:    "ls -la   ",
			expected: []byte("ls -la\n"),
		},
		{
			name:     "surrounding whitespace is trimmed",
			input:    "  ls -la  ",
			expected: []byte("ls -la\n"),
		},
		{
			name:     "empty string becomes a bare newline",
			input:    "",
			expected: []byte("\n"),
		},
		{
			name:     "whitespace-only string becomes a bare newline",
			input:    "   ",
			expected: []byte("\n"),
		},
		{
			name:     "command with internal spaces preserved",
			input:    "echo hello world",
			expected: []byte("echo hello world\n"),
		},
		{
			name:     "tabs are trimmed from edges but internal ones preserved",
			input:    "\techo\thello\t",
			expected: []byte("echo\thello\n"),
		},
		{
			name:     "command with special shell characters",
			input:    "grep -r 'pattern' ./src",
			expected: []byte("grep -r 'pattern' ./src\n"),
		},
		{
			name:     "single character command",
			input:    "q",
			expected: []byte("q\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatCommand(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ---------------------------------------------------------------------------
// authBackoffDelay
// ---------------------------------------------------------------------------

func TestAuthBackoffDelay(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		expected time.Duration
	}{
		{"zero attempts returns no delay", 0, 0},
		{"negative attempts returns no delay", -1, 0},
		{"1st failure: 1s", 1, 1 * time.Second},
		{"2nd failure: 2s", 2, 2 * time.Second},
		{"3rd failure: 4s", 3, 4 * time.Second},
		{"4th failure: 8s", 4, 8 * time.Second},
		{"5th failure: 16s", 5, 16 * time.Second},
		{"6th failure caps at 30s", 6, 30 * time.Second},
		{"large attempt count caps at 30s", 100, 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, authBackoffDelay(tt.attempts))
		})
	}
}

// ---------------------------------------------------------------------------
// setSizeHandler
// ---------------------------------------------------------------------------

func TestSetSizeHandler(t *testing.T) {
	t.Run("GET is rejected with 405", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/size?cols=80&rows=24", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.setSizeHandler(w, req) })
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Contains(t, logged, "method not allowed")
		// State must not be mutated on error
		assert.Equal(t, uint16(DEFAULT_COLS), ts.orgCols)
		assert.Equal(t, uint16(DEFAULT_ROWS), ts.orgRows)
	})

	t.Run("DELETE is rejected with 405", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodDelete, "/size", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.setSizeHandler(w, req) })
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Contains(t, logged, "method not allowed")
	})

	t.Run("PUT is rejected with 405", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPut, "/size?cols=80&rows=24", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.setSizeHandler(w, req) })
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Contains(t, logged, "method not allowed")
	})

	t.Run("POST with valid params updates orgCols and orgRows", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size?cols=132&rows=50", nil)
		w := httptest.NewRecorder()
		ts.setSizeHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, uint16(132), ts.orgCols)
		assert.Equal(t, uint16(50), ts.orgRows)
	})

	t.Run("POST with missing cols falls back to default", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size?rows=40", nil)
		w := httptest.NewRecorder()
		ts.setSizeHandler(w, req)
		assert.Equal(t, uint16(DEFAULT_COLS), ts.orgCols)
		assert.Equal(t, uint16(40), ts.orgRows)
	})

	t.Run("POST with missing rows falls back to default", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size?cols=100", nil)
		w := httptest.NewRecorder()
		ts.setSizeHandler(w, req)
		assert.Equal(t, uint16(100), ts.orgCols)
		assert.Equal(t, uint16(DEFAULT_ROWS), ts.orgRows)
	})

	t.Run("POST with no params falls back to both defaults", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size", nil)
		w := httptest.NewRecorder()
		ts.setSizeHandler(w, req)
		assert.Equal(t, uint16(DEFAULT_COLS), ts.orgCols)
		assert.Equal(t, uint16(DEFAULT_ROWS), ts.orgRows)
	})

	t.Run("POST with non-numeric cols falls back to default", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size?cols=wide&rows=24", nil)
		w := httptest.NewRecorder()
		ts.setSizeHandler(w, req)
		assert.Equal(t, uint16(DEFAULT_COLS), ts.orgCols)
		assert.Equal(t, uint16(24), ts.orgRows)
	})

	t.Run("POST with zero dimensions stores zero", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size?cols=0&rows=0", nil)
		w := httptest.NewRecorder()
		ts.setSizeHandler(w, req)
		assert.Equal(t, uint16(0), ts.orgCols)
		assert.Equal(t, uint16(0), ts.orgRows)
	})

	t.Run("POST returns no body", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size?cols=80&rows=24", nil)
		w := httptest.NewRecorder()
		ts.setSizeHandler(w, req)
		assert.Empty(t, w.Body.String())
	})

	t.Run("POST with Sec-Fetch-Site same-origin is allowed", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size?cols=132&rows=50", nil)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		ts.setSizeHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, uint16(132), ts.orgCols)
		assert.Equal(t, uint16(50), ts.orgRows)
	})

	t.Run("POST with Sec-Fetch-Site cross-site is rejected with 403", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size?cols=132&rows=50", nil)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.setSizeHandler(w, req) })
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, logged, "forbidden")
		assert.Contains(t, logged, "cross-site")
		// State must not be mutated on error
		assert.Equal(t, uint16(DEFAULT_COLS), ts.orgCols)
		assert.Equal(t, uint16(DEFAULT_ROWS), ts.orgRows)
	})

	t.Run("POST with Sec-Fetch-Site same-site is rejected with 403", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size?cols=132&rows=50", nil)
		req.Header.Set("Sec-Fetch-Site", "same-site")
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.setSizeHandler(w, req) })
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, logged, "forbidden")
		assert.Contains(t, logged, "same-site")
		assert.Equal(t, uint16(DEFAULT_COLS), ts.orgCols)
		assert.Equal(t, uint16(DEFAULT_ROWS), ts.orgRows)
	})

	t.Run("POST without Sec-Fetch-Site (non-browser client) is allowed", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size?cols=100&rows=30", nil)
		w := httptest.NewRecorder()
		ts.setSizeHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, uint16(100), ts.orgCols)
		assert.Equal(t, uint16(30), ts.orgRows)
	})
}

// ---------------------------------------------------------------------------
// displayTermHandler
// ---------------------------------------------------------------------------

func TestDisplayTermHandler(t *testing.T) {
	t.Run("non-root paths return 404 without touching auth", func(t *testing.T) {
		for _, path := range []string{"/favicon.ico", "/apple-touch-icon.png", "/apple-touch-icon-precomposed.png", "/robots.txt"} {
			ts := newTestTerminalServer()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			ts.displayTermHandler(w, req)
			assert.Equal(t, http.StatusNotFound, w.Code, "expected 404 for %s", path)
			assert.Equal(t, 0, ts.failedAttempts, "backoff counter must not increment for %s", path)
		}
	})

	t.Run("missing token returns 403", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.displayTermHandler(w, req) })
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, logged, "forbidden")
	})

	t.Run("wrong token returns 403", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/?token=wrong-token", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.displayTermHandler(w, req) })
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, logged, "forbidden")
	})

	t.Run("correct token returns 200", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("no-auth mode: empty server token accepts request without token param", func(t *testing.T) {
		ts := newTestTerminalServer()
		ts.token = "" // simulate no-auth
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("response body contains window.B3TTY assignment", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		assert.Contains(t, w.Body.String(), "window.B3TTY")
	})

	t.Run("response body contains serialised JSON config", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		body := w.Body.String()
		assert.Contains(t, body, `"uri"`)
		assert.Contains(t, body, `"port"`)
		assert.Contains(t, body, `"fontSize"`)
	})

	t.Run("default profile title is rendered in the page", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		// The default profile title is "b3tty"; the template sets <title>b3tty</title>
		assert.Contains(t, w.Body.String(), "b3tty")
	})

	t.Run("profile param selects an alternative profile", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234&profile=work", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "work", ts.profileName)
		assert.Contains(t, w.Body.String(), "Work Terminal")
	})

	t.Run("absent profile param resets profileName to 'default'", func(t *testing.T) {
		ts := newTestTerminalServer()
		ts.profileName = "work" // pre-set a different profile
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		assert.Equal(t, "default", ts.profileName)
	})

	t.Run("failed attempt increments counter and is logged", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/?token=wrong", nil)
		w := httptest.NewRecorder()
		var logged string
		logged = captureLog(func() { ts.displayTermHandler(w, req) })
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Equal(t, 1, ts.failedAttempts)
		assert.Contains(t, logged, "attempt 1")
	})

	t.Run("successful auth after failures resets counter", func(t *testing.T) {
		ts := newTestTerminalServer()
		ts.failedAttempts = 5

		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 0, ts.failedAttempts)
	})

	t.Run("no-auth mode: token mismatch skips backoff and counter", func(t *testing.T) {
		ts := newTestTerminalServer()
		ts.token = "" // no-auth mode; validateToken always passes with empty server token
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 0, ts.failedAttempts)
	})

	t.Run("unknown profile param returns empty profile (zero value)", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234&profile=nonexistent", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		// Profiles map returns zero-value Profile; handler should still render 200
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "nonexistent", ts.profileName)
	})

	t.Run("response Content-Type is text/html", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		ct := w.Header().Get("Content-Type")
		// text/template writes no explicit Content-Type; Go's http package detects HTML
		assert.Contains(t, ct, "text/html")
	})

	t.Run("response body is valid HTML shell", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		body := w.Body.String()
		assert.Contains(t, body, "<!doctype html>")
		assert.Contains(t, body, `<div id="terminal">`)
	})

	t.Run("TLS flag is reflected in the embedded config", func(t *testing.T) {
		ts := newTestTerminalServer()
		ts.server.TLS.Enabled = true
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		assert.Contains(t, w.Body.String(), `"tls":true`)
	})
}
