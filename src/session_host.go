package src

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// A session host is a process that owns one Session and serves the session
// protocol (session_protocol.go) on a Unix socket, so the session can
// outlive the server and be re-attached. ServeSessionHost is its logic and
// takes any listener, so tests run it in-process; RunSessionHostProcess
// (session_host_process.go) wraps it as the `b3tty session-host` process.

// SESSION_SOCKET_TOUCH_INTERVAL is how often a host refreshes its socket's
// mtime, so temp-directory cleaners don't delete it.
const SESSION_SOCKET_TOUCH_INTERVAL = time.Hour

// SESSION_HOST_WRITE_TIMEOUT bounds one frame written to a host connection.
// A connection that can't take a frame in that time is closed: otherwise a
// stalled reader would block the pty reader and, through it, the shell.
const SESSION_HOST_WRITE_TIMEOUT = 10 * time.Second

// SessionHostConfig describes the session a host serves.
type SessionHostConfig struct {
	Mode SessionMode
	// Nickname is the initial nickname; rename changes it.
	Nickname string
	// Profile is the name of the profile the session started from.
	Profile string
	// PID is reported in the hello reply: the host process's pid.
	PID        int
	Cols, Rows uint16
	CreatedAt  time.Time
	// Owner is the connection from the server that spawned the host. A
	// quick host ends its session when Owner closes, so it goes away even
	// if the server is SIGKILLed; quick mode requires it. A persistent host
	// treats it like any other connection.
	Owner net.Conn
	// TouchInterval overrides SESSION_SOCKET_TOUCH_INTERVAL.
	TouchInterval time.Duration
	// WriteTimeout overrides SESSION_HOST_WRITE_TIMEOUT.
	WriteTimeout time.Duration
}

// screenAttacher is implemented by sessions that can report the alternate
// screen state atomically with the replay (localSession).
type screenAttacher interface {
	AttachScreen(w io.Writer) (replay []byte, altScreen bool, err error)
}

type sessionHost struct {
	sess Session
	cfg  SessionHostConfig
	ln   net.Listener

	mu       sync.Mutex // guards the fields below
	nickname string
	cols     uint16
	rows     uint16
	conns    map[*hostConn]struct{}
	attached *hostConn
	closing  bool

	wg sync.WaitGroup // accept loop and connection handlers
}

// hostConn is one client connection: the server's control connection or a
// CLI's.
type hostConn struct {
	h     *sessionHost
	conn  net.Conn
	enc   *SessionEncoder
	owner bool
}

// ServeSessionHost serves sess on ln until the session ends: through its
// shell exiting, a kill message, or (quick mode) the owner connection
// closing. It then sends exited to every connection, closes them and ln,
// and removes ln's socket file. It returns once all of that is done.
func ServeSessionHost(ln net.Listener, sess Session, cfg SessionHostConfig) error {
	if cfg.Mode == SessionQuick && cfg.Owner == nil {
		return errors.New("session host: a quick session needs an owner connection")
	}
	if cfg.TouchInterval <= 0 {
		cfg.TouchInterval = SESSION_SOCKET_TOUCH_INTERVAL
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = SESSION_HOST_WRITE_TIMEOUT
	}
	h := &sessionHost{
		sess:     sess,
		cfg:      cfg,
		ln:       ln,
		nickname: cfg.Nickname,
		cols:     cfg.Cols,
		rows:     cfg.Rows,
		conns:    make(map[*hostConn]struct{}),
	}
	socketPath := ""
	if addr, ok := ln.Addr().(*net.UnixAddr); ok {
		socketPath = addr.Name
	}

	if cfg.Owner != nil {
		h.addConn(cfg.Owner, true)
	}
	h.wg.Add(1)
	go h.acceptLoop()

	touch := time.NewTicker(cfg.TouchInterval)
	defer touch.Stop()
	for done := false; !done; {
		select {
		case <-sess.Done():
			done = true
		case <-touch.C:
			if socketPath != "" {
				now := time.Now()
				_ = os.Chtimes(socketPath, now, now)
			}
		}
	}

	code := sess.ExitCode()
	h.mu.Lock()
	h.closing = true
	conns := make([]*hostConn, 0, len(h.conns))
	for c := range h.conns {
		conns = append(conns, c)
	}
	h.mu.Unlock()
	_ = ln.Close()
	for _, c := range conns {
		_ = c.send(ExitedMsg{Code: code})
		_ = c.conn.Close()
	}
	h.wg.Wait()
	if socketPath != "" {
		if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove socket: %w", err)
		}
	}
	return nil
}

func (h *sessionHost) acceptLoop() {
	defer h.wg.Done()
	for {
		conn, err := h.ln.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				Errorf("session host: accept: %v", err)
				// A persistent error (e.g. out of fds) would spin; back off.
				time.Sleep(100 * time.Millisecond)
				h.mu.Lock()
				closing := h.closing
				h.mu.Unlock()
				if !closing {
					continue
				}
			}
			return
		}
		h.addConn(conn, false)
	}
}

// addConn registers conn and starts its handler, or closes it if the host
// is shutting down.
func (h *sessionHost) addConn(conn net.Conn, owner bool) {
	h.mu.Lock()
	if h.closing {
		h.mu.Unlock()
		_ = conn.Close()
		if owner && h.cfg.Mode == SessionQuick {
			_ = h.sess.Kill()
		}
		return
	}
	c := &hostConn{h: h, conn: conn, enc: NewSessionEncoder(conn), owner: owner}
	h.conns[c] = struct{}{}
	h.wg.Add(1)
	h.mu.Unlock()
	go c.serve()
}

// send writes one frame with a write deadline. A failed write may leave a
// partial frame behind, so the connection is closed.
func (c *hostConn) send(msg SessionMessage) error {
	_ = c.conn.SetWriteDeadline(time.Now().Add(c.h.cfg.WriteTimeout))
	if err := c.enc.Send(msg); err != nil {
		_ = c.conn.Close()
		return err
	}
	return nil
}

func (c *hostConn) sendError(format string, args ...any) {
	_ = c.send(ErrorMsg{Message: fmt.Sprintf(format, args...)})
}

func (c *hostConn) serve() {
	h := c.h
	defer h.wg.Done()
	defer func() {
		_ = c.conn.Close()
		h.mu.Lock()
		delete(h.conns, c)
		if h.attached == c {
			h.attached = nil
			h.sess.Detach()
		}
		h.mu.Unlock()
		if c.owner && h.cfg.Mode == SessionQuick {
			_ = h.sess.Kill()
		}
	}()

	dec := NewSessionDecoder(c.conn)
	for {
		msg, err := dec.Next()
		if err != nil {
			if errors.Is(err, ErrMalformedPayload) {
				c.sendError("%v", err)
				continue
			}
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				Debugf("session host: read: %v", err)
			}
			return
		}
		switch m := msg.(type) {
		case HelloMsg:
			_ = c.send(h.helloReply())
		case AttachMsg:
			if err := h.attach(c); err != nil {
				c.sendError("attach: %v", err)
			}
		case DetachMsg:
			h.mu.Lock()
			if h.attached == c {
				h.attached = nil
				h.sess.Detach()
			}
			h.mu.Unlock()
		case InputMsg:
			if _, err := h.sess.Write(m); err != nil {
				c.sendError("input: %v", err)
			}
		case ResizeMsg:
			if m.Cols == 0 || m.Rows == 0 {
				c.sendError("resize: invalid size %dx%d", m.Cols, m.Rows)
				continue
			}
			h.mu.Lock()
			err := h.sess.Resize(m.Cols, m.Rows)
			if err == nil {
				h.cols, h.rows = m.Cols, m.Rows
			}
			h.mu.Unlock()
			if err != nil {
				c.sendError("resize: %v", err)
			}
		case RenameMsg:
			h.mu.Lock()
			h.nickname = m.Nickname
			h.mu.Unlock()
		case KillMsg:
			if err := h.sess.Kill(); err != nil {
				c.sendError("kill: %v", err)
			}
		default:
			c.sendError("unexpected %s message", msg.frameType())
		}
	}
}

func (h *sessionHost) helloReply() HelloReplyMsg {
	h.mu.Lock()
	defer h.mu.Unlock()
	return HelloReplyMsg{
		Version:   ProtocolVersion,
		ID:        h.sess.ID(),
		Nickname:  h.nickname,
		Mode:      h.cfg.Mode,
		Profile:   h.cfg.Profile,
		PID:       h.cfg.PID,
		Cols:      h.cols,
		Rows:      h.rows,
		CreatedAt: h.cfg.CreatedAt,
		Attached:  h.attached != nil,
	}
}

// attach makes c the attached connection, taking over from any other: the
// previous one stops receiving output. c gets the attached message (the
// replay) before any output.
func (h *sessionHost) attach(c *hostConn) error {
	w := &attachWriter{c: c}
	h.mu.Lock()
	var (
		replay []byte
		alt    bool
		err    error
	)
	if sa, ok := h.sess.(screenAttacher); ok {
		replay, alt, err = sa.AttachScreen(w)
	} else {
		replay, err = h.sess.Attach(w)
	}
	if err == nil {
		h.attached = c
	}
	h.mu.Unlock()
	if err != nil {
		return err
	}
	return w.start(AttachedMsg{Replay: replay, AltScreen: alt})
}

// attachWriter is the session's output target for one attach. Output that
// arrives before the attached message is sent is held back, so it can't
// overtake the replay.
type attachWriter struct {
	c       *hostConn
	mu      sync.Mutex
	started bool
	pending []byte
}

func (w *attachWriter) start(attached AttachedMsg) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.started = true
	if err := w.c.send(attached); err != nil {
		return err
	}
	if len(w.pending) > 0 {
		err := w.sendOutput(w.pending)
		w.pending = nil
		return err
	}
	return nil
}

func (w *attachWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.started {
		w.pending = append(w.pending, p...)
		return len(p), nil
	}
	if err := w.sendOutput(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *attachWriter) sendOutput(p []byte) error {
	for len(p) > 0 {
		n := min(len(p), MaxFramePayload)
		if err := w.c.send(OutputMsg(p[:n])); err != nil {
			return err
		}
		p = p[n:]
	}
	return nil
}

// Socket placement.

// sessionIDPattern matches a session id: 8 lowercase hex characters.
var sessionIDPattern = regexp.MustCompile(`^[0-9a-f]{8}$`)

// ValidSessionID reports whether id is a well-formed session id. Ids become
// file names, so anything else is rejected.
func ValidSessionID(id string) bool { return sessionIDPattern.MatchString(id) }

// sunPathMax is the size of sockaddr_un's sun_path on goos, including the
// trailing NUL.
func sunPathMax(goos string) int {
	if goos == "linux" {
		return 108
	}
	return 104 // macOS and the BSDs
}

// checkSocketPath fails with a clear error for a path too long to bind,
// which bind() would otherwise report only as "invalid argument".
func checkSocketPath(path, goos string) error {
	if max := sunPathMax(goos); len(path)+1 > max {
		return fmt.Errorf("socket path %s is %d bytes; %s allows at most %d (sun_path is %d bytes with the trailing NUL)",
			path, len(path), goos, max-1, max)
	}
	return nil
}

// socketPlacement is the environment sessionSocketPlacement decides from.
type socketPlacement struct {
	goos          string
	home          string
	xdgRuntimeDir string
	tempDir       string
	uid           int
}

// sessionSocketPlacement returns where a session's socket goes and the
// directory to create for it: ~/.config/b3tty/sessions/<id>.sock, or, if
// that's too long for sun_path, a private directory under $XDG_RUNTIME_DIR
// (Linux, when set) or the temp directory. It touches no files.
func sessionSocketPlacement(env socketPlacement, id string) (path, dir string, err error) {
	if !ValidSessionID(id) {
		return "", "", fmt.Errorf("invalid session id %q", id)
	}
	name := id + ".sock"
	dir = filepath.Join(env.home, ".config", "b3tty", "sessions")
	path = filepath.Join(dir, name)
	defaultErr := checkSocketPath(path, env.goos)
	if defaultErr == nil {
		return path, dir, nil
	}
	if env.goos == "linux" && env.xdgRuntimeDir != "" {
		dir = filepath.Join(env.xdgRuntimeDir, "b3tty")
	} else {
		// The temp directory may be shared (/tmp), hence a per-user name.
		dir = filepath.Join(env.tempDir, "b3tty-"+strconv.Itoa(env.uid))
	}
	path = filepath.Join(dir, name)
	if err := checkSocketPath(path, env.goos); err != nil {
		return "", "", fmt.Errorf("%v, and the fallback is too long too: %w", defaultErr, err)
	}
	return path, dir, nil
}

// SessionSocketPath returns the socket path for session id, creating its
// directory (0700) if needed. Callers store it, since it may not be in the
// default directory.
func SessionSocketPath(id string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	env := socketPlacement{
		goos:          runtime.GOOS,
		home:          home,
		xdgRuntimeDir: os.Getenv("XDG_RUNTIME_DIR"),
		tempDir:       os.TempDir(),
		uid:           os.Getuid(),
	}
	path, dir, err := sessionSocketPlacement(env, id)
	if err != nil {
		return "", err
	}
	if err := ensurePrivateDir(dir); err != nil {
		return "", err
	}
	return path, nil
}

// ensurePrivateDir creates dir (and its parents) if needed and makes sure
// it is a real directory owned by this user with mode 0700. A directory
// someone else owns, or a symlink, is refused: in a shared temp directory
// either could let another user reach the sockets.
func ensurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s is owned by uid %d, not %d", dir, st.Uid, os.Getuid())
	}
	if fi.Mode().Perm() != 0o700 {
		return os.Chmod(dir, 0o700)
	}
	return nil
}

// listenSessionSocket listens on a Unix socket at path, mode 0600, after
// checking the path's length.
func listenSessionSocket(path string) (*net.UnixListener, error) {
	if err := checkSocketPath(path, runtime.GOOS); err != nil {
		return nil, err
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	// The directory is 0700, so the moment before the chmod exposes
	// nothing.
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, err
	}
	return ln, nil
}
