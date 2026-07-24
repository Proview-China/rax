package api

import (
	"encoding/json"
)

func marshalJSON(value any) ([]byte, error) { return json.Marshal(value) }
