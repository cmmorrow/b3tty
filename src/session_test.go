package src

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collector is a concurrency-safe io.Writer that records what a session
// delivers to it.
type collector struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *collector) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *collector) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

// startSession starts a local session running shell and kills it when the
// test ends.
func startSession(t *testing.T, mode SessionMode, shell string) *localSession {
	t.Helper()
	s, err := newLocalSession(SessionOptions{
		Mode:         mode,
		Profile:      Profile{Shell: shell},
		Cols:         80,
		Rows:         24,
		CommandSleep: func(time.Duration) {},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Kill(); <-s.Done() })
	return s
}

func waitDone(t *testing.T, s Session) {
	t.Helper()
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not end")
	}
}

func TestLocalSession(t *testing.T) {
	t.Run("has a short id", func(t *testing.T) {
		s := startSession(t, SessionQuick, "cat")
		assert.Regexp(t, `^[0-9a-f]{8}$`, s.ID())
	})

	t.Run("replay plus live output has no gap or duplicate", func(t *testing.T) {
		s := startSession(t, SessionPersistent,
			`i=0; while [ $i -lt 3000 ]; do echo line$i; i=$((i+1)); done; sleep 5`)
		// Attach part-way through the output.
		require.Eventually(t, func() bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.buf.Len() > 2000
		}, 5*time.Second, time.Millisecond)
		out := &collector{}
		replay, err := s.Attach(out)
		require.NoError(t, err)
		require.Eventually(t, func() bool { return strings.Contains(out.String(), "line2999") },
			5*time.Second, 10*time.Millisecond)

		all := string(replay) + out.String()
		matches := regexp.MustCompile(`line(\d+)\r\n`).FindAllStringSubmatch(all, -1)
		require.Len(t, matches, 3000)
		for i, m := range matches {
			n, _ := strconv.Atoi(m[1])
			require.Equal(t, i, n, "output out of order, repeated, or missing at line %d", i)
		}
	})

	t.Run("detach stops delivery and a re-attach replays what was missed", func(t *testing.T) {
		s := startSession(t, SessionPersistent, "cat")
		first := &collector{}
		_, err := s.Attach(first)
		require.NoError(t, err)
		_, err = s.Write([]byte("before\n"))
		require.NoError(t, err)
		require.Eventually(t, func() bool { return strings.Count(first.String(), "before") == 2 },
			2*time.Second, 10*time.Millisecond, "cat echoes the line and then prints it")

		s.Detach()
		_, err = s.Write([]byte("while-detached\n"))
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			return strings.Count(string(s.buf.Bytes()), "while-detached") == 2
		}, 2*time.Second, 10*time.Millisecond)
		assert.NotContains(t, first.String(), "while-detached")

		replay, err := s.Attach(&collector{})
		require.NoError(t, err)
		assert.Contains(t, string(replay), "before")
		assert.Contains(t, string(replay), "while-detached")
	})

	t.Run("a detached session keeps draining the pty", func(t *testing.T) {
		// Far more than a pty's kernel buffer: the shell would block on
		// write, and never exit, if nothing read the pty.
		s := startSession(t, SessionPersistent, `yes 0123456789abcdef | head -n 40000; exit 7`)
		waitDone(t, s)
		assert.Equal(t, 7, s.ExitCode())
	})

	t.Run("a quick session hands over pre-attach output once, then keeps none", func(t *testing.T) {
		s := startSession(t, SessionQuick, "echo early; cat")
		require.Eventually(t, func() bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			return strings.Contains(string(s.buf.Bytes()), "early")
		}, 2*time.Second, 10*time.Millisecond)
		out := &collector{}
		replay, err := s.Attach(out)
		require.NoError(t, err)
		assert.Contains(t, string(replay), "early")

		_, err = s.Write([]byte("later\n"))
		require.NoError(t, err)
		require.Eventually(t, func() bool { return strings.Contains(out.String(), "later") },
			2*time.Second, 10*time.Millisecond)
		s.Detach()
		replay, err = s.Attach(&collector{})
		require.NoError(t, err)
		assert.Empty(t, replay)
	})

	t.Run("a failing output target is detached", func(t *testing.T) {
		s := startSession(t, SessionQuick, "cat")
		_, err := s.Attach(failingWriter{})
		require.NoError(t, err)
		_, err = s.Write([]byte("x\n"))
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.out == nil
		}, 2*time.Second, 10*time.Millisecond)
	})

	t.Run("Done and ExitCode report the shell's exit", func(t *testing.T) {
		s := startSession(t, SessionQuick, "exit 3")
		waitDone(t, s)
		assert.Equal(t, 3, s.ExitCode())
	})

	t.Run("Kill ends a running shell", func(t *testing.T) {
		s := startSession(t, SessionQuick, "sleep 30")
		assert.Equal(t, -1, s.ExitCode(), "no exit code while running")
		require.NoError(t, s.Kill())
		waitDone(t, s)
		assert.Equal(t, -1, s.ExitCode(), "killed by a signal")
		assert.NoError(t, s.Kill(), "a second Kill is harmless")
	})

	t.Run("runs the profile's startup commands", func(t *testing.T) {
		s, err := newLocalSession(SessionOptions{
			Mode:         SessionQuick,
			Profile:      Profile{Shell: "cat", Commands: []string{"echo startmarker"}},
			Cols:         80,
			Rows:         24,
			CommandSleep: func(time.Duration) {},
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Kill() })
		out := &collector{}
		replay, err := s.Attach(out)
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			return strings.Contains(string(replay)+out.String(), "echo startmarker")
		}, 2*time.Second, 10*time.Millisecond)
	})

	t.Run("resize succeeds while running", func(t *testing.T) {
		s := startSession(t, SessionQuick, "cat")
		assert.NoError(t, s.Resize(100, 40))
	})
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, assert.AnError }
