// Package dto holds input/output shapes for the resume service that are not
// GORM models.
package dto

// UploadInput carries an upload request through the decorator chain.
// ContentType, Extension, ID, and StorageKey start empty and are filled in by
// ValidationService (content-type sniffing) and ImplService (ID + storage
// key generation) before reaching the repository.
type UploadInput struct {
	ID               string
	Label            string
	OriginalFilename string
	ContentType      string
	Extension        string
	RawBytes         []byte
	DeclaredSize     int64
	StorageKey       string
}
