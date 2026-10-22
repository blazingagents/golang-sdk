package blazingagents

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

var (
	dateType = reflect.TypeFor[Date]()
	timeType = reflect.TypeFor[time.Time]()
)

// encodeQueryAndHeaders reads the query:"name" and header:"Name" fields of params, including
// those of an embedded *DateRange. A query field is sent only when the operation lists it, so
// a name the operation takes in its body is left to the body. It is the first to read params,
// so it also rejects a typed nil behind a params interface ((*SlackConnection)(nil)).
func encodeQueryAndHeaders(o *op.Operation, params any) (url.Values, http.Header, error) {
	q, h := url.Values{}, http.Header{}
	v := reflect.ValueOf(params)
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return nil, nil, &ArgumentError{Name: "params", Reason: "nil"}
	}
	if v.Kind() != reflect.Struct {
		return q, h, nil
	}
	for _, pf := range fields(v) {
		f, fv := pf.f, pf.v
		if name, ok := f.Tag.Lookup("query"); ok && slices.Contains(o.Query, name) {
			if s, ok := queryValue(fv); ok {
				q.Set(name, s)
			}
		}
		if name, ok := f.Tag.Lookup("header"); ok {
			s := fv.String()
			if s == "" || !validHeaderValue(s) {
				return nil, nil, &ArgumentError{Name: "params." + f.Name, Reason: "must be a non-empty valid header value"}
			}
			h.Set(name, s)
		}
	}
	return q, h, nil
}

type paramField struct {
	f reflect.StructField
	v reflect.Value
}

// fields lists the fields of a struct value, with those of a non-nil embedded struct
// pointer (*DateRange) in its place.
func fields(v reflect.Value) []paramField {
	var out []paramField
	t := v.Type()
	for i := range t.NumField() {
		f, fv := t.Field(i), v.Field(i)
		switch {
		case !f.Anonymous:
			out = append(out, paramField{f, fv})
		case !fv.IsNil():
			out = append(out, fields(fv.Elem())...)
		}
	}
	return out
}

// queryValue formats v, or reports false for a nil pointer.
func queryValue(v reflect.Value) (string, bool) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return "", false
		}
		return queryValue(v.Elem())
	}
	switch v.Type() {
	case dateType:
		return v.Interface().(Date).String(), true
	case timeType:
		return v.Interface().(time.Time).UTC().Format(time.RFC3339Nano), true
	}
	return fmt.Sprint(v.Interface()), true
}
