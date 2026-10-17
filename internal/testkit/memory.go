package testkit

import (
	"context"
	"io"
	"net/http"
	"sync"
)

// Transport is an in-process RoundTripper that serves the mock's exchanges with io.Pipe
// bodies, so a client inside a testing/synctest bubble never touches the network. Call it
// before Client, which then uses it: m.Client(ba.WithTransport(m.Transport())).
func (m *Mock) Transport() http.RoundTripper {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.memory = true
	return memoryTransport{m}
}

// InMemory reports whether Transport was called.
func (m *Mock) InMemory() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.memory
}

type memoryTransport struct{ m *Mock }

func (t memoryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	pr, pw := io.Pipe()
	w := &memoryWriter{header: http.Header{}, pw: pw, ready: make(chan struct{})}
	in := req.Clone(ctx)
	if in.Body == nil {
		in.Body = http.NoBody
	}
	go func() {
		t.m.serve(w, in)
		w.WriteHeader(http.StatusOK)
		in.Body.Close()
		if ctx.Err() != nil {
			pw.CloseWithError(context.Cause(ctx)) // a canceled request never reads as a clean end
			return
		}
		pw.Close()
	}()
	select {
	case <-w.ready:
	case <-ctx.Done():
		pw.CloseWithError(ctx.Err())
		return nil, ctx.Err()
	}
	stop := context.AfterFunc(ctx, func() { pr.CloseWithError(context.Cause(ctx)) })
	return &http.Response{
		Status: http.StatusText(w.status), StatusCode: w.status, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: w.sent, Body: &memoryBody{pr, stop}, ContentLength: -1, Request: req,
	}, nil
}

type memoryWriter struct {
	header http.Header
	sent   http.Header
	pw     *io.PipeWriter
	status int
	once   sync.Once
	ready  chan struct{}
}

func (w *memoryWriter) Header() http.Header { return w.header }

func (w *memoryWriter) WriteHeader(code int) {
	w.once.Do(func() {
		w.status, w.sent = code, w.header.Clone()
		close(w.ready)
	})
}

func (w *memoryWriter) Write(b []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.pw.Write(b)
}

func (w *memoryWriter) Flush() {}

type memoryBody struct {
	r    *io.PipeReader
	stop func() bool
}

func (b *memoryBody) Read(p []byte) (int, error) { return b.r.Read(p) }

func (b *memoryBody) Close() error {
	b.stop()
	return b.r.Close()
}
