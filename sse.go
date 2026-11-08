package blazingagents

import (
	"bytes"
	"io"
)

// frames splits a text/event-stream body into complete events, each with its delimiter.
type frames struct {
	r     io.Reader
	buf   []byte
	chunk [4 << 10]byte
}

// next returns the next event. At the end of the body, a remainder of ASCII whitespace ends
// cleanly with io.EOF; anything else is io.ErrUnexpectedEOF. A read error is returned as is.
func (f *frames) next() ([]byte, error) {
	for {
		if ev, ok := f.split(false); ok {
			return ev, nil
		}
		n, err := f.r.Read(f.chunk[:])
		f.buf = append(f.buf, f.chunk[:n]...)
		switch {
		case n > 0:
		case err == io.EOF:
			if ev, ok := f.split(true); ok {
				return ev, nil
			}
			if len(bytes.TrimLeft(f.buf, " \t\r\n\f\v")) == 0 {
				return nil, io.EOF
			}
			return nil, io.ErrUnexpectedEOF
		case err != nil:
			return nil, err
		}
	}
}

func (f *frames) split(atEOF bool) ([]byte, bool) {
	end := frameEnd(f.buf, atEOF)
	if end == 0 {
		return nil, false
	}
	ev := bytes.Clone(f.buf[:end])
	f.buf = f.buf[end:]
	return ev, true
}

// frameEnd is the index just past the first blank line, or 0. A line ends at LF, CR,
// or CRLF, and a blank line is two of those in a row. A trailing CR waits for the next
// byte unless atEOF, because that byte decides CR from CRLF.
func frameEnd(b []byte, atEOF bool) int {
	for i := 0; i < len(b); {
		n, ok := lineBreak(b[i:], atEOF)
		if !ok {
			return 0
		}
		if n == 0 {
			i++
			continue
		}
		m, ok := lineBreak(b[i+n:], atEOF)
		if !ok {
			return 0
		}
		if m > 0 {
			return i + n + m
		}
		i += n
	}
	return 0
}

// lineBreak is the length of a line ending at the start of b. ok is false when more
// bytes are required to know the ending.
func lineBreak(b []byte, atEOF bool) (n int, ok bool) {
	if len(b) == 0 {
		return 0, atEOF
	}
	switch b[0] {
	case '\n':
		return 1, true
	case '\r':
		if len(b) == 1 {
			if atEOF {
				return 1, true
			}
			return 0, false
		}
		if b[1] == '\n' {
			return 2, true
		}
		return 1, true
	default:
		return 0, true
	}
}
