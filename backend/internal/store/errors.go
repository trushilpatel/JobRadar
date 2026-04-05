package store

import (
	"errors"

	"gorm.io/gorm"
)

// IsNotFound returns true if the error is a GORM "record not found" error.
func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
