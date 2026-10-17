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
