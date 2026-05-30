package usermodel

// UserConfig holds per-user application preferences. It is persisted as a
// JSONB blob so future fields require no schema migration — only a new Go
// field with a json tag. Zero values are the correct defaults for every field
// (e.g. false for booleans) so a missing row is equivalent to an all-zero
// struct.
type UserConfig struct {
	ShowKeyboardShortcuts bool `json:"show_keyboard_shortcuts"`
}
