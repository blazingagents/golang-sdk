package blazingagents

import (
	"net/http"
	"strings"
	"time"
)

// RequestOption changes one call.
type RequestOption func(*callOptions)

type callOptions struct {
	timeout         *time.Duration
	header          http.Header
	clientRequestID string
	capture         *ResponseMeta
	err             error
}

// WithRequestTimeout overrides WithTimeout for this call; 0 disables the timeout.
func WithRequestTimeout(d time.Duration) RequestOption {
	return func(o *callOptions) { o.timeout = &d }
}

// WithRequestHeader adds a header to this call. An SDK-owned or invalid header fails the
// call with an *ArgumentError wrapping ErrInvalidHeader. It replaces a WithHeader value.
func WithRequestHeader(name, value string) RequestOption {
	return func(o *callOptions) {
		if err := checkHeader(name, value); err != nil {
			o.fail(&ArgumentError{Name: "header " + name, Reason: err.reason, Err: ErrInvalidHeader})
			return
		}
		if o.header == nil {
			o.header = http.Header{}
		}
		o.header.Add(name, value)
	}
}

// WithClientRequestID sends X-Client-Request-Id with this call, over the client's value.
// "" keeps the client's value. The ID must be printable ASCII with no leading or trailing space,
// so ResponseMeta.ClientRequestID is exactly what was sent.
func WithClientRequestID(id string) RequestOption {
	return func(o *callOptions) {
		if !validRequestID(id) {
			o.fail(&ArgumentError{Name: "clientRequestID", Reason: requestIDRule, Err: ErrInvalidHeader})
			return
		}
		o.clientRequestID = id
	}
}

// CaptureResponse fills into once the response headers arrive, on success and on *APIError.
func CaptureResponse(into *ResponseMeta) RequestOption {
	return func(o *callOptions) { o.capture = into }
}

func (o *callOptions) fail(err error) {
	if o.err == nil {
		o.err = err
	}
}

// ResponseMeta describes a response.
type ResponseMeta struct {
	OperationID     string
	Status          int
	Header          http.Header // response headers, submitted secrets redacted
	RequestID       string      // X-Request-Id from the response, "" if absent
	ClientRequestID string      // the X-Client-Request-Id that was sent, "" if none
}

// ResponseObservation is what WithObserver receives once per response.
type ResponseObservation struct {
	ResponseMeta
	Method   string
	Path     string        // escaped path, no query
	Duration time.Duration // send to response headers
}

// ownedHeaders are set only by the SDK.
var ownedHeaders = map[string]bool{
	"Authorization":       true,
	"X-Ba-User-Id":        true,
	"X-Request-Id":        true,
	"X-Client-Request-Id": true,
	"Content-Type":        true,
	"Content-Length":      true,
	"Idempotency-Key":     true,
	"Host":                true,
}

type headerError struct{ reason string }

func checkHeader(name, value string) *headerError {
	switch {
	case !validHeaderName(name):
		return &headerError{"not a valid header name"}
	case ownedHeaders[http.CanonicalHeaderKey(name)]:
		return &headerError{"set by the SDK"}
	case !validHeaderValue(value):
		return &headerError{"not a valid header value"}
	}
	return nil
}

// validHeaderName reports whether name is an RFC 9110 token.
func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 0x80 || c <= ' ' || c == 0x7f || strings.IndexByte(`"(),/:;<=>?@[\]{}`, c) >= 0 {
			return false
		}
	}
	return true
}

// validHeaderValue reports whether v has no control characters other than tab.
func validHeaderValue(v string) bool {
	for i := 0; i < len(v); i++ {
		if c := v[i]; c < ' ' && c != '\t' || c == 0x7f {
			return false
		}
	}
	return true
}
