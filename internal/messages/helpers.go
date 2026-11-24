package messages

import (
	"encoding/json"
	"errors"
)

// ErrNotFound is returned when a message row does not exist.
var ErrNotFound = errors.New("messages: not found")

// jsonUnmarshal decodes a JSONB blob into a generic map, tolerating empty input.
func jsonUnmarshal(b []byte, dst *map[string]any) error {
	if len(b) == 0 {
		*dst = nil
		return nil
	}
	return json.Unmarshal(b, dst)
}
