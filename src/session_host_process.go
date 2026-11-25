package src

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"
)

// SESSION_HOST_MEMORY_LIMIT is the soft memory limit (GOMEMLIMIT) a host
// sets for itself unless GOMEMLIMIT is already set, so the runtime returns
// freed memory to the OS sooner. A host's live heap is the replay buffer
// plus small change, well under it. Measured on macOS after a burst of
// output, 8 MiB keeps a host's footprint at 8–10 MB where 16 MiB let it sit
// at 10–12 MB; a lower limit saved nothing more.
const SESSION_HOST_MEMORY_LIMIT = 8 << 20

// SESSION_HOST_START_TIMEOUT bounds how long SpawnSessionHost waits for a
// new host to answer its hello.
const SESSION_HOST_START_TIMEOUT = 10 * time.Second

// maxSessionHostSpec caps the spawn spec a host reads from stdin.
const maxSessionHostSpec = 1 << 20

// sessionHostOwnerFD is the descriptor a host finds its owner connection on:
// the first of exec.Cmd's ExtraFiles.
const sessionHostOwnerFD = 3

// SessionHostSpec is everything a new host needs, written by the server as
// JSON on the host's stdin, which is then closed. It goes neither in argv
// (visible in ps) nor in the environment (inherited by the shell). It never
// carries the auth token: the host doesn't need it.
type SessionHostSpec struct {
	ID          string      `json:"id"`
	Mode        SessionMode `json:"mode"`
	Nickname    string      `json:"nickname"`
	ProfileName string      `json:"profile_name"`
	Profile     Profile     `json:"profile"`
	// Env is the shell's environment; nil means the host's own, which it
	// inherits from the server.
	Env    []string `json:"env,omitempty"`
	Cols   uint16   `json:"cols"`
	Rows   uint16   `json:"rows"`
	Socket string   `json:"socket"`
}

// SessionHostSpawnOptions overrides how SpawnSessionHost starts a host.
// Tests use it to run the test binary instead of b3tty.
type SessionHostSpawnOptions struct {
	// Executable defaults to os.Executable().
	Executable string
	// Args defaults to {"session-host"}.
	Args []string
	// Env is the host's environment; nil means the server's own.
	Env []string
	// Timeout overrides SESSION_HOST_START_TIMEOUT.
	Timeout time.Duration
}

// SpawnedHost is a running host started by SpawnSessionHost.
type SpawnedHost struct {
	ID     string
	Socket string
	PID    int
	// Conn is the owner connection, already through the hello. A quick
	// host ends when it is closed, including when this process dies.
	Conn  net.Conn
	Hello HelloReplyMsg
	// Exited is closed once the host process has exited and been reaped.
	Exited <-chan struct{}
}

// SpawnSessionHost starts a session host for spec, in its own session
// (setsid) so it leaves the server's process group, and returns once the
// host is listening on its socket and has answered a hello on the owner
// connection. An empty spec.ID gets a new one and an empty spec.Socket the
// default path for it (SessionSocketPath).
func SpawnSessionHost(spec SessionHostSpec, opts SessionHostSpawnOptions) (*SpawnedHost, error) {
	var err error
	if spec.ID == "" {
		if spec.ID, err = newSessionID(); err != nil {
			return nil, fmt.Errorf("session id: %w", err)
		}
	}
	if !ValidSessionID(spec.ID) {
		return nil, fmt.Errorf("invalid session id %q", spec.ID)
	}
	if spec.Socket == "" {
		if spec.Socket, err = SessionSocketPath(spec.ID); err != nil {
			return nil, err
		}
	}
	specJSON, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	exe := opts.Executable
	if exe == "" {
		if exe, err = os.Executable(); err != nil {
			return nil, err
		}
	}
	args := opts.Args
	if args == nil {
		args = []string{"session-host"}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = SESSION_HOST_START_TIMEOUT
	}

	ours, theirs, err := socketPair()
	if err != nil {
		return nil, fmt.Errorf("owner socket: %w", err)
	}
	defer theirs.Close()

	cmd := exec.Command(exe, args...)
	cmd.Env = opts.Env
	cmd.ExtraFiles = []*os.File{theirs}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = ours.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = ours.Close()
		return nil, fmt.Errorf("start session host: %w", err)
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	_ = theirs.Close()

	fail := func(err error) (*SpawnedHost, error) {
		_ = cmd.Process.Kill()
		_ = ours.Close()
		return nil, err
	}
	conn, err := net.FileConn(ours)
	_ = ours.Close()
	if err != nil {
		return fail(fmt.Errorf("owner connection: %w", err))
	}
	fail = func(err error) (*SpawnedHost, error) {
		_ = cmd.Process.Kill()
		_ = conn.Close()
		return nil, err
	}
	// A pipe holds far more than a spec, so this doesn't wait on the host.
	_, err = stdin.Write(specJSON)
	if cerr := stdin.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fail(fmt.Errorf("write spec: %w", err))
	}

	_ = conn.SetDeadline(time.Now().Add(timeout))
	hello, err := hostHandshake(conn)
	if err != nil {
		return fail(fmt.Errorf("session host %s: %w", spec.ID, err))
	}
	_ = conn.SetDeadline(time.Time{})
	return &SpawnedHost{
		ID:     spec.ID,
		Socket: spec.Socket,
		PID:    cmd.Process.Pid,
		Conn:   conn,
		Hello:  hello,
		Exited: exited,
	}, nil
}

// hostHandshake is SessionHandshake for a host that may have failed to
// start: such a host sends an error message and exits without reading, so
// the hello may fail to send while the error is still waiting to be read.
func hostHandshake(conn net.Conn) (HelloReplyMsg, error) {
	enc, dec := NewSessionEncoder(conn), NewSessionDecoder(conn)
	sendErr := enc.Send(HelloMsg{Version: ProtocolVersion})
	msg, err := dec.Next()
	if err != nil {
		if sendErr != nil {
			return HelloReplyMsg{}, fmt.Errorf("send hello: %w", sendErr)
		}
		if errors.Is(err, io.EOF) {
			return HelloReplyMsg{}, errors.New("exited before answering hello")
		}
		return HelloReplyMsg{}, fmt.Errorf("read hello reply: %w", err)
	}
	switch m := msg.(type) {
	case HelloReplyMsg:
		if m.Version != ProtocolVersion {
			return m, &VersionMismatchError{Got: m.Version, Want: ProtocolVersion}
		}
		return m, nil
	case ErrorMsg:
		return HelloReplyMsg{}, errors.New(m.Message)
	}
	return HelloReplyMsg{}, fmt.Errorf("expected hello-reply, got %s", msg.frameType())
}

// socketPair returns a connected pair of Unix stream sockets, both
// close-on-exec (exec.Cmd's ExtraFiles clears that for the child's copy).
func socketPair() (*os.File, *os.File, error) {
	// Hold ForkLock so a concurrent fork can't inherit the descriptors
	// before they are marked close-on-exec.
	syscall.ForkLock.RLock()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err == nil {
		syscall.CloseOnExec(fds[0])
		syscall.CloseOnExec(fds[1])
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(fds[0]), "session-host-owner"), os.NewFile(uintptr(fds[1]), "session-host-owner"), nil
}

// RunSessionHostProcess is the `b3tty session-host` process: it reads a
// SessionHostSpec from stdin, takes its owner connection from fd 3, starts
// the session, and serves it on the spec's socket until it ends. It returns
// the process exit status. It loads no config, themes or plugins.
func RunSessionHostProcess() int {
	hardenSessionHost()

	// The owner connection first, so a failure below can be reported on it.
	// Run by hand, fd 3 is whatever the runtime opened first (the
	// netpoller's kqueue or epoll), which must not be touched.
	var st syscall.Stat_t
	if err := syscall.Fstat(sessionHostOwnerFD, &st); err != nil || st.Mode&syscall.S_IFMT != syscall.S_IFSOCK {
		fmt.Fprintln(os.Stderr, "b3tty session-host is started by the b3tty server; it can't be run directly")
		return 2
	}
	ownerFile := os.NewFile(sessionHostOwnerFD, "owner")
	owner, err := net.FileConn(ownerFile) // a close-on-exec duplicate
	_ = ownerFile.Close()
	if err != nil {
		return 2
	}
	fail := func(format string, args ...any) int {
		_ = owner.SetWriteDeadline(time.Now().Add(time.Second))
		_ = NewSessionEncoder(owner).Send(ErrorMsg{Message: fmt.Sprintf(format, args...)})
		_ = owner.Close()
		return 1
	}

	var spec SessionHostSpec
	dec := json.NewDecoder(io.LimitReader(os.Stdin, maxSessionHostSpec))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		return fail("read spec: %v", err)
	}
	if err := redirectStdioToDevNull(); err != nil {
		return fail("redirect stdio: %v", err)
	}
	if !ValidSessionID(spec.ID) {
		return fail("invalid session id %q", spec.ID)
	}
	if spec.Mode != SessionQuick && spec.Mode != SessionPersistent {
		return fail("invalid session mode %d", int(spec.Mode))
	}

	ln, err := listenSessionSocket(spec.Socket)
	if err != nil {
		return fail("listen: %v", err)
	}
	sess, err := newLocalSession(SessionOptions{
		ID:      spec.ID,
		Mode:    spec.Mode,
		Profile: spec.Profile,
		Env:     spec.Env,
		Cols:    spec.Cols,
		Rows:    spec.Rows,
	})
	if err != nil {
		_ = ln.Close()
		return fail("start session: %v", err)
	}

	// SIGTERM and SIGINT end the session cleanly, so the socket is removed.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		_ = sess.Kill()
	}()

	err = ServeSessionHost(ln, sess, SessionHostConfig{
		Mode:      spec.Mode,
		Nickname:  spec.Nickname,
		Profile:   spec.ProfileName,
		PID:       os.Getpid(),
		Cols:      spec.Cols,
		Rows:      spec.Rows,
		CreatedAt: time.Now(),
		Owner:     owner,
	})
	if err != nil {
		return 1
	}
	return 0
}

// hardenSessionHost sets up the process before it holds any terminal
// output: no SIGHUP (from the terminal the server was started in), no core
// dumps (the replay buffer can hold secrets), and a low memory limit.
func hardenSessionHost() {
	signal.Ignore(syscall.SIGHUP)
	_ = syscall.Setrlimit(syscall.RLIMIT_CORE, &syscall.Rlimit{Cur: 0, Max: 0})
	disableDumpable()
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(SESSION_HOST_MEMORY_LIMIT)
	}
}

// redirectStdioToDevNull points fds 0–2 at /dev/null, so nothing the host
// logs goes anywhere and nothing waits on the server's terminal.
func redirectStdioToDevNull() error {
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer null.Close()
	for fd := 0; fd <= 2; fd++ {
		if err := dupFD(int(null.Fd()), fd); err != nil {
			return err
		}
	}
	return nil
}
