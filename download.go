package blazingagents

import (
	"errors"
	"io"
	"net/http"
	"sync"
)

// Download is a response body read as a stream. Close it when done.
type Download struct{ d *download }

type download struct {
	c      *call
	resp   *http.Response
	mu     sync.Mutex
	closed bool
}

// Read reads the body. A failure is a *StreamError: StreamRead, or StreamIncomplete when
// the body ends short of its Content-Length.
func (d *Download) Read(p []byte) (int, error) {
	n, err := d.d.resp.Body.Read(p)
	switch {
	case err == nil || err == io.EOF:
		return n, err
	case errors.Is(err, io.ErrUnexpectedEOF):
		return n, &StreamError{Kind: StreamIncomplete, Meta: d.d.c.meta}
	}
	return n, &StreamError{Kind: StreamRead, Meta: d.d.c.meta, Err: d.d.c.streamCause(err)}
}

// Close ends the request. It is safe to call more than once.
func (d *Download) Close() error {
	d.d.mu.Lock()
	defer d.d.mu.Unlock()
	if d.d.closed {
		return nil
	}
	d.d.closed = true
	d.d.c.cancel(ErrStreamClosed)
	err := d.d.resp.Body.Close()
	d.d.c.done()
	return err
}

// Meta describes the response; it is available before the body is read.
func (d *Download) Meta() ResponseMeta { return d.d.c.meta }

// ContentType is the response's Content-Type.
func (d *Download) ContentType() string { return d.d.resp.Header.Get("Content-Type") }

// ContentLength is the response's length, or -1 when unknown.
func (d *Download) ContentLength() int64 { return d.d.resp.ContentLength }

func newDownload(resp *http.Response, c *call) *Download {
	return &Download{&download{c: c, resp: resp}}
}
