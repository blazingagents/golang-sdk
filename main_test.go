package blazingagents_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func TestMain(m *testing.M) { os.Exit(testkit.Main(m)) }

func jsonUnmarshalString(s string, v any) error { return json.Unmarshal([]byte(s), v) }

func jsonEqualString(a, b string) bool {
	var x, y any
	return json.Unmarshal([]byte(a), &x) == nil && json.Unmarshal([]byte(b), &y) == nil && jsonString(x) == jsonString(y)
}

func jsonString(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// A nested struct the example omits marshals as an object of zero values, which
// MatchesExample treats as absent, as it does a zero scalar or time.
func TestMatchesExampleZeroObject(t *testing.T) {
	type health struct {
		OK        bool      `json:"ok"`
		CheckedAt time.Time `json:"checkedAt"`
		Checks    []string  `json:"checks"`
	}
	testkit.MatchesExample(t, `{"id":"c1"}`, struct {
		ID     string `json:"id"`
		Health health `json:"health"`
	}{ID: "c1"})
}

// An example's explicit null still fails against an object: a nullable object modeled as a
// struct value is a defect the example must catch.
func TestMatchesExampleNullObject(t *testing.T) {
	type health struct {
		OK bool `json:"ok"`
	}
	r := &failRecorder{TB: t}
	testkit.MatchesExample(r, `{"health":null}`, struct {
		Health health `json:"health"`
	}{})
	if !r.failed {
		t.Fatal("null matched an object of zero values")
	}
}

type failRecorder struct {
	testing.TB
	failed bool
}

func (r *failRecorder) Errorf(string, ...any) { r.failed = true }
