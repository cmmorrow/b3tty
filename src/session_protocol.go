package src

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// The session protocol is what the server and a session-host process speak
// over the host's Unix socket. It is only a codec: it opens no sockets and
// starts no goroutines.
//
// Each frame is a 1-byte FrameType, a 4-byte big-endian payload length, and
// the payload. Control messages carry JSON; terminal data (input, output,
// and the replay in attached) is raw bytes.

// ProtocolVersion is the session protocol version. Bump it on any change a
// host built from an older binary couldn't understand. The hello and hello
// reply frames must stay decodable across versions, so that a server can
// still identify (and kill) a host running an older binary.
const ProtocolVersion = 1

// MaxFramePayload is the largest payload a frame may carry. It leaves room
// for a full default replay buffer (DEFAULT_REPLAY_BUFFER_SIZE) in one
// attached frame; senders must keep input and output chunks below it.
const MaxFramePayload = 4 << 20

// frameHeaderSize is the type byte plus the payload length.
const frameHeaderSize = 5

// FrameType identifies a frame's message.
type FrameType byte

const (
	FrameHello      FrameType = iota + 1 // server → host: HelloMsg
	FrameHelloReply                      // host → server: HelloReplyMsg
	FrameAttach                          // server → host: AttachMsg
	FrameAttached                        // host → server: AttachedMsg
	FrameDetach                          // server → host: DetachMsg
	FrameInput                           // server → host: InputMsg
	FrameOutput                          // host → server: OutputMsg
	FrameResize                          // server → host: ResizeMsg
	FrameRename                          // server → host: RenameMsg
	FrameExited                          // host → server: ExitedMsg
	FrameKill                            // server → host: KillMsg
	FrameError                           // host → server: ErrorMsg
	frameTypeEnd                         // one past the last known type
)

var frameTypeNames = [...]string{
	FrameHello:      "hello",
	FrameHelloReply: "hello-reply",
	FrameAttach:     "attach",
	FrameAttached:   "attached",
	FrameDetach:     "detach",
	FrameInput:      "input",
	FrameOutput:     "output",
	FrameResize:     "resize",
	FrameRename:     "rename",
	FrameExited:     "exited",
	FrameKill:       "kill",
	FrameError:      "error",
}

func (t FrameType) known() bool { return t >= FrameHello && t < frameTypeEnd }

func (t FrameType) String() string {
	if t.known() {
		return frameTypeNames[t]
	}
	return fmt.Sprintf("unknown(%d)", byte(t))
}

var (
	// ErrFrameTooLarge is returned for a frame whose payload exceeds
	// MaxFramePayload, before anything is allocated for it.
	ErrFrameTooLarge = errors.New("session protocol: frame too large")
	// ErrUnknownFrameType is returned for a frame type this version
	// doesn't define.
	ErrUnknownFrameType = errors.New("session protocol: unknown frame type")
	// ErrTruncatedFrame is returned when the stream ends inside a frame.
	// It wraps io.ErrUnexpectedEOF. A stream that ends between frames
	// returns io.EOF instead.
	ErrTruncatedFrame = fmt.Errorf("session protocol: truncated frame: %w", io.ErrUnexpectedEOF)
	// ErrMalformedPayload is returned for a payload that doesn't decode as
	// its frame type's message.
	ErrMalformedPayload = errors.New("session protocol: malformed payload")
)

// VersionMismatchError reports a peer speaking another protocol version.
// The server lists such a host as "needs old version, kill only".
type VersionMismatchError struct {
	Got, Want int
}

func (e *VersionMismatchError) Error() string {
	return fmt.Sprintf("session protocol: peer speaks version %d, want %d", e.Got, e.Want)
}

// IsVersionMismatch reports whether err is, or wraps, a VersionMismatchError.
func IsVersionMismatch(err error) bool {
	var vm *VersionMismatchError
	return errors.As(err, &vm)
}

// SessionMessage is a decoded protocol message: one of the *Msg types below.
type SessionMessage interface {
	frameType() FrameType
}

// HelloMsg opens a connection (server → host).
type HelloMsg struct {
	Version int `json:"version"`
}

// HelloReplyMsg answers HelloMsg with the host's version and the session's
// metadata (host → server). A host replies with its own Version even when
// the hello's version differs, so the server can tell a mismatch from a
// failure.
type HelloReplyMsg struct {
	Version   int         `json:"version"`
	ID        string      `json:"id"`
	Nickname  string      `json:"nickname"`
	Mode      SessionMode `json:"mode"`
	Profile   string      `json:"profile"`
	PID       int         `json:"pid"`
	Cols      uint16      `json:"cols"`
	Rows      uint16      `json:"rows"`
	CreatedAt time.Time   `json:"created_at"`
	// Attached reports whether a client is attached to the session.
	Attached bool `json:"attached"`
}

// AttachMsg asks the host to send output to this connection (server → host).
type AttachMsg struct{}

// AttachedMsg answers AttachMsg with the output to replay first, as from
// Session.Attach (host → server). Its payload is one flags byte followed by
// the raw replay, so the replay isn't base64-inflated by JSON.
type AttachedMsg struct {
	Replay []byte
	// AltScreen reports whether the replayed output left the alternate
	// screen on.
	AltScreen bool
}

// DetachMsg stops output to this connection; the session keeps running
// (server → host).
type DetachMsg struct{}

// InputMsg is raw keyboard input for the pty (server → host).
type InputMsg []byte

// OutputMsg is raw pty output (host → server).
type OutputMsg []byte

// ResizeMsg resizes the pty (server → host).
type ResizeMsg struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// RenameMsg sets the session's nickname (server → host).
type RenameMsg struct {
	Nickname string `json:"nickname"`
}

// ExitedMsg reports that the child exited, with Session.ExitCode's value
// (host → server).
type ExitedMsg struct {
	Code int `json:"code"`
}

// KillMsg ends the session (server → host).
type KillMsg struct{}

// ErrorMsg reports that the host failed a request (host → server).
type ErrorMsg struct {
	Message string `json:"message"`
}

func (HelloMsg) frameType() FrameType      { return FrameHello }
func (HelloReplyMsg) frameType() FrameType { return FrameHelloReply }
func (AttachMsg) frameType() FrameType     { return FrameAttach }
func (AttachedMsg) frameType() FrameType   { return FrameAttached }
func (DetachMsg) frameType() FrameType     { return FrameDetach }
func (InputMsg) frameType() FrameType      { return FrameInput }
func (OutputMsg) frameType() FrameType     { return FrameOutput }
func (ResizeMsg) frameType() FrameType     { return FrameResize }
func (RenameMsg) frameType() FrameType     { return FrameRename }
func (ExitedMsg) frameType() FrameType     { return FrameExited }
func (KillMsg) frameType() FrameType       { return FrameKill }
func (ErrorMsg) frameType() FrameType      { return FrameError }

// String returns "quick" or "persistent".
func (m SessionMode) String() string {
	switch m {
	case SessionQuick:
		return "quick"
	case SessionPersistent:
		return "persistent"
	}
	return fmt.Sprintf("SessionMode(%d)", int(m))
}

// MarshalText encodes the mode as "quick" or "persistent".
func (m SessionMode) MarshalText() ([]byte, error) {
	switch m {
	case SessionQuick, SessionPersistent:
		return []byte(m.String()), nil
	}
	return nil, fmt.Errorf("invalid session mode %d", int(m))
}

// UnmarshalText decodes "quick" or "persistent".
func (m *SessionMode) UnmarshalText(b []byte) error {
	switch string(b) {
	case "quick":
		*m = SessionQuick
	case "persistent":
		*m = SessionPersistent
	default:
		return fmt.Errorf("invalid session mode %q", b)
	}
	return nil
}

// encodePayload returns msg's frame payload.
func encodePayload(msg SessionMessage) ([]byte, error) {
	switch m := msg.(type) {
	case AttachMsg, DetachMsg, KillMsg:
		return nil, nil
	case InputMsg:
		return m, nil
	case OutputMsg:
		return m, nil
	case AttachedMsg:
		p := make([]byte, 1+len(m.Replay))
		if m.AltScreen {
			p[0] = 1
		}
		copy(p[1:], m.Replay)
		return p, nil
	case nil:
		return nil, errors.New("session protocol: nil message")
	default:
		return json.Marshal(m)
	}
}

// decodePayload decodes a payload of a known frame type. Unknown JSON
// fields are ignored, so a newer peer can add fields.
func decodePayload(t FrameType, p []byte) (SessionMessage, error) {
	malformed := func(err error) error {
		return fmt.Errorf("%w: %s: %v", ErrMalformedPayload, t, err)
	}
	var msg SessionMessage
	switch t {
	case FrameAttach, FrameDetach, FrameKill:
		if len(p) != 0 {
			return nil, malformed(fmt.Errorf("unexpected %d-byte payload", len(p)))
		}
		switch t {
		case FrameAttach:
			return AttachMsg{}, nil
		case FrameDetach:
			return DetachMsg{}, nil
		}
		return KillMsg{}, nil
	case FrameInput:
		return InputMsg(p), nil
	case FrameOutput:
		return OutputMsg(p), nil
	case FrameAttached:
		if len(p) == 0 || p[0] > 1 {
			return nil, malformed(errors.New("missing or invalid flags byte"))
		}
		return AttachedMsg{Replay: p[1:], AltScreen: p[0] == 1}, nil
	case FrameHello:
		var m HelloMsg
		if err := json.Unmarshal(p, &m); err != nil {
			return nil, malformed(err)
		}
		msg = m
	case FrameHelloReply:
		var m HelloReplyMsg
		if err := json.Unmarshal(p, &m); err != nil {
			return nil, malformed(err)
		}
		msg = m
	case FrameResize:
		var m ResizeMsg
		if err := json.Unmarshal(p, &m); err != nil {
			return nil, malformed(err)
		}
		msg = m
	case FrameRename:
		var m RenameMsg
		if err := json.Unmarshal(p, &m); err != nil {
			return nil, malformed(err)
		}
		msg = m
	case FrameExited:
		var m ExitedMsg
		if err := json.Unmarshal(p, &m); err != nil {
			return nil, malformed(err)
		}
		msg = m
	case FrameError:
		var m ErrorMsg
		if err := json.Unmarshal(p, &m); err != nil {
			return nil, malformed(err)
		}
		msg = m
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnknownFrameType, byte(t))
	}
	return msg, nil
}

// SessionEncoder writes messages to a connection. Send is safe to call from
// several goroutines: each frame is written whole under a mutex.
type SessionEncoder struct {
	mu sync.Mutex
	w  io.Writer
}

// NewSessionEncoder returns an encoder writing to w.
func NewSessionEncoder(w io.Writer) *SessionEncoder { return &SessionEncoder{w: w} }

// Send writes msg as one frame. A payload over MaxFramePayload returns
// ErrFrameTooLarge and writes nothing.
func (e *SessionEncoder) Send(msg SessionMessage) error {
	payload, err := encodePayload(msg)
	if err != nil {
		return fmt.Errorf("session protocol: encode: %w", err)
	}
	t := msg.frameType()
	if len(payload) > MaxFramePayload {
		return fmt.Errorf("%w: %s payload is %d bytes, max %d", ErrFrameTooLarge, t, len(payload), MaxFramePayload)
	}
	var header [frameHeaderSize]byte
	header[0] = byte(t)
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))

	e.mu.Lock()
	defer e.mu.Unlock()
	// On a socket this is one writev; otherwise two writes, still under
	// the mutex. An empty payload is left out: a zero-length Write on a
	// net.Pipe blocks until the peer reads, which it never would.
	bufs := net.Buffers{header[:]}
	if len(payload) > 0 {
		bufs = append(bufs, payload)
	}
	_, err = bufs.WriteTo(e.w)
	return err
}

// SessionDecoder reads messages from a connection. It is not safe for
// concurrent use: one goroutine reads.
type SessionDecoder struct {
	r      io.Reader
	header [frameHeaderSize]byte
}

// NewSessionDecoder returns a decoder reading from r.
func NewSessionDecoder(r io.Reader) *SessionDecoder { return &SessionDecoder{r: r} }

// Next reads one frame and decodes it. It returns io.EOF if the stream
// ended cleanly between frames, ErrTruncatedFrame if it ended inside one,
// and ErrUnknownFrameType, ErrFrameTooLarge or ErrMalformedPayload (all
// wrapped) for a bad frame. After an error other than ErrMalformedPayload
// the stream is out of sync and the connection should be closed.
func (d *SessionDecoder) Next() (SessionMessage, error) {
	if _, err := io.ReadFull(d.r, d.header[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("%w (header)", ErrTruncatedFrame)
		}
		return nil, err // io.EOF between frames, or a read error
	}
	t := FrameType(d.header[0])
	n := binary.BigEndian.Uint32(d.header[1:])
	if !t.known() {
		return nil, fmt.Errorf("%w: %d", ErrUnknownFrameType, byte(t))
	}
	if n > MaxFramePayload {
		return nil, fmt.Errorf("%w: %s payload is %d bytes, max %d", ErrFrameTooLarge, t, n, MaxFramePayload)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(d.r, payload); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("%w (%s payload)", ErrTruncatedFrame, t)
		}
		return nil, err
	}
	return decodePayload(t, payload)
}

// SessionHandshake sends a hello and reads the host's reply. A reply with
// another protocol version returns the reply together with a
// *VersionMismatchError (see IsVersionMismatch), so the caller still has
// the host's metadata, such as its pid, to list or kill it. An ErrorMsg
// reply or any other message is an error.
func SessionHandshake(enc *SessionEncoder, dec *SessionDecoder) (HelloReplyMsg, error) {
	if err := enc.Send(HelloMsg{Version: ProtocolVersion}); err != nil {
		return HelloReplyMsg{}, fmt.Errorf("send hello: %w", err)
	}
	msg, err := dec.Next()
	if err != nil {
		return HelloReplyMsg{}, fmt.Errorf("read hello reply: %w", err)
	}
	switch m := msg.(type) {
	case HelloReplyMsg:
		if m.Version != ProtocolVersion {
			return m, &VersionMismatchError{Got: m.Version, Want: ProtocolVersion}
		}
		return m, nil
	case ErrorMsg:
		return HelloReplyMsg{}, fmt.Errorf("host refused hello: %s", m.Message)
	default:
		return HelloReplyMsg{}, fmt.Errorf("session protocol: expected hello-reply, got %s", msg.frameType())
	}
}
