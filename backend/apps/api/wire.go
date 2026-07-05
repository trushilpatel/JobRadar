//go:build wireinject

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/google/wire"
	"github.com/trushilpatel/jobradar/common/config"
	"github.com/trushilpatel/jobradar/common/logger"
	commonredis "github.com/trushilpatel/jobradar/common/redis"
	"github.com/trushilpatel/jobradar/platform/objectstorage"
	"github.com/trushilpatel/jobradar/service/application"
	"github.com/trushilpatel/jobradar/service/applyqueue"
	"github.com/trushilpatel/jobradar/service/company"
	companysvc "github.com/trushilpatel/jobradar/service/company/services"
	"github.com/trushilpatel/jobradar/service/job"
	jobsvc "github.com/trushilpatel/jobradar/service/job/services"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
	llmcore "github.com/trushilpatel/jobradar/service/llm/services"
	llmstore "github.com/trushilpatel/jobradar/service/llm/store"
	"github.com/trushilpatel/jobradar/service/resume"
	resumesvc "github.com/trushilpatel/jobradar/service/resume/services"
	"github.com/trushilpatel/jobradar/service/scan"
	scansvc "github.com/trushilpatel/jobradar/service/scan/services"
	"gorm.io/gorm"
)

func provideScanConfig(cfg *config.Config) scansvc.Config {
	return scansvc.Config{ScanIntervalMinutes: cfg.Scheduler.ScanIntervalMinutes}
}

// provideScanLogReader binds ScanLogStore to the ScanLogReader interface
// needed by scan.Service.
func provideScanLogReader(s scansvc.ScanLogStore) scansvc.ScanLogReader {
	return s
}

// provideScanHistoryReader binds ScanLogStore to the ScanHistoryReader interface
// needed by company.Service.
func provideScanHistoryReader(s scansvc.ScanLogStore) companysvc.ScanHistoryReader {
	return s
}

// provideSeenLedgerWriter binds SeenLedgerStore to the SeenLedgerWriter interface
// needed by job.Service.
func provideSeenLedgerWriter(s jobsvc.SeenLedgerStore) jobsvc.SeenLedgerWriter {
	return s
}

// provideJobApplicationWriter constructs a dedicated ApplicationWriter for job.Service.
func provideJobApplicationWriter(db *gorm.DB) jobsvc.ApplicationWriter {
	return application.NewApplicationWriter(db)
}

// provideObjectStorage constructs the S3-compatible client used by the
// resume slice. Wire calls this once during InitializeApp. LoadDefaultConfig
// does no network I/O — it only assembles client config — so context.Background()
// at boot time is the standard pattern here (no request context exists yet).
func provideObjectStorage(cfg *config.Config) (objectstorage.Storage, error) {
	return objectstorage.NewS3Storage(context.Background(), objectstorage.Config{
		Endpoint:  cfg.ObjectStorage.Endpoint,
		Region:    cfg.ObjectStorage.Region,
		Bucket:    cfg.ObjectStorage.Bucket,
		AccessKey: cfg.ObjectStorage.AccessKey,
		SecretKey: cfg.ObjectStorage.SecretKey,
	})
}

func InitializeApp(db *gorm.DB, cfg *config.Config, log logger.Logger) (*App, error) {
	wire.Build(
		provideScanConfig,
		provideScanLogReader,
		provideScanHistoryReader,
		provideSeenLedgerWriter,
		provideJobApplicationWriter,
		provideObjectStorage,
		provideKeywordExtractor,
		company.ProviderSet,
		scan.ProviderSet,
		job.ProviderSet,
		application.ProviderSet,
		resume.ProviderSet,
		applyqueue.ProviderSet,
		NewApp,
	)
	return nil, nil
}

// noopKeywordExtractor is the resume slice's KeywordExtractor when the LLM
// scorer could not be constructed (e.g. no LLM slots configured in this
// environment) — resume upload/extraction remain fully functional, keywords
// simply stay empty until the environment is configured. Never returned as
// nil: ImplService's constructor would panic on a nil dependency, and a
// typed no-op is clearer at the call site than threading an extra bool.
type noopKeywordExtractor struct{ reason error }

func (n noopKeywordExtractor) ExtractResumeKeywords(context.Context, llmdto.ResumeKeywordInput) (*llmdto.ResumeKeywordResult, error) {
	return nil, fmt.Errorf("resume: keyword extraction unavailable: %w", n.reason)
}

// buildLLMQuotaStores returns Redis-backed rate-limit/cooldown stores, or an
// in-memory fallback (non-fatal — matches buildMultiUserPlatform's Redis
// fallback precedent) when Redis is unreachable. Resume keyword extraction
// is best-effort, so a missing Redis must degrade the feature, never block
// API startup.
func buildLLMQuotaStores(ctx context.Context, cfg *config.Config, log logger.Logger) (llmstore.LLMRateLimitStore, llmstore.LLMCooldownStore) {
	rdb, err := commonredis.New(ctx, cfg.Redis.URL)
	if err != nil {
		log.Warn("redis unavailable; resume keyword extraction will use in-memory LLM quota tracking", logger.Err(err))
		return llmstore.NewLLMRateLimitMemoryStore(), llmstore.NewLLMCooldownMemoryStore(llmstore.CooldownPolicy{
			Base:   time.Duration(cfg.Cooldown.BaseSeconds) * time.Second,
			Max:    time.Duration(cfg.Cooldown.MaxSeconds) * time.Second,
			Jitter: cfg.Cooldown.Jitter,
		})
	}
	return llmstore.NewRedisStores(rdb, cfg.Cooldown)
}

// provideKeywordExtractor builds the LLM Scorer used for resume keyword
// extraction, reusing the enricher's slot configuration (same model, same
// env vars — see project convention against introducing a new LLM model)
// so the API process shares the exact same Redis-backed per-slot RPM/RPD/TPM
// budget as the analyzer worker rather than double-spending it. Never fails
// API startup: any construction error degrades to a noopKeywordExtractor and
// is logged, since resume upload must keep working even with no LLM slots
// configured in a given environment (e.g. local dev without Doppler LLM keys).
func provideKeywordExtractor(cfg *config.Config, log logger.Logger) resumesvc.KeywordExtractor {
	ctx, cancel := context.WithTimeout(context.Background(), redisConnectTimeout)
	defer cancel()

	rateStore, cooldownStore := buildLLMQuotaStores(ctx, cfg, log)

	scorer, err := llmcore.NewScorer(ctx, llmcore.Options{
		Provider:       cfg.LLM.Provider,
		TargetRoles:    cfg.LLM.TargetRoles,
		YearsExp:       cfg.LLM.YearsExp,
		Log:            log,
		Slots:          llmcore.SlotsFromConfig(cfg.Enricher.Slots),
		RateLimitStore: rateStore,
		CooldownStore:  cooldownStore,
	})
	if err != nil {
		log.Warn("llm scorer init failed; resume keyword extraction disabled", logger.Err(err))
		return noopKeywordExtractor{reason: err}
	}
	return scorer
}
