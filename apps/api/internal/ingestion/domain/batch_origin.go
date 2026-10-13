package domain

// BatchOrigin says where the files of a batch came from: a browser upload (RF-03), or a
// compressed file that Samba copied into No asignados (RF-27a).
type BatchOrigin string

// The origins a batch can have.
const (
	// OriginUpload: the files were dropped in the browser (RF-03).
	OriginUpload BatchOrigin = "upload"
	// OriginSamba: the file was copied by Samba into _unassigned/ and is taken as an upload (RF-27a).
	OriginSamba BatchOrigin = "samba"
)

// Valid reports whether o is one of the origins above.
func (o BatchOrigin) Valid() bool {
	return o == OriginUpload || o == OriginSamba
}
