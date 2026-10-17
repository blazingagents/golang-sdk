package testkit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
)

var loadSpec = sync.OnceValue(func() map[string]any {
	_, file, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "spec", "openapi.json"))
	if err != nil {
		panic("testkit: " + err.Error())
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		panic("testkit: spec: " + err.Error())
	}
	return doc
})

// Example is the spec's example response of an operation, as JSON, or as the text itself
// for a text/plain response. It panics when the spec has none.
func Example(opID string, status int) string {
	doc := loadSpec()
	paths, _ := doc["paths"].(map[string]any)
	for _, item := range paths {
		methods, _ := item.(map[string]any)
		for _, v := range methods {
			o, _ := v.(map[string]any)
			if o["operationId"] != opID {
				continue
			}
			responses, _ := o["responses"].(map[string]any)
			r, _ := responses[strconv.Itoa(status)].(map[string]any)
			content, _ := r["content"].(map[string]any)
			for _, c := range content {
				ex, ok := c.(map[string]any)["example"]
				if !ok {
					continue
				}
				if s, ok := ex.(string); ok {
					return s
				}
				data, _ := json.Marshal(ex)
				return string(data)
			}
		}
	}
	panic(fmt.Sprintf("testkit: no example for %s %d", opID, status))
}
