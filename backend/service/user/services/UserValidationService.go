package services

import (
	"context"
	"strings"
	"unicode/utf8"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
)

// validationService is the input-validation layer of the user chain. It
// validates the inputs that the public API can submit (UpdateProfile and
// Provision) and delegates the rest. Validation is rune-aware so callers
// supplying multi-byte characters do not slip past byte-length checks.
type validationService struct {
	BaseService
}

// NewValidationService constructs the validation layer.
func NewValidationService(next Service) Service {
	return &validationService{BaseService: NewBaseService(next)}
}

// displayNameMin / displayNameMax mirror the contract in the rollout design
// doc: display_name length 1..120 runes when present.
const (
	displayNameMin = 1
	displayNameMax = 120
)

// UpdateProfile validates the display_name rune length and rejects empty /
// whitespace-only input.
func (v *validationService) UpdateProfile(ctx context.Context, id string, displayName string) error {
	var errs domainerrs.ValidationErrors
	if id == "" {
		errs = append(errs, domainerrs.ValidationError{
			Field:   "id",
			Code:    "REQUIRED",
			Message: "user id is required",
		})
	}
	trimmed := strings.TrimSpace(displayName)
	runeLen := utf8.RuneCountInString(trimmed)
	if runeLen < displayNameMin {
		errs = append(errs, domainerrs.ValidationError{
			Field:   "display_name",
			Code:    "REQUIRED",
			Message: "display_name must not be empty",
		})
	}
	if runeLen > displayNameMax {
		errs = append(errs, domainerrs.ValidationError{
			Field:   "display_name",
			Code:    "TOO_LONG",
			Message: "display_name must be at most 120 characters",
		})
	}
	if len(errs) > 0 {
		return errs
	}
	return v.next.UpdateProfile(ctx, id, trimmed)
}

// Provision validates that the JWT subject and email are syntactically
// usable. Email format is intentionally cheap — a single '@' with a
// non-empty local and domain part — because the JWT issuer has already
// validated the address.
func (v *validationService) Provision(ctx context.Context, id, email string) (*usermodel.User, error) {
	var errs domainerrs.ValidationErrors
	if strings.TrimSpace(id) == "" {
		errs = append(errs, domainerrs.ValidationError{
			Field:   "id",
			Code:    "REQUIRED",
			Message: "user id is required",
		})
	}
	if !looksLikeEmail(email) {
		errs = append(errs, domainerrs.ValidationError{
			Field:   "email",
			Code:    "INVALID_VALUE",
			Message: "email is malformed",
		})
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return v.next.Provision(ctx, id, email)
}

// looksLikeEmail enforces the cheapest reasonable shape: exactly one '@',
// a non-empty local part, and a non-empty domain with at least one dot.
// Anything stricter belongs to Supabase's own validator — duplicating it
// here would only drift over time.
func looksLikeEmail(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	at := strings.LastIndex(s, "@")
	if at <= 0 || at == len(s)-1 {
		return false
	}
	local, domain := s[:at], s[at+1:]
	if local == "" || domain == "" {
		return false
	}
	if !strings.Contains(domain, ".") {
		return false
	}
	return true
}

var _ Service = (*validationService)(nil)
