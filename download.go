package blazingagents

import (
	"errors"
	"io"
)

// Download is a response body read as a stream. Close it when done.
type Download struct{ streamBody }

// Read reads the body. A failure is a *StreamError: StreamRead, or StreamIncomplete when
// the body ends short of its Content-Length.
func (d *Download) Read(p []byte) (int, error) {
	n, err := d.resp.Body.Read(p)
	switch {
	case err == nil || err == io.EOF:
		return n, err
	case errors.Is(err, io.ErrUnexpectedEOF):
		return n, &StreamError{Kind: StreamIncomplete, Meta: d.c.meta}
	}
	return n, &StreamError{Kind: StreamRead, Meta: d.c.meta, Err: d.c.streamCause(err)}
}

// Close ends the request. It is safe to call more than once.
func (d *Download) Close() error { return d.close() }

// Meta describes the response; it is available before the body is read.
func (d *Download) Meta() ResponseMeta { return d.c.meta }

// ContentType is the response's Content-Type.
func (d *Download) ContentType() string { return d.resp.Header.Get("Content-Type") }

// ContentLength is the response's length, or -1 when unknown.
func (d *Download) ContentLength() int64 { return d.resp.ContentLength }
