package src

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
)

// Session is one shell running under a pty. terminalHandler depends only on
// this interface, so the pty can live in this process (localSession) or,
// later, in a separate session-host process.
type Session interface {
	// Write sends input to the pty.
	io.Writer
	ID() string
	Resize(cols, rows uint16) error
	// Attach makes w the session's output target, replacing any previous
	// one, and returns the output to replay first. Both happen atomically:
	// output produced after the replay was taken goes to w and nothing is
	// lost or repeated between them. If a Write to w fails, the session
	// detaches w.
	Attach(w io.Writer) (replay []byte, err error)
	// Detach stops delivering output; the session keeps running.
	Detach()
	// Kill ends the session: kills the child, then closes the pty.
	Kill() error
	// Done is closed once the child has exited and the pty is closed.
	Done() <-chan struct{}
	// ExitCode is the child's exit code, or -1 if it was ended by a signal
	// (including Kill). It is valid only after Done is closed.
	ExitCode() int
}

// SessionMode is how long a session lives, set when it is created.
type SessionMode int

const (
	// SessionQuick ends when its tab closes; it never re-attaches, so it
	// keeps no replay buffer.
	SessionQuick SessionMode = iota
	// SessionPersistent outlives its tab and keeps recent output in an
	// outputBuffer for replay on re-attach.
	SessionPersistent
)

// SessionOptions configures newLocalSession.
type SessionOptions struct {
	Mode       SessionMode
	Profile    Profile
	Cols, Rows uint16
	// BufferSize caps a persistent session's replay buffer, in bytes;
	// 0 means DEFAULT_REPLAY_BUFFER_SIZE.
	BufferSize int
	// CommandSleep pauses before and between the profile's startup
	// commands; nil means time.Sleep.
	CommandSleep func(time.Duration)
}

// localSession is a Session whose pty and child process live in this
// process.
type localSession struct {
	id   string
	mode SessionMode
	cmd  *exec.Cmd
	ptmx *os.File

	mu sync.Mutex // guards out and buf
	// out is the attached output target, or nil.
	out io.Writer
	// buf holds output for replay. A persistent session always has one. A
	// quick session has one only until its first Attach, so output written
	// before the tab attaches (the first prompt, startup command echo) is
	// not lost; after that it keeps nothing.
	buf *outputBuffer

	done     chan struct{}
	exitCode int
}

// newSessionID returns a short random id (8 hex characters).
func newSessionID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// newLocalSession starts the profile's shell under a pty of the given size,
// starts reading its output, and runs the profile's startup commands once.
func newLocalSession(opts SessionOptions) (*localSession, error) {
	id, err := newSessionID()
	if err != nil {
		return nil, fmt.Errorf("session id: %w", err)
	}
	profile := opts.Profile
	// Start the shell via /bin/sh -c so that shell flags and paths are
	// handled uniformly regardless of the configured shell binary.
	c := exec.Command("/bin/sh", "-c", profile.Shell)
	c, err = profile.ApplyToCommand(c)
	if err != nil {
		return nil, fmt.Errorf("apply profile to command: %w", err)
	}

	Debugf("cols: %d", opts.Cols)
	Debugf("rows: %d", opts.Rows)
	Debug("starting pty....")
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Cols: opts.Cols, Rows: opts.Rows})
	if err != nil {
		return nil, fmt.Errorf("start pty: %w", err)
	}

	s := &localSession{
		id:       id,
		mode:     opts.Mode,
		cmd:      c,
		ptmx:     ptmx,
		buf:      newOutputBuffer(opts.BufferSize),
		done:     make(chan struct{}),
		exitCode: -1,
	}
	go s.readLoop()
	if len(profile.Commands) > 0 {
		sleep := opts.CommandSleep
		if sleep == nil {
			sleep = time.Sleep
		}
		go s.runCommands(profile.Commands, sleep)
	}
	return s, nil
}

func (s *localSession) ID() string { return s.id }

func (s *localSession) Write(p []byte) (int, error) { return s.ptmx.Write(p) }

func (s *localSession) Resize(cols, rows uint16) error {
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

func (s *localSession) Attach(w io.Writer) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var replay []byte
	if s.buf != nil {
		replay = s.buf.Bytes()
		if s.mode == SessionQuick {
			s.buf = nil
		}
	}
	s.out = w
	return replay, nil
}

func (s *localSession) Detach() {
	s.mu.Lock()
	s.out = nil
	s.mu.Unlock()
}

// AltScreen reports whether the output so far left the alternate screen on.
// It is always false for a quick session.
func (s *localSession) AltScreen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf != nil && s.buf.AltScreen()
}

// Kill kills the child process, then closes the pty master. Killing the
// process is essential, not just cosmetic: on some platforms (observed on
// macOS) closing ptmx from one goroutine does not interrupt a concurrent
// blocking Read() on it from another, and SetReadDeadline is not supported
// on pty file descriptors ("file type does not support deadline"), so a
// still-running child could leave readLoop blocked on Read() forever.
// Killing the child closes its pty slave, which reliably unblocks a pending
// master-side Read() with EOF.
func (s *localSession) Kill() error {
	err := s.cmd.Process.Kill()
	_ = s.ptmx.Close() // Best effort.
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

func (s *localSession) Done() <-chan struct{} { return s.done }

func (s *localSession) ExitCode() int {
	select {
	case <-s.done:
		return s.exitCode
	default:
		return -1
	}
}

// readLoop reads the pty until it closes, whether or not anything is
// attached: a pty nobody reads fills the kernel buffer and blocks the shell.
func (s *localSession) readLoop() {
	buf := make([]byte, BUFFER_SIZE)
	for {
		n, err := s.ptmx.Read(buf)
		Debugf("bytes read from buffer: %d", n)
		if err != nil {
			switch err {
			case io.EOF:
				Info("terminal session closed")
			default:
				Errorf("pty read: %v", err)
			}
			_ = s.Kill()
			_ = s.cmd.Wait()
			if s.cmd.ProcessState != nil {
				s.exitCode = s.cmd.ProcessState.ExitCode()
			}
			close(s.done)
			return
		}
		s.deliver(buf[:n])
	}
}

// deliver appends output to the replay buffer and writes it to the attached
// target, detaching a target whose Write fails.
func (s *localSession) deliver(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.buf != nil {
		_, _ = s.buf.Write(p)
	}
	if s.out != nil {
		if _, err := s.out.Write(p); err != nil {
			s.out = nil
		}
	}
}

// runCommands writes the profile's startup commands to the pty, after a
// pause for the shell to start and with a short pause between commands.
func (s *localSession) runCommands(commands []string, sleep func(time.Duration)) {
	sleep(time.Second * 1)
	for _, command := range commands {
		select {
		case <-s.done:
			return
		default:
		}
		if _, err := s.Write(formatCommand(command)); err != nil {
			if errors.Is(err, os.ErrClosed) {
				Debugf("write to pty: %v", err)
			} else {
				Errorf("write to pty: %v", err)
			}
			_ = s.Kill()
			return
		}
		sleep(time.Millisecond * 200)
	}
}
