package store

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

//go:embed reserve.lua
var reserveLua string

//go:embed release.lua
var releaseLua string

var (
	reserveScript = goredis.NewScript(reserveLua)
	releaseScript = goredis.NewScript(releaseLua)
)

type redisStore struct {
	rdb *goredis.Client
}

// NewLLMRateLimitRedisStore returns a Redis-backed rate-limit store. Reserve
// and Release are each a single atomic Lua script invocation — one Redis
// round-trip per call.
func NewLLMRateLimitRedisStore(rdb *goredis.Client) LLMRateLimitStore {
	return &redisStore{rdb: rdb}
}

func (s *redisStore) keys(slotID string, day string) []string {
	return []string{
		fmt.Sprintf("ratelimit:%s:minute", slotID),
		fmt.Sprintf("ratelimit:%s:day:%s", slotID, day),
		fmt.Sprintf("ratelimit:%s:tpm:window", slotID),
		fmt.Sprintf("ratelimit:%s:tpm:tokens", slotID),
		fmt.Sprintf("ratelimit:%s:tpm:sum", slotID),
	}
}

func endOfUTCDayMillis(now time.Time) int64 {
	y, m, d := now.UTC().Date()
	end := time.Date(y, m, d+1, 0, 0, 0, 0, time.UTC).Add(time.Hour)
	return end.UnixMilli()
}

func (s *redisStore) Reserve(ctx context.Context, r ReserveReq) (ReserveResult, error) {
	now := time.Now().UTC()
	day := now.Format("2006-01-02")
	keys := s.keys(r.SlotID, day)
	args := []any{
		now.UnixMilli(), r.RPM, r.RPD, r.TPMIn, r.Tokens, r.ReqID, endOfUTCDayMillis(now),
	}
	raw, err := reserveScript.Run(ctx, s.rdb, keys, args...).Int()
	if err != nil {
		return 0, fmt.Errorf("ratelimit redis reserve: %w", err)
	}
	return ReserveResult(raw), nil
}

func (s *redisStore) Release(ctx context.Context, slotID, reqID string) error {
	day := time.Now().UTC().Format("2006-01-02")
	keys := s.keys(slotID, day)
	if _, err := releaseScript.Run(ctx, s.rdb, keys, reqID).Result(); err != nil && !errors.Is(err, goredis.Nil) {
		return fmt.Errorf("ratelimit redis release: %w", err)
	}
	return nil
}

func (s *redisStore) Usage(ctx context.Context, slotID string) (Usage, error) {
	day := time.Now().UTC().Format("2006-01-02")
	keys := s.keys(slotID, day)

	pipe := s.rdb.Pipeline()
	rpm := pipe.ZCard(ctx, keys[0])
	rpd := pipe.Get(ctx, keys[1])
	tpm := pipe.Get(ctx, keys[4])
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, goredis.Nil) {
		return Usage{}, fmt.Errorf("ratelimit redis usage: %w", err)
	}
	u := Usage{}
	u.RPM = int(rpm.Val())
	if v, err := rpd.Int(); err == nil {
		u.RPD = v
	}
	if v, err := tpm.Int(); err == nil {
		u.TPM = v
	}
	return u, nil
}
