package src

import "bytes"

// escState is a state of escParser.
type escState uint8

const (
	escGround    escState = iota // between sequences and characters
	escEscape                    // after ESC
	escCSI                       // inside ESC [ … until a final byte 0x40–0x7E
	escString                    // inside an OSC/DCS/APC/PM/SOS string
	escStringEsc                 // ESC seen inside a string: `ESC \` ends it
	escUTF8                      // inside a multi-byte UTF-8 character
)

// escParser is a minimal byte-level parser of terminal output. It knows only
// enough to tell whether a byte offset is a safe place to start replaying
// output (escGround: not inside an escape sequence, control string, or
// UTF-8 character) and whether the alternate screen is on. It never
// interprets anything else.
type escParser struct {
	state        escState
	intermediate bool   // an ESC sequence has seen an intermediate byte (ESC ( B)
	utf8Left     int    // continuation bytes still expected
	params       []byte // CSI parameter bytes, capped; for alt-screen detection
	altScreen    bool
}

// maxCSIParams caps the CSI parameter bytes kept for alt-screen detection.
const maxCSIParams = 32

func (p *escParser) ground() bool { return p.state == escGround }

// feed advances the parser by one byte.
func (p *escParser) feed(b byte) {
	switch p.state {
	case escGround:
		switch {
		case b == 0x1b:
			p.state, p.intermediate = escEscape, false
		case b >= 0xc0 && b <= 0xdf:
			p.state, p.utf8Left = escUTF8, 1
		case b >= 0xe0 && b <= 0xef:
			p.state, p.utf8Left = escUTF8, 2
		case b >= 0xf0 && b <= 0xf7:
			p.state, p.utf8Left = escUTF8, 3
		}
	case escUTF8:
		if b&0xc0 != 0x80 { // invalid sequence: resync on this byte
			p.state = escGround
			p.feed(b)
			return
		}
		if p.utf8Left--; p.utf8Left == 0 {
			p.state = escGround
		}
	case escEscape:
		switch {
		case b == 0x1b:
			p.intermediate = false
		case b >= 0x20 && b <= 0x2f:
			p.intermediate = true
		case p.intermediate:
			p.state = escGround // final byte of e.g. ESC ( B
		case b == '[':
			p.state, p.params = escCSI, p.params[:0]
		case b == ']' || b == 'P' || b == '_' || b == '^' || b == 'X':
			p.state = escString
		default:
			p.state = escGround
		}
	case escCSI:
		switch {
		case b >= 0x40 && b <= 0x7e:
			p.state = escGround
			p.csiFinal(b)
		case b == 0x1b:
			p.state, p.intermediate = escEscape, false
		case b == 0x18 || b == 0x1a: // CAN, SUB abort the sequence
			p.state = escGround
		case len(p.params) < maxCSIParams:
			p.params = append(p.params, b)
		}
	case escString:
		switch b {
		case 0x07, 0x18, 0x1a: // BEL ends an OSC; CAN, SUB abort
			p.state = escGround
		case 0x1b:
			p.state = escStringEsc
		}
	case escStringEsc:
		if b == '\\' {
			p.state = escGround
			return
		}
		// ESC followed by anything else aborts the string and starts a new
		// escape sequence.
		p.state, p.intermediate = escEscape, false
		p.feed(b)
	}
}

// csiFinal tracks DEC private modes 1049, 1047 and 47 (alternate screen).
func (p *escParser) csiFinal(final byte) {
	if (final != 'h' && final != 'l') || len(p.params) == 0 || p.params[0] != '?' {
		return
	}
	for _, mode := range bytes.Split(p.params[1:], []byte{';'}) {
		switch string(mode) {
		case "1049", "1047", "47":
			p.altScreen = final == 'h'
		}
	}
}

// newlineSearchWindow is how far past the minimum cut point eviction looks
// for a newline to start at, before settling for any safe point.
const newlineSearchWindow = 8 << 10

// outputBuffer keeps the most recent pty output, capped at a number of
// bytes, for replay when a tab re-attaches. Its start is always a point
// where escParser was in ground state, so a replay never begins inside an
// escape sequence or a UTF-8 character. It is not safe for concurrent use.
type outputBuffer struct {
	limit  int
	data   []byte
	parser escParser // state at the end of data
	// synced is false after eviction found no safe point at all (one
	// control string longer than the buffer); bytes are then dropped until
	// the parser returns to ground.
	synced bool
}

func newOutputBuffer(limit int) *outputBuffer {
	if limit <= 0 {
		limit = DEFAULT_REPLAY_BUFFER_SIZE
	}
	return &outputBuffer{limit: limit, synced: true}
}

// Write appends p, trimming the front at a safe point when the buffer would
// grow past its limit. It always reports len(p) written.
func (b *outputBuffer) Write(p []byte) (int, error) {
	n := len(p)
	for !b.synced && len(p) > 0 {
		wasGround := b.parser.ground()
		if wasGround {
			b.synced = true
			break
		}
		b.parser.feed(p[0])
		p = p[1:]
	}
	for _, c := range p {
		b.parser.feed(c)
	}
	b.data = append(b.data, p...)
	if len(b.data) > b.limit {
		// Trim to three quarters of the limit, so the copy that compacts
		// the buffer happens once per quarter-limit of output rather than
		// on every read.
		b.evict(len(b.data) - b.limit*3/4)
	}
	return n, nil
}

// evict drops at least min bytes from the front, starting the buffer at the
// first safe point at or after min, preferring one just after a newline.
func (b *outputBuffer) evict(min int) {
	var q escParser // the buffer's start is always in ground state
	cut := -1
	for i := 0; i <= len(b.data); i++ {
		if i >= min && q.ground() {
			if i > 0 && b.data[i-1] == '\n' {
				cut = i
				break
			}
			if cut < 0 {
				cut = i
			}
		}
		if cut >= 0 && i >= min+newlineSearchWindow {
			break
		}
		if i < len(b.data) {
			q.feed(b.data[i])
		}
	}
	if cut < 0 {
		b.data = b.data[:0]
		b.synced = false
		return
	}
	b.data = b.data[:copy(b.data, b.data[cut:])]
}

// Bytes returns a copy of the buffered output.
func (b *outputBuffer) Bytes() []byte { return bytes.Clone(b.data) }

// Len returns the number of bytes buffered.
func (b *outputBuffer) Len() int { return len(b.data) }

// AltScreen reports whether the output so far left the alternate screen on.
func (b *outputBuffer) AltScreen() bool { return b.parser.altScreen }
