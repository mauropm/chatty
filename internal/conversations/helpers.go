package conversations

import "encoding/json"

// jsonUnmarshal decodes a JSONB blob into a generic map.
func jsonUnmarshal(b []byte, dst *map[string]any) error {
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, dst)
}
