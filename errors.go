package blazingagents

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrMissingAPIKey      = errors.New("blazingagents: missing API key: pass WithAPIKey or set " + APIKeyEnv)
	ErrInvalidAPIKey      = errors.New("blazingagents: invalid API key")
	ErrInvalidBaseURL     = errors.New("blazingagents: invalid base URL")
	ErrInvalidUserID      = errors.New("blazingagents: invalid user ID")
	ErrInvalidHeader      = errors.New("blazingagents: invalid header")
	ErrConflictingOptions = errors.New("blazingagents: conflicting options")
	ErrStreamIdle         = errors.New("blazingagents: stream idle timeout")
	ErrStreamClosed       = errors.New("blazingagents: stream closed")
)

// APIError is a non-2xx response. Branch on Code: apiErr.Code == ba.ErrorCodeNotFound.
type APIError struct {
	Status        int
	Code          ErrorCode // "" when the body was not the error envelope
	Message       string    // redacted
	Details       map[string]any
	Param         string
	RetryAfter    time.Duration // 0 when absent
	Meta          ResponseMeta
	Body          string // redacted, at most 64 KiB
	BodyTruncated bool
}

// Error reads like "blazingagents: getAgent: 404 not_found: Agent not found (request req_123)".
func (e *APIError) Error() string {
	s := fmt.Sprintf("blazingagents: %s: %d", e.Meta.OperationID, e.Status)
	if e.Code != "" {
		s += " " + string(e.Code)
	}
	if e.Message != "" {
		s += ": " + e.Message
	}
	if e.Meta.RequestID != "" {
		s += " (request " + e.Meta.RequestID + ")"
	}
	return s
}

// TimeoutError is the SDK's own timeout. It matches context.DeadlineExceeded.
type TimeoutError struct {
	OperationID string
	After       time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("blazingagents: %s: timed out after %s", e.OperationID, e.After)
}

func (e *TimeoutError) Is(target error) bool { return target == context.DeadlineExceeded }

// ConnectionError is a failure with no complete response, including the caller's own
// cancellation or deadline, which Err wraps.
type ConnectionError struct {
	OperationID string
	Err         error
}

func (e *ConnectionError) Error() string {
	return "blazingagents: " + e.OperationID + ": " + e.Err.Error()
}
func (e *ConnectionError) Unwrap() error { return e.Err }

// StreamErrorKind says why a stream failed.
type StreamErrorKind string

const (
	StreamRead                  StreamErrorKind = "read"                    // Err: read error, ErrStreamIdle, ErrStreamClosed, ctx error
	StreamIncomplete            StreamErrorKind = "incomplete"              // body ended inside an event or a UTF-8 sequence
	StreamMissingSessionID      StreamErrorKind = "missing_session_id"      // new session without Location
	StreamMalformedSessionID    StreamErrorKind = "malformed_session_id"    // last Location segment is not a session ID
	StreamMissingTurnID         StreamErrorKind = "missing_turn_id"         // no X-BA-Turn-Id
	StreamMalformedTurnID       StreamErrorKind = "malformed_turn_id"       // X-BA-Turn-Id is not a turn ID
	StreamMalformedFunctionCall StreamErrorKind = "malformed_function_call" // a function-call event that does not parse
	StreamFunctionCall          StreamErrorKind = "function_call"           // permanent claim or result failure; Err is it
)

// StreamError is a failure of a stream after its response headers arrived.
type StreamError struct {
	Kind StreamErrorKind
	Meta ResponseMeta
	Err  error
}

func (e *StreamError) Error() string {
	s := "blazingagents: " + e.Meta.OperationID + ": stream " + string(e.Kind)
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *StreamError) Unwrap() error { return e.Err }

// ObjectErrorKind says why generated text did not decode into the requested type.
type ObjectErrorKind string

const (
	ObjectTruncated   ObjectErrorKind = "truncated"    // empty, or JSON that ends early
	ObjectInvalidJSON ObjectErrorKind = "invalid_json" // syntax error or trailing data
	ObjectMismatch    ObjectErrorKind = "mismatch"     // valid JSON that does not decode into T
)

// ObjectError is generated text that is not the requested object.
type ObjectError struct {
	Kind ObjectErrorKind
	Text string // the full generated text
	Meta ResponseMeta
	Err  error
}

func (e *ObjectError) Error() string {
	return "blazingagents: " + e.Meta.OperationID + ": object " + string(e.Kind) + ": " + e.Err.Error()
}

func (e *ObjectError) Unwrap() error { return e.Err }

// DecodeError is a 2xx body that does not decode into the documented shape.
type DecodeError struct {
	OperationID string
	RequestID   string
	Err         error
}

func (e *DecodeError) Error() string {
	return "blazingagents: " + e.OperationID + ": decode response: " + e.Err.Error()
}

func (e *DecodeError) Unwrap() error { return e.Err }

// ArgumentError is an input the SDK rejected before sending anything.
type ArgumentError struct {
	Name   string // "agentId", "params.Model", "header X-Trace", ...
	Reason string
	Err    error // a sentinel such as ErrInvalidHeader, or nil
}

func (e *ArgumentError) Error() string {
	return "blazingagents: invalid argument " + e.Name + ": " + e.Reason
}

func (e *ArgumentError) Unwrap() error { return e.Err }

// CredentialEchoError is a 2xx response of a credential operation that contained a
// credential field. The body is discarded.
type CredentialEchoError struct {
	OperationID string
	RequestID   string
	Field       string
}

func (e *CredentialEchoError) Error() string {
	return "blazingagents: " + e.OperationID + ": response echoed credential field " + e.Field + "; body discarded"
}
