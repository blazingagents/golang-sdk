package blazingagents

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var rawMessageType = reflect.TypeFor[json.RawMessage]()

// SchemaFor derives a JSON Schema from T. It covers structs with exported fields and json
// tags (a field is required unless it is a pointer or tagged omitempty or omitzero, and no
// other properties are allowed), strings, bools, integers, floats, slices, arrays,
// string-keyed maps, time.Time (date-time), pointers (which add "null"), and json.RawMessage
// or any (unconstrained). A `jsonschema:"..."` tag is the field's description. Recursive
// types, embedded fields, nesting deeper than 32 levels and other kinds are errors; use
// RawSchema for those.
func SchemaFor[T any]() (ObjectSchema[T], error) {
	s, err := schemaOf(reflect.TypeFor[T](), map[reflect.Type]bool{}, 0)
	if err != nil {
		return ObjectSchema[T]{}, err
	}
	raw, err := json.Marshal(s)
	return ObjectSchema[T]{raw}, err
}

type jsonSchema = map[string]any

func schemaOf(t reflect.Type, visiting map[reflect.Type]bool, depth int) (jsonSchema, error) {
	if depth > 32 {
		return nil, errors.New("blazingagents: schema: nesting deeper than 32 levels")
	}
	switch {
	case t == timeType:
		return jsonSchema{"type": "string", "format": "date-time"}, nil
	case t == rawMessageType || t.Kind() == reflect.Interface && t.NumMethod() == 0:
		return jsonSchema{}, nil
	}
	switch t.Kind() {
	case reflect.Pointer:
		s, err := schemaOf(t.Elem(), visiting, depth+1)
		if typ, ok := s["type"].(string); ok {
			s["type"] = []string{typ, "null"}
		}
		return s, err
	case reflect.String:
		return jsonSchema{"type": "string"}, nil
	case reflect.Bool:
		return jsonSchema{"type": "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return jsonSchema{"type": "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return jsonSchema{"type": "number"}, nil
	case reflect.Slice, reflect.Array:
		items, err := schemaOf(t.Elem(), visiting, depth+1)
		return jsonSchema{"type": "array", "items": items}, err
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("blazingagents: schema: map key %s is not a string", t.Key())
		}
		values, err := schemaOf(t.Elem(), visiting, depth+1)
		return jsonSchema{"type": "object", "additionalProperties": values}, err
	case reflect.Struct:
		return structSchema(t, visiting, depth)
	}
	return nil, fmt.Errorf("blazingagents: schema: %s is not supported", t)
}

func structSchema(t reflect.Type, visiting map[reflect.Type]bool, depth int) (jsonSchema, error) {
	if visiting[t] {
		return nil, fmt.Errorf("blazingagents: schema: %s is recursive", t)
	}
	visiting[t] = true
	defer delete(visiting, t)
	props, required := jsonSchema{}, []string{}
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Anonymous {
			return nil, fmt.Errorf("blazingagents: schema: embedded field %s of %s is not supported", f.Name, t)
		}
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		s, err := schemaOf(f.Type, visiting, depth+1)
		if err != nil {
			return nil, err
		}
		if d, ok := f.Tag.Lookup("jsonschema"); ok {
			s["description"] = d
		}
		props[name] = s
		optional := f.Type.Kind() == reflect.Pointer || strings.Contains(","+opts+",", ",omitempty,") || strings.Contains(","+opts+",", ",omitzero,")
		if !optional {
			required = append(required, name)
		}
	}
	return jsonSchema{"type": "object", "properties": props, "required": required, "additionalProperties": false}, nil
}
