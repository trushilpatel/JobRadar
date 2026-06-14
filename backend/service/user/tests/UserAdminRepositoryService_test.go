package tests

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	usermodel "github.com/trushilpatel/jobradar/service/user/models"
	usersvc "github.com/trushilpatel/jobradar/service/user/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// adminSQLCapture is a GORM logger that records every emitted SQL statement,
// used for asserting the shape of queries that cannot execute on sqlite
// (notably the ILIKE branch of List filter.Email).
type adminSQLCapture struct {
	gormlogger.Interface
	mu  sync.Mutex
	sql []string
}

func (c *adminSQLCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sql = append(c.sql, sql)
}

func (c *adminSQLCapture) LogMode(gormlogger.LogLevel) gormlogger.Interface { return c }
func (c *adminSQLCapture) Info(context.Context, string, ...any)             {}
func (c *adminSQLCapture) Warn(context.Context, string, ...any)             {}
func (c *adminSQLCapture) Error(context.Context, string, ...any)            {}
func (c *adminSQLCapture) statements() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.sql))
	copy(out, c.sql)
	return out
}

func newAdminDryRunDB(t *testing.T) (*gorm.DB, *adminSQLCapture) {
	t.Helper()
	cap := &adminSQLCapture{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DryRun: true, Logger: cap})
	if err != nil {
		t.Fatalf("open dryrun: %v", err)
	}
	return db, cap
}

// ---------- List ----------

func TestAdminRepo_List_HappyPath(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	now := time.Now().UTC()
	seed := []usermodel.User{
		{ID: "u1", Email: "alpha@example.com", Status: usermodel.StatusActive, CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: "u2", Email: "beta@example.com", Status: usermodel.StatusActive, CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now.Add(-1 * time.Hour)},
		{ID: "u3", Email: "gamma@example.com", Status: usermodel.StatusSuspended, CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatalf("seed users: %v", err)
	}
	// Roles are no longer stored in DB (migration 081 dropped roles/user_roles).
	// List always returns Roles: []string{} for all users.

	repo := usersvc.NewAdminRepositoryService(db)
	res, err := repo.List(context.Background(), usersvc.AdminUserFilter{}, 1, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if res.Total != 3 {
		t.Errorf("total = %d, want 3", res.Total)
	}
	if len(res.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(res.Items))
	}
	// Order is created_at DESC → u3, u2, u1.
	if res.Items[0].ID != "u3" || res.Items[1].ID != "u2" || res.Items[2].ID != "u1" {
		t.Errorf("order = [%s,%s,%s], want [u3,u2,u1]",
			res.Items[0].ID, res.Items[1].ID, res.Items[2].ID)
	}
	// All users have empty roles slice (not nil) — roles are in Clerk JWT claims.
	for _, item := range res.Items {
		if item.Roles == nil {
			t.Errorf("%s.Roles is nil; want empty slice for JSON [] output", item.ID)
		}
		if len(item.Roles) != 0 {
			t.Errorf("%s.Roles = %v, want empty (roles now in JWT claims)", item.ID, item.Roles)
		}
	}
}

func TestAdminRepo_List_StatusFilter(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	now := time.Now().UTC()
	if err := db.Create(&[]usermodel.User{
		{ID: "u1", Email: "a@b.com", Status: usermodel.StatusActive, CreatedAt: now, UpdatedAt: now},
		{ID: "u2", Email: "c@b.com", Status: usermodel.StatusSuspended, CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	repo := usersvc.NewAdminRepositoryService(db)
	suspended := usermodel.StatusSuspended
	res, err := repo.List(context.Background(), usersvc.AdminUserFilter{Status: &suspended}, 1, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if res.Total != 1 || len(res.Items) != 1 {
		t.Fatalf("total/len = %d/%d, want 1/1", res.Total, len(res.Items))
	}
	if res.Items[0].ID != "u2" {
		t.Errorf("got id %q, want u2", res.Items[0].ID)
	}
}

// The Email branch of List uses ILIKE — Postgres-only. Drive the code path
// in DryRun mode and confirm the captured SQL has the ILIKE clause with the
// trimmed search term wrapped in % wildcards.
func TestAdminRepo_List_EmailFilter_EmitsILIKE(t *testing.T) {
	t.Parallel()
	db, cap := newAdminDryRunDB(t)
	repo := usersvc.NewAdminRepositoryService(db)
	_, _ = repo.List(context.Background(), usersvc.AdminUserFilter{Email: "  alice  "}, 1, 10)

	captured := cap.statements()
	if len(captured) == 0 {
		t.Fatal("no SQL captured")
	}
	foundILIKE := false
	for _, s := range captured {
		if strings.Contains(s, "ILIKE") && strings.Contains(s, "%alice%") {
			foundILIKE = true
			break
		}
	}
	if !foundILIKE {
		t.Errorf("expected ILIKE with trimmed %%alice%% in captured SQL; got %v", captured)
	}
}

func TestAdminRepo_List_PaginationDefaultsAndClamps(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	repo := usersvc.NewAdminRepositoryService(db)
	now := time.Now().UTC()
	users := make([]usermodel.User, 25)
	for i := 0; i < 25; i++ {
		users[i] = usermodel.User{
			ID:        "u" + string(rune('a'+i)),
			Email:     string(rune('a'+i)) + "@b.com",
			Status:    usermodel.StatusActive,
			CreatedAt: now.Add(time.Duration(i) * time.Second),
			UpdatedAt: now.Add(time.Duration(i) * time.Second),
		}
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	// limit <= 0 → defaults to 20.
	res, err := repo.List(context.Background(), usersvc.AdminUserFilter{}, 0, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(res.Items) != 20 {
		t.Errorf("default limit yielded %d items, want 20", len(res.Items))
	}
	// page < 1 → defaults to 1.
	if res.Total != 25 {
		t.Errorf("total = %d, want 25", res.Total)
	}
	// limit > 100 → clamped to 100 (cannot exceed 25 here, but exercises the
	// branch by setting limit=200 and expecting the test seed cap of 25).
	res2, err := repo.List(context.Background(), usersvc.AdminUserFilter{}, 1, 200)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(res2.Items) != 25 {
		t.Errorf("clamped limit yielded %d items, want 25", len(res2.Items))
	}
}

func TestAdminRepo_List_EmptyResultEmptySlice(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	repo := usersvc.NewAdminRepositoryService(db)
	res, err := repo.List(context.Background(), usersvc.AdminUserFilter{}, 1, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if res.Total != 0 {
		t.Errorf("total = %d, want 0", res.Total)
	}
	if res.Items == nil {
		t.Error("items is nil; want empty slice for JSON [] output")
	}
	if len(res.Items) != 0 {
		t.Errorf("items = %d, want 0", len(res.Items))
	}
}

func TestAdminRepo_List_DBError(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	repo := usersvc.NewAdminRepositoryService(db)
	_, err := repo.List(context.Background(), usersvc.AdminUserFilter{}, 1, 10)
	if err == nil {
		t.Fatal("expected DB error")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrInternal.Code {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// ---------- GetWithRoles ----------

func TestAdminRepo_GetWithRoles_HappyPath(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	now := time.Now().UTC()
	if err := db.Create(&usermodel.User{ID: "u1", Email: "a@b.com", Status: usermodel.StatusActive, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// No role rows to seed — roles are in Clerk JWT claims after migration 081.

	repo := usersvc.NewAdminRepositoryService(db)
	view, err := repo.GetWithRoles(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetWithRoles: %v", err)
	}
	if view.ID != "u1" {
		t.Errorf("id = %q, want u1", view.ID)
	}
	// Roles are always an empty slice — carried in Clerk JWT, not in DB.
	if view.Roles == nil {
		t.Error("roles is nil; want non-nil empty slice for JSON [] output")
	}
	if len(view.Roles) != 0 {
		t.Errorf("roles = %v, want empty (roles now in JWT claims)", view.Roles)
	}
}

func TestAdminRepo_GetWithRoles_NotFound(t *testing.T) {
	t.Parallel()
	repo := usersvc.NewAdminRepositoryService(newUserTestDB(t))
	_, err := repo.GetWithRoles(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected NOT_FOUND")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrNotFound.Code {
		t.Errorf("code = %q, want NOT_FOUND", se.Code)
	}
}

func TestAdminRepo_GetWithRoles_NoRolesReturnsEmptySlice(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	now := time.Now().UTC()
	if err := db.Create(&usermodel.User{ID: "u1", Email: "a@b.com", Status: usermodel.StatusActive, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	repo := usersvc.NewAdminRepositoryService(db)
	view, err := repo.GetWithRoles(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetWithRoles: %v", err)
	}
	if view.Roles == nil {
		t.Error("roles is nil; want non-nil empty slice")
	}
	if len(view.Roles) != 0 {
		t.Errorf("roles = %v, want empty", view.Roles)
	}
}

func TestAdminRepo_GetWithRoles_DBError(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	repo := usersvc.NewAdminRepositoryService(db)
	_, err := repo.GetWithRoles(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected DB error")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrInternal.Code {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// ---------- SetRoles ----------

// SetRoles is a no-op in the Clerk auth model — roles are managed via
// Clerk publicMetadata. Any call must return nil without touching the DB.
func TestAdminRepo_SetRoles_IsNoOp(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	now := time.Now().UTC()
	if err := db.Create(&usermodel.User{ID: "u1", Email: "a@b.com", Status: usermodel.StatusActive, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	repo := usersvc.NewAdminRepositoryService(db)

	// Non-empty role slice — must succeed without error.
	if err := repo.SetRoles(context.Background(), "u1", []string{"admin"}, "actor"); err != nil {
		t.Fatalf("SetRoles with roles: %v", err)
	}
	// Empty/nil role slice — must also succeed.
	if err := repo.SetRoles(context.Background(), "u1", nil, "actor"); err != nil {
		t.Fatalf("SetRoles nil: %v", err)
	}
	// Unknown user — still must succeed (no-op means no DB check).
	if err := repo.SetRoles(context.Background(), "ghost", []string{"user"}, "actor"); err != nil {
		t.Fatalf("SetRoles unknown user: %v", err)
	}
}

// SetRoles with a closed DB must still return nil — the method is a pure
// no-op and must not issue any DB call regardless of the DB state.
func TestAdminRepo_SetRoles_ClosedDB_StillSucceeds(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	repo := usersvc.NewAdminRepositoryService(db)
	// A closed DB would cause any real DB call to fail; nil return proves no
	// DB call is issued.
	if err := repo.SetRoles(context.Background(), "u1", []string{"user"}, "actor"); err != nil {
		t.Fatalf("expected nil from no-op SetRoles on closed DB; got %v", err)
	}
}

// ---------- SetStatus ----------

func TestAdminRepo_SetStatus_HappyPath(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	now := time.Now().UTC()
	if err := db.Create(&usermodel.User{ID: "u1", Email: "a@b.com", Status: usermodel.StatusActive, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := usersvc.NewAdminRepositoryService(db)

	if err := repo.SetStatus(context.Background(), "u1", usermodel.StatusSuspended); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	var u usermodel.User
	if err := db.First(&u, "id = ?", "u1").Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if u.Status != usermodel.StatusSuspended {
		t.Errorf("status = %q, want suspended", u.Status)
	}
}

func TestAdminRepo_SetStatus_NotFound(t *testing.T) {
	t.Parallel()
	repo := usersvc.NewAdminRepositoryService(newUserTestDB(t))
	err := repo.SetStatus(context.Background(), "ghost", usermodel.StatusSuspended)
	if err == nil {
		t.Fatal("expected NOT_FOUND")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrNotFound.Code {
		t.Errorf("code = %q, want NOT_FOUND", se.Code)
	}
}

func TestAdminRepo_SetStatus_DBError(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	repo := usersvc.NewAdminRepositoryService(db)
	err := repo.SetStatus(context.Background(), "u1", usermodel.StatusSuspended)
	if err == nil {
		t.Fatal("expected DB error")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrInternal.Code {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}

// ---------- AdminDelete ----------

func TestAdminRepo_AdminDelete_HappyPath(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	now := time.Now().UTC()
	if err := db.Create(&usermodel.User{ID: "u1", Email: "a@b.com", Status: usermodel.StatusActive, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := usersvc.NewAdminRepositoryService(db)

	if err := repo.AdminDelete(context.Background(), "u1"); err != nil {
		t.Fatalf("AdminDelete: %v", err)
	}
	var count int64
	if err := db.Model(&usermodel.User{}).Where("id = ?", "u1").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("row count = %d, want 0", count)
	}
}

func TestAdminRepo_AdminDelete_NotFound(t *testing.T) {
	t.Parallel()
	repo := usersvc.NewAdminRepositoryService(newUserTestDB(t))
	err := repo.AdminDelete(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected NOT_FOUND")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrNotFound.Code {
		t.Errorf("code = %q, want NOT_FOUND", se.Code)
	}
}

func TestAdminRepo_AdminDelete_DBError(t *testing.T) {
	t.Parallel()
	db := newUserTestDB(t)
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	repo := usersvc.NewAdminRepositoryService(db)
	err := repo.AdminDelete(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected DB error")
	}
	if se := asUserSvcErr(t, err); se.Code != domainerrs.ErrInternal.Code {
		t.Errorf("code = %q, want INTERNAL", se.Code)
	}
}
