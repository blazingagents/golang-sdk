// Package testkit is the test support of the SDK's external tests: a mock API that asserts
// every request exactly, spec examples, and the gates TestMain runs.
package testkit

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
)

// APIKey matches op.APIKeyPattern. Every mock request must carry it.
const APIKey = "ba_full_0123456789abcdefghij0123456789ABCDEFGHIJ"

// Exchange is one expected request and the response the mock sends.
type Exchange struct {
	Op       string      // operationId; method and path template are checked against op.All
	Path     string      // exact escaped path; "" checks only the template
	Query    url.Values  // exact; nil means no query
	Header   http.Header // each listed header must have exactly these values
	NoHeader []string    // each must be absent
	// Body is the exact JSON body, compared decoded with UseNumber, so an absent key differs
	// from null. "" means no body unless Multipart or Raw is set.
	Body      string
	Multipart []Part // exact parts in order
	Raw       []byte // exact octet-stream body
	// BodyFunc checks a JSON body that cannot be written out in advance, such as one with a
	// random ID; it replaces Body.
	BodyFunc func(body []byte) error

	Status     int // default 200
	RespHeader http.Header
	RespBody   string        // JSON or text
	SSE        []string      // chunks written and flushed one at a time
	Hang       bool          // after the response so far, wait until the request is canceled
	Delay      time.Duration // wait before writing the response headers
	Interval   time.Duration // wait before each SSE chunk
}

// Part is one expected multipart part. When Disposition is set, the part's raw
// Content-Disposition must equal it, and Filename is not compared: Go's parser reads quoted
// filenames differently from the API's WHATWG parser.
type Part struct {
	Name, Filename, ContentType string
	Disposition                 string
	Data                        []byte
}

// Mock is a mock of the API. Expectations are served in order; a mismatch, an unexpected
// request or a leftover expectation fails the test.
type Mock struct {
	t      testing.TB
	mu     sync.Mutex
	queue  []Exchange
	srv    *httptest.Server
	memory bool
}

// New returns a mock whose leftovers fail t at cleanup.
func New(t testing.TB) *Mock {
	m := &Mock{t: t}
	t.Cleanup(func() {
		if m.srv != nil {
			m.srv.Close()
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, e := range m.queue {
			t.Errorf("testkit: expected %s %s was never sent", e.Op, e.Path)
		}
	})
	return m
}

// Expect queues an exchange.
func (m *Mock) Expect(e Exchange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if opByID(e.Op) == nil {
		m.t.Fatalf("testkit: unknown operation %q", e.Op)
	}
	m.queue = append(m.queue, e)
}

// URL is the base URL of the mock's HTTP server, started on first use.
func (m *Mock) URL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.srv == nil {
		m.srv = httptest.NewServer(http.HandlerFunc(m.serve))
	}
	return m.srv.URL
}

// memoryBase is the base URL of a client that uses Transport.
const memoryBase = "http://testkit.invalid"

// Client is a tenant client with the mock's base URL and APIKey. Options come after those.
// When Transport was called first, the client must use it.
func (m *Mock) Client(opts ...ba.Option) *ba.Client {
	base := memoryBase
	if !m.memory {
		base = m.URL()
	}
	c, err := ba.NewClient(append([]ba.Option{ba.WithAPIKey(APIKey), ba.WithBaseURL(base)}, opts...)...)
	if err != nil {
		m.t.Fatalf("testkit: NewClient: %v", err)
	}
	return c
}

func (m *Mock) serve(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	if len(m.queue) == 0 {
		m.mu.Unlock()
		m.t.Errorf("testkit: unexpected request %s %s", r.Method, r.URL)
		http.Error(w, "testkit: unexpected request", http.StatusTeapot)
		return
	}
	e := m.queue[0]
	m.queue = m.queue[1:]
	m.mu.Unlock()
	o := opByID(e.Op)
	if r.Header.Get("X-BA-User-Id") != "" && !o.UserScoped {
		m.t.Errorf("testkit: %s is not user-scoped but carried X-BA-User-Id", e.Op)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"error":{"code":"forbidden","message":"user-scoped key cannot call this operation"}}`)
		return
	}
	if problems := check(e, o, r); len(problems) > 0 {
		m.t.Errorf("testkit: %s: %s", e.Op, strings.Join(problems, "; "))
		http.Error(w, "testkit: request mismatch", http.StatusTeapot)
		return
	}
	record(&reg.served, e.Op)
	respond(w, r, e)
}

func check(e Exchange, o *op.Operation, r *http.Request) []string {
	var p []string
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }
	if r.Method != o.Method {
		add("method %s, want %s", r.Method, o.Method)
	}
	path := r.URL.EscapedPath()
	if !templateRegexp(o.Path).MatchString(path) {
		add("path %s does not match %s", path, o.Path)
	}
	if e.Path != "" && path != e.Path {
		add("path %s, want %s", path, e.Path)
	}
	switch {
	case e.Query == nil && r.URL.RawQuery != "":
		add("query %q, want none", r.URL.RawQuery)
	case e.Query != nil && !reflect.DeepEqual(r.URL.Query(), e.Query):
		add("query %v, want %v", r.URL.Query(), e.Query)
	}
	if got := r.Header.Values("Authorization"); !slices.Equal(got, []string{"Bearer " + APIKey}) {
		add("Authorization %q", got)
	}
	for k, want := range e.Header {
		if got := r.Header.Values(k); !slices.Equal(got, want) {
			add("header %s %q, want %q", k, got, want)
		}
	}
	for _, k := range e.NoHeader {
		if got := r.Header.Values(k); len(got) > 0 {
			add("header %s %q, want absent", k, got)
		}
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		add("read body: %v", err)
	}
	ct := r.Header.Get("Content-Type")
	switch {
	case e.Multipart != nil:
		p = append(p, checkMultipart(ct, body, e.Multipart)...)
	case e.Raw != nil:
		if ct != "application/octet-stream" || !bytes.Equal(body, e.Raw) {
			add("raw body %q (%s), want %q", body, ct, e.Raw)
		}
	case e.BodyFunc != nil:
		if err := e.BodyFunc(body); err != nil {
			add("body %s: %v", body, err)
		}
	case e.Body != "":
		if ct != "application/json" {
			add("Content-Type %q, want application/json", ct)
		}
		if !jsonEqual(body, []byte(e.Body)) {
			add("body %s, want %s", body, e.Body)
		}
	case len(body) > 0:
		add("body %q, want none", body)
	}
	return p
}

func checkMultipart(ct string, body []byte, want []Part) []string {
	mt, params, err := mime.ParseMediaType(ct)
	if err != nil || mt != "multipart/form-data" {
		return []string{fmt.Sprintf("Content-Type %q, want multipart/form-data", ct)}
	}
	mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	var got []Part
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return []string{"multipart: " + err.Error()}
		}
		data, _ := io.ReadAll(part)
		p := Part{Name: part.FormName(), Filename: part.FileName(), ContentType: part.Header.Get("Content-Type"), Data: data}
		if i := len(got); i < len(want) && want[i].Disposition != "" {
			p.Filename, p.Disposition = "", part.Header.Get("Content-Disposition")
		}
		got = append(got, p)
	}
	if !reflect.DeepEqual(got, want) {
		return []string{fmt.Sprintf("multipart parts %+v, want %+v", got, want)}
	}
	return nil
}

func jsonEqual(a, b []byte) bool {
	x, errA := decodeNumber(a)
	y, errB := decodeNumber(b)
	return errA == nil && errB == nil && reflect.DeepEqual(x, y)
}

func decodeNumber(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	err := dec.Decode(&v)
	return v, err
}

func respond(w http.ResponseWriter, r *http.Request, e Exchange) {
	sleep(r, e.Delay)
	for k, v := range e.RespHeader {
		w.Header()[k] = v
	}
	status := e.Status
	if status == 0 {
		status = http.StatusOK
	}
	flush := func() {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	switch {
	case e.SSE != nil:
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "text/event-stream")
		}
		w.WriteHeader(status)
		flush()
		for _, chunk := range e.SSE {
			if !sleep(r, e.Interval) {
				return
			}
			io.WriteString(w, chunk)
			flush()
		}
	case e.RespBody != "":
		if w.Header().Get("Content-Type") == "" {
			ct := "text/plain; charset=utf-8"
			if s := strings.TrimSpace(e.RespBody); strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
				ct = "application/json"
			}
			w.Header().Set("Content-Type", ct)
		}
		w.WriteHeader(status)
		io.WriteString(w, e.RespBody)
		flush()
	case e.Hang:
		w.WriteHeader(status)
		flush()
	default:
		w.WriteHeader(status)
	}
	if e.Hang {
		<-r.Context().Done()
	}
}

// sleep waits d unless the request ends first, and reports whether d passed.
func sleep(r *http.Request, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-r.Context().Done():
		return false
	}
}

func opByID(id string) *op.Operation {
	for _, o := range op.All {
		if o.ID == id {
			return o
		}
	}
	return nil
}

var templates sync.Map // path template -> *regexp.Regexp

func templateRegexp(path string) *regexp.Regexp {
	if re, ok := templates.Load(path); ok {
		return re.(*regexp.Regexp)
	}
	re := regexp.MustCompile("^" + regexp.MustCompile(`\\\{[^}]+\\\}`).ReplaceAllString(regexp.QuoteMeta(path), `[^/]+`) + "$")
	templates.Store(path, re)
	return re
}

// registry is what Main checks once every test has run.
type registry struct {
	mu           sync.Mutex
	served       map[string]bool
	roundTripped map[string]bool
	redactions   map[string]func(secret string) any
}

var reg = registry{served: map[string]bool{}, roundTripped: map[string]bool{}, redactions: map[string]func(string) any{}}

func record(m *map[string]bool, key string) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	(*m)[key] = true
}

// RedactionCase registers a sample of the named Secret-holding type for secret_test.go.
// Call it from a package-level var in the owner's test file.
func RedactionCase(name string, sample func(secret string) any) bool {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if _, dup := reg.redactions[name]; dup {
		panic("testkit: RedactionCase " + name + " registered twice")
	}
	reg.redactions[name] = sample
	return true
}

// RedactionCases returns the registered samples by type name.
func RedactionCases() map[string]func(secret string) any {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	out := make(map[string]func(string) any, len(reg.redactions))
	for k, v := range reg.redactions {
		out[k] = v
	}
	return out
}

// Main is the body of TestMain. After a passing, unfiltered run it fails when an operation
// was never served or a string-tagged union was never round-tripped by UnknownRoundTrip.
func Main(m *testing.M) int {
	code := m.Run()
	if code != 0 || filtered() {
		return code
	}
	var problems []string
	for _, o := range op.All {
		if !reg.served[o.ID] {
			problems = append(problems, fmt.Sprintf("operation %s (%s %s) was never sent through the public API", o.ID, o.Method, o.Path))
		}
	}
	for _, name := range sortedKeys(op.Unions) {
		if booleanTag(op.Unions[name]) || reg.roundTripped[name] {
			continue
		}
		problems = append(problems, fmt.Sprintf("union %s: no test round-trips an unknown tag through testkit.UnknownRoundTrip", name))
	}
	if len(problems) == 0 {
		return 0
	}
	fmt.Fprintln(os.Stderr, "testkit: FAIL")
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, "  "+p)
	}
	return 1
}

func filtered() bool {
	for _, name := range []string{"test.run", "test.skip", "test.list"} {
		if f := flag.Lookup(name); f != nil && f.Value.String() != "" {
			return true
		}
	}
	return false
}

func booleanTag(u *op.Union) bool {
	_, t := u.Variants["true"]
	_, f := u.Variants["false"]
	return t || f
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// UnknownRoundTrip checks that got, decoded from the served exchange e, kept every object
// with an unknown union tag: for a variant union the object at the same place in
// json.Marshal(got) must equal it, for a flat union its tag value must survive. Each match
// records the union for Main. Finding no unknown-tag object fails t.
func UnknownRoundTrip(t testing.TB, e Exchange, got any) {
	t.Helper()
	served, err := decodeNumber([]byte(e.RespBody))
	if err != nil {
		t.Fatalf("testkit: UnknownRoundTrip: response body: %v", err)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("testkit: UnknownRoundTrip: marshal: %v", err)
	}
	gotJSON, _ := decodeNumber(data)
	found := 0
	for _, s := range op.UnionSites {
		if s.OperationID != e.Op {
			continue
		}
		u := op.Unions[s.Union]
		for path, obj := range at(served, locator(s.Locator), "") {
			m, ok := obj.(map[string]any)
			if !ok {
				continue
			}
			tag, ok := m[u.Tag].(string)
			if _, known := u.Variants[tag]; !ok || known {
				continue
			}
			found++
			g := at(gotJSON, locator(s.Locator), "")[path]
			gm, _ := g.(map[string]any)
			switch {
			case u.Flat && gm[u.Tag] != tag:
				t.Errorf("testkit: %s at %s: tag %q did not survive: %v", s.Union, path, tag, g)
			case !u.Flat && !reflect.DeepEqual(g, obj):
				t.Errorf("testkit: %s at %s: got %v, want %v", s.Union, path, g, obj)
			default:
				record(&reg.roundTripped, s.Union)
			}
		}
	}
	if found == 0 {
		t.Errorf("testkit: UnknownRoundTrip: %s served no object with an unknown union tag", e.Op)
	}
}

func locator(l string) []string {
	if l == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(l, ".") {
		if name, ok := strings.CutSuffix(part, "[]"); ok {
			if name != "" {
				out = append(out, name)
			}
			out = append(out, "[]")
			continue
		}
		out = append(out, part)
	}
	return out
}

// at maps each concrete path matching tokens to its value.
func at(v any, tokens []string, path string) map[string]any {
	if len(tokens) == 0 {
		return map[string]any{path: v}
	}
	out := map[string]any{}
	switch tok := tokens[0]; tok {
	case "[]":
		arr, _ := v.([]any)
		for i, x := range arr {
			for k, y := range at(x, tokens[1:], fmt.Sprintf("%s[%d]", path, i)) {
				out[k] = y
			}
		}
	default:
		m, _ := v.(map[string]any)
		if x, ok := m[tok]; ok {
			for k, y := range at(x, tokens[1:], path+"."+tok) {
				out[k] = y
			}
		}
	}
	return out
}

// CoversExample checks that every key of the example JSON survives json.Marshal(got).
func CoversExample(t testing.TB, example string, got any) {
	t.Helper()
	want, err := decodeNumber([]byte(example))
	if err != nil {
		t.Fatalf("testkit: CoversExample: example: %v", err)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("testkit: CoversExample: marshal: %v", err)
	}
	have, _ := decodeNumber(data)
	for _, missing := range missingKeys(want, have, "") {
		t.Errorf("testkit: CoversExample: %s is lost", missing)
	}
}

func missingKeys(want, have any, path string) []string {
	var out []string
	switch w := want.(type) {
	case map[string]any:
		h, _ := have.(map[string]any)
		for _, k := range sortedKeys(w) {
			hv, ok := h[k]
			if !ok {
				out = append(out, path+"."+k)
				continue
			}
			out = append(out, missingKeys(w[k], hv, path+"."+k)...)
		}
	case []any:
		h, _ := have.([]any)
		for i := range min(len(w), len(h)) {
			out = append(out, missingKeys(w[i], h[i], fmt.Sprintf("%s[%d]", path, i))...)
		}
	}
	return out
}

// NoSecret fails t when secret appears in any rendering of v: v, &v, struct{ V any }{&v}
// and []any{&v} under %v %+v %#v %s %q %x %d, the slog Text and JSON handlers (top-level
// and in a group), and encoding/json.
func NoSecret(t testing.TB, secret string, v any) {
	t.Helper()
	values := []any{v, &v, struct{ V any }{&v}, []any{&v}}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%d"} {
		for i, x := range values {
			if s := fmt.Sprintf(verb, x); strings.Contains(s, secret) {
				t.Errorf("testkit: secret leaked through %s (form %d): %s", verb, i, s)
			}
		}
	}
	var buf bytes.Buffer
	for _, h := range []slog.Handler{slog.NewTextHandler(&buf, nil), slog.NewJSONHandler(&buf, nil)} {
		slog.New(h).Info("m", "v", v, slog.Group("g", "v", v))
	}
	if strings.Contains(buf.String(), secret) {
		t.Errorf("testkit: secret leaked through slog: %s", buf.String())
	}
	if data, err := json.Marshal(v); err == nil && bytes.Contains(data, []byte(secret)) {
		t.Errorf("testkit: secret leaked through encoding/json: %s", data)
	}
}

var idCounter struct {
	sync.Mutex
	n uint64
}

var idPatternShape = regexp.MustCompile(`^\^([a-z]+)_\[0-9A-Za-z\]\{16\}\$$`)

// ID returns a fresh value matching an ID pattern such as op.IDPatterns["TurnID"]. It panics
// on any other pattern, including "".
func ID(pattern string) string {
	m := idPatternShape.FindStringSubmatch(pattern)
	if m == nil {
		panic(fmt.Sprintf("testkit: ID: %q is not an ID pattern", pattern))
	}
	idCounter.Lock()
	idCounter.n++
	n := idCounter.n
	idCounter.Unlock()
	const digits = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := []byte("0000000000000000")
	for i := len(b) - 1; n > 0; i-- {
		b[i] = digits[n%62]
		n /= 62
	}
	return m[1] + "_" + string(b)
}

// MatchesExample checks the reverse of CoversExample: every value json.Marshal(got) emits
// equals the example's value at the same place, so a skeleton model pins what it decodes.
// A key the example lacks must marshal as a zero value. Timestamps compare as instants.
func MatchesExample(t testing.TB, example string, got any) {
	t.Helper()
	want, err := decodeNumber([]byte(example))
	if err != nil {
		t.Fatalf("testkit: MatchesExample: example: %v", err)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("testkit: MatchesExample: marshal: %v", err)
	}
	have, _ := decodeNumber(data)
	for _, d := range differences(have, want, "") {
		t.Errorf("testkit: MatchesExample: %s", d)
	}
}

func differences(have, want any, path string) []string {
	// An explicit null matches a zero scalar or an empty object, but not an object of zero
	// values: that is a struct value where the API sends null.
	if obj, isObj := have.(map[string]any); want == nil && zeroJSON(have) && (!isObj || len(obj) == 0) {
		return nil
	}
	switch h := have.(type) {
	case map[string]any:
		w, ok := want.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s is an object, example has %v", path, want)}
		}
		var out []string
		for _, k := range sortedKeys(h) {
			wv, ok := w[k]
			if !ok {
				if !zeroJSON(h[k]) {
					out = append(out, fmt.Sprintf("%s.%s is %v, absent from the example", path, k, h[k]))
				}
				continue
			}
			out = append(out, differences(h[k], wv, path+"."+k)...)
		}
		return out
	case []any:
		w, ok := want.([]any)
		if !ok || len(w) != len(h) {
			return []string{fmt.Sprintf("%s is %v, example has %v", path, have, want)}
		}
		var out []string
		for i := range h {
			out = append(out, differences(h[i], w[i], fmt.Sprintf("%s[%d]", path, i))...)
		}
		return out
	case string:
		if w, ok := want.(string); ok && sameInstant(h, w) {
			return nil
		}
	}
	if !reflect.DeepEqual(have, want) {
		return []string{fmt.Sprintf("%s is %v, example has %v", path, have, want)}
	}
	return nil
}

func sameInstant(a, b string) bool {
	x, errA := time.Parse(time.RFC3339Nano, a)
	y, errB := time.Parse(time.RFC3339Nano, b)
	return errA == nil && errB == nil && x.Equal(y)
}

// zeroTime is how a required time.Time field that the example omits marshals.
var zeroTime = time.Time{}.Format(time.RFC3339Nano)

func zeroJSON(v any) bool {
	switch v := v.(type) {
	case nil:
		return true
	case string:
		return v == "" || v == zeroTime
	case bool:
		return !v
	case json.Number:
		return v.String() == "0"
	case []any:
		return len(v) == 0
	case map[string]any: // a struct the example omits marshals as an object of zero values
		for _, x := range v {
			if !zeroJSON(x) {
				return false
			}
		}
		return true
	}
	return false
}
