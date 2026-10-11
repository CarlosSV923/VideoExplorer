package domain

// NameError is returned for a name input that cannot be written as a library name.
// Field names the input, with the names of the golden vectors (contracts/naming-cases.json).
type NameError struct {
	Field string
}

// Error describes the rejected input.
func (e *NameError) Error() string {
	return "invalid name: " + e.Field
}
