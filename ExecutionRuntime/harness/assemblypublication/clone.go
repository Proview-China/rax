package assemblypublication

import "encoding/json"

func clone[T any](value T) T {
	payload, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var result T
	if err := json.Unmarshal(payload, &result); err != nil {
		return value
	}
	return result
}
