package src

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// feedAll runs a fresh parser over s and returns it.
func feedAll(s string) *escParser {
	var p escParser
	for i := 0; i < len(s); i++ {
		p.feed(s[i])
	}
	return &p
}

// groundAfter reports, for each prefix length of s, whether the parser is in
// ground state after that prefix (index 0 = before any byte).
func groundAfter(s string) []bool {
	var p escParser
	out := []bool{p.ground()}
	for i := 0; i < len(s); i++ {
		p.feed(s[i])
		out = append(out, p.ground())
	}
	return out
}

func TestEscParser(t *testing.T) {
	cases := []struct {
		name string
		seq  string
	}{
		{"CSI color", "\x1b[38;5;196m"},
		{"CSI private mode", "\x1b[?25l"},
		{"OSC ended by BEL", "\x1b]0;title\x07"},
		{"OSC ended by ST", "\x1b]8;;https://example.com\x1b\\"},
		{"DCS", "\x1bP1$r0m\x1b\\"},
		{"APC", "\x1b_Gf=100;AAAA\x1b\\"},
		{"charset designation", "\x1b(B"},
		{"2-byte UTF-8", "é"},
		{"3-byte UTF-8", "€"},
		{"4-byte UTF-8", "😀"},
	}
	for _, tc := range cases {
		t.Run(tc.name+" is safe only at its ends", func(t *testing.T) {
			g := groundAfter(tc.seq)
			assert.True(t, g[0])
			assert.True(t, g[len(tc.seq)], "ground after the whole sequence")
			for i := 1; i < len(tc.seq); i++ {
				assert.False(t, g[i], "offset %d of %q must not be a cut point", i, tc.seq)
			}
		})
	}

	t.Run("plain text and controls stay in ground", func(t *testing.T) {
		for _, g := range groundAfter("ab\r\n\tc") {
			assert.True(t, g)
		}
	})

	t.Run("BEL inside CSI does not end it", func(t *testing.T) {
		assert.False(t, feedAll("\x1b[1\x07").ground())
	})

	t.Run("ESC inside CSI starts a new sequence", func(t *testing.T) {
		assert.True(t, feedAll("\x1b[12\x1b[0m").ground())
	})

	t.Run("ESC followed by other than backslash inside a string starts a new sequence", func(t *testing.T) {
		p := feedAll("\x1b]0;x\x1b[")
		assert.Equal(t, escCSI, p.state)
	})

	t.Run("an invalid UTF-8 continuation resyncs", func(t *testing.T) {
		assert.True(t, feedAll("\xe2a").ground())
		assert.Equal(t, escEscape, feedAll("\xe2\x1b").state)
	})

	t.Run("tracks the alternate screen", func(t *testing.T) {
		for _, mode := range []string{"1049", "1047", "47"} {
			p := feedAll("\x1b[?" + mode + "h")
			assert.True(t, p.altScreen, mode+" on")
			for _, c := range []byte("\x1b[?" + mode + "l") {
				p.feed(c)
			}
			assert.False(t, p.altScreen, mode+" off")
		}
	})

	t.Run("alternate screen among several modes", func(t *testing.T) {
		assert.True(t, feedAll("\x1b[?1;1049h").altScreen)
		assert.False(t, feedAll("\x1b[?1049h\x1b[?25;1049l").altScreen)
	})

	t.Run("non-private or unrelated modes do not touch the alternate screen", func(t *testing.T) {
		assert.False(t, feedAll("\x1b[1049h").altScreen)
		assert.False(t, feedAll("\x1b[?10490h").altScreen)
		assert.True(t, feedAll("\x1b[?1049h\x1b[?25l").altScreen)
	})
}

func TestOutputBuffer(t *testing.T) {
	t.Run("keeps everything under the limit", func(t *testing.T) {
		b := newOutputBuffer(64)
		b.Write([]byte("hello "))
		b.Write([]byte("world"))
		assert.Equal(t, "hello world", string(b.Bytes()))
	})

	t.Run("Bytes is a copy", func(t *testing.T) {
		b := newOutputBuffer(64)
		b.Write([]byte("abc"))
		got := b.Bytes()
		got[0] = 'x'
		assert.Equal(t, "abc", string(b.Bytes()))
	})

	t.Run("never exceeds its byte cap and keeps the newest output", func(t *testing.T) {
		b := newOutputBuffer(1000)
		var all bytes.Buffer
		for i := 0; i < 500; i++ {
			line := []byte(strings.Repeat("x", i%37) + "\r\n")
			all.Write(line)
			b.Write(line)
			require.LessOrEqual(t, b.Len(), 1000)
		}
		got := b.Bytes()
		assert.Greater(t, len(got), 0)
		assert.True(t, bytes.HasSuffix(all.Bytes(), got), "replay must be the newest output, unchanged")
	})

	t.Run("cuts after a newline when one is near", func(t *testing.T) {
		b := newOutputBuffer(40)
		b.Write([]byte(strings.Repeat("a", 20) + "\n" + strings.Repeat("b", 15) + "\n"))
		b.Write([]byte(strings.Repeat("c", 10)))
		// 47 bytes > 40: trimming to 30 drops at least 17; the first
		// newline after that ends at offset 21.
		assert.Equal(t, strings.Repeat("b", 15)+"\n"+strings.Repeat("c", 10), string(b.Bytes()))
	})

	t.Run("without a newline cuts at the first safe point", func(t *testing.T) {
		b := newOutputBuffer(40)
		b.Write([]byte(strings.Repeat("a", 50)))
		assert.Equal(t, strings.Repeat("a", 30), string(b.Bytes()))
	})

	t.Run("a CSI split at the cut point is dropped whole", func(t *testing.T) {
		b := newOutputBuffer(40)
		data := strings.Repeat("a", 26) + "\x1b[38;5;196m" + strings.Repeat("z", 23)
		b.Write([]byte(data))
		// 60 bytes, keep 30: minimum cut at 30, inside the CSI (26–36).
		assert.Equal(t, strings.Repeat("z", 23), string(b.Bytes()))
	})

	cutCases := []struct {
		name, seq string
	}{
		{"OSC ended by BEL", "\x1b]0;a window title\x07"},
		{"OSC ended by ST", "\x1b]0;a window title\x1b\\"},
		{"DCS", "\x1bPq#0;2;0;0;0#1;2;100\x1b\\"},
		{"2-byte UTF-8", "éééééééééé"},
		{"3-byte UTF-8", "€€€€€€€€€€"},
		{"4-byte UTF-8", "😀😀😀😀😀😀😀😀"},
	}
	for _, tc := range cutCases {
		t.Run("cut point never lands inside "+tc.name, func(t *testing.T) {
			// The buffer keeps 48 of its 64-byte limit, so the minimum cut is
			// at len(data)-48. Shorten the tail one byte at a time to slide
			// the minimum cut from the sequence's end back to its start.
			const prefix = 40
			for j := 0; j <= len(tc.seq); j++ {
				b := newOutputBuffer(64)
				data := strings.Repeat("a", prefix) + tc.seq + strings.Repeat("z", 48-j)
				b.Write([]byte(data))
				got := string(b.Bytes())
				require.True(t, strings.HasSuffix(data, got))
				require.True(t, strings.HasSuffix(got, strings.Repeat("z", 48-j)), "j %d: live tail lost", j)
				start := len(data) - len(got)
				assert.True(t, groundAfter(data)[start], "j %d: replay starts at offset %d, inside the sequence", j, start)
				assert.GreaterOrEqual(t, start, prefix+len(tc.seq)-j, "j %d: cut before the minimum", j)
			}
		})
	}

	t.Run("a string longer than the buffer drops output until it ends", func(t *testing.T) {
		b := newOutputBuffer(32)
		b.Write([]byte("\x1b]0;" + strings.Repeat("t", 100)))
		assert.Equal(t, 0, b.Len())
		b.Write([]byte(strings.Repeat("t", 10) + "\x07after"))
		assert.Equal(t, "after", string(b.Bytes()))
	})

	t.Run("tracks the alternate screen across evictions", func(t *testing.T) {
		b := newOutputBuffer(32)
		b.Write([]byte("\x1b[?1049h"))
		b.Write([]byte(strings.Repeat("x", 100)))
		assert.True(t, b.AltScreen())
		b.Write([]byte("\x1b[?1049l"))
		assert.False(t, b.AltScreen())
	})
}
