// Package objectstorage provides a small object-storage abstraction backed by
// an S3-compatible API. The only production implementation targets OCI
// Object Storage's S3-compatible endpoint, but the interface carries no
// OCI-specific shape — swapping providers later touches only s3_storage.go.
package objectstorage

import (
	"context"
	"errors"
	"io"
)

// ErrNotFound is returned by Get when key does not exist in the bucket.
var ErrNotFound = errors.New("objectstorage: object not found")

// Storage is the contract every implementation honours.
type Storage interface {
	// Put uploads r (exactly size bytes) to key with the given content type.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// Delete removes the object at key. Deleting a missing key is not an error.
	Delete(ctx context.Context, key string) error
	// Get returns a reader over key's bytes plus its size. The caller must
	// Close() the reader. Returns ErrNotFound if key does not exist.
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
}
