package blazingagents

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// decodeUnion decodes raw as op.Unions[name]. An unknown tag returns unknown(tag, raw) and is
// not an error. A missing, null or non-scalar tag, a known tag whose payload lacks a required
// key or has it as null, or a payload that does not decode is an error.
func decodeUnion[T any](name string, raw []byte, known map[string]func([]byte) (T, error), unknown func(tag string, raw json.RawMessage) T) (T, error) {
	var zero T
	u := op.Unions[name]
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return zero, fmt.Errorf("%s: %w", name, err)
	}
	tagRaw, ok := obj[u.Tag]
	if !ok {
		return zero, fmt.Errorf("%s: missing %q", name, u.Tag)
	}
	if bytes.Equal(tagRaw, []byte("null")) { // Unmarshal of null into a string succeeds and leaves ""
		return zero, fmt.Errorf("%s: %q is null", name, u.Tag)
	}
	var tag string
	if err := json.Unmarshal(tagRaw, &tag); err != nil {
		if s := string(tagRaw); s != "true" && s != "false" {
			return zero, fmt.Errorf("%s: %q is not a string or boolean", name, u.Tag)
		}
		tag = string(tagRaw)
	}
	decode, ok := known[tag]
	if !ok {
		return unknown(tag, append(json.RawMessage(nil), raw...)), nil
	}
	payload := raw
	if u.Payload != "" {
		if payload, ok = obj[u.Payload]; !ok {
			return zero, fmt.Errorf("%s %s: missing %q", name, tag, u.Payload)
		}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return zero, fmt.Errorf("%s %s: %w", name, tag, err)
	}
	for _, k := range u.Variants[tag] {
		if v, ok := fields[k]; !ok || bytes.Equal(v, []byte("null")) {
			return zero, fmt.Errorf("%s %s: missing %q", name, tag, k)
		}
	}
	return decode(payload)
}

// variant decodes a known variant's payload into V.
func variant[U any, V any](wrap func(V) U) func([]byte) (U, error) {
	return func(b []byte) (U, error) {
		var v V
		err := json.Unmarshal(b, &v)
		return wrap(v), err
	}
}

// encodeVariant marshals a known variant: the tag, then fields (an object), nested under the
// union's payload key when it has one.
func encodeVariant(name, tag string, fields any) ([]byte, error) {
	u := op.Unions[name]
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	tagJSON, _ := json.Marshal(tag)
	var b bytes.Buffer
	b.WriteString(`{"` + u.Tag + `":`)
	b.Write(tagJSON)
	switch {
	case u.Payload != "":
		b.WriteString(`,"` + u.Payload + `":`)
		b.Write(data)
		b.WriteByte('}')
	case len(data) > 2:
		b.WriteByte(',')
		b.Write(data[1:])
	default:
		b.WriteByte('}')
	}
	return b.Bytes(), nil
}
