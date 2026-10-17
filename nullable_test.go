package blazingagents_test

import (
	"encoding/json"
	"fmt"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
)

func TestNullable(t *testing.T) {
	type body struct {
		A ba.Nullable[string]            `json:"a,omitzero"`
		B ba.Nullable[[]string]          `json:"b,omitzero"`
		C ba.Nullable[map[string]string] `json:"c,omitzero"`
		D ba.Nullable[int]               `json:"d,omitzero"`
	}
	for _, tc := range []struct {
		in   body
		want string
	}{
		{body{}, `{}`},
		{body{A: ba.Null[string](), D: ba.Value(0)}, `{"a":null,"d":0}`},
		{body{A: ba.Value("x"), B: ba.Value[[]string](nil), C: ba.Value[map[string]string](nil)}, `{"a":"x","b":[],"c":{}}`},
	} {
		if got := jsonString(tc.in); got != tc.want {
			t.Errorf("%+v: %s, want %s", tc.in, got, tc.want)
		}
	}
	if _, err := json.Marshal(struct{ A ba.Nullable[int] }{}); err == nil {
		t.Fatal("an omitted Nullable without omitzero marshaled")
	}
	v := ba.Value(3)
	if got, ok := v.Get(); !ok || got != 3 || v.IsNull() || v.IsZero() {
		t.Fatalf("Value: %v %v", got, ok)
	}
	if n := ba.Null[int](); !n.IsNull() || n.IsZero() {
		t.Fatal("Null")
	}
	if _, ok := (ba.Nullable[int]{}).Get(); ok {
		t.Fatal("zero Nullable has a value")
	}
	if got := fmt.Sprintf("%v %v %v %q", ba.Nullable[int]{}, ba.Null[int](), ba.Value(7), ba.Value("s")); got != `<omitted> null 7 "s"` {
		t.Fatalf("format %s", got)
	}
}
