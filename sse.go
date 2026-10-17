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
		if end := frameEnd(f.buf); end > 0 {
			ev := bytes.Clone(f.buf[:end])
			f.buf = f.buf[end:]
			return ev, nil
		}
		n, err := f.r.Read(f.chunk[:])
		f.buf = append(f.buf, f.chunk[:n]...)
		switch {
		case n > 0:
		case err == io.EOF:
			if len(bytes.TrimLeft(f.buf, " \t\r\n\f\v")) == 0 {
				return nil, io.EOF
			}
			return nil, io.ErrUnexpectedEOF
		case err != nil:
			return nil, err
		}
	}
}

// frameEnd is the index just past the first "\n\n" or "\r\n\r\n", whichever starts first, or 0.
func frameEnd(b []byte) int {
	lf, crlf := bytes.Index(b, []byte("\n\n")), bytes.Index(b, []byte("\r\n\r\n"))
	switch {
	case crlf >= 0 && (lf < 0 || crlf < lf):
		return crlf + 4
	case lf >= 0:
		return lf + 2
	}
	return 0
}
