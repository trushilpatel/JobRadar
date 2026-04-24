package store

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

//go:embed reserve.lua
var reserveLua string

var reserveScript = goredis.NewScript(reserveLua)

type redisStore struct {
	rdb *goredis.Client
}

// NewLLMRateLimitRedisStore returns a Redis-backed rate-limit store. The
// reservation logic is implemented as an atomic Lua script.
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

	pipe := s.rdb.Pipeline()
	pipe.ZRem(ctx, keys[0], reqID)
	tokenCmd := pipe.HGet(ctx, keys[3], reqID)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, goredis.Nil) {
		return fmt.Errorf("ratelimit redis release lookup: %w", err)
	}
	tokensStr, err := tokenCmd.Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		return fmt.Errorf("ratelimit redis release hget tokens: %w", err)
	}

	pipe = s.rdb.Pipeline()
	pipe.ZRem(ctx, keys[2], reqID)
	pipe.HDel(ctx, keys[3], reqID)
	if tokensStr != "" {
		n, parseErr := strconv.Atoi(tokensStr)
		if parseErr != nil {
			return fmt.Errorf("ratelimit redis release parse tokens %q: %w", tokensStr, parseErr)
		}
		if n > 0 {
			pipe.DecrBy(ctx, keys[4], int64(n))
		}
	}
	pipe.Decr(ctx, keys[1])
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("ratelimit redis release apply: %w", err)
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
