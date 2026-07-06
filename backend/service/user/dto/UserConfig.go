package userdto

// UserConfig holds per-user application preferences. It is persisted as a
// JSONB blob in user_preferences so future fields require no schema migration —
// only a new Go field with a json tag. Zero values are the correct defaults
// for every field (e.g. false for booleans) so a missing row is equivalent to
// an all-zero struct.
type UserConfig struct {
	// required:"false" — PATCH /api/me/config is a partial merge; an absent
	// key must leave the field at its Go zero value, matching the pre-Huma
	// handler's plain json.Decode rather than requiring every field present.
	ShowKeyboardShortcuts bool `json:"show_keyboard_shortcuts" required:"false"`
}
