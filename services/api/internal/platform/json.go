package platform

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
)

// Fiber requires non-variadic adapters. Preserve null collections on the wire;
// retain v2's strict UTF-8 and duplicate-name checks.
func MarshalJSON(value any) ([]byte, error) {
	return json.Marshal(value, json.FormatNilSliceAsNull(true), json.FormatNilMapAsNull(true), jsontext.EscapeForHTML(true))
}

func UnmarshalJSON(data []byte, value any) error { return json.Unmarshal(data, value) }
