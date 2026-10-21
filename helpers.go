package hookbase

import (
	"encoding/json"
	"strconv"
)

func itoa(i int) string {
	return strconv.Itoa(i)
}

func btoa(b bool) string {
	return strconv.FormatBool(b)
}

// Ptr returns a pointer to the given value. Useful for setting optional fields.
func Ptr[T any](v T) *T {
	return &v
}

// FlexBool handles JSON booleans that may arrive as integers (0/1) from D1/SQLite.
type FlexBool bool

func (b *FlexBool) UnmarshalJSON(data []byte) error {
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch v := raw.(type) {
	case bool:
		*b = FlexBool(v)
	case float64:
		*b = FlexBool(v != 0)
	default:
		*b = false
	}
	return nil
}

func (b FlexBool) MarshalJSON() ([]byte, error) {
	return json.Marshal(bool(b))
}

func (b FlexBool) Bool() bool {
	return bool(b)
}

// JSONString handles fields stored as JSON strings in D1 that may be returned
// as either a raw JSON string or a parsed value.
type JSONString[T any] struct {
	Value T
}

func (j *JSONString[T]) UnmarshalJSON(data []byte) error {
	// Try to unmarshal directly (already parsed)
	if err := json.Unmarshal(data, &j.Value); err == nil {
		return nil
	}
	// Try as a JSON string (double-encoded)
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return nil // leave zero value
	}
	json.Unmarshal([]byte(s), &j.Value)
	return nil
}

// boolAsInt decodes a JSON boolean into the 1/0 *int some older response types declare. The API
// sends signatureValid as a real boolean, which encoding/json refuses to put in an int, failing the
// whole response; this keeps those fields' type (true is 1, false is 0, null is nil). A number is
// accepted too.
type boolAsInt struct {
	v *int
}

func (b *boolAsInt) UnmarshalJSON(data []byte) error {
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch v := raw.(type) {
	case bool:
		n := 0
		if v {
			n = 1
		}
		b.v = &n
	case float64:
		n := int(v)
		b.v = &n
	default:
		b.v = nil
	}
	return nil
}
