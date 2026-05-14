package tests

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	rolemodel "github.com/trushilpatel/jobradar/service/role/models"
	rolesvc "github.com/trushilpatel/jobradar/service/role/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newRoleTestDB returns an in-memory sqlite DB with the `roles` and
// `user_roles` tables created from the GORM models, plus two seeded role
// rows that mirror migration 043. The schema mirrors Postgres closely enough
// for the repository methods we exercise — GORM emits identical SQL for
// Where, Pluck, Delete, Order, and the OnConflict-DoNothing clause translates
// to INSERT OR IGNORE on sqlite.
func newRoleTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Discard,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Enable foreign-key enforcement so cascade behaviour matches Postgres.
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable fks: %v", err)
	}
	if err := db.AutoMigrate(&rolemodel.Role{}, &rolemodel.UserRole{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Seed the two roles migration 043 inserts.
	rows := []rolemodel.Role{
		{ID: rolemodel.RoleUser, Name: "Regular User"},
		{ID: rolemodel.RoleAdmin, Name: "Administrator"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed roles: %v", err)
	}
	return db
}

// closedDB returns a DB whose underlying *sql.DB has been closed — every
// subsequent operation surfaces "sql: database is closed", exercising the
// repository's error-mapping branches.
func closedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newRoleTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

func asServiceErr(t *testing.T, err error) *domainerrs.ServiceError {
	t.Helper()
	var se *domainerrs.ServiceError
	if !errors.As(err, &se) {
		t.Fatalf("expected *ServiceError, got %T: %v", err, err)
	}
	return se
}

// ---------- ListForUser ----------

func TestRoleRepo_ListForUser_EmptyUserID(t *testing.T) {
	t.Parallel()
	repo := rolesvc.NewRepositoryService(newRoleTestDB(t))
	_, err := repo.ListForUser(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
	if se := asServiceErr(t, err); se.Code != domainerrs.ErrBadRequest.Code {
		t.Errorf("code = %q, want BAD_REQUEST", se.Code)
	}
}

func TestRoleRepo_ListForUser_NoRoles(t *testing.T) {
	t.Parallel()
	repo := rolesvc.NewRepositoryService(newRoleTestDB(t))
	ids, err := repo.ListForUser(context.Background(), "user-with-no-grants")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("ids = %v, want empty", ids)
	}
}

func TestRoleRepo_ListForUser_MultipleRoles(t *testing.T) {
	t.Parallel()
	db := newRoleTestDB(t)
	repo := rolesvc.NewRepositoryService(db)
	ctx := context.Background()

	if err := repo.Grant(ctx, "u1", rolemodel.RoleUser, nil); err != nil {
		t.Fatalf("grant user: %v", err)
	}
	if err := repo.Grant(ctx, "u1", rolemodel.RoleAdmin, nil); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	// Grant for another user — must not bleed into u1's list.
	if err := repo.Grant(ctx, "u2", rolemodel.RoleAdmin, nil); err != nil {
		t.Fatalf("grant u2 admin: %v", err)
	}

	ids, err := repo.ListForUser(ctx, "u1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len = %d, want 2: %v", len(ids), ids)
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if !got[rolemodel.RoleUser] || !got[rolemodel.RoleAdmin] {
		t.Errorf("ids = %v, want user+admin", ids)
	}
}

func TestRoleRepo_ListForUser_DBError(t *testing.T) {
	t.Parallel()
	repo := rolesvc.NewRepositoryService(closedDB(t))
	_, err := repo.ListForUser(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected DB error")
	}
	if se := asServiceErr(t, err); se.Code != domainerrs.ErrInternal.Code {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// ---------- Grant ----------

func TestRoleRepo_Grant_EmptyInputs(t *testing.T) {
	t.Parallel()
	repo := rolesvc.NewRepositoryService(newRoleTestDB(t))
	cases := []struct {
		name, userID, roleID string
	}{
		{"empty user id", "", rolemodel.RoleUser},
		{"empty role id", "u1", ""},
		{"both empty", "", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := repo.Grant(context.Background(), tc.userID, tc.roleID, nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if se := asServiceErr(t, err); se.Code != domainerrs.ErrBadRequest.Code {
				t.Errorf("code = %q, want BAD_REQUEST", se.Code)
			}
		})
	}
}

func TestRoleRepo_Grant_HappyPath(t *testing.T) {
	t.Parallel()
	db := newRoleTestDB(t)
	repo := rolesvc.NewRepositoryService(db)
	if err := repo.Grant(context.Background(), "u1", rolemodel.RoleAdmin, nil); err != nil {
		t.Fatalf("grant: %v", err)
	}

	var count int64
	if err := db.Model(&rolemodel.UserRole{}).
		Where("user_id = ? AND role_id = ?", "u1", rolemodel.RoleAdmin).
		Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("row count = %d, want 1", count)
	}

	// Also assert granted_at was populated (UTC, non-zero) and granted_by is NULL.
	var row rolemodel.UserRole
	if err := db.Where("user_id = ? AND role_id = ?", "u1", rolemodel.RoleAdmin).First(&row).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if row.GrantedAt.IsZero() {
		t.Error("granted_at is zero")
	}
	if row.GrantedBy != nil {
		t.Errorf("granted_by = %v, want nil", *row.GrantedBy)
	}
}

// Idempotent grants — repeat calls must not error and must not insert a duplicate.
// Mirrors the auth-middleware flow that calls Grant on every login.
func TestRoleRepo_Grant_Idempotent(t *testing.T) {
	t.Parallel()
	db := newRoleTestDB(t)
	repo := rolesvc.NewRepositoryService(db)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := repo.Grant(ctx, "u1", rolemodel.RoleUser, nil); err != nil {
			t.Fatalf("grant #%d: %v", i, err)
		}
	}

	var count int64
	if err := db.Model(&rolemodel.UserRole{}).
		Where("user_id = ? AND role_id = ?", "u1", rolemodel.RoleUser).
		Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("row count = %d after 3 grants, want 1 (idempotent OnConflict DoNothing)", count)
	}
}

func TestRoleRepo_Grant_WithGrantedBy(t *testing.T) {
	t.Parallel()
	db := newRoleTestDB(t)
	repo := rolesvc.NewRepositoryService(db)

	grantor := "admin-1"
	if err := repo.Grant(context.Background(), "u1", rolemodel.RoleAdmin, &grantor); err != nil {
		t.Fatalf("grant: %v", err)
	}

	var row rolemodel.UserRole
	if err := db.Where("user_id = ? AND role_id = ?", "u1", rolemodel.RoleAdmin).First(&row).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if row.GrantedBy == nil {
		t.Fatal("granted_by is nil; want admin-1")
	}
	if *row.GrantedBy != grantor {
		t.Errorf("granted_by = %q, want %q", *row.GrantedBy, grantor)
	}
}

func TestRoleRepo_Grant_DBError(t *testing.T) {
	t.Parallel()
	repo := rolesvc.NewRepositoryService(closedDB(t))
	err := repo.Grant(context.Background(), "u1", rolemodel.RoleUser, nil)
	if err == nil {
		t.Fatal("expected DB error")
	}
	if se := asServiceErr(t, err); se.Code != domainerrs.ErrInternal.Code {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// ---------- Revoke ----------

func TestRoleRepo_Revoke_EmptyInputs(t *testing.T) {
	t.Parallel()
	repo := rolesvc.NewRepositoryService(newRoleTestDB(t))
	cases := []struct {
		name, userID, roleID string
	}{
		{"empty user id", "", rolemodel.RoleUser},
		{"empty role id", "u1", ""},
		{"both empty", "", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := repo.Revoke(context.Background(), tc.userID, tc.roleID)
			if err == nil {
				t.Fatal("expected error")
			}
			if se := asServiceErr(t, err); se.Code != domainerrs.ErrBadRequest.Code {
				t.Errorf("code = %q, want BAD_REQUEST", se.Code)
			}
		})
	}
}

func TestRoleRepo_Revoke_HappyPath(t *testing.T) {
	t.Parallel()
	db := newRoleTestDB(t)
	repo := rolesvc.NewRepositoryService(db)
	ctx := context.Background()
	if err := repo.Grant(ctx, "u1", rolemodel.RoleAdmin, nil); err != nil {
		t.Fatalf("setup grant: %v", err)
	}
	if err := repo.Revoke(ctx, "u1", rolemodel.RoleAdmin); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	var count int64
	if err := db.Model(&rolemodel.UserRole{}).
		Where("user_id = ? AND role_id = ?", "u1", rolemodel.RoleAdmin).
		Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("row count = %d, want 0 (revoked)", count)
	}
}

// Revoking a row that does not exist must surface NOT_FOUND so callers can
// distinguish "this grant never existed" from a true DB failure.
func TestRoleRepo_Revoke_NotFound(t *testing.T) {
	t.Parallel()
	repo := rolesvc.NewRepositoryService(newRoleTestDB(t))
	err := repo.Revoke(context.Background(), "u-nonexistent", rolemodel.RoleAdmin)
	if err == nil {
		t.Fatal("expected NotFound error")
	}
	if se := asServiceErr(t, err); se.Code != domainerrs.ErrNotFound.Code {
		t.Errorf("code = %q, want NOT_FOUND", se.Code)
	}
}

// Phase 1 has no business-level guard against revoking the only admin. This
// test pins that behaviour: revoking the last admin succeeds at the
// repository layer. If the team adds such a guard later, this test must be
// updated alongside the new check.
func TestRoleRepo_Revoke_LastAdmin_NoGuard(t *testing.T) {
	t.Parallel()
	db := newRoleTestDB(t)
	repo := rolesvc.NewRepositoryService(db)
	ctx := context.Background()

	if err := repo.Grant(ctx, "only-admin", rolemodel.RoleAdmin, nil); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := repo.Revoke(ctx, "only-admin", rolemodel.RoleAdmin); err != nil {
		t.Fatalf("revoke last admin: %v", err)
	}
	var count int64
	if err := db.Model(&rolemodel.UserRole{}).
		Where("role_id = ?", rolemodel.RoleAdmin).
		Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("admin row count = %d, want 0 (no last-admin guard in Phase 1)", count)
	}
}

func TestRoleRepo_Revoke_DBError(t *testing.T) {
	t.Parallel()
	repo := rolesvc.NewRepositoryService(closedDB(t))
	err := repo.Revoke(context.Background(), "u1", rolemodel.RoleAdmin)
	if err == nil {
		t.Fatal("expected DB error")
	}
	if se := asServiceErr(t, err); se.Code != domainerrs.ErrInternal.Code {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// ---------- ListAll ----------

func TestRoleRepo_ListAll_HappyPath(t *testing.T) {
	t.Parallel()
	repo := rolesvc.NewRepositoryService(newRoleTestDB(t))
	rows, err := repo.ListAll(context.Background())
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len = %d, want 2 (seeded roles)", len(rows))
	}
	// Rows are ordered by id ascending — `admin` < `user` lexically.
	if rows[0].ID != rolemodel.RoleAdmin || rows[1].ID != rolemodel.RoleUser {
		t.Errorf("rows = [%s,%s], want [admin,user] (ordered by id)", rows[0].ID, rows[1].ID)
	}
}

func TestRoleRepo_ListAll_Empty(t *testing.T) {
	t.Parallel()
	db := newRoleTestDB(t)
	if err := db.Exec("DELETE FROM roles").Error; err != nil {
		t.Fatalf("delete: %v", err)
	}
	repo := rolesvc.NewRepositoryService(db)
	rows, err := repo.ListAll(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %v, want empty", rows)
	}
}

func TestRoleRepo_ListAll_DBError(t *testing.T) {
	t.Parallel()
	repo := rolesvc.NewRepositoryService(closedDB(t))
	_, err := repo.ListAll(context.Background())
	if err == nil {
		t.Fatal("expected DB error")
	}
	if se := asServiceErr(t, err); se.Code != domainerrs.ErrInternal.Code {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// ---------- Sanity: raw *sql.DB error propagation ----------

// Belt-and-braces check that the closedDB helper produces a real failure
// signature — guards against a future GORM upgrade that swallows the close
// error and reports zero rows.
func TestRoleRepo_ClosedDBProducesError(t *testing.T) {
	t.Parallel()
	db := closedDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	if pingErr := sqlDB.Ping(); !errors.Is(pingErr, sql.ErrConnDone) && pingErr == nil {
		t.Fatalf("ping err = %v, want a non-nil error", pingErr)
	}
}
