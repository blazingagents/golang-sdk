package blazingagents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

type transport struct {
	http            *http.Client
	baseURL         string
	authorization   Secret // "Bearer <key>"; a Secret so that no fmt path prints it
	userAgent       string
	header          http.Header // canonical, validated
	timeout         time.Duration
	idleTimeout     time.Duration
	observer        func(ResponseObservation)
	logger          *slog.Logger
	clientRequestID string
	userID          string // "" on the tenant client
}

type call struct {
	t       *transport
	op      *op.Operation
	req     *http.Request
	cancel  context.CancelCauseFunc
	header  *time.Timer // fires the SDK timeout with a *TimeoutError cause
	idle    *time.Timer // a stream's idle timer, armed when headers arrive
	secrets []string    // from wire(), for redaction
	capture *ResponseMeta
	meta    ResponseMeta
	opts    callOptions
}

type body struct {
	data []byte
	meta ResponseMeta
}

func (t *transport) send(ctx context.Context, r op.Request, params any, opts []RequestOption) (*body, error) {
	c, err := t.prepare(ctx, r, params, opts)
	if err != nil {
		return nil, err
	}
	defer c.done()
	resp, err := c.send()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, c.connectionError(err) // never a DecodeError
	}
	if r.Op.CredentialBearing {
		if err := rejectCredentialEcho(data, c.meta); err != nil {
			return nil, err
		}
	}
	return &body{data: data, meta: c.meta}, nil
}

// decode is the body of most methods. A failed call returns a nil model.
func decode[T any](b *body, err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	var out T
	if err := json.Unmarshal(b.data, &out); err != nil {
		return nil, b.decodeError(err)
	}
	return &out, nil
}

func (b *body) decodeError(err error) *DecodeError {
	return &DecodeError{OperationID: b.meta.OperationID, RequestID: b.meta.RequestID, Err: err}
}

func empty(_ *body, err error) error { return err }

// open is send for streaming operations (chat, generation, download): it returns the
// response with its body unread, and the caller calls c.done when it is finished.
func (s userOps) open(ctx context.Context, r op.UserScoped, params any, opts []RequestOption) (*http.Response, *call, error) {
	c, err := s.t.prepare(ctx, r.Request, params, opts)
	if err != nil {
		return nil, nil, err
	}
	resp, err := c.send()
	if err != nil {
		c.done()
		return nil, nil, err
	}
	return resp, c, nil
}

// streamBody is a response from open that its owner reads and then closes.
type streamBody struct {
	c      *call
	resp   *http.Response
	mu     sync.Mutex
	closed bool
}

// close ends the request. It is safe from any goroutine and more than once.
func (b *streamBody) close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	b.c.cancel(ErrStreamClosed)
	err := b.resp.Body.Close()
	b.c.done()
	return err
}

func (t *transport) prepare(ctx context.Context, r op.Request, params any, opts []RequestOption) (*call, error) {
	var o callOptions
	for _, opt := range opts {
		opt(&o)
	}
	if o.err != nil {
		return nil, o.err
	}
	path, err := expandPath(r)
	if err != nil {
		return nil, err
	}
	query, header, err := encodeQueryAndHeaders(r.Op, params)
	if err != nil {
		return nil, err
	}
	u := t.baseURL + path
	if q := query.Encode(); q != "" {
		u += "?" + q
	}
	// An UploadFile is opened here, so a missing file is an *ArgumentError before anything is sent.
	reqBody, contentType, secrets, err := encodeBody(r.Op, params)
	if err != nil {
		return nil, err
	}
	// NewRequest sets GetBody for a JSON body, so net/http can replay it on a stale
	// connection or after an HTTP/2 GOAWAY.
	req, err := http.NewRequest(r.Op.Method, u, reqBody)
	if err != nil {
		if c, ok := reqBody.(io.Closer); ok {
			c.Close()
		}
		return nil, &ArgumentError{Name: "request", Reason: err.Error()}
	}
	timeout := t.timeout
	if o.timeout != nil {
		timeout = *o.timeout
	}
	c := &call{t: t, op: r.Op, secrets: secrets, capture: o.capture, opts: o}
	ctx, c.cancel = context.WithCancelCause(ctx)
	if timeout > 0 {
		cancel, cause := c.cancel, &TimeoutError{OperationID: r.Op.ID, After: timeout}
		c.header = time.AfterFunc(timeout, func() { cancel(cause) })
	}
	req = req.WithContext(ctx)
	h := req.Header
	h.Set("User-Agent", t.userAgent)
	for k, v := range t.header {
		h[k] = append([]string(nil), v...)
	}
	for k, v := range o.header { // a per-call header replaces the client's value
		h[k] = append([]string(nil), v...)
	}
	for k, v := range header {
		h[k] = v
	}
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	h.Set("Authorization", t.authorization.Reveal())
	crid := t.clientRequestID
	if o.clientRequestID != "" {
		crid = o.clientRequestID
	}
	if crid != "" {
		h.Set("X-Client-Request-Id", crid)
	}
	if t.userID != "" {
		h.Set("X-BA-User-Id", t.userID)
	}
	c.req = req
	return c, nil
}

func (c *call) done() {
	if c.header != nil {
		c.header.Stop()
	}
	if c.idle != nil {
		c.idle.Stop()
	}
	c.cancel(nil)
}

// send performs the request, observes it, and maps non-2xx to *APIError. For an ordinary call
// the timer runs through the body read; for a streaming call it stops when headers arrive.
func (c *call) send() (*http.Response, error) {
	start := time.Now()
	resp, err := c.t.http.Do(c.req)
	if err != nil {
		return nil, c.connectionError(err)
	}
	c.meta = ResponseMeta{
		OperationID:     c.op.ID,
		Status:          resp.StatusCode,
		Header:          redactHeader(resp.Header, c.secrets),
		RequestID:       resp.Header.Get("X-Request-Id"),
		ClientRequestID: c.req.Header.Get("X-Client-Request-Id"),
	}
	if c.capture != nil {
		*c.capture = c.meta
	}
	c.observe(time.Since(start))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// An error body is an ordinary document, so the timer still bounds it, streaming or not.
		defer resp.Body.Close()
		return nil, c.apiError(resp)
	}
	if c.op.Response.Streaming() {
		if c.header != nil {
			c.header.Stop()
		}
		if d := c.t.idleTimeout; d > 0 {
			cancel := c.cancel
			c.idle = time.AfterFunc(d, func() { cancel(ErrStreamIdle) })
			resp.Body = &idleReader{ReadCloser: resp.Body, timer: c.idle, d: d}
		}
	}
	return resp, nil
}

// connectionError: the SDK's own timeout is the context cause, so it surfaces as
// *TimeoutError; everything else, including the caller's cancel or deadline, is *ConnectionError.
func (c *call) connectionError(err error) error {
	var te *TimeoutError
	if errors.As(context.Cause(c.req.Context()), &te) {
		return te
	}
	return &ConnectionError{OperationID: c.op.ID, Err: err}
}

// streamCause is the error a failed stream read reports: why its context ended, if it did.
func (c *call) streamCause(err error) error {
	if cause := context.Cause(c.req.Context()); cause != nil {
		return cause
	}
	return err
}

func (c *call) observe(d time.Duration) {
	path := c.req.URL.EscapedPath()
	c.t.logger.LogAttrs(c.req.Context(), slog.LevelDebug, "blazingagents response",
		slog.String("operation", c.op.ID), slog.String("method", c.op.Method), slog.String("path", path),
		slog.Int("status", c.meta.Status), slog.Duration("duration", d),
		slog.String("requestId", c.meta.RequestID), slog.String("clientRequestId", c.meta.ClientRequestID))
	if c.t.observer == nil {
		return
	}
	defer func() { _ = recover() }()
	obs := ResponseObservation{ResponseMeta: c.meta, Method: c.op.Method, Path: path, Duration: d}
	obs.Header = c.meta.Header.Clone() // the caller's meta and error keep their own map
	c.t.observer(obs)
}

const maxErrorBody = 64 << 10

// apiError reads a non-2xx response. A body that fails to read makes the call fail as a
// connection or timeout error: a partial body is not the server's answer.
func (c *call) apiError(resp *http.Response) error {
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody+1))
	if err != nil {
		return c.connectionError(err)
	}
	e := &APIError{Status: resp.StatusCode, Meta: c.meta}
	e.RetryAfter, _ = retryAfter(resp.Header.Get("Retry-After"))
	if len(data) > maxErrorBody {
		data, e.BodyTruncated = data[:maxErrorBody], true
	}
	var env struct {
		Error *struct {
			Code    ErrorCode      `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
			Param   string         `json:"param"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &env) == nil && env.Error != nil {
		e.Code, e.Message, e.Param = env.Error.Code, env.Error.Message, env.Error.Param
		e.Details, _ = redactValue(env.Error.Details, c.secrets).(map[string]any)
	} else {
		e.Message = string(data)
	}
	e.Message = redactString(e.Message, c.secrets)
	e.Body = redactString(string(data), c.secrets)
	return e
}

// retryAfter parses delta-seconds or an HTTP-date, reporting whether v is either.
func retryAfter(v string) (time.Duration, bool) {
	if n, err := strconv.Atoi(v); err == nil {
		return max(time.Duration(n)*time.Second, 0), true
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(time.Until(t), 0), true
	}
	return 0, false
}

// redactString hides each secret: anywhere when it is 8 bytes or longer, else only as the whole value.
func redactString(s string, secrets []string) string {
	for _, sec := range secrets {
		switch {
		case sec == "":
		case len(sec) >= 8:
			s = strings.ReplaceAll(s, sec, redacted)
		case s == sec:
			s = redacted
		}
	}
	return s
}

func redactValue(v any, secrets []string) any {
	switch v := v.(type) {
	case string:
		return redactString(v, secrets)
	case map[string]any:
		for k, x := range v {
			v[k] = redactValue(x, secrets)
		}
	case []any:
		for i, x := range v {
			v[i] = redactValue(x, secrets)
		}
	}
	return v
}

func redactHeader(h http.Header, secrets []string) http.Header {
	out := h.Clone()
	for _, vs := range out {
		for i, v := range vs {
			vs[i] = redactString(v, secrets)
		}
	}
	return out
}

// credentialKeys are fields no credential operation's response may contain.
var credentialKeys = map[string]bool{
	"apiKey": true, "api_key": true, "botToken": true, "bot_token": true,
	"signingSecret": true, "signing_secret": true, "webhookSecret": true, "webhook_secret": true,
	"bearerToken": true, "bearer_token": true, "clientSecret": true, "client_secret": true,
	"credential": true,
}

func rejectCredentialEcho(data []byte, meta ResponseMeta) error {
	var v any
	if json.Unmarshal(data, &v) != nil {
		return nil // not JSON: decoding reports it
	}
	if key, ok := findCredentialKey(v); ok {
		return &CredentialEchoError{OperationID: meta.OperationID, RequestID: meta.RequestID, Field: key}
	}
	return nil
}

func findCredentialKey(v any) (string, bool) {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			if credentialKeys[k] {
				return k, true
			}
			if key, ok := findCredentialKey(x); ok {
				return key, true
			}
		}
	case []any:
		for _, x := range v {
			if key, ok := findCredentialKey(x); ok {
				return key, true
			}
		}
	}
	return "", false
}

// expandPath fills the path template, rejecting "", "." and ".." and escaping every byte
// outside RFC 3986 unreserved characters.
func expandPath(r op.Request) (string, error) {
	path := r.Op.Path
	for i, name := range r.Op.Params {
		seg := r.Segments[i]
		if seg == "" || seg == "." || seg == ".." {
			return "", &ArgumentError{Name: name, Reason: strconv.Quote(seg) + " is not a valid path segment"}
		}
		path = strings.Replace(path, "{"+name+"}", escapeSegment(seg), 1)
	}
	return path, nil
}

func escapeSegment(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}

// idleReader resets a stream's idle timer after every read.
type idleReader struct {
	io.ReadCloser
	timer *time.Timer
	d     time.Duration
}

func (r *idleReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.timer.Reset(r.d)
	return n, err
}

// encodeBody encodes params for the operation's body kind and returns the secrets wire() revealed.
func encodeBody(o *op.Operation, params any) (io.Reader, string, []string, error) {
	switch o.Body {
	case op.NoBody:
		return nil, "", nil, nil
	case op.Multipart:
		return encodeMultipart(params.(multipartParams))
	case op.BinaryBody:
		r := params.(binaryParams).r
		if r == nil {
			return nil, "", nil, &ArgumentError{Name: "content", Reason: "nil reader"}
		}
		return r, "application/octet-stream", nil, nil
	}
	v, secrets := params, []string(nil)
	if w, ok := params.(wirer); ok {
		v, secrets = w.wire()
	}
	data, err := json.Marshal(v)
	if err != nil {
		if ae := (*ArgumentError)(nil); errors.As(err, &ae) {
			return nil, "", nil, ae
		}
		return nil, "", nil, &ArgumentError{Name: "params", Reason: err.Error(), Err: err}
	}
	if o.CredentialBearing && bytes.Contains(data, []byte(`"`+redacted+`"`)) {
		return nil, "", nil, &ArgumentError{Name: "params", Reason: "a Secret would be sent as " + redacted}
	}
	return bytes.NewReader(data), "application/json", secrets, nil
}

// unwrap decodes a single-object envelope {"<key>": T}. A null or missing T is a *DecodeError.
func unwrap[T any](key string) func(*body, error) (*T, error) {
	return func(b *body, err error) (*T, error) {
		v, err := unwrapOptional[T](key)(b, err)
		if err == nil && v == nil {
			return nil, b.decodeError(errors.New(strconv.Quote(key) + " is null"))
		}
		return v, err
	}
}

// unwrapOptional decodes {"<key>": T | null}; null is nil, nil.
func unwrapOptional[T any](key string) func(*body, error) (*T, error) {
	return func(b *body, err error) (*T, error) {
		if err != nil {
			return nil, err
		}
		var env map[string]json.RawMessage
		if err := json.Unmarshal(b.data, &env); err != nil {
			return nil, b.decodeError(err)
		}
		if absent(env[key]) {
			return nil, nil
		}
		var v T
		if err := json.Unmarshal(env[key], &v); err != nil {
			return nil, b.decodeError(err)
		}
		return &v, nil
	}
}

// list decodes an unpaged list {"<key>": [T]}.
func list[T any](key string) func(*body, error) ([]T, error) {
	return func(b *body, err error) ([]T, error) {
		v, err := unwrapOptional[[]T](key)(b, err)
		if v == nil {
			return nil, err
		}
		return *v, nil
	}
}

// invalidBody fails to marshal with err, so a wire() that finds its input invalid makes the
// call fail with an *ArgumentError before anything is sent.
type invalidBody struct{ err error }

func (b invalidBody) MarshalJSON() ([]byte, error) { return nil, b.err }
