package src

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSessionHostEnv makes the test binary act as `b3tty session-host` (see
// TestMain), for the one test that runs a real host process.
const testSessionHostEnv = "B3TTY_TEST_SESSION_HOST"

func TestMain(m *testing.M) {
	if os.Getenv(testSessionHostEnv) == "1" {
		os.Exit(RunSessionHostProcess())
	}
	os.Exit(m.Run())
}

const hostWait = 5 * time.Second

// shortTempDir returns a temp directory short enough for a socket path;
// t.TempDir's, which include the test name, can pass the sun_path limit.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "b3h")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// hostClient is a test client of a session host. It records every message
// it receives.
type hostClient struct {
	conn net.Conn
	enc  *SessionEncoder

	mu     sync.Mutex
	msgs   []SessionMessage
	closed bool
}

func newHostClient(t *testing.T, conn net.Conn) *hostClient {
	t.Helper()
	c := &hostClient{conn: conn, enc: NewSessionEncoder(conn)}
	go func() {
		dec := NewSessionDecoder(conn)
		for {
			msg, err := dec.Next()
			c.mu.Lock()
			if err != nil {
				c.closed = true
				c.mu.Unlock()
				return
			}
			c.msgs = append(c.msgs, msg)
			c.mu.Unlock()
		}
	}()
	t.Cleanup(func() { _ = conn.Close() })
	return c
}

func dialHost(t *testing.T, path string) *hostClient {
	t.Helper()
	conn, err := net.Dial("unix", path)
	require.NoError(t, err)
	return newHostClient(t, conn)
}

func (c *hostClient) send(t *testing.T, msg SessionMessage) {
	t.Helper()
	require.NoError(t, c.enc.Send(msg))
}

func (c *hostClient) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.msgs)
}

// since returns the messages received after the first n.
func (c *hostClient) since(n int) []SessionMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]SessionMessage(nil), c.msgs[n:]...)
}

// waitFor waits for a message after the first n that matches ok.
func waitFor[M SessionMessage](t *testing.T, c *hostClient, n int) M {
	t.Helper()
	var found M
	require.Eventually(t, func() bool {
		for _, msg := range c.since(n) {
			if m, ok := msg.(M); ok {
				found = m
				return true
			}
		}
		return false
	}, hostWait, 5*time.Millisecond, "no %T received", found)
	return found
}

// output returns the replays and output received after the first n
// messages, concatenated.
func (c *hostClient) output(n int) string {
	var b strings.Builder
	for _, msg := range c.since(n) {
		switch m := msg.(type) {
		case AttachedMsg:
			b.Write(m.Replay)
		case OutputMsg:
			b.Write(m)
		}
	}
	return b.String()
}

func (c *hostClient) waitOutput(t *testing.T, n int, substr string) {
	t.Helper()
	require.Eventually(t, func() bool { return strings.Contains(c.output(n), substr) },
		hostWait, 5*time.Millisecond, "output never contained %q", substr)
}

func (c *hostClient) hello(t *testing.T) HelloReplyMsg {
	t.Helper()
	n := c.count()
	c.send(t, HelloMsg{Version: ProtocolVersion})
	return waitFor[HelloReplyMsg](t, c, n)
}

func (c *hostClient) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// testHost is a session host running in-process.
type testHost struct {
	socket string
	sess   *localSession
	owner  *hostClient
	done   chan error
}

var testHostCreatedAt = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

// startTestHost serves a new session running shell on a socket in a temp
// directory, with an owner connection.
func startTestHost(t *testing.T, mode SessionMode, shell string) *testHost {
	t.Helper()
	socket := filepath.Join(shortTempDir(t), "0badcafe.sock")
	ln, err := listenSessionSocket(socket)
	require.NoError(t, err)
	sess, err := newLocalSession(SessionOptions{
		ID:           "0badcafe",
		Mode:         mode,
		Profile:      Profile{Shell: shell},
		Cols:         80,
		Rows:         24,
		CommandSleep: func(time.Duration) {},
	})
	require.NoError(t, err)

	ownerFile, hostFile, err := socketPair()
	require.NoError(t, err)
	ownerConn, err := net.FileConn(ownerFile)
	require.NoError(t, err)
	hostConn, err := net.FileConn(hostFile)
	require.NoError(t, err)
	_ = ownerFile.Close()
	_ = hostFile.Close()

	h := &testHost{socket: socket, sess: sess, owner: newHostClient(t, ownerConn), done: make(chan error, 1)}
	go func() {
		h.done <- ServeSessionHost(ln, sess, SessionHostConfig{
			Mode:      mode,
			Nickname:  "nick",
			Profile:   "work",
			PID:       4242,
			Cols:      80,
			Rows:      24,
			CreatedAt: testHostCreatedAt,
			Owner:     hostConn,
		})
	}()
	t.Cleanup(func() {
		_ = sess.Kill()
		select {
		case <-h.done:
		case <-time.After(hostWait):
			t.Error("host did not stop")
		}
	})
	return h
}

// waitStopped waits for ServeSessionHost to return and checks it cleaned
// up its socket.
func (h *testHost) waitStopped(t *testing.T) {
	t.Helper()
	select {
	case err := <-h.done:
		require.NoError(t, err)
		h.done <- err // for the cleanup
	case <-time.After(hostWait):
		t.Fatal("host did not stop")
	}
	_, err := os.Stat(h.socket)
	assert.ErrorIs(t, err, os.ErrNotExist, "socket file removed")
}

func TestSessionHost(t *testing.T) {
	t.Run("hello reports the session's metadata", func(t *testing.T) {
		h := startTestHost(t, SessionPersistent, "cat")
		c := dialHost(t, h.socket)
		got := c.hello(t)
		assert.Equal(t, HelloReplyMsg{
			Version:   ProtocolVersion,
			ID:        "0badcafe",
			Nickname:  "nick",
			Mode:      SessionPersistent,
			Profile:   "work",
			PID:       4242,
			Cols:      80,
			Rows:      24,
			CreatedAt: testHostCreatedAt,
			Attached:  false,
		}, got)

		n := c.count()
		c.send(t, AttachMsg{})
		waitFor[AttachedMsg](t, c, n)
		assert.True(t, dialHost(t, h.socket).hello(t).Attached)
	})

	t.Run("the socket is mode 0600", func(t *testing.T) {
		h := startTestHost(t, SessionPersistent, "cat")
		fi, err := os.Stat(h.socket)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
	})

	t.Run("attach replays earlier output, then streams live output", func(t *testing.T) {
		h := startTestHost(t, SessionPersistent, "echo before; cat")
		require.Eventually(t, func() bool {
			h.sess.mu.Lock()
			defer h.sess.mu.Unlock()
			return strings.Contains(string(h.sess.buf.Bytes()), "before")
		}, hostWait, 5*time.Millisecond)

		c := dialHost(t, h.socket)
		c.send(t, AttachMsg{})
		attached := waitFor[AttachedMsg](t, c, 0)
		assert.Contains(t, string(attached.Replay), "before")
		assert.False(t, attached.AltScreen)
		_, isAttached := c.since(0)[0].(AttachedMsg)
		assert.True(t, isAttached, "the replay comes before any output")

		c.send(t, InputMsg("live\n"))
		c.waitOutput(t, 1, "live")
	})

	t.Run("attach reports the alternate screen", func(t *testing.T) {
		h := startTestHost(t, SessionPersistent, `printf '\033[?1049h'; cat`)
		require.Eventually(t, func() bool { return h.sess.AltScreen() }, hostWait, 5*time.Millisecond)
		c := dialHost(t, h.socket)
		c.send(t, AttachMsg{})
		assert.True(t, waitFor[AttachedMsg](t, c, 0).AltScreen)
	})

	t.Run("a second attach takes over from the first", func(t *testing.T) {
		h := startTestHost(t, SessionPersistent, "cat")
		a := dialHost(t, h.socket)
		a.send(t, AttachMsg{})
		waitFor[AttachedMsg](t, a, 0)
		a.send(t, InputMsg("first\n"))
		a.waitOutput(t, 0, "first")

		b := dialHost(t, h.socket)
		b.send(t, AttachMsg{})
		assert.Contains(t, string(waitFor[AttachedMsg](t, b, 0).Replay), "first")
		aCount := a.count()

		b.send(t, InputMsg("second\n"))
		b.waitOutput(t, 1, "second")
		time.Sleep(50 * time.Millisecond)
		assert.NotContains(t, a.output(aCount), "second", "the first connection stops receiving output")
		assert.False(t, a.isClosed(), "but stays connected")
		assert.True(t, a.hello(t).Attached)
	})

	t.Run("detach stops output and the session keeps running", func(t *testing.T) {
		h := startTestHost(t, SessionPersistent, "cat")
		c := dialHost(t, h.socket)
		c.send(t, AttachMsg{})
		waitFor[AttachedMsg](t, c, 0)
		c.send(t, DetachMsg{})
		assert.False(t, c.hello(t).Attached)

		n := c.count()
		c.send(t, InputMsg("while-detached\n"))
		require.Eventually(t, func() bool {
			h.sess.mu.Lock()
			defer h.sess.mu.Unlock()
			return strings.Count(string(h.sess.buf.Bytes()), "while-detached") == 2
		}, hostWait, 5*time.Millisecond)
		assert.Empty(t, c.output(n))

		c.send(t, AttachMsg{})
		assert.Contains(t, string(waitFor[AttachedMsg](t, c, n).Replay), "while-detached")
	})

	t.Run("closing an attached connection detaches it", func(t *testing.T) {
		h := startTestHost(t, SessionPersistent, "cat")
		c := dialHost(t, h.socket)
		c.send(t, AttachMsg{})
		waitFor[AttachedMsg](t, c, 0)
		_ = c.conn.Close()
		other := dialHost(t, h.socket)
		require.Eventually(t, func() bool { return !other.hello(t).Attached }, hostWait, 10*time.Millisecond)
	})

	t.Run("resize changes the pty size", func(t *testing.T) {
		h := startTestHost(t, SessionPersistent, "/bin/sh")
		c := dialHost(t, h.socket)
		c.send(t, AttachMsg{})
		waitFor[AttachedMsg](t, c, 0)
		c.send(t, ResizeMsg{Cols: 101, Rows: 31})
		got := c.hello(t)
		assert.Equal(t, [2]uint16{101, 31}, [2]uint16{got.Cols, got.Rows})
		c.send(t, InputMsg("stty size\n"))
		c.waitOutput(t, 0, "31 101")

		n := c.count()
		c.send(t, ResizeMsg{Cols: 0, Rows: 31})
		assert.Contains(t, waitFor[ErrorMsg](t, c, n).Message, "invalid size")
	})

	t.Run("rename changes the nickname", func(t *testing.T) {
		h := startTestHost(t, SessionPersistent, "cat")
		c := dialHost(t, h.socket)
		c.send(t, RenameMsg{Nickname: "build box"})
		assert.Equal(t, "build box", c.hello(t).Nickname)
	})

	t.Run("kill ends the session and the host", func(t *testing.T) {
		h := startTestHost(t, SessionPersistent, "sleep 30")
		c := dialHost(t, h.socket)
		c.send(t, KillMsg{})
		assert.Equal(t, -1, waitFor[ExitedMsg](t, c, 0).Code)
		h.waitStopped(t)
		require.Eventually(t, c.isClosed, hostWait, 5*time.Millisecond)
	})

	t.Run("every connection gets exited with the shell's exit code", func(t *testing.T) {
		h := startTestHost(t, SessionPersistent, "read code; exit $code")
		a := dialHost(t, h.socket)
		b := dialHost(t, h.socket)
		a.send(t, AttachMsg{})
		waitFor[AttachedMsg](t, a, 0)
		b.hello(t) // b is registered before the shell exits
		a.send(t, InputMsg("3\n"))
		for _, c := range []*hostClient{a, b, h.owner} {
			assert.Equal(t, 3, waitFor[ExitedMsg](t, c, 0).Code)
		}
		h.waitStopped(t)
	})

	t.Run("a persistent host outlives its owner connection", func(t *testing.T) {
		h := startTestHost(t, SessionPersistent, "cat")
		_ = h.owner.conn.Close()
		c := dialHost(t, h.socket)
		c.send(t, InputMsg("still\n"))
		c.send(t, AttachMsg{})
		c.waitOutput(t, 0, "still")
		select {
		case <-h.sess.Done():
			t.Fatal("session ended with its owner connection")
		default:
		}
	})

	t.Run("a quick host exits when its owner connection closes", func(t *testing.T) {
		h := startTestHost(t, SessionQuick, "cat")
		other := dialHost(t, h.socket)
		other.hello(t)
		_ = h.owner.conn.Close()
		assert.Equal(t, -1, waitFor[ExitedMsg](t, other, 0).Code)
		h.waitStopped(t)
	})

	t.Run("a quick host needs an owner", func(t *testing.T) {
		ln, err := listenSessionSocket(filepath.Join(shortTempDir(t), "x.sock"))
		require.NoError(t, err)
		defer ln.Close()
		err = ServeSessionHost(ln, nil, SessionHostConfig{Mode: SessionQuick})
		assert.ErrorContains(t, err, "owner")
	})

	t.Run("an unexpected message gets an error", func(t *testing.T) {
		h := startTestHost(t, SessionPersistent, "cat")
		c := dialHost(t, h.socket)
		c.send(t, ExitedMsg{Code: 1})
		assert.Contains(t, waitFor[ErrorMsg](t, c, 0).Message, "unexpected exited")
	})

	t.Run("the host refreshes its socket's mtime", func(t *testing.T) {
		socket := filepath.Join(shortTempDir(t), "touch.sock")
		ln, err := listenSessionSocket(socket)
		require.NoError(t, err)
		old := time.Now().Add(-48 * time.Hour)
		require.NoError(t, os.Chtimes(socket, old, old))
		sess, err := newLocalSession(SessionOptions{Mode: SessionPersistent, Profile: Profile{Shell: "cat"}, Cols: 80, Rows: 24})
		require.NoError(t, err)
		done := make(chan error, 1)
		go func() {
			done <- ServeSessionHost(ln, sess, SessionHostConfig{Mode: SessionPersistent, TouchInterval: 10 * time.Millisecond})
		}()
		require.Eventually(t, func() bool {
			fi, err := os.Stat(socket)
			return err == nil && fi.ModTime().After(old.Add(time.Hour))
		}, hostWait, 10*time.Millisecond)
		_ = sess.Kill()
		require.NoError(t, <-done)
	})
}

func TestSessionSocketPlacement(t *testing.T) {
	const id = "0badcafe"
	shortHome := "/home/u"
	longHome := "/home/" + strings.Repeat("x", 80)

	tests := []struct {
		name    string
		env     socketPlacement
		want    string
		wantErr string
	}{
		{
			name: "default directory when it fits",
			env:  socketPlacement{goos: "darwin", home: shortHome, tempDir: "/tmp", uid: 501},
			want: "/home/u/.config/b3tty/sessions/0badcafe.sock",
		},
		{
			name: "macOS falls back to the temp directory",
			env:  socketPlacement{goos: "darwin", home: longHome, xdgRuntimeDir: "/run/user/501", tempDir: "/var/folders/ab/T", uid: 501},
			want: "/var/folders/ab/T/b3tty-501/0badcafe.sock",
		},
		{
			name: "Linux prefers XDG_RUNTIME_DIR",
			env:  socketPlacement{goos: "linux", home: longHome, xdgRuntimeDir: "/run/user/1000", tempDir: "/tmp", uid: 1000},
			want: "/run/user/1000/b3tty/0badcafe.sock",
		},
		{
			name: "Linux without XDG_RUNTIME_DIR uses the temp directory",
			env:  socketPlacement{goos: "linux", home: longHome, tempDir: "/tmp", uid: 1000},
			want: "/tmp/b3tty-1000/0badcafe.sock",
		},
		{
			name:    "both too long is a clear error",
			env:     socketPlacement{goos: "darwin", home: longHome, tempDir: "/" + strings.Repeat("t", 100), uid: 501},
			wantErr: "fallback is too long too",
		},
		{
			name:    "a malformed id is refused",
			env:     socketPlacement{goos: "darwin", home: shortHome, tempDir: "/tmp"},
			wantErr: "invalid session id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := id
			if tt.name == "a malformed id is refused" {
				id = "../evil"
			}
			path, dir, err := sessionSocketPlacement(tt.env, id)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, path)
			assert.Equal(t, filepath.Dir(tt.want), dir)
		})
	}
}

func TestCheckSocketPath(t *testing.T) {
	fits := "/" + strings.Repeat("a", 102) // 103 bytes + NUL = 104
	assert.NoError(t, checkSocketPath(fits, "darwin"))
	assert.ErrorContains(t, checkSocketPath(fits+"a", "darwin"), "allows at most 103")
	assert.NoError(t, checkSocketPath(fits+"aaaa", "linux"))
	assert.ErrorContains(t, checkSocketPath(fits+"aaaaa", "linux"), "allows at most 107")

	_, err := listenSessionSocket(filepath.Join(shortTempDir(t), strings.Repeat("s", 120)+".sock"))
	assert.ErrorContains(t, err, "allows at most", "a clear error rather than bind's invalid argument")
}

func TestSessionSocketPath(t *testing.T) {
	home := shortTempDir(t)
	t.Setenv("HOME", home)
	path, err := SessionSocketPath("0badcafe")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config", "b3tty", "sessions", "0badcafe.sock"), path)
	fi, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), fi.Mode().Perm())
}

func TestEnsurePrivateDir(t *testing.T) {
	base := shortTempDir(t)

	loose := filepath.Join(base, "loose")
	require.NoError(t, os.Mkdir(loose, 0o755))
	require.NoError(t, ensurePrivateDir(loose))
	fi, err := os.Stat(loose)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), fi.Mode().Perm(), "tightened to 0700")

	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(loose, link))
	assert.ErrorContains(t, ensurePrivateDir(link), "not a directory", "a symlink is refused")
}

// TestSessionHostProcess runs a real host process: the test binary, which
// TestMain turns into one.
func TestSessionHostProcess(t *testing.T) {
	socket := filepath.Join(shortTempDir(t), "c0ffee00.sock")
	exe, err := os.Executable()
	require.NoError(t, err)
	host, err := SpawnSessionHost(SessionHostSpec{
		ID:          "c0ffee00",
		Mode:        SessionPersistent,
		Nickname:    "integration",
		ProfileName: "default",
		Profile:     Profile{Shell: "/bin/sh"},
		Cols:        80,
		Rows:        24,
		Socket:      socket,
	}, SessionHostSpawnOptions{
		Executable: exe,
		Env:        append(os.Environ(), testSessionHostEnv+"=1"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = host.Conn.Close()
		if p, err := os.FindProcess(host.PID); err == nil {
			_ = p.Kill()
		}
		<-host.Exited
	})
	assert.Equal(t, "c0ffee00", host.Hello.ID)
	assert.Equal(t, host.PID, host.Hello.PID)
	assert.Equal(t, SessionPersistent, host.Hello.Mode)
	assert.Equal(t, "integration", host.Hello.Nickname)
	assert.Equal(t, socket, host.Socket)

	c := dialHost(t, socket)
	c.send(t, AttachMsg{})
	waitFor[AttachedMsg](t, c, 0)
	c.send(t, InputMsg("echo b3tty-$((40+2)); exit 7\n"))
	c.waitOutput(t, 0, "b3tty-42")
	assert.Equal(t, 7, waitFor[ExitedMsg](t, c, 0).Code)

	select {
	case <-host.Exited:
	case <-time.After(hostWait):
		t.Fatal("host process did not exit")
	}
	_, err = os.Stat(socket)
	assert.True(t, errors.Is(err, os.ErrNotExist), "socket removed")
}
