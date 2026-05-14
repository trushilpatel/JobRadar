package tests

import (
	"context"
	stderr "errors"
	"strings"
	"testing"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	auditmodel "github.com/trushilpatel/jobradar/service/audit/models"
	auditsvc "github.com/trushilpatel/jobradar/service/audit/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newAuditDB returns an in-memory sqlite DB pre-populated with a schema that
// mirrors `admin_audit_log` closely enough for the repository under test. The
// production schema uses Postgres-specific features (UUID + JSONB +
// gen_random_uuid()) that have no sqlite equivalent; the tests supply IDs
// explicitly and store payloads as plain BLOB. The schema also creates a
// minimal `users` table so the LEFT JOIN in List() can resolve actor emails.
//
// The connection pool is capped at one connection so a single in-memory
// database serves every query inside one test — `:memory:` databases are
// per-connection by default and would otherwise vanish between calls.
func newAuditDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Exec(`
		CREATE TABLE admin_audit_log (
			id            TEXT PRIMARY KEY,
			actor_user_id TEXT,
			action        TEXT NOT NULL,
			target_type   TEXT,
			target_id     TEXT,
			payload       BLOB,
			created_at    DATETIME NOT NULL
		)`).Error; err != nil {
		t.Fatalf("create admin_audit_log: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE users (
			id    TEXT PRIMARY KEY,
			email TEXT
		)`).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	return db
}

// seedEntry inserts a row through GORM so the test exercises the same write
// path the repository uses. ID is required because sqlite has no
// gen_random_uuid() default.
func seedEntry(t *testing.T, db *gorm.DB, id, action string, actor *string, createdAt time.Time) {
	t.Helper()
	row := auditmodel.Entry{
		ID:          id,
		ActorUserID: actor,
		Action:      action,
		CreatedAt:   createdAt,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func seedUser(t *testing.T, db *gorm.DB, id, email string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO users (id, email) VALUES (?, ?)`, id, email).Error; err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func ptr(s string) *string { return &s }

// TestAuditRepo_Append_RejectsEmptyAction asserts the only validation rule
// the terminal layer owns. Returning BAD_REQUEST means callers never write
// rows with the most important field missing.
func TestAuditRepo_Append_RejectsEmptyAction(t *testing.T) {
	t.Parallel()
	db := newAuditDB(t)
	repo := auditsvc.NewRepositoryService(db)

	err := repo.Append(context.Background(), auditmodel.Entry{Action: ""})
	if err == nil {
		t.Fatal("expected error for empty action")
	}
	var sErr *apperr.ServiceError
	if !stderr.As(err, &sErr) {
		t.Fatalf("expected *ServiceError, got %T: %v", err, err)
	}
	if sErr.Code != "BAD_REQUEST" {
		t.Errorf("Code = %q, want BAD_REQUEST", sErr.Code)
	}
}

// TestAuditRepo_Append_WritesRow asserts a happy-path insert produces a
// readable row.
func TestAuditRepo_Append_WritesRow(t *testing.T) {
	t.Parallel()
	db := newAuditDB(t)
	repo := auditsvc.NewRepositoryService(db)

	entry := auditmodel.Entry{
		ID:        "row-1",
		Action:    auditmodel.ActionUserRoleGrant,
		CreatedAt: time.Now(),
	}
	if err := repo.Append(context.Background(), entry); err != nil {
		t.Fatalf("Append: %v", err)
	}

	var got auditmodel.Entry
	if err := db.Table("admin_audit_log").Where("id = ?", "row-1").Scan(&got).Error; err != nil {
		t.Fatalf("readback: %v", err)
	}
	if got.Action != auditmodel.ActionUserRoleGrant {
		t.Errorf("Action = %q, want %q", got.Action, auditmodel.ActionUserRoleGrant)
	}
}

// TestAuditRepo_Append_WrapsDBError asserts an underlying DB error is mapped
// to ErrInternal with the original message preserved internally.
func TestAuditRepo_Append_WrapsDBError(t *testing.T) {
	t.Parallel()
	db := newAuditDB(t)
	// Drop the table so Create fails — the only field GORM emits matters
	// here is the wrapped sql error from sqlite.
	if err := db.Exec(`DROP TABLE admin_audit_log`).Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	repo := auditsvc.NewRepositoryService(db)

	err := repo.Append(context.Background(), auditmodel.Entry{ID: "x", Action: "user.delete"})
	if err == nil {
		t.Fatal("expected DB error, got nil")
	}
	var sErr *apperr.ServiceError
	if !stderr.As(err, &sErr) {
		t.Fatalf("err type = %T, want *ServiceError", err)
	}
	if sErr.Code != "INTERNAL" {
		t.Errorf("Code = %q, want INTERNAL", sErr.Code)
	}
	if !strings.Contains(sErr.InternalMessage, "audit repo: append:") {
		t.Errorf("InternalMessage = %q, want it to mention append", sErr.InternalMessage)
	}
}

// TestAuditRepo_List_Empty asserts a zero-row table returns (empty, 0, nil)
// — not a not-found error.
func TestAuditRepo_List_Empty(t *testing.T) {
	t.Parallel()
	db := newAuditDB(t)
	repo := auditsvc.NewRepositoryService(db)

	rows, total, err := repo.List(context.Background(), auditsvc.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %d, want 0", len(rows))
	}
}

// TestAuditRepo_List_DescendingOrder verifies the newest-first contract.
// Audit logs are read from newest to oldest by the admin UI, so order is a
// load-bearing property of the contract.
func TestAuditRepo_List_DescendingOrder(t *testing.T) {
	t.Parallel()
	db := newAuditDB(t)
	repo := auditsvc.NewRepositoryService(db)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	seedEntry(t, db, "a", "one", nil, base)
	seedEntry(t, db, "b", "two", nil, base.Add(1*time.Hour))
	seedEntry(t, db, "c", "three", nil, base.Add(2*time.Hour))

	rows, total, err := repo.List(context.Background(), auditsvc.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}
	if rows[0].ID != "c" || rows[1].ID != "b" || rows[2].ID != "a" {
		t.Errorf("ordering: %v, want c,b,a", []string{rows[0].ID, rows[1].ID, rows[2].ID})
	}
}

// TestAuditRepo_List_FilterByActor scopes results to a single actor.
func TestAuditRepo_List_FilterByActor(t *testing.T) {
	t.Parallel()
	db := newAuditDB(t)
	repo := auditsvc.NewRepositoryService(db)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	seedUser(t, db, "alice", "alice@example.com")
	seedUser(t, db, "bob", "bob@example.com")
	seedEntry(t, db, "a", "user.delete", ptr("alice"), base)
	seedEntry(t, db, "b", "user.delete", ptr("bob"), base.Add(time.Hour))
	seedEntry(t, db, "c", "user.role.grant", ptr("alice"), base.Add(2*time.Hour))

	actor := "alice"
	rows, total, err := repo.List(context.Background(), auditsvc.ListFilter{ActorUserID: &actor})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2 (alice has 2 rows)", total)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	for _, r := range rows {
		if r.ActorUserID == nil || *r.ActorUserID != "alice" {
			t.Errorf("row %s actor = %v, want alice", r.ID, r.ActorUserID)
		}
		if r.ActorEmail == nil || *r.ActorEmail != "alice@example.com" {
			t.Errorf("row %s actor_email = %v, want alice@example.com", r.ID, r.ActorEmail)
		}
	}
}

// TestAuditRepo_List_FilterByAction scopes results to a single action label.
func TestAuditRepo_List_FilterByAction(t *testing.T) {
	t.Parallel()
	db := newAuditDB(t)
	repo := auditsvc.NewRepositoryService(db)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	seedEntry(t, db, "a", "user.delete", nil, base)
	seedEntry(t, db, "b", "user.role.grant", nil, base.Add(time.Hour))
	seedEntry(t, db, "c", "user.delete", nil, base.Add(2*time.Hour))

	action := "user.delete"
	rows, total, err := repo.List(context.Background(), auditsvc.ListFilter{Action: &action})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	for _, r := range rows {
		if r.Action != "user.delete" {
			t.Errorf("Action = %q, want user.delete", r.Action)
		}
	}
}

// TestAuditRepo_List_LimitDefaultsApplied asserts the contract that
// Limit ≤ 0 ⇒ 50 and Limit > 200 ⇒ 50. Total reports the unpaged count.
func TestAuditRepo_List_LimitDefaultsApplied(t *testing.T) {
	t.Parallel()
	db := newAuditDB(t)
	repo := auditsvc.NewRepositoryService(db)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 60; i++ {
		seedEntry(t, db, "row-"+itoa(i), "user.delete", nil, base.Add(time.Duration(i)*time.Minute))
	}

	cases := []struct {
		name  string
		limit int
	}{
		{"zero", 0},
		{"negative", -5},
		{"too-large", 999},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rows, total, err := repo.List(context.Background(), auditsvc.ListFilter{Limit: tc.limit})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if total != 60 {
				t.Errorf("total = %d, want 60 (unpaged count)", total)
			}
			if len(rows) != 50 {
				t.Errorf("len(rows) = %d, want 50 (default limit)", len(rows))
			}
		})
	}
}

// TestAuditRepo_List_LimitHonoured asserts a valid limit (1..200) is used
// verbatim.
func TestAuditRepo_List_LimitHonoured(t *testing.T) {
	t.Parallel()
	db := newAuditDB(t)
	repo := auditsvc.NewRepositoryService(db)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		seedEntry(t, db, "row-"+itoa(i), "user.delete", nil, base.Add(time.Duration(i)*time.Minute))
	}

	rows, total, err := repo.List(context.Background(), auditsvc.ListFilter{Limit: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 10 {
		t.Errorf("total = %d, want 10", total)
	}
	if len(rows) != 3 {
		t.Errorf("len(rows) = %d, want 3", len(rows))
	}
}

// TestAuditRepo_List_OffsetClampedAndApplied asserts negative offsets are
// treated as 0 and a positive offset skips rows.
func TestAuditRepo_List_OffsetClampedAndApplied(t *testing.T) {
	t.Parallel()
	db := newAuditDB(t)
	repo := auditsvc.NewRepositoryService(db)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		seedEntry(t, db, "row-"+itoa(i), "user.delete", nil, base.Add(time.Duration(i)*time.Minute))
	}

	t.Run("negative-offset", func(t *testing.T) {
		t.Parallel()
		rows, _, err := repo.List(context.Background(), auditsvc.ListFilter{Limit: 2, Offset: -3})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(rows) != 2 {
			t.Errorf("len(rows) = %d, want 2 (clamped offset)", len(rows))
		}
	})

	t.Run("positive-offset", func(t *testing.T) {
		t.Parallel()
		// Rows are returned newest-first. With createdAt 0..4, ID row-4
		// is newest. Offset=2, Limit=2 should yield row-2 and row-1.
		rows, _, err := repo.List(context.Background(), auditsvc.ListFilter{Limit: 2, Offset: 2})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("len(rows) = %d, want 2", len(rows))
		}
		if rows[0].ID != "row-2" || rows[1].ID != "row-1" {
			t.Errorf("ids = [%s, %s], want [row-2, row-1]", rows[0].ID, rows[1].ID)
		}
	})
}

// TestAuditRepo_List_CountErrorWrapped asserts a count failure is wrapped
// as ErrInternal so handlers can render a 500.
func TestAuditRepo_List_CountErrorWrapped(t *testing.T) {
	t.Parallel()
	db := newAuditDB(t)
	if err := db.Exec(`DROP TABLE admin_audit_log`).Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	repo := auditsvc.NewRepositoryService(db)

	_, _, err := repo.List(context.Background(), auditsvc.ListFilter{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var sErr *apperr.ServiceError
	if !stderr.As(err, &sErr) {
		t.Fatalf("err type = %T, want *ServiceError", err)
	}
	if sErr.Code != "INTERNAL" {
		t.Errorf("Code = %q, want INTERNAL", sErr.Code)
	}
	if !strings.Contains(sErr.InternalMessage, "audit repo: ") {
		t.Errorf("InternalMessage = %q, want it to mention audit repo", sErr.InternalMessage)
	}
}

// TestAuditRepo_List_ScanErrorWrapped asserts a Scan failure (table dropped
// AFTER Count succeeded) is wrapped as ErrInternal. We simulate it by
// dropping the JOIN target so the SELECT-with-JOIN fails — Count() on the
// base table still succeeds, exercising the scan-error branch.
func TestAuditRepo_List_ScanErrorWrapped(t *testing.T) {
	t.Parallel()
	db := newAuditDB(t)
	if err := db.Exec(`DROP TABLE users`).Error; err != nil {
		t.Fatalf("drop users: %v", err)
	}
	// Seed a row so Count > 0 and the scan path is reached.
	seedEntry(t, db, "row-1", "user.delete", nil, time.Now())
	repo := auditsvc.NewRepositoryService(db)

	_, _, err := repo.List(context.Background(), auditsvc.ListFilter{})
	if err == nil {
		t.Fatal("expected scan error, got nil")
	}
	var sErr *apperr.ServiceError
	if !stderr.As(err, &sErr) {
		t.Fatalf("err type = %T, want *ServiceError", err)
	}
	if sErr.Code != "INTERNAL" {
		t.Errorf("Code = %q, want INTERNAL", sErr.Code)
	}
	if !strings.Contains(sErr.InternalMessage, "audit repo: list:") {
		t.Errorf("InternalMessage = %q, want it to mention list", sErr.InternalMessage)
	}
}

// itoa is a tiny helper that avoids dragging strconv into a list of
// seed-only call sites.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
