package domain_test

import (
	"encoding/json"
	"os"
	"testing"
)

// expectation is the expected value of a golden vector: a string, or an object
// {"error": field} for an input that must be rejected (the field is the one that
// is not usable).
type expectation struct {
	Value string
	Error string
}

// UnmarshalJSON reads either form of expected.
func (e *expectation) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &e.Value)
	}
	var rejected struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(b, &rejected); err != nil {
		return err
	}
	e.Error = rejected.Error
	return nil
}

// readVectors loads the whole contracts/naming-cases.json into v.
func readVectors(t *testing.T, v any) {
	t.Helper()
	raw, err := os.ReadFile("../../../../../contracts/naming-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatal(err)
	}
}
