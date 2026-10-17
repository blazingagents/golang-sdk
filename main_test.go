package blazingagents_test

import (
	"encoding/json"
	"os"
	"testing"

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
