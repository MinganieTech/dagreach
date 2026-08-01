package dagreach

// A JSON decoder that preserves key order.
//
// Declaration order is part of dagreach's contract - every list it prints
// follows the order the source declared - so a decoder that hands objects back
// in map order would make reports depend on nothing. encoding/json's token
// stream gives the order back, at the cost of walking it ourselves.
//
// Numbers keep their literal spelling (json.Number), so `180` stays `180`
// rather than becoming `180.0`.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Object is a JSON object with its keys in the order the document declared them.
type Object struct {
	keys   []string
	values map[string]any
}

func newObject() *Object {
	return &Object{values: map[string]any{}}
}

// set declares a key, and refuses to declare it twice.
//
// JSON allows a repeated key and says nothing about which one wins, so two
// readers of the same file can disagree about the graph it describes. A tool
// whose answers are meant to be arguable cannot start from a document that means
// two things.
func (o *Object) set(key string, value any) error {
	if _, present := o.values[key]; present {
		return fmt.Errorf("the key '%s' is declared twice in the same object, "+
			"so the document does not have one meaning", key)
	}
	o.keys = append(o.keys, key)
	o.values[key] = value
	return nil
}

// Get returns the value for key, and whether it was present.
func (o *Object) Get(key string) (any, bool) {
	if o == nil {
		return nil, false
	}
	value, present := o.values[key]
	return value, present
}

// Value returns the value for key, or nil.
func (o *Object) Value(key string) any {
	value, _ := o.Get(key)
	return value
}

// Keys returns the keys in declaration order.
func (o *Object) Keys() []string {
	if o == nil {
		return nil
	}
	return o.keys
}

func (o *Object) Len() int {
	if o == nil {
		return 0
	}
	return len(o.keys)
}

// DecodeOrderedJSON decodes a document, preserving object key order.
//
// A document is one value followed by the end of the file. Anything after it -
// a second value, a truncated copy appended by a broken pipeline, trailing
// junk - is refused rather than ignored: reading the first half of a file in
// silence is how an analysis ends up describing a graph nobody has.
func DecodeOrderedJSON(text string) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader([]byte(text)))
	decoder.UseNumber()
	value, err := decodeValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after the end of the document, at byte %d",
			decoder.InputOffset())
	}
	return value, nil
}

func decodeValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	return decodeFrom(decoder, token)
}

func decodeFrom(decoder *json.Decoder, token json.Token) (any, error) {
	switch typed := token.(type) {
	case json.Delim:
		switch typed {
		case '{':
			object := newObject()
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, _ := keyToken.(string)
				value, err := decodeValue(decoder)
				if err != nil {
					return nil, err
				}
				if err := object.set(key, value); err != nil {
					return nil, err
				}
			}
			if _, err := decoder.Token(); err != nil { // consume '}'
				return nil, err
			}
			return object, nil
		case '[':
			items := []any{}
			for decoder.More() {
				value, err := decodeValue(decoder)
				if err != nil {
					return nil, err
				}
				items = append(items, value)
			}
			if _, err := decoder.Token(); err != nil { // consume ']'
				return nil, err
			}
			return items, nil
		}
		return nil, nil
	default:
		return token, nil
	}
}

// renderValue serialises a container that is kept verbatim as an attribute
// value - a list of owners, a nested object - so it can travel as text and be
// selected on. Keys are sorted and separators spaced (", ", ": ") so the same
// metadata always produces the same string.
func renderValue(value any) string {
	var built strings.Builder
	writeValue(&built, value)
	return built.String()
}

func writeValue(built *strings.Builder, value any) {
	switch typed := value.(type) {
	case nil:
		built.WriteString("null")
	case bool:
		if typed {
			built.WriteString("true")
		} else {
			built.WriteString("false")
		}
	case json.Number:
		built.WriteString(typed.String())
	case string:
		encoded, _ := json.Marshal(typed)
		built.Write(encoded)
	case []any:
		built.WriteString("[")
		for index, item := range typed {
			if index > 0 {
				built.WriteString(", ")
			}
			writeValue(built, item)
		}
		built.WriteString("]")
	case *Object:
		built.WriteString("{")
		keys := append([]string{}, typed.Keys()...)
		sort.Strings(keys)
		for index, key := range keys {
			if index > 0 {
				built.WriteString(", ")
			}
			encoded, _ := json.Marshal(key)
			built.Write(encoded)
			built.WriteString(": ")
			writeValue(built, typed.Value(key))
		}
		built.WriteString("}")
	}
}

// renderScalar quotes a value for a warning message: strings in quotes, numbers
// bare, containers rendered by renderValue.
func renderScalar(value any) string {
	if text, ok := value.(string); ok {
		return "'" + text + "'"
	}
	if number, ok := value.(json.Number); ok {
		return number.String()
	}
	return renderValue(value)
}
