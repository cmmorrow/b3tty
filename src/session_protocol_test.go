package src

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"
)

// pipeCodec returns an encoder on one end of a net.Pipe and a decoder on
// the other.
func pipeCodec(t *testing.T) (*SessionEncoder, *SessionDecoder, net.Conn, net.Conn) {
	t.Helper()
	a, b := net.Pipe()
	t.Cleanup(func() { a.Close(); b.Close() })
	return NewSessionEncoder(a), NewSessionDecoder(b), a, b
}

// sendAsync sends msg from a goroutine (net.Pipe writes block until read)
// and returns a channel carrying Send's error.
func sendAsync(enc *SessionEncoder, msg SessionMessage) <-chan error {
	errc := make(chan error, 1)
	go func() { errc <- enc.Send(msg) }()
	return errc
}

// writeAsync writes raw bytes from a goroutine, then closes w.
func writeAsync(w net.Conn, p []byte) {
	go func() {
		_, _ = w.Write(p)
		_ = w.Close()
	}()
}

func frameHeader(t FrameType, n uint32) []byte {
	h := make([]byte, frameHeaderSize)
	h[0] = byte(t)
	binary.BigEndian.PutUint32(h[1:], n)
	return h
}

func TestSessionProtocolRoundTrip(t *testing.T) {
	created := time.Date(2026, 11, 24, 9, 30, 0, 0, time.UTC)
	msgs := []SessionMessage{
		HelloMsg{Version: ProtocolVersion},
		HelloReplyMsg{
			Version: ProtocolVersion, ID: "7f3a0c21", Nickname: "build",
			Mode: SessionPersistent, Profile: "default", PID: 48211,
			Cols: 120, Rows: 40, CreatedAt: created, Attached: true,
		},
		HelloReplyMsg{Version: ProtocolVersion, ID: "00000001", Mode: SessionQuick, CreatedAt: created},
		AttachMsg{},
		AttachedMsg{Replay: []byte("\x1b[?1049h$ ls\r\n"), AltScreen: true},
		AttachedMsg{Replay: []byte{}, AltScreen: false},
		DetachMsg{},
		InputMsg("ls -la\r"),
		OutputMsg("total 0\r\n"),
		ResizeMsg{Cols: 80, Rows: 24},
		RenameMsg{Nickname: "räksmörgås"},
		ExitedMsg{Code: 0},
		ExitedMsg{Code: -1},
		KillMsg{},
		ErrorMsg{Message: "resize: bad file descriptor"},
	}
	enc, dec, _, _ := pipeCodec(t)
	for _, want := range msgs {
		errc := sendAsync(enc, want)
		got, err := dec.Next()
		if err != nil {
			t.Fatalf("%T: Next: %v", want, err)
		}
		if err := <-errc; err != nil {
			t.Fatalf("%T: Send: %v", want, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("round trip:\n got %#v\nwant %#v", got, want)
		}
	}
}

func TestSessionProtocolBinaryPayloads(t *testing.T) {
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	big := bytes.Repeat(all, DEFAULT_REPLAY_BUFFER_SIZE/len(all))
	cases := []SessionMessage{
		InputMsg{}, OutputMsg{},
		InputMsg(all), OutputMsg(all),
		InputMsg{0}, OutputMsg{0xff},
		AttachedMsg{Replay: all, AltScreen: true},
		AttachedMsg{Replay: big}, // a full default replay fits in one frame
	}
	enc, dec, _, _ := pipeCodec(t)
	for _, want := range cases {
		errc := sendAsync(enc, want)
		got, err := dec.Next()
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if err := <-errc; err != nil {
			t.Fatalf("Send: %v", err)
		}
		switch w := want.(type) {
		case InputMsg:
			g, ok := got.(InputMsg)
			if !ok || !bytes.Equal(g, w) {
				t.Errorf("input: got %v, want %v", got, w)
			}
		case OutputMsg:
			g, ok := got.(OutputMsg)
			if !ok || !bytes.Equal(g, w) {
				t.Errorf("output: got %v, want %v", got, w)
			}
		case AttachedMsg:
			g, ok := got.(AttachedMsg)
			if !ok || !bytes.Equal(g.Replay, w.Replay) || g.AltScreen != w.AltScreen {
				t.Errorf("attached: got %d bytes alt=%v, want %d bytes alt=%v",
					len(g.Replay), g.AltScreen, len(w.Replay), w.AltScreen)
			}
		}
	}
}

func TestSessionProtocolSendRejectsOversizedPayload(t *testing.T) {
	var buf bytes.Buffer
	enc := NewSessionEncoder(&buf)
	err := enc.Send(OutputMsg(make([]byte, MaxFramePayload+1)))
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("Send: got %v, want ErrFrameTooLarge", err)
	}
	if buf.Len() != 0 {
		t.Errorf("wrote %d bytes for a rejected frame", buf.Len())
	}
	// The largest allowed payload is fine.
	if err := enc.Send(OutputMsg(make([]byte, MaxFramePayload))); err != nil {
		t.Fatalf("Send max payload: %v", err)
	}
}

func TestSessionProtocolRejectsOversizedLengthWithoutAllocating(t *testing.T) {
	_, _, w, r := pipeCodec(t)
	dec := NewSessionDecoder(r)
	// Only the header is sent: a decoder that tried to read the payload
	// would block forever.
	writeAsync(w, frameHeader(FrameOutput, 0xffffffff))
	if _, err := dec.Next(); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("Next: got %v, want ErrFrameTooLarge", err)
	}

	// Measure allocation without the pipe's goroutines.
	header := frameHeader(FrameAttached, MaxFramePayload+1)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for range 100 {
		_, err := NewSessionDecoder(bytes.NewReader(header)).Next()
		if !errors.Is(err, ErrFrameTooLarge) {
			t.Fatalf("Next: got %v, want ErrFrameTooLarge", err)
		}
	}
	runtime.ReadMemStats(&after)
	if got := after.TotalAlloc - before.TotalAlloc; got > 1<<20 {
		t.Errorf("rejecting 100 oversized frames allocated %d bytes", got)
	}
}

func TestSessionProtocolTruncated(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"partial header", []byte{byte(FrameOutput), 0, 0}},
		{"type byte only", []byte{byte(FrameKill)}},
		{"missing payload", frameHeader(FrameOutput, 10)},
		{"short payload", append(frameHeader(FrameInput, 10), "abc"...)},
		{"short json payload", append(frameHeader(FrameResize, 20), `{"cols":8`...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, w, r := pipeCodec(t)
			writeAsync(w, tc.data)
			_, err := NewSessionDecoder(r).Next()
			if !errors.Is(err, ErrTruncatedFrame) || !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("got %v, want ErrTruncatedFrame", err)
			}
		})
	}
}

func TestSessionProtocolCleanEOF(t *testing.T) {
	_, _, w, r := pipeCodec(t)
	writeAsync(w, frameHeader(FrameKill, 0))
	dec := NewSessionDecoder(r)
	if msg, err := dec.Next(); err != nil || msg != (KillMsg{}) {
		t.Fatalf("first Next: %v, %v", msg, err)
	}
	if _, err := dec.Next(); err != io.EOF {
		t.Fatalf("got %v, want io.EOF between frames", err)
	}
}

func TestSessionProtocolUnknownType(t *testing.T) {
	for _, typ := range []FrameType{0, frameTypeEnd, 0xff} {
		_, _, w, r := pipeCodec(t)
		// The length is huge too: the type is checked first, and nothing
		// past the header is read.
		writeAsync(w, frameHeader(typ, 0xffffffff))
		_, err := NewSessionDecoder(r).Next()
		if !errors.Is(err, ErrUnknownFrameType) {
			t.Errorf("type %d: got %v, want ErrUnknownFrameType", typ, err)
		}
	}
}

func TestSessionProtocolMalformedPayload(t *testing.T) {
	cases := []struct {
		name    string
		typ     FrameType
		payload string
	}{
		{"hello not json", FrameHello, "hello"},
		{"hello empty", FrameHello, ""},
		{"hello-reply wrong type", FrameHelloReply, `{"version":"one"}`},
		{"hello-reply bad mode", FrameHelloReply, `{"version":1,"mode":"forever"}`},
		{"resize truncated json", FrameResize, `{"cols":80,`},
		{"resize negative", FrameResize, `{"cols":-1,"rows":24}`},
		{"rename not object", FrameRename, `["x"]`},
		{"exited string", FrameExited, `{"code":"0"}`},
		{"error bad json", FrameError, `{message}`},
		{"attached empty", FrameAttached, ""},
		{"attached bad flags", FrameAttached, "\x02replay"},
		{"kill with payload", FrameKill, "x"},
		{"attach with payload", FrameAttach, "{}"},
		{"detach with payload", FrameDetach, "\x00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, w, r := pipeCodec(t)
			writeAsync(w, append(frameHeader(tc.typ, uint32(len(tc.payload))), tc.payload...))
			_, err := NewSessionDecoder(r).Next()
			if !errors.Is(err, ErrMalformedPayload) {
				t.Fatalf("got %v, want ErrMalformedPayload", err)
			}
		})
	}
}

func TestSessionProtocolIgnoresUnknownJSONFields(t *testing.T) {
	_, _, w, r := pipeCodec(t)
	payload := `{"cols":80,"rows":24,"pixels":true}`
	writeAsync(w, append(frameHeader(FrameResize, uint32(len(payload))), payload...))
	msg, err := NewSessionDecoder(r).Next()
	if err != nil || msg != (ResizeMsg{Cols: 80, Rows: 24}) {
		t.Fatalf("got %v, %v", msg, err)
	}
}

// fakeHost answers one hello with reply.
func fakeHost(t *testing.T, conn net.Conn, reply SessionMessage) {
	t.Helper()
	go func() {
		dec := NewSessionDecoder(conn)
		msg, err := dec.Next()
		if err != nil {
			return
		}
		if _, ok := msg.(HelloMsg); !ok {
			return
		}
		_ = NewSessionEncoder(conn).Send(reply)
	}()
}

func TestSessionHandshake(t *testing.T) {
	a, b := net.Pipe()
	t.Cleanup(func() { a.Close(); b.Close() })
	want := HelloReplyMsg{Version: ProtocolVersion, ID: "7f3a0c21", Mode: SessionQuick, PID: 7}
	fakeHost(t, b, want)
	got, err := SessionHandshake(NewSessionEncoder(a), NewSessionDecoder(a))
	if err != nil {
		t.Fatalf("SessionHandshake: %v", err)
	}
	if got != want {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestSessionHandshakeVersionMismatch(t *testing.T) {
	a, b := net.Pipe()
	t.Cleanup(func() { a.Close(); b.Close() })
	old := HelloReplyMsg{Version: ProtocolVersion + 1, ID: "7f3a0c21", PID: 48211}
	fakeHost(t, b, old)
	got, err := SessionHandshake(NewSessionEncoder(a), NewSessionDecoder(a))
	if !IsVersionMismatch(err) {
		t.Fatalf("got %v, want a version mismatch", err)
	}
	var vm *VersionMismatchError
	if !errors.As(err, &vm) || vm.Got != ProtocolVersion+1 || vm.Want != ProtocolVersion {
		t.Errorf("VersionMismatchError = %+v", vm)
	}
	// The reply is still returned so the server can kill the host by pid.
	if got.PID != 48211 || got.ID != "7f3a0c21" {
		t.Errorf("reply not returned with mismatch: %#v", got)
	}
}

func TestSessionHandshakeErrors(t *testing.T) {
	cases := []struct {
		name  string
		reply SessionMessage
	}{
		{"error reply", ErrorMsg{Message: "busy"}},
		{"wrong message", OutputMsg("$ ")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := net.Pipe()
			t.Cleanup(func() { a.Close(); b.Close() })
			fakeHost(t, b, tc.reply)
			_, err := SessionHandshake(NewSessionEncoder(a), NewSessionDecoder(a))
			if err == nil || IsVersionMismatch(err) {
				t.Fatalf("got %v, want a non-mismatch error", err)
			}
		})
	}
	if IsVersionMismatch(ErrTruncatedFrame) || IsVersionMismatch(nil) {
		t.Error("IsVersionMismatch true for an unrelated error")
	}
}

func TestSessionModeText(t *testing.T) {
	for _, m := range []SessionMode{SessionQuick, SessionPersistent} {
		b, err := m.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText(%v): %v", m, err)
		}
		var got SessionMode
		if err := got.UnmarshalText(b); err != nil || got != m {
			t.Errorf("UnmarshalText(%q) = %v, %v", b, got, err)
		}
	}
	if _, err := SessionMode(7).MarshalText(); err == nil {
		t.Error("MarshalText accepted an invalid mode")
	}
	if err := NewSessionEncoder(io.Discard).Send(HelloReplyMsg{Mode: SessionMode(7)}); err == nil {
		t.Error("Send accepted a hello reply with an invalid mode")
	}
}

func TestSessionProtocolConcurrentWriters(t *testing.T) {
	const writers, perWriter, size = 8, 50, 8 << 10
	enc, dec, _, _ := pipeCodec(t)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			// Each writer's frames hold one repeated byte, so any
			// interleaving shows up as a mixed or misframed payload.
			chunk := bytes.Repeat([]byte{byte('a' + i)}, size)
			for range perWriter {
				if err := enc.Send(OutputMsg(chunk)); err != nil {
					t.Errorf("Send: %v", err)
					return
				}
			}
		})
	}
	counts := make(map[byte]int)
	for range writers * perWriter {
		msg, err := dec.Next()
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		out, ok := msg.(OutputMsg)
		if !ok || len(out) != size {
			t.Fatalf("got %T of %d bytes", msg, len(out))
		}
		if bytes.Count(out, out[:1]) != size {
			t.Fatal("frame payload mixes bytes from different writers")
		}
		counts[out[0]]++
	}
	wg.Wait()
	for i := range writers {
		if c := counts[byte('a'+i)]; c != perWriter {
			t.Errorf("writer %d: %d frames, want %d", i, c, perWriter)
		}
	}
}
