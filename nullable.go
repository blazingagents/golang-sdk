package blazingagents

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
)

type nullState uint8

const (
	omitted nullState = iota
	isNull
	isSet
)

// Nullable is a request field the API lets you omit, send as null, or send with a value.
// The zero value omits the field.
type Nullable[T any] struct {
	value T
	state nullState
}

// Null sends JSON null.
func Null[T any]() Nullable[T] { return Nullable[T]{state: isNull} }

// Value sends v. A nil slice or map sends [] or {}.
func Value[T any](v T) Nullable[T] { return Nullable[T]{value: v, state: isSet} }

// Get returns the value and whether one is set.
func (n Nullable[T]) Get() (T, bool) { return n.value, n.state == isSet }

// IsNull reports whether the field sends null.
func (n Nullable[T]) IsNull() bool { return n.state == isNull }

// IsZero reports whether the field is omitted; encoding/json's omitzero calls it.
func (n Nullable[T]) IsZero() bool { return n.state == omitted }

var errOmittedNullable = errors.New("blazingagents: omitted Nullable marshaled without omitzero")

func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	switch n.state {
	case isNull:
		return []byte("null"), nil
	case isSet:
		return marshalNonNil(n.value)
	}
	return nil, errOmittedNullable
}

// Format prints <omitted>, null, or the value with the same verb, so a Secret inside stays redacted.
func (n Nullable[T]) Format(f fmt.State, verb rune) {
	switch n.state {
	case omitted:
		io.WriteString(f, "<omitted>")
	case isNull:
		io.WriteString(f, "null")
	default:
		fmt.Fprintf(f, fmt.FormatString(f, verb), n.value)
	}
}

func marshalNonNil(v any) ([]byte, error) {
	switch rv := reflect.ValueOf(v); rv.Kind() {
	case reflect.Slice:
		if rv.IsNil() {
			return []byte("[]"), nil
		}
	case reflect.Map:
		if rv.IsNil() {
			return []byte("{}"), nil
		}
	}
	return json.Marshal(v)
}
