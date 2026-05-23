package dashboard_test

import (
	"context"
	"errors"
	"testing"
	"time"

	apperr "github.com/trushilpatel/jobradar/common/errors"
	appcommon "github.com/trushilpatel/jobradar/service/application/common"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
	appsvc "github.com/trushilpatel/jobradar/service/application/services"
	"github.com/trushilpatel/jobradar/service/dashboard/dto"
	"github.com/trushilpatel/jobradar/service/dashboard/services"
	insightscommon "github.com/trushilpatel/jobradar/service/insights/common"
)

// fakeUserDashboardStore is a test double for services.UserDashboardStore.
// Each method has an injectable error so error branches in the impl service
// are exercised independently.
type fakeUserDashboardStore struct {
	progress    dto.ProgressStats
	progressErr error

	market    dto.MarketStats
	marketErr error

	weekly    []dto.WeeklyCount
	weeklyErr error

	velocity    []dto.CompanyVelocityRow
	velocityErr error

	tech    []dto.TechAdoptionRow
	techErr error

	// Capture inputs so callers can assert default-limit values, etc.
	weeklyWeeks   int
	velocityLimit int
}

func (f *fakeUserDashboardStore) GetProgressStats(_ context.Context, _ string) (dto.ProgressStats, error) {
	return f.progress, f.progressErr
}
func (f *fakeUserDashboardStore) GetMarketStats(_ context.Context) (dto.MarketStats, error) {
	return f.market, f.marketErr
}
func (f *fakeUserDashboardStore) GetWeeklyHiringActivity(_ context.Context, weeks int) ([]dto.WeeklyCount, error) {
	f.weeklyWeeks = weeks
	return f.weekly, f.weeklyErr
}
func (f *fakeUserDashboardStore) GetCompanyVelocityDetailed(_ context.Context, limit int) ([]dto.CompanyVelocityRow, error) {
	f.velocityLimit = limit
	return f.velocity, f.velocityErr
}
func (f *fakeUserDashboardStore) GetTechStackQuarterlyAdoption(_ context.Context) ([]dto.TechAdoptionRow, error) {
	return f.tech, f.techErr
}

// fakeApplicationListReader is a test double for services.ApplicationListReader.
type fakeApplicationListReader struct {
	active    []appsvc.ApplicationWithJob
	activeErr error
}

func (f *fakeApplicationListReader) ListActive(_ context.Context) ([]appsvc.ApplicationWithJob, error) {
	return f.active, f.activeErr
}

func mustServiceErr(t *testing.T, err error) *apperr.ServiceError {
	t.Helper()
	var se *apperr.ServiceError
	if !errors.As(err, &se) {
		t.Fatalf("expected *ServiceError, got %T: %v", err, err)
	}
	return se
}

// ---------- GetProgressStats ----------

func TestUserDashboard_GetProgressStats_OK(t *testing.T) {
	store := &fakeUserDashboardStore{progress: dto.ProgressStats{AppliedCount: 3, AwaitingReply: 2}}
	svc := services.NewUserDashboardService(store, &fakeApplicationListReader{})

	got, err := svc.GetProgressStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AppliedCount != 3 || got.AwaitingReply != 2 {
		t.Errorf("got = %+v, want applied=3 awaiting=2", got)
	}
}

// Zero user_job_state rows must yield a zero-valued ProgressStats (not nil).
// The store returns the zero struct on no rows; the service must pass it through.
func TestUserDashboard_GetProgressStats_ZeroRows(t *testing.T) {
	store := &fakeUserDashboardStore{progress: dto.ProgressStats{}}
	svc := services.NewUserDashboardService(store, &fakeApplicationListReader{})

	got, err := svc.GetProgressStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil zero stats")
	}
	if got.AppliedCount != 0 || got.AwaitingReply != 0 || got.InterviewingCount != 0 || got.InterviewingThisWeek != 0 || got.OfferCount != 0 {
		t.Errorf("non-zero fields in zero stats: %+v", got)
	}
}

func TestUserDashboard_GetProgressStats_StoreError(t *testing.T) {
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{progressErr: errors.New("db down")},
		&fakeApplicationListReader{},
	)

	_, err := svc.GetProgressStats(context.Background())
	if err == nil {
		t.Fatal("expected error from store failure")
	}
	if se := mustServiceErr(t, err); se.Status != 500 {
		t.Errorf("status = %d, want 500", se.Status)
	}
}

// ---------- GetMarketStats ----------

func TestUserDashboard_GetMarketStats_OK(t *testing.T) {
	store := &fakeUserDashboardStore{market: dto.MarketStats{NewSinceYesterday: 5, TotalOpen: 100, AddedThisWeek: 12, AddedLastWeek: 8, WeeklyDelta: 4}}
	svc := services.NewUserDashboardService(store, &fakeApplicationListReader{})

	got, err := svc.GetMarketStats(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.NewSinceYesterday != 5 || got.TotalOpen != 100 || got.AddedThisWeek != 12 || got.AddedLastWeek != 8 || got.WeeklyDelta != 4 {
		t.Errorf("got = %+v", got)
	}
}

func TestUserDashboard_GetMarketStats_StoreError(t *testing.T) {
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{marketErr: errors.New("db down")},
		&fakeApplicationListReader{},
	)
	_, err := svc.GetMarketStats(context.Background())
	if err == nil {
		t.Fatal("expected error from store failure")
	}
	if se := mustServiceErr(t, err); se.Status != 500 {
		t.Errorf("status = %d, want 500", se.Status)
	}
}

// ---------- GetHeatmap ----------

// GetHeatmap must call the underlying store with weeks=52 — the fixed window
// drives the 1-year heatmap rendered in the UI.
func TestUserDashboard_GetHeatmap_OK(t *testing.T) {
	store := &fakeUserDashboardStore{weekly: []dto.WeeklyCount{{WeekStart: "2026-04-06", JobCount: 7}}}
	svc := services.NewUserDashboardService(store, &fakeApplicationListReader{})

	got, err := svc.GetHeatmap(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].JobCount != 7 {
		t.Errorf("got = %+v", got)
	}
	if store.weeklyWeeks != 52 {
		t.Errorf("weeks passed to store = %d, want 52", store.weeklyWeeks)
	}
}

func TestUserDashboard_GetHeatmap_Empty(t *testing.T) {
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{weekly: []dto.WeeklyCount{}},
		&fakeApplicationListReader{},
	)

	got, err := svc.GetHeatmap(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("got nil; want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestUserDashboard_GetHeatmap_StoreError(t *testing.T) {
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{weeklyErr: errors.New("db down")},
		&fakeApplicationListReader{},
	)
	_, err := svc.GetHeatmap(context.Background())
	if err == nil {
		t.Fatal("expected error from store failure")
	}
	if se := mustServiceErr(t, err); se.Status != 500 {
		t.Errorf("status = %d, want 500", se.Status)
	}
}

// ---------- GetInterviews ----------

// fixedTime is a deterministic timestamp for InterviewRow.StageAt assertions.
var fixedTime = time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)

// app builds an ApplicationWithJob with the supplied stage for compact test cases.
func app(stage string, id int64) appsvc.ApplicationWithJob {
	return appsvc.ApplicationWithJob{
		JobApplication: appmodel.JobApplication{
			ID:          id,
			JobResultID: "job-1",
			Stage:       stage,
			StageAt:     fixedTime,
		},
		JobTitle:    "Software Engineer",
		CompanyName: "Acme",
	}
}

// GetInterviews maps every active application 1:1 and stamps DisplayStage via
// toDisplayStage. The output ordering preserves the input ordering.
func TestUserDashboard_GetInterviews_MapsAllStages(t *testing.T) {
	apps := []appsvc.ApplicationWithJob{
		app(appcommon.StageApplied, 1),
		app(appcommon.StagePhoneScreen, 2),
		app(appcommon.StageTechnical, 3),
		app(appcommon.StageFinalRound, 4),
		app(appcommon.StageOffer, 5),
		app("custom_stage", 6), // unmapped stage falls through to default branch
	}
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{},
		&fakeApplicationListReader{active: apps},
	)

	got, err := svc.GetInterviews(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(apps) {
		t.Fatalf("len = %d, want %d", len(got), len(apps))
	}

	want := []string{
		appcommon.DisplayStageWaiting,
		appcommon.DisplayStageInterviewing,
		appcommon.DisplayStageInterviewing,
		appcommon.DisplayStageInterviewing,
		appcommon.DisplayStageOffer,
		"custom_stage", // default: return raw stage
	}
	for i, w := range want {
		if got[i].DisplayStage != w {
			t.Errorf("row %d display = %q, want %q", i, got[i].DisplayStage, w)
		}
	}
	// StageAt must use the documented ISO-8601 format.
	if got[0].StageAt != "2026-04-13T12:00:00Z" {
		t.Errorf("stage_at = %q, want 2026-04-13T12:00:00Z", got[0].StageAt)
	}
	// Identity fields propagate verbatim.
	if got[0].ApplicationID != 1 || got[0].JobID != "job-1" || got[0].CompanyName != "Acme" {
		t.Errorf("row 0 identity wrong: %+v", got[0])
	}
}

// Empty active-applications must produce a non-nil empty slice.
func TestUserDashboard_GetInterviews_Empty(t *testing.T) {
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{},
		&fakeApplicationListReader{active: []appsvc.ApplicationWithJob{}},
	)
	got, err := svc.GetInterviews(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("got nil; want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestUserDashboard_GetInterviews_StoreError(t *testing.T) {
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{},
		&fakeApplicationListReader{activeErr: errors.New("db down")},
	)
	_, err := svc.GetInterviews(context.Background())
	if err == nil {
		t.Fatal("expected error from app store failure")
	}
	if se := mustServiceErr(t, err); se.Status != 500 {
		t.Errorf("status = %d, want 500", se.Status)
	}
}

// ---------- GetVelocity ----------

// Velocity classification: rising window where current ≥1.3× the prior 3-week
// average and below the 2.5× spike threshold.
func TestUserDashboard_GetVelocity_Rising(t *testing.T) {
	store := &fakeUserDashboardStore{velocity: []dto.CompanyVelocityRow{
		{CompanyID: "c1", CompanyName: "Acme", WeekCurrent: 4, Week1: 3, Week2: 3, Week3: 3},
	}}
	svc := services.NewUserDashboardService(store, &fakeApplicationListReader{})

	got, err := svc.GetVelocity(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.velocityLimit != 5 {
		t.Errorf("limit passed to store = %d, want 5", store.velocityLimit)
	}
	if got[0].VelocityTag != insightscommon.VelocityRising {
		t.Errorf("tag = %q, want rising", got[0].VelocityTag)
	}
	// 4 / 3 = 1.333… → rounded to 1.3
	if got[0].Multiplier != 1.3 {
		t.Errorf("multiplier = %v, want 1.3", got[0].Multiplier)
	}
}

// Spike threshold at 2.5×: current at 8 vs avg 3 → 2.7×, classified as spike.
func TestUserDashboard_GetVelocity_Spike(t *testing.T) {
	store := &fakeUserDashboardStore{velocity: []dto.CompanyVelocityRow{
		{WeekCurrent: 8, Week1: 3, Week2: 3, Week3: 3},
	}}
	svc := services.NewUserDashboardService(store, &fakeApplicationListReader{})

	got, _ := svc.GetVelocity(context.Background(), 10)
	if got[0].VelocityTag != insightscommon.VelocitySpike {
		t.Errorf("tag = %q, want spike", got[0].VelocityTag)
	}
	if got[0].Multiplier != 2.7 {
		t.Errorf("multiplier = %v, want 2.7", got[0].Multiplier)
	}
}

// Normal classification: current = prior avg, multiplier = 1.0 (< 1.3).
func TestUserDashboard_GetVelocity_Normal(t *testing.T) {
	store := &fakeUserDashboardStore{velocity: []dto.CompanyVelocityRow{
		{WeekCurrent: 3, Week1: 3, Week2: 3, Week3: 3},
	}}
	svc := services.NewUserDashboardService(store, &fakeApplicationListReader{})

	got, _ := svc.GetVelocity(context.Background(), 10)
	if got[0].VelocityTag != insightscommon.VelocityNormal {
		t.Errorf("tag = %q, want normal", got[0].VelocityTag)
	}
	if got[0].Multiplier != 1 {
		t.Errorf("multiplier = %v, want 1.0", got[0].Multiplier)
	}
}

// avg_prior=0 with current>0 must classify as spike (multiplier forced to 3.0)
// — without this rule a new company would always render `normal` and the UI
// would mask brand-new arrivals with a placeholder dash.
func TestUserDashboard_GetVelocity_NewCompanySpike(t *testing.T) {
	store := &fakeUserDashboardStore{velocity: []dto.CompanyVelocityRow{
		{WeekCurrent: 5, Week1: 0, Week2: 0, Week3: 0},
	}}
	svc := services.NewUserDashboardService(store, &fakeApplicationListReader{})

	got, _ := svc.GetVelocity(context.Background(), 10)
	if got[0].VelocityTag != insightscommon.VelocitySpike {
		t.Errorf("tag = %q, want spike", got[0].VelocityTag)
	}
	if got[0].Multiplier != 3.0 {
		t.Errorf("multiplier = %v, want 3.0", got[0].Multiplier)
	}
}

// avg_prior=0 with current=0 is truly inactive — multiplier 0, tag normal.
func TestUserDashboard_GetVelocity_AllZero(t *testing.T) {
	store := &fakeUserDashboardStore{velocity: []dto.CompanyVelocityRow{
		{WeekCurrent: 0, Week1: 0, Week2: 0, Week3: 0},
	}}
	svc := services.NewUserDashboardService(store, &fakeApplicationListReader{})

	got, _ := svc.GetVelocity(context.Background(), 10)
	if got[0].VelocityTag != insightscommon.VelocityNormal {
		t.Errorf("tag = %q, want normal", got[0].VelocityTag)
	}
	if got[0].Multiplier != 0 {
		t.Errorf("multiplier = %v, want 0", got[0].Multiplier)
	}
}

func TestUserDashboard_GetVelocity_StoreError(t *testing.T) {
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{velocityErr: errors.New("db down")},
		&fakeApplicationListReader{},
	)
	_, err := svc.GetVelocity(context.Background(), 10)
	if err == nil {
		t.Fatal("expected error from store failure")
	}
	if se := mustServiceErr(t, err); se.Status != 500 {
		t.Errorf("status = %d, want 500", se.Status)
	}
}

func TestUserDashboard_GetVelocity_Empty(t *testing.T) {
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{velocity: []dto.CompanyVelocityRow{}},
		&fakeApplicationListReader{},
	)
	got, err := svc.GetVelocity(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("got = %+v, want empty slice", got)
	}
}

// ---------- GetTechTrends ----------

// When the store returns zero rows, the response must still be non-nil with
// zero totals and empty (non-nil) lists.
func TestUserDashboard_GetTechTrends_Empty(t *testing.T) {
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{tech: []dto.TechAdoptionRow{}},
		&fakeApplicationListReader{},
	)
	got, err := svc.GetTechTrends(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("got nil")
	}
	if got.TotalJobs != 0 {
		t.Errorf("total = %d, want 0", got.TotalJobs)
	}
}

// TotalJobs == 0 must short-circuit per-row classification — every row is
// skipped so Rising/NewEntrants are nil (zero-length). This guards against a
// divide-by-zero in the Q4 percent computation.
func TestUserDashboard_GetTechTrends_ZeroTotal(t *testing.T) {
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{tech: []dto.TechAdoptionRow{
			{Tech: "Go", Q4: 1, TotalJobs: 0},
		}},
		&fakeApplicationListReader{},
	)
	got, err := svc.GetTechTrends(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.TotalJobs != 0 {
		t.Errorf("total = %d, want 0", got.TotalJobs)
	}
	if len(got.Rising) != 0 || len(got.NewEntrants) != 0 {
		t.Errorf("expected empty Rising/NewEntrants, got %+v", got)
	}
}

// Rising = q4% > 5% AND ((q3 > 0 AND growth > 15%) OR q3 = 0 (new entrant flag)).
// This covers both Rising-with-growth and Rising-via-IsNew branches.
func TestUserDashboard_GetTechTrends_RisingAndNew(t *testing.T) {
	rows := []dto.TechAdoptionRow{
		// Q4=20, Q3=10, total=100 → q4%=20, growth=100% (>15%) → Rising (with GrowthPct populated)
		{Tech: "Go", Q1: 5, Q2: 5, Q3: 10, Q4: 20, TotalJobs: 100},
		// Q4=10, Q3=0, total=100 → q4%=10, q3=0 → Rising via IsNew=true
		{Tech: "Rust", Q1: 0, Q2: 0, Q3: 0, Q4: 10, TotalJobs: 100},
		// Q4=5, total=100 → q4%=5 (NOT >5) → skipped
		{Tech: "Skip", Q1: 0, Q2: 0, Q3: 0, Q4: 5, TotalJobs: 100},
		// q4%=20, q2%=0 (Q2=0) → NewEntrant
		{Tech: "Zig", Q1: 0, Q2: 0, Q3: 1, Q4: 20, TotalJobs: 100},
	}
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{tech: rows},
		&fakeApplicationListReader{},
	)

	got, err := svc.GetTechTrends(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.TotalJobs != 100 {
		t.Errorf("total = %d, want 100 (from first row)", got.TotalJobs)
	}
	// Both Go and Rust qualify as Rising. Zig also qualifies (Q3>0 → growth check; growth = (20-1)/1 = 19 → >15%).
	if len(got.Rising) != 3 {
		t.Errorf("rising len = %d, want 3; got = %+v", len(got.Rising), got.Rising)
	}
	// Find Rust and verify IsNew was set.
	var rust *dto.TechTrendRow
	for i := range got.Rising {
		if got.Rising[i].Tech == "Rust" {
			rust = &got.Rising[i]
			break
		}
	}
	if rust == nil {
		t.Fatal("Rust missing from Rising")
	}
	if !rust.IsNew {
		t.Error("Rust.IsNew = false, want true (Q3=0 path)")
	}
	if rust.GrowthPct != 0 {
		t.Errorf("Rust.GrowthPct = %v, want 0 for IsNew row", rust.GrowthPct)
	}

	// Go growth = (20-10)/10 = 1.0 → 100% → rounded.
	var goRow *dto.TechTrendRow
	for i := range got.Rising {
		if got.Rising[i].Tech == "Go" {
			goRow = &got.Rising[i]
			break
		}
	}
	if goRow == nil {
		t.Fatal("Go missing from Rising")
	}
	if goRow.IsNew {
		t.Error("Go.IsNew = true, want false (Q3>0 path)")
	}
	if goRow.GrowthPct != 100.0 {
		t.Errorf("Go.GrowthPct = %v, want 100.0", goRow.GrowthPct)
	}
	// adoption_pct = 20/100*100 = 20.0
	if goRow.AdoptionPct != 20.0 {
		t.Errorf("Go.AdoptionPct = %v, want 20.0", goRow.AdoptionPct)
	}

	// NewEntrants: Zig and Rust qualify (q4%=20, q2%<1). Go has q2=5 → q2%=5 → excluded.
	if len(got.NewEntrants) != 2 {
		t.Errorf("new_entrants len = %d, want 2; got = %+v", len(got.NewEntrants), got.NewEntrants)
	}
}

// Growth <=15% must NOT be added to Rising even when q4%>5 and q3>0 —
// guards the "growth threshold not met" branch.
func TestUserDashboard_GetTechTrends_BelowGrowthThreshold(t *testing.T) {
	rows := []dto.TechAdoptionRow{
		// Q4=10, Q3=10, total=100 → q4%=10, growth=0% (not >15%), q3>0 → excluded
		// q2%=10 — also fails new-entrant check (q2% >= 1)
		{Tech: "Stable", Q1: 10, Q2: 10, Q3: 10, Q4: 10, TotalJobs: 100},
	}
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{tech: rows},
		&fakeApplicationListReader{},
	)

	got, _ := svc.GetTechTrends(context.Background())
	if len(got.Rising) != 0 {
		t.Errorf("rising = %+v, want empty (growth=0%% < 15%%)", got.Rising)
	}
	if len(got.NewEntrants) != 0 {
		t.Errorf("new_entrants = %+v, want empty (q2%% = 10 not <1)", got.NewEntrants)
	}
}

// Cap both Rising and NewEntrants at 10 rows — guards the capping branches
// at the bottom of GetTechTrends.
func TestUserDashboard_GetTechTrends_CapAt10(t *testing.T) {
	rows := make([]dto.TechAdoptionRow, 15)
	for i := range rows {
		rows[i] = dto.TechAdoptionRow{
			Tech:      "tech-" + string(rune('a'+i)),
			Q1:        0,
			Q2:        0,
			Q3:        0,
			Q4:        20,
			TotalJobs: 100,
		}
	}
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{tech: rows},
		&fakeApplicationListReader{},
	)

	got, _ := svc.GetTechTrends(context.Background())
	if len(got.Rising) != 10 {
		t.Errorf("rising len = %d, want capped at 10", len(got.Rising))
	}
	if len(got.NewEntrants) != 10 {
		t.Errorf("new_entrants len = %d, want capped at 10", len(got.NewEntrants))
	}
}

func TestUserDashboard_GetTechTrends_StoreError(t *testing.T) {
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{techErr: errors.New("db down")},
		&fakeApplicationListReader{},
	)
	_, err := svc.GetTechTrends(context.Background())
	if err == nil {
		t.Fatal("expected error from store failure")
	}
	if se := mustServiceErr(t, err); se.Status != 500 {
		t.Errorf("status = %d, want 500", se.Status)
	}
}

// Ensure test doubles implement interfaces.
var _ services.UserDashboardStore = (*fakeUserDashboardStore)(nil)
var _ services.ApplicationListReader = (*fakeApplicationListReader)(nil)
