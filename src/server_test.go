package src

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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
		log.SetOutput(os.Stderr) // restore default (stderr)
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
		Client: client,
		Server: server,
		Profiles: map[string]Profile{
			"default": {Title: "b3tty", Shell: "/bin/bash"},
			"work":    {Title: "Work Terminal", Shell: "/bin/zsh"},
		},
		Token:          "test-token-1234",
		OrgCols:        DEFAULT_COLS,
		OrgRows:        DEFAULT_ROWS,
		ProfileName:    DEFAULT_PROFILE_NAME,
		StartupProfile: DEFAULT_PROFILE_NAME,
		AuthSleep:      func(time.Duration) {}, // no-op: avoid real delays in tests
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
// logProfileURLs
// ---------------------------------------------------------------------------

func TestLogProfileURLs(t *testing.T) {
	profiles := map[string]Profile{
		DEFAULT_PROFILE_NAME: {Shell: "/bin/bash", WorkingDirectory: "/home/user"},
		"work":               {Shell: "/bin/zsh", WorkingDirectory: "/home/user/work"},
		"dev":                {Shell: "/bin/fish", WorkingDirectory: "/home/user/dev"},
	}

	t.Run("no-auth no startup profile uses ? separator", func(t *testing.T) {
		logged := captureLog(func() {
			logProfileURLs(profiles, "http://localhost:8080/")
		})
		assert.Contains(t, logged, "?profile=dev")
		assert.Contains(t, logged, "?profile=work")
		assert.NotContains(t, logged, "&profile=")
	})

	t.Run("token present no startup profile uses & separator", func(t *testing.T) {
		logged := captureLog(func() {
			logProfileURLs(profiles, "http://localhost:8080/?token=abc")
		})
		assert.Contains(t, logged, "?token=abc&profile=dev")
		assert.Contains(t, logged, "?token=abc&profile=work")
	})

	t.Run("token and startup profile present uses URL as-is without duplicate profile param", func(t *testing.T) {
		// When uiUrl already contains &profile=, prfQuery is empty and the URL is used
		// verbatim — this prevents a duplicate &profile=work&profile=work in the output.
		logged := captureLog(func() {
			logProfileURLs(profiles, "http://localhost:8080/?token=abc&profile=work")
		})
		assert.NotContains(t, logged, "&profile=work&profile=work")
	})

	t.Run("no-auth startup profile as first query param does not append &profile=", func(t *testing.T) {
		// When uiUrl starts with ?profile= (no token), the profile param is already the
		// first query param. prfQuery must be "" so no &profile= is appended, which would
		// otherwise produce malformed URLs like ?profile=work&profile=dev.
		logged := captureLog(func() {
			logProfileURLs(profiles, "http://localhost:8080/?profile=work")
		})
		assert.NotContains(t, logged, "?profile=work&profile=")
	})

	t.Run("default profile is excluded from output", func(t *testing.T) {
		logged := captureLog(func() {
			logProfileURLs(profiles, "http://localhost:8080/")
		})
		assert.NotContains(t, logged, "?profile="+DEFAULT_PROFILE_NAME)
	})

	t.Run("non-default profiles are printed in sorted order", func(t *testing.T) {
		logged := captureLog(func() {
			logProfileURLs(profiles, "http://localhost:8080/")
		})
		devIdx := strings.Index(logged, "dev")
		workIdx := strings.Index(logged, "work")
		assert.Less(t, devIdx, workIdx)
	})

	t.Run("shell and working directory are included in output", func(t *testing.T) {
		logged := captureLog(func() {
			logProfileURLs(profiles, "http://localhost:8080/")
		})
		assert.Contains(t, logged, "/bin/zsh")
		assert.Contains(t, logged, "/home/user/work")
		assert.Contains(t, logged, "/bin/fish")
		assert.Contains(t, logged, "/home/user/dev")
	})

	t.Run("configured profiles header is logged", func(t *testing.T) {
		logged := captureLog(func() {
			logProfileURLs(profiles, "http://localhost:8080/")
		})
		assert.Contains(t, logged, "Configured profiles:")
	})
}

// ---------------------------------------------------------------------------
// resolveProfileName
// ---------------------------------------------------------------------------

func TestResolveProfileName(t *testing.T) {
	tests := []struct {
		name     string
		query    url.Values
		profiles map[string]Profile
		expected string
	}{
		{
			name:     "profile param present returns its value",
			query:    queryWith("profile", "work"),
			profiles: map[string]Profile{"work": {}},
			expected: "work",
		},
		{
			name:     "absent profile param returns default",
			query:    url.Values{},
			profiles: map[string]Profile{},
			expected: "default",
		},
		{
			name:     "absent profile param returns default despite StartupProfile",
			query:    url.Values{},
			profiles: map[string]Profile{"work": {}},
			expected: "default",
		},
		{
			name:     "empty profile param returns default",
			query:    queryWith("profile", ""),
			profiles: map[string]Profile{},
			expected: "default",
		},
		{
			name:     "unknown profile returns default",
			query:    queryWith("profile", " dev "),
			profiles: map[string]Profile{},
			expected: "default",
		},
		{
			name:     "profile param with other params present",
			query:    queryWith("token", "abc", "profile", "staging"),
			profiles: map[string]Profile{"staging": {}},
			expected: "staging",
		},
		{
			name:     "profile name with hyphens",
			query:    queryWith("profile", "my-profile"),
			profiles: map[string]Profile{"my-profile": {}},
			expected: "my-profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, resolveProfileName(tt.query, tt.profiles))
		})
	}
}

// ---------------------------------------------------------------------------
// buildUIUrl
// ---------------------------------------------------------------------------

func TestBuildUIUrl(t *testing.T) {
	tests := []struct {
		name           string
		protocol       string
		addr           string
		tokenQuery     string
		startupProfile string
		expected       string
	}{
		{
			name:           "http with token, default profile",
			protocol:       "http",
			addr:           "localhost:8080",
			tokenQuery:     "?token=abc123",
			startupProfile: DEFAULT_PROFILE_NAME,
			expected:       "http://localhost:8080/?token=abc123",
		},
		{
			name:           "http no-auth, default profile",
			protocol:       "http",
			addr:           "localhost:8080",
			tokenQuery:     "",
			startupProfile: DEFAULT_PROFILE_NAME,
			expected:       "http://localhost:8080/",
		},
		{
			name:           "https with token, default profile",
			protocol:       "https",
			addr:           "localhost:8443",
			tokenQuery:     "?token=abc123",
			startupProfile: DEFAULT_PROFILE_NAME,
			expected:       "https://localhost:8443/?token=abc123",
		},
		{
			name:           "http with token, non-default profile appends &profile=",
			protocol:       "http",
			addr:           "localhost:8080",
			tokenQuery:     "?token=abc123",
			startupProfile: "work",
			expected:       "http://localhost:8080/?token=abc123&profile=work",
		},
		{
			name:           "http no-auth, non-default profile appends ?profile=",
			protocol:       "http",
			addr:           "localhost:8080",
			tokenQuery:     "",
			startupProfile: "work",
			expected:       "http://localhost:8080/?profile=work",
		},
		{
			name:           "https no-auth, non-default profile appends ?profile=",
			protocol:       "https",
			addr:           "localhost:8443",
			tokenQuery:     "",
			startupProfile: "dev",
			expected:       "https://localhost:8443/?profile=dev",
		},
		{
			name:           "profile name with hyphens",
			protocol:       "http",
			addr:           "localhost:8080",
			tokenQuery:     "?token=xyz",
			startupProfile: "my-profile",
			expected:       "http://localhost:8080/?token=xyz&profile=my-profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, buildUIUrl(tt.protocol, tt.addr, tt.tokenQuery, tt.startupProfile))
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
		data, err := buildConfigJSON(srv, clnt, thm, nil, nil, nil, "")
		require.NoError(t, err)
		assert.True(t, json.Valid(data))
	})

	t.Run("JSON contains expected scalar fields", func(t *testing.T) {
		data, err := buildConfigJSON(srv, clnt, thm, nil, nil, nil, "")
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
		data, err := buildConfigJSON(srv, clnt, thm, nil, nil, nil, "")
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
		data, err := buildConfigJSON(tlsSrv, clnt, thm, nil, nil, nil, "")
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))
		assert.Equal(t, true, result["tls"])
	})

	t.Run("empty theme produces valid JSON", func(t *testing.T) {
		emptyTheme := &Theme{}
		data, err := buildConfigJSON(srv, clnt, emptyTheme, nil, nil, nil, "")
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
		assert.Equal(t, uint16(DEFAULT_COLS), ts.OrgCols)
		assert.Equal(t, uint16(DEFAULT_ROWS), ts.OrgRows)
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
		assert.Equal(t, uint16(132), ts.OrgCols)
		assert.Equal(t, uint16(50), ts.OrgRows)
	})

	t.Run("POST with missing cols falls back to default", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size?rows=40", nil)
		w := httptest.NewRecorder()
		ts.setSizeHandler(w, req)
		assert.Equal(t, uint16(DEFAULT_COLS), ts.OrgCols)
		assert.Equal(t, uint16(40), ts.OrgRows)
	})

	t.Run("POST with missing rows falls back to default", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size?cols=100", nil)
		w := httptest.NewRecorder()
		ts.setSizeHandler(w, req)
		assert.Equal(t, uint16(100), ts.OrgCols)
		assert.Equal(t, uint16(DEFAULT_ROWS), ts.OrgRows)
	})

	t.Run("POST with no params falls back to both defaults", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size", nil)
		w := httptest.NewRecorder()
		ts.setSizeHandler(w, req)
		assert.Equal(t, uint16(DEFAULT_COLS), ts.OrgCols)
		assert.Equal(t, uint16(DEFAULT_ROWS), ts.OrgRows)
	})

	t.Run("POST with non-numeric cols falls back to default", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size?cols=wide&rows=24", nil)
		w := httptest.NewRecorder()
		ts.setSizeHandler(w, req)
		assert.Equal(t, uint16(DEFAULT_COLS), ts.OrgCols)
		assert.Equal(t, uint16(24), ts.OrgRows)
	})

	t.Run("POST with zero dimensions stores zero", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size?cols=0&rows=0", nil)
		w := httptest.NewRecorder()
		ts.setSizeHandler(w, req)
		assert.Equal(t, uint16(0), ts.OrgCols)
		assert.Equal(t, uint16(0), ts.OrgRows)
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
		assert.Equal(t, uint16(132), ts.OrgCols)
		assert.Equal(t, uint16(50), ts.OrgRows)
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
		assert.Equal(t, uint16(DEFAULT_COLS), ts.OrgCols)
		assert.Equal(t, uint16(DEFAULT_ROWS), ts.OrgRows)
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
		assert.Equal(t, uint16(DEFAULT_COLS), ts.OrgCols)
		assert.Equal(t, uint16(DEFAULT_ROWS), ts.OrgRows)
	})

	t.Run("POST without Sec-Fetch-Site (non-browser client) is allowed", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/size?cols=100&rows=30", nil)
		w := httptest.NewRecorder()
		ts.setSizeHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, uint16(100), ts.OrgCols)
		assert.Equal(t, uint16(30), ts.OrgRows)
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
			assert.Equal(t, 0, ts.FailedAttempts, "backoff counter must not increment for %s", path)
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
		ts.Token = "" // simulate no-auth
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
		assert.Equal(t, "work", ts.ProfileName)
		assert.Contains(t, w.Body.String(), "work")
	})

	t.Run("absent profile param falls back to StartupProfile", func(t *testing.T) {
		ts := newTestTerminalServer()
		// StartupProfile defaults to DEFAULT_PROFILE_NAME; ProfileName should match.
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		assert.Equal(t, DEFAULT_PROFILE_NAME, ts.ProfileName)
	})

	t.Run("absent profile param uses default despite StartupProfile", func(t *testing.T) {
		ts := newTestTerminalServer()
		ts.StartupProfile = "work"
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		assert.Equal(t, DEFAULT_PROFILE_NAME, ts.ProfileName)
	})

	t.Run("failed attempt increments counter and is logged", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/?token=wrong", nil)
		w := httptest.NewRecorder()
		var logged string
		logged = captureLog(func() { ts.displayTermHandler(w, req) })
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Equal(t, 1, ts.FailedAttempts)
		assert.Contains(t, logged, "attempt 1")
	})

	t.Run("successful auth after failures resets counter", func(t *testing.T) {
		ts := newTestTerminalServer()
		ts.FailedAttempts = 5

		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 0, ts.FailedAttempts)
	})

	t.Run("no-auth mode: token mismatch skips backoff and counter", func(t *testing.T) {
		ts := newTestTerminalServer()
		ts.Token = "" // no-auth mode; validateToken always passes with empty server token
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 0, ts.FailedAttempts)
	})

	t.Run("unknown profile param returns empty profile (zero value)", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234&profile=nonexistent", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		// Unknown profile falls back to default; handler should still render 200
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, DEFAULT_PROFILE_NAME, ts.ProfileName)
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
		ts.Server.TLS.Enabled = true
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		assert.Contains(t, w.Body.String(), `"tls":true`)
	})
}

// ---------------------------------------------------------------------------
// themePaletteHandler
// ---------------------------------------------------------------------------

func TestThemePaletteHandler(t *testing.T) {
	t.Run("POST is rejected with 405", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodPost, "/theme?name=b3tty-dark", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.themePaletteHandler(w, req) })
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Contains(t, logged, "method not allowed")
	})

	t.Run("DELETE is rejected with 405", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodDelete, "/theme?name=b3tty-dark", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.themePaletteHandler(w, req) })
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Contains(t, logged, "method not allowed")
	})

	t.Run("GET with unknown name returns 400", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/theme?name=unknown", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.themePaletteHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "unknown theme name")
	})

	t.Run("GET with missing name returns 400", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/theme", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.themePaletteHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "unknown theme name")
	})

	t.Run("GET name=b3tty-dark returns 200 with application/json", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/theme?name=b3tty-dark", nil)
		w := httptest.NewRecorder()
		ts.themePaletteHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	})

	t.Run("GET name=b3tty-dark returns valid JSON", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/theme?name=b3tty-dark", nil)
		w := httptest.NewRecorder()
		ts.themePaletteHandler(w, req)
		assert.True(t, json.Valid(w.Body.Bytes()))
	})

	t.Run("GET name=b3tty-dark returns correct bg, fg, selBg, and cursor", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/theme?name=b3tty-dark", nil)
		w := httptest.NewRecorder()
		ts.themePaletteHandler(w, req)

		var resp themePaletteResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "#1e1e1e", resp.Bg)
		assert.Equal(t, "#d4d4d4", resp.Fg)
		assert.Equal(t, "#474747", resp.SelBg)
		assert.Equal(t, "#aeafad", resp.Cursor)
	})

	t.Run("GET name=b3tty-dark returns 8-element normal array in ANSI display order", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/theme?name=b3tty-dark", nil)
		w := httptest.NewRecorder()
		ts.themePaletteHandler(w, req)

		var resp themePaletteResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		// Order: black, red, yellow, green, cyan, blue, magenta, white
		expected := []string{"#1e1e1e", "#f44747", "#dcdcaa", "#608b4e", "#56b6c2", "#569cd6", "#c678dd", "#d4d4d4"}
		assert.Equal(t, expected, resp.Normal)
	})

	t.Run("GET name=b3tty-dark returns 8-element bright array in ANSI display order", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/theme?name=b3tty-dark", nil)
		w := httptest.NewRecorder()
		ts.themePaletteHandler(w, req)

		var resp themePaletteResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		// Order: bright-black, bright-red, bright-yellow, bright-green, bright-cyan, bright-blue, bright-magenta, bright-white
		expected := []string{"#808080", "#f44747", "#dcdcaa", "#608b4e", "#56b6c2", "#569cd6", "#c678dd", "#ffffff"}
		assert.Equal(t, expected, resp.Bright)
	})

	t.Run("GET name=b3tty-light returns 200 with correct bg, fg, selBg, and cursor", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/theme?name=b3tty-light", nil)
		w := httptest.NewRecorder()
		ts.themePaletteHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp themePaletteResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "#fafafa", resp.Bg)
		assert.Equal(t, "#383a42", resp.Fg)
		assert.Equal(t, "#bad5fb", resp.SelBg)
		assert.Equal(t, "#526fff", resp.Cursor)
	})
}

// ---------------------------------------------------------------------------
// themeConfigHandler
// ---------------------------------------------------------------------------

func TestThemeConfigHandler(t *testing.T) {
	solarizedTheme := Theme{
		Foreground: "#657b83",
		Background: "#002b36",
		Cursor:     "#839496",
	}
	imageTheme := Theme{
		Foreground:      "#ffffff",
		Background:      "#000000",
		BackgroundImage: "/path/to/bg.jpg",
	}

	newTS := func() *TerminalServer {
		ts := newTestTerminalServer()
		ts.Themes = map[string]Theme{
			"solarized": solarizedTheme,
			"image":     imageTheme,
		}
		return ts
	}

	t.Run("DELETE is rejected with 405", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodDelete, "/theme-config?name=solarized", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.themeConfigHandler(w, req) })
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Contains(t, logged, "method not allowed")
	})

	t.Run("PUT is rejected with 405", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodPut, "/theme-config?name=solarized", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.themeConfigHandler(w, req) })
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Contains(t, logged, "method not allowed")
	})

	t.Run("GET with missing name returns 400", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodGet, "/theme-config", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.themeConfigHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "missing name")
	})

	t.Run("GET with unknown name returns 404", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodGet, "/theme-config?name=nonexistent", nil)
		w := httptest.NewRecorder()
		ts.themeConfigHandler(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("GET with valid name returns 200 with application/json", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodGet, "/theme-config?name=solarized", nil)
		w := httptest.NewRecorder()
		ts.themeConfigHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	})

	t.Run("GET returns correct theme colors", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodGet, "/theme-config?name=solarized", nil)
		w := httptest.NewRecorder()
		ts.themeConfigHandler(w, req)

		var resp themeConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "#657b83", resp.Foreground)
		assert.Equal(t, "#002b36", resp.Background)
		assert.Equal(t, "#839496", resp.Cursor)
	})

	t.Run("GET returns hasBackgroundImage=false when no background image", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodGet, "/theme-config?name=solarized", nil)
		w := httptest.NewRecorder()
		ts.themeConfigHandler(w, req)

		var resp themeConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.HasBackgroundImage)
	})

	t.Run("GET returns hasBackgroundImage=true when background image is set", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodGet, "/theme-config?name=image", nil)
		w := httptest.NewRecorder()
		ts.themeConfigHandler(w, req)

		var resp themeConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.True(t, resp.HasBackgroundImage)
	})

	t.Run("GET does not mutate ts.client.Theme", func(t *testing.T) {
		ts := newTS()
		original := ts.Client.Theme
		req := httptest.NewRequest(http.MethodGet, "/theme-config?name=solarized", nil)
		w := httptest.NewRecorder()
		ts.themeConfigHandler(w, req)
		assert.Equal(t, original, ts.Client.Theme)
	})

	t.Run("POST with valid name returns 200 and activates theme", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		ts := newTS()
		req := httptest.NewRequest(http.MethodPost, "/theme-config?name=solarized", nil)
		w := httptest.NewRecorder()
		ts.themeConfigHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, solarizedTheme, ts.Client.Theme)
	})

	t.Run("POST with same-origin Sec-Fetch-Site is allowed", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		ts := newTS()
		req := httptest.NewRequest(http.MethodPost, "/theme-config?name=solarized", nil)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		ts.themeConfigHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, solarizedTheme, ts.Client.Theme)
	})

	t.Run("POST with cross-site Sec-Fetch-Site returns 403 and does not mutate theme", func(t *testing.T) {
		ts := newTS()
		original := ts.Client.Theme
		req := httptest.NewRequest(http.MethodPost, "/theme-config?name=solarized", nil)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.themeConfigHandler(w, req) })
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, logged, "forbidden")
		assert.Equal(t, original, ts.Client.Theme)
	})

	t.Run("POST with same-site Sec-Fetch-Site returns 403 and does not mutate theme", func(t *testing.T) {
		ts := newTS()
		original := ts.Client.Theme
		req := httptest.NewRequest(http.MethodPost, "/theme-config?name=solarized", nil)
		req.Header.Set("Sec-Fetch-Site", "same-site")
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.themeConfigHandler(w, req) })
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, logged, "forbidden")
		assert.Equal(t, original, ts.Client.Theme)
	})

	t.Run("POST without Sec-Fetch-Site (non-browser client) is allowed", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		ts := newTS()
		req := httptest.NewRequest(http.MethodPost, "/theme-config?name=solarized", nil)
		w := httptest.NewRecorder()
		ts.themeConfigHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, solarizedTheme, ts.Client.Theme)
	})

	t.Run("POST with missing name returns 400 and does not mutate theme", func(t *testing.T) {
		ts := newTS()
		original := ts.Client.Theme
		req := httptest.NewRequest(http.MethodPost, "/theme-config", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.themeConfigHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "missing name")
		assert.Equal(t, original, ts.Client.Theme)
	})

	t.Run("POST with unknown name returns 404 and does not mutate theme", func(t *testing.T) {
		ts := newTS()
		original := ts.Client.Theme
		req := httptest.NewRequest(http.MethodPost, "/theme-config?name=nonexistent", nil)
		w := httptest.NewRecorder()
		ts.themeConfigHandler(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, original, ts.Client.Theme)
	})

	t.Run("POST response contains activated theme colors", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		ts := newTS()
		req := httptest.NewRequest(http.MethodPost, "/theme-config?name=solarized", nil)
		w := httptest.NewRecorder()
		ts.themeConfigHandler(w, req)

		var resp themeConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "#657b83", resp.Foreground)
		assert.Equal(t, "#002b36", resp.Background)
	})

	t.Run("BackgroundImage path is not exposed in JSON response", func(t *testing.T) {
		ts := newTS()
		req := httptest.NewRequest(http.MethodGet, "/theme-config?name=image", nil)
		w := httptest.NewRecorder()
		ts.themeConfigHandler(w, req)
		assert.NotContains(t, w.Body.String(), "/path/to/bg.jpg")
	})
}
