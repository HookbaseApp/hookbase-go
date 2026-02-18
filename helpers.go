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
