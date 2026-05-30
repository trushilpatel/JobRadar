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
func (f *fakeUserDashboardStore) GetTechStackMonthlyAdoption(_ context.Context) ([]dto.TechAdoptionRow, error) {
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
// divide-by-zero in the M4 percent computation.
func TestUserDashboard_GetTechTrends_ZeroTotal(t *testing.T) {
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{tech: []dto.TechAdoptionRow{
			{Tech: "Go", M4: 1, TotalJobs: 0},
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

// Rising = m4% >= 2% AND m4 >= 10 AND ((m3 > 0 AND growth > 15%) OR m3 = 0 (new entrant flag)).
// This covers both Rising-with-growth and Rising-via-IsNew branches.
func TestUserDashboard_GetTechTrends_RisingAndNew(t *testing.T) {
	rows := []dto.TechAdoptionRow{
		// M4=20, M3=10, total=100 → m4%=20, growth=100% (>15%) → Rising (with GrowthPct populated)
		{Tech: "Go", M1: 5, M2: 5, M3: 10, M4: 20, TotalJobs: 100},
		// M4=10, M3=0, total=100 → m4%=10, m3=0 → Rising via IsNew=true
		{Tech: "Rust", M1: 0, M2: 0, M3: 0, M4: 10, TotalJobs: 100},
		// M4=5, total=100 → m4%=5 passes pct gate (>=2%) but M4=5 < 10 floor → skipped
		{Tech: "Skip", M1: 0, M2: 0, M3: 0, M4: 5, TotalJobs: 100},
		// m4%=20, m2%=0 (M2=0) → NewEntrant
		{Tech: "Zig", M1: 0, M2: 0, M3: 1, M4: 20, TotalJobs: 100},
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
	// Both Go and Rust qualify as Rising. Zig also qualifies (M3>0 → growth check; growth = (20-1)/1 = 19 → >15%).
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
		t.Error("Rust.IsNew = false, want true (M3=0 path)")
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
		t.Error("Go.IsNew = true, want false (M3>0 path)")
	}
	if goRow.GrowthPct != 100.0 {
		t.Errorf("Go.GrowthPct = %v, want 100.0", goRow.GrowthPct)
	}
	// adoption_pct = 20/100*100 = 20.0
	if goRow.AdoptionPct != 20.0 {
		t.Errorf("Go.AdoptionPct = %v, want 20.0", goRow.AdoptionPct)
	}

	// NewEntrants: Zig and Rust qualify (m4%=20, m2%<1). Go has m2=5 → m2%=5 → excluded.
	if len(got.NewEntrants) != 2 {
		t.Errorf("new_entrants len = %d, want 2; got = %+v", len(got.NewEntrants), got.NewEntrants)
	}
}

// Growth <=15% must NOT be added to Rising even when m4%>=2% and m4>=10 and m3>0 —
// guards the "growth threshold not met" branch.
func TestUserDashboard_GetTechTrends_BelowGrowthThreshold(t *testing.T) {
	rows := []dto.TechAdoptionRow{
		// M4=10, M3=10, total=100 → m4%=10, growth=0% (not >15%), m3>0 → excluded
		// m2%=10 — also fails new-entrant check (m2% >= 1)
		{Tech: "Stable", M1: 10, M2: 10, M3: 10, M4: 10, TotalJobs: 100},
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
		t.Errorf("new_entrants = %+v, want empty (m2%% = 10 not <1)", got.NewEntrants)
	}
}

// Cap both Rising and NewEntrants at 10 rows — guards the capping branches
// at the bottom of GetTechTrends.
func TestUserDashboard_GetTechTrends_CapAt10(t *testing.T) {
	rows := make([]dto.TechAdoptionRow, 15)
	for i := range rows {
		rows[i] = dto.TechAdoptionRow{
			Tech:      "tech-" + string(rune('a'+i)),
			M1:        0,
			M2:        0,
			M3:        0,
			M4:        20,
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

// AC-3: both conditions must hold — m4_count/total_jobs >= 2% AND m4_count >= 10.
// The impl reads TotalJobs from the first row; each scenario uses a separate store call
// so the denominator is controlled per-scenario.
func TestUserDashboard_GetTechTrends_AbsoluteFloor(t *testing.T) {
	t.Run("floor only fails: pct>=2% but M4<10 excluded", func(t *testing.T) {
		// TotalJobs=100; TooFew: pct=2/100*100=2% (passes pct) but M4=2 < 10 floor
		rows := []dto.TechAdoptionRow{
			{Tech: "TooFew", M1: 0, M2: 0, M3: 0, M4: 2, TotalJobs: 100},
			{Tech: "Enough", M1: 0, M2: 0, M3: 0, M4: 15, TotalJobs: 100},
		}
		svc := services.NewUserDashboardService(&fakeUserDashboardStore{tech: rows}, &fakeApplicationListReader{})
		got, err := svc.GetTechTrends(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, row := range got.Rising {
			if row.Tech == "TooFew" {
				t.Error("TooFew should be excluded (M4=2 < 10 floor)")
			}
		}
		found := false
		for _, row := range got.Rising {
			if row.Tech == "Enough" {
				found = true
			}
		}
		if !found {
			t.Error("Enough should be in Rising (M4=15 >= 10, m4%=15 >= 2)")
		}
	})

	t.Run("pct only fails: M4>=10 but pct<2% excluded", func(t *testing.T) {
		// TotalJobs=2000; LowPct: pct=10/2000*100=0.5% < 2% (fails pct gate); M4=10 >=10
		// BigEnough: pct=50/2000*100=2.5% >=2% and M4=50 >=10 → both gates pass
		rows := []dto.TechAdoptionRow{
			{Tech: "LowPct", M1: 0, M2: 0, M3: 0, M4: 10, TotalJobs: 2000},
			{Tech: "BigEnough", M1: 0, M2: 0, M3: 0, M4: 50, TotalJobs: 2000},
		}
		svc := services.NewUserDashboardService(&fakeUserDashboardStore{tech: rows}, &fakeApplicationListReader{})
		got, err := svc.GetTechTrends(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, row := range got.Rising {
			if row.Tech == "LowPct" {
				t.Error("LowPct should be excluded (m4%=0.5 < 2% pct gate)")
			}
		}
		found := false
		for _, row := range got.Rising {
			if row.Tech == "BigEnough" {
				found = true
			}
		}
		if !found {
			t.Error("BigEnough should be in Rising (M4=50 >= 10, m4%=2.5 >= 2)")
		}
	})
}

// AC-4: Rising list sorted by growth_pct * log(m4+1) descending before cap at 10.
// IsNew rows (growth_pct=0) always rank below non-new rows; among themselves by log(m4+1).
func TestUserDashboard_GetTechTrends_MomentumSort(t *testing.T) {
	rows := []dto.TechAdoptionRow{
		// IsNew: M3=0, M4=20 — growth=0, momentum=log(21)≈3.04
		{Tech: "NewA", M1: 0, M2: 0, M3: 0, M4: 20, TotalJobs: 100},
		// Growth: M3=10, M4=30 → growth=200%, momentum=200*log(31)≈679
		{Tech: "HighGrowth", M1: 0, M2: 0, M3: 10, M4: 30, TotalJobs: 100},
		// IsNew: M3=0, M4=15 — growth=0, momentum=log(16)≈2.77
		{Tech: "NewB", M1: 0, M2: 0, M3: 0, M4: 15, TotalJobs: 100},
		// Growth: M3=10, M4=20 → growth=100%, momentum=100*log(21)≈304
		{Tech: "LowGrowth", M1: 0, M2: 0, M3: 10, M4: 20, TotalJobs: 100},
	}
	svc := services.NewUserDashboardService(
		&fakeUserDashboardStore{tech: rows},
		&fakeApplicationListReader{},
	)

	got, err := svc.GetTechTrends(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rising := got.Rising
	if len(rising) < 4 {
		t.Fatalf("expected 4 rising rows, got %d: %+v", len(rising), rising)
	}

	// Non-new rows must come before IsNew rows.
	// HighGrowth and LowGrowth must rank above NewA and NewB.
	names := make([]string, len(rising))
	for i, r := range rising {
		names[i] = r.Tech
	}

	// First two must be non-IsNew (HighGrowth then LowGrowth).
	if rising[0].Tech != "HighGrowth" {
		t.Errorf("rising[0] = %q, want HighGrowth (highest momentum)", rising[0].Tech)
	}
	if rising[1].Tech != "LowGrowth" {
		t.Errorf("rising[1] = %q, want LowGrowth", rising[1].Tech)
	}
	// IsNew rows follow, NewA (M4=20) before NewB (M4=15) by log(m4+1).
	if rising[2].Tech != "NewA" {
		t.Errorf("rising[2] = %q, want NewA (M4=20 > NewB M4=15)", rising[2].Tech)
	}
	if rising[3].Tech != "NewB" {
		t.Errorf("rising[3] = %q, want NewB", rising[3].Tech)
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
