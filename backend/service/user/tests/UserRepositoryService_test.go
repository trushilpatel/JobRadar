package tests

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	rolemodel "github.com/trushilpatel/jobradar/service/role/models"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newUserTestDB returns an in-memory sqlite DB with `users` and `user_roles`
// migrated. SQLite supports the GORM features the repo uses (OnConflict /
// DoNothing → INSERT OR IGNORE), so the happy paths can be exercised
// directly. The `citext` type collapses to TEXT on sqlite — the column still
// works, only case-insensitive matching is lost (which the repo handles via
// strings.ToLower already so behaviour is unchanged).
func newUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Discard,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// AutoMigrate emits Postgres-specific `now()` defaults and a `citext`
	// column type for the email column. Build the schema by hand so it
	// works on sqlite while staying behaviourally identical for the
	// columns the repository touches.
	stmts := []string{
		`CREATE TABLE users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			display_name TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE roles (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE user_roles (
			user_id TEXT NOT NULL,
			role_id TEXT NOT NULL,
			granted_at DATETIME,
			granted_by TEXT,
			PRIMARY KEY (user_id, role_id)
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	// Seed the two roles migration 043 inserts so the provision read-back of
	// the `user` role grant has a parent row if FKs are enabled.
	if err := db.Create(&[]rolemodel.Role{
		{ID: rolemodel.RoleUser, Name: "Regular User"},
		{ID: rolemodel.RoleAdmin, Name: "Administrator"},
	}).Error; err != nil {
		t.Fatalf("seed roles: %v", err)
	}
	return db
}

// closedUserDB returns a DB whose underlying *sql.DB has been closed — every
// query surfaces "sql: database is closed", exercising the repository's
// error-mapping branches.
func closedUserDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newUserTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

func asUserSvcErr(t *testing.T, err error) *domainerrs.ServiceError {
	t.Helper()
	var se *domainerrs.ServiceError
	if !errors.As(err, &se) {
		t.Fatalf("expected *ServiceError, got %T: %v", err, err)
	}
	return se
}

// ---------- Get ----------

func TestUserRepo_Get_HappyPath(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	now := time.Now().UTC()
	if err := db.Create(&usermodel.User{ID: "u1", Email: "a@b.com", Status: usermodel.StatusActive, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := usersvc.NewRepositoryService(db)
	got, err := repo.Get(context.Background(), "u1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.ID != "u1" || got.Email != "a@b.com" {
		t.Errorf("unexpected user: %+v", got)
	}
}

func TestUserRepo_Get_NotFound(t *testing.T) {
	t.Parallel()
	repo := usersvc.NewRepositoryService(newUserTestDB(t))
	_, err := repo.Get(context.Background(), "nope")
	if err == nil {
		t.Fatal("expected NOT_FOUND")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrNotFound.Code {
		t.Errorf("code = %q, want NOT_FOUND", se.Code)
	}
}

func TestUserRepo_Get_DBError(t *testing.T) {
	t.Parallel()
	repo := usersvc.NewRepositoryService(closedUserDB(t))
	_, err := repo.Get(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected DB error")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrInternal.Code {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// ---------- GetByEmail ----------

func TestUserRepo_GetByEmail_HappyPath(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	now := time.Now().UTC()
	if err := db.Create(&usermodel.User{ID: "u1", Email: "a@b.com", Status: usermodel.StatusActive, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := usersvc.NewRepositoryService(db)
	got, err := repo.GetByEmail(context.Background(), "  A@B.com ")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got == nil || got.ID != "u1" {
		t.Errorf("unexpected user: %+v", got)
	}
}

func TestUserRepo_GetByEmail_NotFound(t *testing.T) {
	t.Parallel()
	repo := usersvc.NewRepositoryService(newUserTestDB(t))
	_, err := repo.GetByEmail(context.Background(), "missing@example.com")
	if err == nil {
		t.Fatal("expected NOT_FOUND")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrNotFound.Code {
		t.Errorf("code = %q, want NOT_FOUND", se.Code)
	}
}

func TestUserRepo_GetByEmail_DBError(t *testing.T) {
	t.Parallel()
	repo := usersvc.NewRepositoryService(closedUserDB(t))
	_, err := repo.GetByEmail(context.Background(), "a@b.com")
	if err == nil {
		t.Fatal("expected DB error")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrInternal.Code {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// ---------- Provision ----------

func TestUserRepo_Provision_InsertsNew(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	repo := usersvc.NewRepositoryService(db)

	u, err := repo.Provision(context.Background(), "u1", "  Alice@Example.com  ")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if u == nil || u.ID != "u1" {
		t.Fatalf("unexpected user: %+v", u)
	}
	if u.Email != "alice@example.com" {
		t.Errorf("email = %q, want lowercased+trimmed", u.Email)
	}
	if u.Status != usermodel.StatusActive {
		t.Errorf("status = %q, want active", u.Status)
	}

	// Baseline `user` role row must have been inserted.
	var roleCount int64
	if err := db.Model(&rolemodel.UserRole{}).
		Where("user_id = ? AND role_id = ?", "u1", rolemodel.RoleUser).
		Count(&roleCount).Error; err != nil {
		t.Fatalf("count roles: %v", err)
	}
	if roleCount != 1 {
		t.Errorf("user role row count = %d, want 1", roleCount)
	}
}

func TestUserRepo_Provision_Idempotent_KeepsExistingRow(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	// Pre-seed the user with a known timestamp so we can verify the existing
	// row is preserved on the second call (OnConflict DoNothing).
	orig := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := db.Create(&usermodel.User{
		ID: "u1", Email: "first@example.com", Status: usermodel.StatusActive,
		CreatedAt: orig, UpdatedAt: orig,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := usersvc.NewRepositoryService(db)

	u, err := repo.Provision(context.Background(), "u1", "second@example.com")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if u.Email != "first@example.com" {
		t.Errorf("expected pre-existing email kept; got %q", u.Email)
	}
}

// Provision is called once per first login. Repeat calls for the same id
// must all succeed and converge on a single row. Run sequentially: sqlite's
// `:memory:` mode does not share state across goroutines reliably under
// concurrent writes; the OnConflict-DoNothing path is exercised here by the
// repeated invocation, which is the actual idempotence guarantee Provision
// makes.
func TestUserRepo_Provision_RepeatedSameID(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	repo := usersvc.NewRepositoryService(db)

	const N = 5
	for i := 0; i < N; i++ {
		if _, err := repo.Provision(context.Background(), "u1", "alice@example.com"); err != nil {
			t.Errorf("provision[%d]: %v", i, err)
		}
	}
	var count int64
	if err := db.Model(&usermodel.User{}).Where("id = ?", "u1").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("user row count = %d, want 1 (idempotent upsert)", count)
	}
	// Same for the role grant — repeated provisions must not duplicate.
	if err := db.Model(&rolemodel.UserRole{}).
		Where("user_id = ? AND role_id = ?", "u1", rolemodel.RoleUser).
		Count(&count).Error; err != nil {
		t.Fatalf("count roles: %v", err)
	}
	if count != 1 {
		t.Errorf("role row count = %d, want 1 (idempotent grant)", count)
	}
}

func TestUserRepo_Provision_DBError(t *testing.T) {
	t.Parallel()
	repo := usersvc.NewRepositoryService(closedUserDB(t))
	_, err := repo.Provision(context.Background(), "u1", "a@b.com")
	if err == nil {
		t.Fatal("expected DB error")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrInternal.Code {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// ---------- UpdateProfile ----------

func TestUserRepo_UpdateProfile_SetsName(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	now := time.Now().UTC()
	if err := db.Create(&usermodel.User{ID: "u1", Email: "a@b.com", Status: usermodel.StatusActive, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := usersvc.NewRepositoryService(db)

	if err := repo.UpdateProfile(context.Background(), "u1", "  Alice  "); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	var u usermodel.User
	if err := db.First(&u, "id = ?", "u1").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if u.DisplayName == nil || *u.DisplayName != "Alice" {
		got := "<nil>"
		if u.DisplayName != nil {
			got = *u.DisplayName
		}
		t.Errorf("display_name = %q, want trimmed 'Alice'", got)
	}
}

// Empty / whitespace-only display name must clear the column to NULL so the
// caller can revert to "no display name" without a separate API.
func TestUserRepo_UpdateProfile_EmptyClearsToNull(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	now := time.Now().UTC()
	prev := "Alice"
	if err := db.Create(&usermodel.User{
		ID: "u1", Email: "a@b.com", Status: usermodel.StatusActive,
		DisplayName: &prev, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := usersvc.NewRepositoryService(db)

	if err := repo.UpdateProfile(context.Background(), "u1", "   "); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	var u usermodel.User
	if err := db.First(&u, "id = ?", "u1").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if u.DisplayName != nil {
		t.Errorf("display_name = %q, want NULL", *u.DisplayName)
	}
}

func TestUserRepo_UpdateProfile_NotFound(t *testing.T) {
	t.Parallel()
	repo := usersvc.NewRepositoryService(newUserTestDB(t))
	err := repo.UpdateProfile(context.Background(), "ghost", "Alice")
	if err == nil {
		t.Fatal("expected NOT_FOUND")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrNotFound.Code {
		t.Errorf("code = %q, want NOT_FOUND", se.Code)
	}
}

func TestUserRepo_UpdateProfile_DBError(t *testing.T) {
	t.Parallel()
	repo := usersvc.NewRepositoryService(closedUserDB(t))
	err := repo.UpdateProfile(context.Background(), "u1", "Alice")
	if err == nil {
		t.Fatal("expected DB error")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrInternal.Code {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// ---------- Delete ----------

func TestUserRepo_Delete_HappyPath(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	now := time.Now().UTC()
	if err := db.Create(&usermodel.User{ID: "u1", Email: "a@b.com", Status: usermodel.StatusActive, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := usersvc.NewRepositoryService(db)
	if err := repo.Delete(context.Background(), "u1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var count int64
	if err := db.Model(&usermodel.User{}).Where("id = ?", "u1").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("row count = %d, want 0 (deleted)", count)
	}
}

func TestUserRepo_Delete_NotFound(t *testing.T) {
	t.Parallel()
	repo := usersvc.NewRepositoryService(newUserTestDB(t))
	err := repo.Delete(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected NOT_FOUND")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrNotFound.Code {
		t.Errorf("code = %q, want NOT_FOUND", se.Code)
	}
}

func TestUserRepo_Delete_DBError(t *testing.T) {
	t.Parallel()
	repo := usersvc.NewRepositoryService(closedUserDB(t))
	err := repo.Delete(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected DB error")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrInternal.Code {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// Sanity: confirm GORM's GetByEmail uses the lowercased+trimmed form so
// callers passing weird casing still match a seeded row.
func TestUserRepo_GetByEmail_LowercasesInput(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	now := time.Now().UTC()
	if err := db.Create(&usermodel.User{ID: "u1", Email: "alice@example.com", Status: usermodel.StatusActive, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := usersvc.NewRepositoryService(db)

	for _, q := range []string{"ALICE@example.com", "  alice@example.com  ", "Alice@Example.com"} {
		got, err := repo.GetByEmail(context.Background(), q)
		if err != nil {
			t.Errorf("GetByEmail(%q): %v", q, err)
			continue
		}
		if got == nil || got.ID != "u1" {
			t.Errorf("GetByEmail(%q): unexpected result %+v", q, got)
		}
	}
}

// Sanity: Provision's email normalisation must also match the GetByEmail
// behaviour, so the read-back returns the row written by Provision.
func TestUserRepo_Provision_NormalisesEmail(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	repo := usersvc.NewRepositoryService(db)

	u, err := repo.Provision(context.Background(), "u1", "  BoB@Example.com  ")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if !strings.EqualFold(u.Email, "bob@example.com") {
		t.Errorf("Provision email = %q, want bob@example.com", u.Email)
	}
}
