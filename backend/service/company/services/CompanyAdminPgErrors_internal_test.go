package services

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// TestIsUniqueViolation_Synthetic covers the conflict-detection helper used by
// the admin Create classification (P1.1): a synthetic 23505 pgconn error and
// the gorm.ErrDuplicatedKey sentinel both report true; everything else false.
func TestIsUniqueViolation_Synthetic(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"pg 23505", &pgconn.PgError{Code: "23505"}, true},
		{"gorm sentinel", gorm.ErrDuplicatedKey, true},
		{"pg 23503 not unique", &pgconn.PgError{Code: "23503"}, false},
		{"unrelated", errors.New("boom"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isUniqueViolation(tc.err); got != tc.want {
				t.Errorf("isUniqueViolation(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestIsForeignKeyViolation_Synthetic covers the FK-detection helper (P1.1):
// a synthetic 23503 pgconn error and gorm.ErrForeignKeyViolated report true;
// a unique violation and unrelated errors report false.
func TestIsForeignKeyViolation_Synthetic(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"pg 23503", &pgconn.PgError{Code: "23503"}, true},
		{"gorm sentinel", gorm.ErrForeignKeyViolated, true},
		{"pg 23505 not fk", &pgconn.PgError{Code: "23505"}, false},
		{"unrelated", errors.New("boom"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isForeignKeyViolation(tc.err); got != tc.want {
				t.Errorf("isForeignKeyViolation(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
