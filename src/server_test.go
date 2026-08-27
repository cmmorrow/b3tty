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
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncBuffer wraps bytes.Buffer with a mutex guarding both Write and String.
// captureLog's caller reads String() only after f() returns, but a
// background goroutine from an earlier test's real WebSocket+pty session
// (see terminal_handler_test.go — those goroutines have no guaranteed
// happens-before relationship with the test that spawned them, and can
// outlive it) may still be concurrently calling into the logger and writing
// to whatever buffer captureLog swapped in. A plain bytes.Buffer isn't safe
// for that; this is.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLog redirects the standard logger to a buffer for the duration of f
// and returns everything that was logged.
func captureLog(f func()) string {
	var buf syncBuffer
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
		Themes:         map[string]Theme{},
		Token:          "test-token-1234",
		ProfileName:    DEFAULT_PROFILE_NAME,
		StartupProfile: DEFAULT_PROFILE_NAME,
		AuthSleep:      func(time.Duration) {}, // no-op: avoid real delays in tests
		StartTime:      time.Now(),
		// WSClients mirrors what Serve() initializes before registering
		// routes; without it, any handler that registers a WebSocket
		// connection (terminalHandler) panics with "assignment to entry in
		// nil map".
		WSClients: make(map[*websocket.Conn]*wsClient),
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
		data, err := buildConfigJSON(srv, clnt, thm, nil, nil, nil, nil, "")
		require.NoError(t, err)
		assert.True(t, json.Valid(data))
	})

	t.Run("JSON contains expected scalar fields", func(t *testing.T) {
		data, err := buildConfigJSON(srv, clnt, thm, nil, nil, nil, nil, "")
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
		data, err := buildConfigJSON(srv, clnt, thm, nil, nil, nil, nil, "")
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
		data, err := buildConfigJSON(tlsSrv, clnt, thm, nil, nil, nil, nil, "")
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))
		assert.Equal(t, true, result["tls"])
	})

	t.Run("empty theme produces valid JSON", func(t *testing.T) {
		emptyTheme := &Theme{}
		data, err := buildConfigJSON(srv, clnt, emptyTheme, nil, nil, nil, nil, "")
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
// requireSameOrigin
// ---------------------------------------------------------------------------

func TestRequireSameOrigin(t *testing.T) {
	t.Run("absent Sec-Fetch-Site header allows the request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		w := httptest.NewRecorder()
		rejected := requireSameOrigin(w, req)
		assert.False(t, rejected)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("same-origin header allows the request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		rejected := requireSameOrigin(w, req)
		assert.False(t, rejected)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("cross-site header rejects with 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		w := httptest.NewRecorder()
		var rejected bool
		logged := captureLog(func() { rejected = requireSameOrigin(w, req) })
		assert.True(t, rejected)
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, logged, "forbidden")
		assert.Contains(t, logged, "cross-site")
	})

	t.Run("same-site header rejects with 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Header.Set("Sec-Fetch-Site", "same-site")
		w := httptest.NewRecorder()
		var rejected bool
		logged := captureLog(func() { rejected = requireSameOrigin(w, req) })
		assert.True(t, rejected)
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, logged, "forbidden")
		assert.Contains(t, logged, "same-site")
	})
}

// ---------------------------------------------------------------------------
// writeJSON
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Run("sets Content-Type to application/json", func(t *testing.T) {
		w := httptest.NewRecorder()
		writeJSON(w, map[string]string{"key": "value"}, "test")
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	})

	t.Run("encodes the value as JSON in the response body", func(t *testing.T) {
		w := httptest.NewRecorder()
		writeJSON(w, map[string]int{"count": 42}, "test")
		var got map[string]int
		require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
		assert.Equal(t, 42, got["count"])
	})

	t.Run("encodes a struct value correctly", func(t *testing.T) {
		type payload struct {
			Name string `json:"name"`
			Port int    `json:"port"`
		}
		w := httptest.NewRecorder()
		writeJSON(w, payload{Name: "b3tty", Port: 8080}, "test")
		var got payload
		require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
		assert.Equal(t, "b3tty", got.Name)
		assert.Equal(t, 8080, got.Port)
	})

	t.Run("status code defaults to 200", func(t *testing.T) {
		w := httptest.NewRecorder()
		writeJSON(w, struct{}{}, "test")
		assert.Equal(t, http.StatusOK, w.Code)
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

	t.Run("HTML in profile Title is escaped, not injected raw", func(t *testing.T) {
		// Regression test for best-practice-violations.md 2.1: displayTermHandler
		// used to render terminal.tmpl with text/template, which does not
		// autoescape, so an attacker-controlled profile Title (settable via
		// POST /edit-profile) could break out of <title> and inject arbitrary
		// HTML. html/template must escape it instead.
		ts := newTestTerminalServer()
		ts.Profiles["default"] = Profile{Title: `</title><script>alert(1)</script>`, Shell: "/bin/bash"}
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		body := w.Body.String()
		assert.NotContains(t, body, "<script>alert(1)</script>")
		assert.Contains(t, body, "&lt;script&gt;alert(1)&lt;/script&gt;")
	})

	t.Run("HTML in profile name is escaped, not injected raw", func(t *testing.T) {
		ts := newTestTerminalServer()
		malicious := `"><img src=x onerror=alert(1)>`
		ts.Profiles[malicious] = Profile{Title: "b3tty", Shell: "/bin/bash"}
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234&profile="+url.QueryEscape(malicious), nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		body := w.Body.String()
		assert.NotContains(t, body, `<img src=x onerror=alert(1)>`)
	})

	t.Run("window.B3TTY is still assigned a raw JS object, not a quoted string", func(t *testing.T) {
		// The fix wraps ConfigJSON in template.JS so html/template's JS-context
		// autoescaper leaves the pre-marshaled JSON alone instead of quoting
		// and escaping it into a JS string literal.
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234", nil)
		w := httptest.NewRecorder()
		ts.displayTermHandler(w, req)
		body := w.Body.String()
		idx := strings.Index(body, "window.B3TTY = ")
		require.NotEqual(t, -1, idx, "window.B3TTY assignment not found")
		rest := body[idx+len("window.B3TTY = "):]
		assert.True(t, strings.HasPrefix(rest, "{"), "expected a raw JS object literal, got: %.40s", rest)
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

	t.Run("GET user-customized builtin in ts.Themes takes precedence over builtinThemes", func(t *testing.T) {
		ts := newTestTerminalServer()
		ts.Themes = map[string]Theme{
			"b3tty-dark": {Foreground: "#custom", Background: "#000001"},
		}
		req := httptest.NewRequest(http.MethodGet, "/theme?name=b3tty-dark", nil)
		w := httptest.NewRecorder()
		ts.themePaletteHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp themePaletteResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "#custom", resp.Fg)
		assert.Equal(t, "#000001", resp.Bg)
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

	t.Run("GET for builtin theme not in ts.Themes returns 200 with builtin colors", func(t *testing.T) {
		ts := newTestTerminalServer()
		ts.Themes = map[string]Theme{} // empty — b3tty-dark not registered
		req := httptest.NewRequest(http.MethodGet, "/theme-config?name=b3tty-dark", nil)
		w := httptest.NewRecorder()
		ts.themeConfigHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp themeConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "#d4d4d4", resp.Foreground)
		assert.Equal(t, "#1e1e1e", resp.Background)
	})

	t.Run("GET user-customized builtin in ts.Themes takes precedence over builtinThemes", func(t *testing.T) {
		ts := newTestTerminalServer()
		ts.Themes = map[string]Theme{
			"b3tty-dark": {Foreground: "#custom", Background: "#000001"},
		}
		req := httptest.NewRequest(http.MethodGet, "/theme-config?name=b3tty-dark", nil)
		w := httptest.NewRecorder()
		ts.themeConfigHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp themeConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "#custom", resp.Foreground)
		assert.Equal(t, "#000001", resp.Background)
	})
}

// ---------------------------------------------------------------------------
// editThemeHandler
// ---------------------------------------------------------------------------

func TestEditThemeHandler(t *testing.T) {
	editBody := func(name, fg, bg string) string {
		return `{"name":"` + name + `","theme":{"foreground":"` + fg + `","background":"` + bg + `"}}`
	}

	t.Run("GET returns 405", func(t *testing.T) {
		ts := newTestTerminalServer()
		req := httptest.NewRequest(http.MethodGet, "/edit-theme", nil)
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.editThemeHandler(w, req) })
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Contains(t, logged, "method not allowed")
	})

	t.Run("POST with cross-site Sec-Fetch-Site returns 403", func(t *testing.T) {
		ts := newTestTerminalServer()
		body := strings.NewReader(editBody("my-theme", "#fff", "#000"))
		req := httptest.NewRequest(http.MethodPost, "/edit-theme", body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.editThemeHandler(w, req) })
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, logged, "forbidden")
		assert.Empty(t, ts.Themes)
	})

	t.Run("POST with missing name returns 400", func(t *testing.T) {
		ts := newTestTerminalServer()
		body := strings.NewReader(`{"name":"","theme":{"foreground":"#fff"}}`)
		req := httptest.NewRequest(http.MethodPost, "/edit-theme", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.editThemeHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "missing name")
	})

	t.Run("POST with invalid color returns 400", func(t *testing.T) {
		ts := newTestTerminalServer()
		body := strings.NewReader(`{"name":"bad","theme":{"foreground":"not#valid"}}`)
		req := httptest.NewRequest(http.MethodPost, "/edit-theme", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		logged := captureLog(func() { ts.editThemeHandler(w, req) })
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, logged, "bad request")
	})

	t.Run("POST creates new theme in ts.Themes and activates it", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		ts := newTestTerminalServer()
		body := strings.NewReader(editBody("my-theme", "#ffffff", "#000000"))
		req := httptest.NewRequest(http.MethodPost, "/edit-theme", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.editThemeHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, ts.Themes, "my-theme")
		assert.Equal(t, "#ffffff", ts.Themes["my-theme"].Foreground)
		assert.Equal(t, "my-theme", ts.ActiveTheme)
		assert.Equal(t, "#ffffff", ts.Client.Theme.Foreground)
	})

	t.Run("POST overwrites existing theme colors in ts.Themes", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		ts := newTestTerminalServer()
		ts.Themes = map[string]Theme{
			"my-theme": {Foreground: "#aaaaaa", Background: "#bbbbbb"},
		}
		body := strings.NewReader(editBody("my-theme", "#ffffff", "#112233"))
		req := httptest.NewRequest(http.MethodPost, "/edit-theme", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.editThemeHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "#ffffff", ts.Themes["my-theme"].Foreground)
		assert.Equal(t, "#112233", ts.Themes["my-theme"].Background)
	})

	t.Run("POST response includes sorted themeNames", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		ts := newTestTerminalServer()
		ts.Themes = map[string]Theme{
			"alpha": {Foreground: "#111"},
		}
		body := strings.NewReader(editBody("zebra", "#fff", "#000"))
		req := httptest.NewRequest(http.MethodPost, "/edit-theme", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.editThemeHandler(w, req)

		var resp themeConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, []string{"alpha", "zebra"}, resp.ThemeNames)
	})

	t.Run("POST response contains activated theme colors", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		ts := newTestTerminalServer()
		body := strings.NewReader(editBody("my-theme", "#aabbcc", "#112233"))
		req := httptest.NewRequest(http.MethodPost, "/edit-theme", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.editThemeHandler(w, req)

		var resp themeConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "#aabbcc", resp.Foreground)
		assert.Equal(t, "#112233", resp.Background)
	})

	t.Run("POST hasBackgroundImage is always false", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		ts := newTestTerminalServer()
		body := strings.NewReader(editBody("my-theme", "#fff", "#000"))
		req := httptest.NewRequest(http.MethodPost, "/edit-theme", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.editThemeHandler(w, req)

		var resp themeConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.HasBackgroundImage)
	})

	t.Run("POST without Sec-Fetch-Site (non-browser client) is allowed", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		ts := newTestTerminalServer()
		body := strings.NewReader(editBody("my-theme", "#fff", "#000"))
		req := httptest.NewRequest(http.MethodPost, "/edit-theme", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.editThemeHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("POST with same-origin Sec-Fetch-Site is allowed", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		ts := newTestTerminalServer()
		body := strings.NewReader(editBody("my-theme", "#fff", "#000"))
		req := httptest.NewRequest(http.MethodPost, "/edit-theme", body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		w := httptest.NewRecorder()
		ts.editThemeHandler(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("BackgroundImage path cannot be sent via JSON (json:\"-\" tag)", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		ts := newTestTerminalServer()
		// Even if a caller tries to set backgroundImage, json:"-" ensures it is ignored
		body := strings.NewReader(`{"name":"my-theme","theme":{"foreground":"#fff","backgroundImage":"/etc/passwd"}}`)
		req := httptest.NewRequest(http.MethodPost, "/edit-theme", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.editThemeHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, ts.Themes["my-theme"].BackgroundImage)
		assert.NotContains(t, w.Body.String(), "/etc/passwd")
	})
}

// ---------------------------------------------------------------------------
// sortedThemeNames
// ---------------------------------------------------------------------------

func TestSortedThemeNames(t *testing.T) {
	t.Run("returns empty slice when no themes are configured", func(t *testing.T) {
		ts := newTestTerminalServer()
		assert.Empty(t, ts.sortedThemeNames())
	})

	t.Run("returns names in alphabetical order", func(t *testing.T) {
		ts := newTestTerminalServer()
		ts.Themes["zebra"] = Theme{}
		ts.Themes["alpha"] = Theme{}
		ts.Themes["mango"] = Theme{}
		assert.Equal(t, []string{"alpha", "mango", "zebra"}, ts.sortedThemeNames())
	})

	t.Run("returns a single-element slice for one theme", func(t *testing.T) {
		ts := newTestTerminalServer()
		ts.Themes["my-theme"] = Theme{}
		assert.Equal(t, []string{"my-theme"}, ts.sortedThemeNames())
	})

	t.Run("does not modify ts.Themes", func(t *testing.T) {
		ts := newTestTerminalServer()
		ts.Themes["b"] = Theme{}
		ts.Themes["a"] = Theme{}
		_ = ts.sortedThemeNames()
		assert.Len(t, ts.Themes, 2)
		_, hasA := ts.Themes["a"]
		_, hasB := ts.Themes["b"]
		assert.True(t, hasA)
		assert.True(t, hasB)
	})
}

// ---------------------------------------------------------------------------
// Concurrent state access (regression test for the StateMu data race)
// ---------------------------------------------------------------------------

// TestConcurrentStateAccess drives the handlers that read and write shared
// TerminalServer state (Client, Profiles, Themes, ActiveTheme, ProfileName,
// FirstRun) from many goroutines at once. Before StateMu was
// added to guard these fields, running this test with `go test -race` (or
// often even without it) reliably reproduced "fatal error: concurrent map
// writes" within a handful of iterations. Several of the handlers below also
// persist to the same conf.yaml concurrently; before configFileMu was added
// (best-practice-violations.md 1.2), that reliably produced logged
// "parse existing config" errors from interleaved read-modify-write cycles
// on the file — this test asserts that no longer happens.
func TestConcurrentStateAccess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ts := newTestTerminalServer()

	const iterations = 30
	var wg sync.WaitGroup

	logged := captureLog(func() {
		run := func(f func()) {
			wg.Go(func() {
				for range iterations {
					f()
				}
			})
		}

		// Page loads: read Profiles/Themes/Client/ActiveTheme, write ProfileName.
		run(func() {
			req := httptest.NewRequest(http.MethodGet, "/?token=test-token-1234&profile=work", nil)
			ts.displayTermHandler(httptest.NewRecorder(), req)
		})

		// Theme edits: read+write Themes, write Client.Theme/ActiveTheme.
		run(func() {
			body := strings.NewReader(`{"name":"concurrent-theme","theme":{"foreground":"#fff","background":"#000"}}`)
			req := httptest.NewRequest(http.MethodPost, "/edit-theme", body)
			req.Header.Set("Content-Type", "application/json")
			ts.editThemeHandler(httptest.NewRecorder(), req)
		})

		// Adding a built-in theme: read+write Themes, write Client.Theme/ActiveTheme.
		run(func() {
			body := strings.NewReader(`{"theme":"b3tty-dark"}`)
			req := httptest.NewRequest(http.MethodPost, "/add-theme", body)
			req.Header.Set("Content-Type", "application/json")
			ts.addThemeHandler(httptest.NewRecorder(), req)
		})

		// Profile edits: read+write Profiles.
		run(func() {
			body := strings.NewReader(`{"name":"concurrent-profile","profile":{"shell":"/bin/bash","commands":[]}}`)
			req := httptest.NewRequest(http.MethodPost, "/edit-profile", body)
			req.Header.Set("Content-Type", "application/json")
			ts.editProfileHandler(httptest.NewRecorder(), req)
		})

		// Settings updates: write Client font/cursor/dimension fields.
		run(func() {
			body := strings.NewReader(`{"server":{"port":8080},"terminal":{"fontSize":14,"rows":24,"columns":80}}`)
			req := httptest.NewRequest(http.MethodPost, "/settings", body)
			req.Header.Set("Content-Type", "application/json")
			ts.settingsHandler(httptest.NewRecorder(), req)
		})

		// A WebSocket session start reads Profiles/ProfileName under StateMu
		// (cols/rows now come from the /ws request's own query string, not
		// shared state). terminalHandler itself needs a real WebSocket
		// upgrade and a pty, which isn't practical here; the read it performs
		// is exercised directly under the same StateMu discipline as the real
		// handler.
		run(func() {
			ts.StateMu.RLock()
			_ = ts.Profiles[ts.ProfileName]
			ts.StateMu.RUnlock()
		})

		wg.Wait()
	})

	assert.NotContains(t, logged, "parse existing config",
		"concurrent config-file writes raced; configFileMu should serialize them")
}
