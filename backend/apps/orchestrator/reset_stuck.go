package main

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
	crawlernodecommon "github.com/trushilpatel/jobradar/service/crawlernode/common"
)

// resetStucker is the narrow interface used by runResetStuck.
type resetStucker interface {
	ResetStuckForNodes(ctx context.Context, staleDuration time.Duration) (int, error)
}

// runResetStuck ticks every interval and calls ResetStuckForNodes to reclaim
// queue rows whose heartbeat expired (crawler crash recovery). Runs until ctx
// is cancelled.
func runResetStuck(ctx context.Context, store resetStucker, interval time.Duration, log logger.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := store.ResetStuckForNodes(ctx, crawlernodecommon.HeartbeatStaleDuration)
			if err != nil {
				log.Error("ResetStuckForNodes failed", err)
			} else if n > 0 {
				log.Info("reset stuck assignments", logger.Int("count", n))
			}
		}
	}
}
