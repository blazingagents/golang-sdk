package blazingagents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

const (
	functionCallEvent = "data-ba-function-call"
	maxResultSize     = 256 << 10
	resultGrace       = 30 * time.Second
	firstRetryDelay   = 250 * time.Millisecond
	maxRetryDelay     = 2 * time.Second

	msgInvalidInput  = "Invalid function input."
	msgExecution     = "Function execution failed."
	msgInvalidResult = "Function returned an invalid result."
)

var functionCallIDPattern = regexp.MustCompile(op.FunctionCallIDPattern)

// runner executes the function calls a chat stream announces. Its context is the chat
// request's, so Close, the caller's ctx and a permanent failure all stop it.
type runner struct {
	t         *transport
	ctx       context.Context
	cancel    context.CancelCauseFunc // the chat request's
	meta      ResponseMeta
	agentID   AgentID
	sessionID SessionID
	fns       *functionSet
	opts      []RequestOption // the chat call's per-call headers and client request ID

	mu   sync.Mutex
	seen map[FunctionCallID]bool
	err  error
}

func newRunner(c *call, agentID AgentID, sessionID SessionID, fns *functionSet) *runner {
	header, crid := c.opts.header.Clone(), c.opts.clientRequestID
	return &runner{
		t: c.t, ctx: c.req.Context(), cancel: c.cancel, meta: c.meta,
		agentID: agentID, sessionID: sessionID, fns: fns, seen: map[FunctionCallID]bool{},
		opts: []RequestOption{func(o *callOptions) { o.header, o.clientRequestID = header, crid }},
	}
}

// failure is the permanent failure that ended the runner, or nil.
func (r *runner) failure() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// fail records the first permanent failure and cancels the chat request with it.
func (r *runner) fail(err error) {
	r.mu.Lock()
	if r.err == nil {
		r.err = &StreamError{Kind: StreamFunctionCall, Meta: r.meta, Err: err}
	}
	first := r.err
	r.mu.Unlock()
	r.cancel(first)
}

type functionCallFrame struct {
	Type      string `json:"type"`
	Transient bool   `json:"transient"`
	Data      struct {
		ID         FunctionCallID  `json:"id"`
		Name       *string         `json:"name"`
		Input      json.RawMessage `json:"input"`
		DeadlineAt string          `json:"deadlineAt"`
	} `json:"data"`
}

var errMalformedCall = errors.New("function-call event needs transient: true, a function-call id, a name and an RFC 3339 deadlineAt")

// intercept consumes a function-call event and starts its call. It reports false for any
// other event, so the stream yields it.
func (r *runner) intercept(ev []byte) (bool, error) {
	if r == nil {
		return false, nil
	}
	data := eventData(ev)
	if !bytes.Contains(data, []byte(functionCallEvent)) {
		return false, nil
	}
	// The type decides first, so another event that only mentions the type passes through
	// whatever the shape of its other fields.
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return false, err
	}
	if m, _ := v.(map[string]any); m["type"] != functionCallEvent {
		return false, nil
	}
	var f functionCallFrame
	if err := json.Unmarshal(data, &f); err != nil {
		return false, err
	}
	deadline, err := time.Parse(time.RFC3339Nano, f.Data.DeadlineAt)
	if err != nil || !f.Transient || f.Data.Name == nil || !functionCallIDPattern.MatchString(string(f.Data.ID)) {
		return false, errMalformedCall
	}
	r.mu.Lock()
	seen := r.seen[f.Data.ID]
	r.seen[f.Data.ID] = true
	r.mu.Unlock()
	if !seen {
		go r.run(FunctionCall{ID: f.Data.ID, Name: *f.Data.Name, Deadline: deadline}, f.Data.Input)
	}
	return true, nil
}

// eventData is the joined data: lines of one SSE event.
func eventData(ev []byte) []byte {
	var lines [][]byte
	for line := range bytes.Lines(ev) {
		line = bytes.TrimRight(line, "\r\n")
		if rest, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			lines = append(lines, bytes.TrimPrefix(rest, []byte(" ")))
		}
	}
	return bytes.Join(lines, []byte("\n"))
}

type outcome struct {
	Kind    string          `json:"kind"`
	Value   json.RawMessage `json:"value,omitzero"`
	Message string          `json:"message,omitzero"`
}

func errorOutcome(msg string) outcome { return outcome{Kind: "error", Message: msg} }

func (r *runner) run(call FunctionCall, input json.RawMessage) {
	claimID := newUUID()
	path := []string{string(r.agentID), string(r.sessionID), string(call.ID)}
	claim := op.ClaimChatFunctionCall(path[0], path[1], path[2])
	if ok := r.post(claim, map[string]string{"claimRequestId": claimID}, "claimed", call.Deadline); !ok {
		return
	}
	if r.ctx.Err() != nil || !time.Now().Before(call.Deadline) {
		return // claimed too late: the server no longer waits, so the handler must not run
	}
	ctx, cancel := context.WithDeadline(r.ctx, call.Deadline)
	defer cancel()
	out := r.execute(ctx, call, input)
	if ctx.Err() != nil {
		return // the deadline passed or the stream closed: the result is discarded
	}
	result := op.ResolveChatFunctionCall(path[0], path[1], path[2])
	r.post(result, struct {
		ClaimRequestID string  `json:"claimRequestId"`
		Outcome        outcome `json:"outcome"`
	}{claimID, out}, "accepted", call.Deadline.Add(resultGrace))
}

func (r *runner) execute(ctx context.Context, call FunctionCall, input json.RawMessage) outcome {
	def, ok := r.fns.byName[call.Name]
	if !ok {
		return errorOutcome("Function " + call.Name + " is not available.")
	}
	data, msg := invoke(ctx, def.handler, call, input)
	if msg != "" {
		r.t.logger.LogAttrs(ctx, slog.LevelDebug, "blazingagents function failed",
			slog.String("function", call.Name), slog.String("callId", string(call.ID)), slog.String("outcome", msg))
		return errorOutcome(msg)
	}
	return outcome{Kind: "output", Value: data}
}

// outcomeEnvelope is what an output outcome adds around its value. The server caps the
// whole outcome, measured as JSON.stringify writes it.
const outcomeEnvelope = len(`{"kind":"output","value":}`)

// invoke runs a handler and encodes its result, turning an error, a panic (in the handler or
// in the result's MarshalJSON) or a result over the cap into its sanitized message.
func invoke(ctx context.Context, h handlerFunc, call FunctionCall, input json.RawMessage) (data []byte, msg string) {
	onPanic := msgExecution
	defer func() {
		if p := recover(); p != nil {
			data, msg = nil, onPanic
		}
	}()
	v, err := h(ctx, call, input)
	switch {
	case errors.Is(err, errInvalidInput):
		return nil, msgInvalidInput
	case err != nil:
		return nil, msgExecution
	}
	onPanic = msgInvalidResult
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false) // JSON.stringify does not escape <, > and &
	if err := enc.Encode(v); err != nil || b.Len()-1+outcomeEnvelope > maxResultSize {
		return nil, msgInvalidResult
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), ""
}

// post sends a claim or a result until the server accepts it, retrying transient failures
// with the same body until until. It reports whether the server accepted it. A permanent
// failure ends the runner.
func (r *runner) post(req op.UserScoped, b any, receipt string, until time.Time) bool {
	for attempt := 0; ; attempt++ {
		if r.ctx.Err() != nil || !time.Now().Before(until) {
			return false
		}
		res, err := r.t.send(r.ctx, req.Request, b, r.opts)
		if err == nil {
			var got map[string]json.RawMessage
			if json.Unmarshal(res.data, &got) == nil && string(got[receipt]) == "true" {
				return true
			}
			r.fail(res.decodeError(errors.New("expected " + receipt + ": true")))
			return false
		}
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == 409 {
			r.log("refused", req, err)
			return false
		}
		delay, retry := retryDelay(err, attempt)
		switch {
		case !retry:
			r.fail(err)
			return false
		case !time.Now().Add(delay).Before(until):
			r.log("expired", req, err)
			return false
		}
		t := time.NewTimer(delay)
		select {
		case <-r.ctx.Done():
			t.Stop()
			return false
		case <-t.C:
		}
	}
}

func (r *runner) log(what string, req op.UserScoped, err error) {
	r.t.logger.LogAttrs(r.ctx, slog.LevelDebug, "blazingagents function call "+what,
		slog.String("operation", req.Op.ID), slog.String("error", err.Error()))
}

// retryDelay is the wait before retrying a failed post, or false when the failure is permanent.
func retryDelay(err error, attempt int) (time.Duration, bool) {
	// A post fails with an *APIError, a *ConnectionError or a *TimeoutError; only some API
	// statuses are permanent.
	if apiErr := (*APIError)(nil); errors.As(err, &apiErr) {
		if s := apiErr.Status; s < 500 && s != 408 && s != 429 {
			return 0, false
		}
		if d, ok := retryAfter(apiErr.Meta.Header.Get("Retry-After")); ok {
			return max(d, firstRetryDelay), true // 0 or a past date retries after the floor
		}
	}
	return min(firstRetryDelay<<min(attempt, 4), maxRetryDelay), true
}
