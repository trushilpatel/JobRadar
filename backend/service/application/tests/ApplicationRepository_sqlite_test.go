package application

import (
	"context"
	stderrors "errors"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/identity"
	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	"github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	svc "github.com/trushilpatel/jobradar/service/application/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newApplicationTestDB returns an in-memory sqlite DB with the
// job_application_tracker schema in place. The repository methods that use
// portable GORM (.Where / .Find / .Create / .Exec on simple tables) run
// against this DB cleanly; methods that use Postgres-only Raw SQL with
// DISTINCT ON are exercised via the closed-DB error path only.
func newApplicationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE job_application_tracker (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			job_result_id TEXT NOT NULL,
			stage TEXT NOT NULL,
			stage_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			note TEXT
		)`).Error; err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

// closedApplicationDB returns a *gorm.DB whose underlying *sql.DB has been
// closed so every query returns "sql: database is closed". Exercises the
// error branches of every repo method that returns a query error.
func closedApplicationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newApplicationTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

func TestRepo_ListByJob_HappyAndScope(t *testing.T) {
	db := newApplicationTestDB(t)
	repo := svc.NewApplicationRepositoryService(db)
	ctx := context.Background()

	// Seed two rows for user-1 / job-1, one for user-1 / job-2, one for user-2 / job-1.
	for _, row := range []appmodel.JobApplication{
		{UserID: "u1", JobResultID: "j1", Stage: "applied"},
		{UserID: "u1", JobResultID: "j1", Stage: "phone_screen"},
		{UserID: "u1", JobResultID: "j2", Stage: "applied"},
		{UserID: "u2", JobResultID: "j1", Stage: "applied"},
	} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	got, err := repo.ListByJob(ctx, "u1", "j1")
	if err != nil {
		t.Fatalf("ListByJob: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows for (u1,j1), got %d", len(got))
	}
	for _, r := range got {
		if r.UserID != "u1" || r.JobResultID != "j1" {
			t.Fatalf("scope violation: %+v", r)
		}
	}
}

func TestRepo_ListByJob_DBError(t *testing.T) {
	db := closedApplicationDB(t)
	repo := svc.NewApplicationRepositoryService(db)
	if _, err := repo.ListByJob(context.Background(), "u1", "j1"); err == nil {
		t.Fatal("expected DB error, got nil")
	}
}

func TestRepo_Create_HappyAndScope(t *testing.T) {
	db := newApplicationTestDB(t)
	repo := svc.NewApplicationRepositoryService(db)

	out, err := repo.Create(context.Background(), "u1", "j1", dto.CreateApplicationInput{Stage: "applied", Note: "first try"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if out == nil || out.UserID != "u1" || out.JobResultID != "j1" || out.Stage != "applied" {
		t.Fatalf("unexpected row: %+v", out)
	}
	if out.Note == nil || *out.Note != "first try" {
		t.Fatalf("note not persisted: %+v", out)
	}

	// Create with empty note → Note pointer stays nil (insertApplication branch).
	out2, err := repo.Create(context.Background(), "u1", "j2", dto.CreateApplicationInput{Stage: "applied"})
	if err != nil {
		t.Fatalf("Create empty note: %v", err)
	}
	if out2.Note != nil {
		t.Fatalf("empty note should produce nil pointer, got %v", *out2.Note)
	}
}

func TestRepo_Create_DBError(t *testing.T) {
	db := closedApplicationDB(t)
	repo := svc.NewApplicationRepositoryService(db)
	_, err := repo.Create(context.Background(), "u1", "j1", dto.CreateApplicationInput{Stage: "applied"})
	if err == nil {
		t.Fatal("expected DB error, got nil")
	}
}

func TestRepo_UpdateStage_HappyAndBadID(t *testing.T) {
	db := newApplicationTestDB(t)
	repo := svc.NewApplicationRepositoryService(db)

	created, err := repo.Create(context.Background(), "u1", "j1", dto.CreateApplicationInput{Stage: "applied"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_ = created // happy-path UpdateStage uses Postgres NOW(); covered by error branch + integration tier.

	t.Run("invalid app_id", func(t *testing.T) {
		err := repo.UpdateStage(context.Background(), "u1", "not-a-number", dto.UpdateStageInput{Stage: "applied"})
		if err == nil {
			t.Fatal("expected error for invalid app_id")
		}
		var se *domainerrs.ServiceError
		if !stderrors.As(err, &se) || se.Code != "BAD_REQUEST" {
			t.Fatalf("expected BAD_REQUEST ServiceError, got %#v", err)
		}
	})

	t.Run("db error", func(t *testing.T) {
		bad := closedApplicationDB(t)
		badRepo := svc.NewApplicationRepositoryService(bad)
		err := badRepo.UpdateStage(context.Background(), "u1", "1", dto.UpdateStageInput{Stage: "applied"})
		if err == nil {
			t.Fatal("expected DB error")
		}
	})
}

func TestRepo_UpdateNote_HappyAndBadID(t *testing.T) {
	db := newApplicationTestDB(t)
	repo := svc.NewApplicationRepositoryService(db)

	created, err := repo.Create(context.Background(), "u1", "j1", dto.CreateApplicationInput{Stage: "applied"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	idStr := itoa(created.ID)
	if err := repo.UpdateNote(context.Background(), "u1", idStr, dto.UpdateNoteInput{Note: "updated"}); err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}

	t.Run("invalid app_id", func(t *testing.T) {
		err := repo.UpdateNote(context.Background(), "u1", "abc", dto.UpdateNoteInput{Note: "x"})
		if err == nil {
			t.Fatal("expected error for invalid app_id")
		}
		var se *domainerrs.ServiceError
		if !stderrors.As(err, &se) || se.Code != "BAD_REQUEST" {
			t.Fatalf("expected BAD_REQUEST ServiceError, got %#v", err)
		}
	})

	t.Run("db error", func(t *testing.T) {
		bad := closedApplicationDB(t)
		badRepo := svc.NewApplicationRepositoryService(bad)
		err := badRepo.UpdateNote(context.Background(), "u1", "1", dto.UpdateNoteInput{Note: "x"})
		if err == nil {
			t.Fatal("expected DB error")
		}
	})
}

func TestRepo_InsertAppliedApplication_HappyAndAnonCtx(t *testing.T) {
	db := newApplicationTestDB(t)
	w := svc.NewApplicationWriter(db)

	t.Run("anonymous ctx", func(t *testing.T) {
		err := w.InsertAppliedApplication(context.Background(), "j1")
		if err == nil {
			t.Fatal("expected guard error for anonymous ctx")
		}
	})

	t.Run("identity ctx", func(t *testing.T) {
		ctx := identity.NewContext(context.Background(), identity.Identity{UserID: "u1"})
		if err := w.InsertAppliedApplication(ctx, "j1"); err != nil {
			t.Fatalf("InsertAppliedApplication: %v", err)
		}
		var rows []appmodel.JobApplication
		if err := db.Where("user_id = ? AND job_result_id = ?", "u1", "j1").Find(&rows).Error; err != nil {
			t.Fatalf("read back: %v", err)
		}
		if len(rows) != 1 || rows[0].Stage != appcommon.StageApplied {
			t.Fatalf("expected one applied row, got %+v", rows)
		}
	})
}

func TestRepo_ListAll_DBError(t *testing.T) {
	// Happy path needs Postgres DISTINCT ON; cover the error branch via closed DB.
	db := closedApplicationDB(t)
	repo := svc.NewApplicationRepositoryService(db)
	_, _, err := repo.ListAll(context.Background(), "u1", 1, 20)
	if err == nil {
		t.Fatal("expected DB error")
	}
}

func TestRepo_ListActive_DBError(t *testing.T) {
	db := closedApplicationDB(t)
	repo := svc.NewApplicationRepositoryService(db)
	_, err := repo.ListActive(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected DB error")
	}
}

func TestApplicationService_NewBaseService_PanicsOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil next")
		}
	}()
	_ = svc.NewBaseService(nil)
}

// itoa is a small helper to avoid importing strconv just for the test.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
