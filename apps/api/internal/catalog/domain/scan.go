package domain

// ScanKind is one of the two scans of the library (RF-28).
type ScanKind string

// The scans of the library.
const (
	// ScanUnassigned reads _unassigned/ and takes its settled copies of Samba (RF-27, RF-27a, RF-28).
	ScanUnassigned ScanKind = "unassigned"
	// ScanLibrary scans Movies/ and TV Shows/: the import and the removals of RF-29.
	ScanLibrary ScanKind = "library"
)

// ScanKinds lists the scans in the order the API shows them.
func ScanKinds() []ScanKind {
	return []ScanKind{ScanUnassigned, ScanLibrary}
}
