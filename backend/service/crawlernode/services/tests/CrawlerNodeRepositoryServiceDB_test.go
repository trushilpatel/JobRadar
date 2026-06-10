package tests

import (
	"context"
	stderrors "errors"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	crawlernodemodel "github.com/trushilpatel/jobradar/service/crawlernode/models"
	"github.com/trushilpatel/jobradar/service/crawlernode/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newNodeTestDB returns an in-memory SQLite DB with the crawler_nodes table
// migrated. All crawler_nodes methods use SQL that translates cleanly to
// SQLite (no FOR UPDATE, RETURNING, or Postgres INTERVAL arithmetic).
func newNodeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&crawlernodemodel.CrawlerNode{}); err != nil {
		t.Fatalf("migrate crawler_nodes: %v", err)
	}
	return db
}

func seedNode(t *testing.T, db *gorm.DB, id, name, status string) {
	t.Helper()
	hash := "hash-" + id
	if err := db.Create(&crawlernodemodel.CrawlerNode{
		ID:          id,
		Name:        name,
		TokenPrefix: id[:8],
		TokenHash:   &hash,
		Status:      status,
	}).Error; err != nil {
		t.Fatalf("seed node %s: %v", id, err)
	}
}

// --- Create ---

func TestNodeRepositoryService_Create_InsertsRow(t *testing.T) {
	db := newNodeTestDB(t)
	store := services.NewNodeRepositoryService(db)
	hash := "testhash"
	node := crawlernodemodel.CrawlerNode{
		ID:          "uuid-001",
		Name:        "node-alpha",
		TokenPrefix: "uuid-001",
		TokenHash:   &hash,
		Status:      "active",
	}
	got, err := store.Create(context.Background(), node)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID != "uuid-001" || got.Name != "node-alpha" {
		t.Errorf("Create() = %+v", got)
	}
}

func TestNodeRepositoryService_Create_ConflictOnDuplicateName(t *testing.T) {
	db := newNodeTestDB(t)
	store := services.NewNodeRepositoryService(db)
	hash := "testhash"
	node := crawlernodemodel.CrawlerNode{ID: "uuid-001", Name: "dup-node", TokenPrefix: "aaaa1234", TokenHash: &hash, Status: "active"}
	if _, err := store.Create(context.Background(), node); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	node.ID = "uuid-002"
	_, err := store.Create(context.Background(), node)
	if !stderrors.Is(err, domainerrs.ErrConflict) {
		t.Errorf("Create() duplicate name error = %v, want ErrConflict", err)
	}
}

// --- GetByTokenPrefix ---

func TestNodeRepositoryService_GetByTokenPrefix_ReturnsRow(t *testing.T) {
	db := newNodeTestDB(t)
	seedNode(t, db, "uuid-0001", "node-1", "active")
	store := services.NewNodeRepositoryService(db)
	got, err := store.GetByTokenPrefix(context.Background(), "uuid-000")
	if err != nil {
		t.Fatalf("GetByTokenPrefix() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "uuid-0001" {
		t.Errorf("GetByTokenPrefix() = %v, want 1 row with ID uuid-0001", got)
	}
}

func TestNodeRepositoryService_GetByTokenPrefix_EmptyOnNoMatch(t *testing.T) {
	db := newNodeTestDB(t)
	store := services.NewNodeRepositoryService(db)
	got, err := store.GetByTokenPrefix(context.Background(), "xxxxxxxx")
	if err != nil {
		t.Fatalf("GetByTokenPrefix() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetByTokenPrefix() len = %d, want 0", len(got))
	}
}

// --- UpdateLastSeen ---

func TestNodeRepositoryService_UpdateLastSeen_Success(t *testing.T) {
	db := newNodeTestDB(t)
	seedNode(t, db, "uuid-001", "node-1", "active")
	store := services.NewNodeRepositoryService(db)
	if err := store.UpdateLastSeen(context.Background(), "uuid-001"); err != nil {
		t.Fatalf("UpdateLastSeen() error = %v", err)
	}
	var got crawlernodemodel.CrawlerNode
	db.First(&got, "id = ?", "uuid-001")
	if got.LastSeenAt == nil {
		t.Error("UpdateLastSeen() LastSeenAt is still nil")
	}
}

func TestNodeRepositoryService_UpdateLastSeen_ErrNotFoundOnMissingID(t *testing.T) {
	db := newNodeTestDB(t)
	store := services.NewNodeRepositoryService(db)
	err := store.UpdateLastSeen(context.Background(), "missing-id")
	if !stderrors.Is(err, domainerrs.ErrNotFound) {
		t.Errorf("UpdateLastSeen() error = %v, want ErrNotFound", err)
	}
}

// --- SetStatus ---

func TestNodeRepositoryService_SetStatus_Active(t *testing.T) {
	db := newNodeTestDB(t)
	seedNode(t, db, "uuid-001", "node-1", "disabled")
	store := services.NewNodeRepositoryService(db)
	if err := store.SetStatus(context.Background(), "uuid-001", "active"); err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	var got crawlernodemodel.CrawlerNode
	db.First(&got, "id = ?", "uuid-001")
	if got.Status != "active" {
		t.Errorf("SetStatus() status = %q, want active", got.Status)
	}
}

func TestNodeRepositoryService_SetStatus_ErrNotFoundOnRetiredNode(t *testing.T) {
	db := newNodeTestDB(t)
	seedNode(t, db, "uuid-001", "node-1", "retired")
	store := services.NewNodeRepositoryService(db)
	// WHERE status != 'retired' matches 0 rows → ErrNotFound
	err := store.SetStatus(context.Background(), "uuid-001", "active")
	if !stderrors.Is(err, domainerrs.ErrNotFound) {
		t.Errorf("SetStatus(retired→active) error = %v, want ErrNotFound", err)
	}
}

func TestNodeRepositoryService_SetStatus_ErrNotFoundOnMissingID(t *testing.T) {
	db := newNodeTestDB(t)
	store := services.NewNodeRepositoryService(db)
	err := store.SetStatus(context.Background(), "missing-id", "active")
	if !stderrors.Is(err, domainerrs.ErrNotFound) {
		t.Errorf("SetStatus() error = %v, want ErrNotFound", err)
	}
}

// --- List ---

func TestNodeRepositoryService_List_ReturnsAll(t *testing.T) {
	db := newNodeTestDB(t)
	seedNode(t, db, "uuid-001", "node-1", "active")
	seedNode(t, db, "uuid-002", "node-2", "disabled")
	store := services.NewNodeRepositoryService(db)
	got, err := store.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("List() len = %d, want 2", len(got))
	}
}

func TestNodeRepositoryService_List_FiltersOnStatus(t *testing.T) {
	db := newNodeTestDB(t)
	seedNode(t, db, "uuid-001", "node-1", "active")
	seedNode(t, db, "uuid-002", "node-2", "disabled")
	store := services.NewNodeRepositoryService(db)
	got, err := store.List(context.Background(), "active")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].Status != "active" {
		t.Errorf("List(active) = %v, want 1 active row", got)
	}
}

func TestNodeRepositoryService_List_ReturnsEmptyOnNoMatch(t *testing.T) {
	db := newNodeTestDB(t)
	store := services.NewNodeRepositoryService(db)
	got, err := store.List(context.Background(), "active")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List() len = %d, want 0", len(got))
	}
}

// --- Retire ---

func TestNodeRepositoryService_Retire_Success(t *testing.T) {
	db := newNodeTestDB(t)
	seedNode(t, db, "uuid-001", "node-1", "active")
	store := services.NewNodeRepositoryService(db)
	if err := store.Retire(context.Background(), "uuid-001"); err != nil {
		t.Fatalf("Retire() error = %v", err)
	}
	var got crawlernodemodel.CrawlerNode
	db.First(&got, "id = ?", "uuid-001")
	if got.Status != "retired" {
		t.Errorf("Retire() status = %q, want retired", got.Status)
	}
	if got.TokenHash != nil {
		t.Errorf("Retire() TokenHash = %v, want nil (cleared)", got.TokenHash)
	}
}

func TestNodeRepositoryService_Retire_ErrNotFoundOnAlreadyRetired(t *testing.T) {
	db := newNodeTestDB(t)
	seedNode(t, db, "uuid-001", "node-1", "retired")
	store := services.NewNodeRepositoryService(db)
	// Second retire: WHERE status != 'retired' matches 0 rows
	err := store.Retire(context.Background(), "uuid-001")
	if !stderrors.Is(err, domainerrs.ErrNotFound) {
		t.Errorf("Retire() already-retired error = %v, want ErrNotFound", err)
	}
}

func TestNodeRepositoryService_Retire_ErrNotFoundOnMissingID(t *testing.T) {
	db := newNodeTestDB(t)
	store := services.NewNodeRepositoryService(db)
	err := store.Retire(context.Background(), "missing-id")
	if !stderrors.Is(err, domainerrs.ErrNotFound) {
		t.Errorf("Retire() missing error = %v, want ErrNotFound", err)
	}
}

// --- Closed-DB error paths ---

func newClosedNodeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newNodeTestDB(t)
	raw, err := db.DB()
	if err != nil {
		t.Fatalf("raw db: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

func TestNodeRepositoryService_Create_ErrInternalOnDBError(t *testing.T) {
	store := services.NewNodeRepositoryService(newClosedNodeDB(t))
	_, err := store.Create(context.Background(), crawlernodemodel.CrawlerNode{ID: "x", Name: "y"})
	if err == nil {
		t.Fatal("Create() on closed DB expected error, got nil")
	}
}

func TestNodeRepositoryService_GetByTokenPrefix_ErrInternalOnDBError(t *testing.T) {
	store := services.NewNodeRepositoryService(newClosedNodeDB(t))
	_, err := store.GetByTokenPrefix(context.Background(), "abcd1234")
	if err == nil {
		t.Fatal("GetByTokenPrefix() on closed DB expected error, got nil")
	}
}

func TestNodeRepositoryService_UpdateLastSeen_ErrInternalOnDBError(t *testing.T) {
	store := services.NewNodeRepositoryService(newClosedNodeDB(t))
	err := store.UpdateLastSeen(context.Background(), "uuid-001")
	if err == nil {
		t.Fatal("UpdateLastSeen() on closed DB expected error, got nil")
	}
}

func TestNodeRepositoryService_SetStatus_ErrInternalOnDBError(t *testing.T) {
	store := services.NewNodeRepositoryService(newClosedNodeDB(t))
	err := store.SetStatus(context.Background(), "uuid-001", "active")
	if err == nil {
		t.Fatal("SetStatus() on closed DB expected error, got nil")
	}
}

func TestNodeRepositoryService_List_ErrInternalOnDBError(t *testing.T) {
	store := services.NewNodeRepositoryService(newClosedNodeDB(t))
	_, err := store.List(context.Background(), "")
	if err == nil {
		t.Fatal("List() on closed DB expected error, got nil")
	}
}

func TestNodeRepositoryService_Retire_ErrInternalOnDBError(t *testing.T) {
	store := services.NewNodeRepositoryService(newClosedNodeDB(t))
	err := store.Retire(context.Background(), "uuid-001")
	if err == nil {
		t.Fatal("Retire() on closed DB expected error, got nil")
	}
}

// --- ScanSettingsRepositoryService ---

// newScanSettingsDB returns an in-memory SQLite DB with a minimal
// company_scan_settings table for testing the scan settings reader.
func newScanSettingsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite for scan settings: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE company_scan_settings (
			company_id TEXT PRIMARY KEY,
			adapter_config TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create company_scan_settings: %v", err)
	}
	return db
}

// GetAdapterConfig returns the adapter config when the row exists.
func TestScanSettingsRepositoryService_GetAdapterConfig_ReturnsConfig(t *testing.T) {
	db := newScanSettingsDB(t)
	cfg := `{"board_token":"tok"}`
	if err := db.Exec("INSERT INTO company_scan_settings (company_id, adapter_config) VALUES (?, ?)", "co-1", cfg).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	reader := services.NewScanSettingsReader(db)
	got, err := reader.GetAdapterConfig(context.Background(), "co-1")
	if err != nil {
		t.Fatalf("GetAdapterConfig() error = %v", err)
	}
	if got == nil || *got != cfg {
		t.Errorf("GetAdapterConfig() = %v, want %q", got, cfg)
	}
}

// GetAdapterConfig returns nil, nil when the company has no settings row.
func TestScanSettingsRepositoryService_GetAdapterConfig_ReturnsNilOnMissing(t *testing.T) {
	db := newScanSettingsDB(t)
	reader := services.NewScanSettingsReader(db)
	got, err := reader.GetAdapterConfig(context.Background(), "no-such-company")
	if err != nil {
		t.Fatalf("GetAdapterConfig() missing error = %v", err)
	}
	if got != nil {
		t.Errorf("GetAdapterConfig() = %v, want nil", got)
	}
}

// GetAdapterConfig wraps DB errors as ErrInternal.
func TestScanSettingsRepositoryService_GetAdapterConfig_ErrInternalOnDBError(t *testing.T) {
	db := newScanSettingsDB(t)
	raw, _ := db.DB()
	_ = raw.Close()
	reader := services.NewScanSettingsReader(db)
	_, err := reader.GetAdapterConfig(context.Background(), "co-1")
	if err == nil {
		t.Fatal("GetAdapterConfig() on closed DB expected error, got nil")
	}
}
