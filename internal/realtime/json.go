package realtime

import "encoding/json"

func mustJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		return []byte(`{"error":"encode snapshot"}`)
	}
	return data
}
