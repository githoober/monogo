package monolog

import (
	"encoding/json"
)

// MarshalJSONMap marshals a map to JSON.
func MarshalJSONMap(m map[string]interface{}) ([]byte, error) {
	return json.Marshal(m)
}
