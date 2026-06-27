package main

import (
	"context"
	"time"

	"github.com/trushilpatel/jobradar/common/logger"
	dashboardsvc "github.com/trushilpatel/jobradar/service/dashboard/services"
)

// Refresh cadences are package-level vars (not consts) so tests can shrink
// them to exercise the ticker branches deterministically without waiting
// minutes. Production never mutates these.
var (
	dashboardPipelineRefreshInterval = 3 * time.Minute
	dashboardHeatmapRefreshInterval  = 5 * time.Minute
)

// runRefresher keeps dashboard materialized views up to date.
// Pipeline stats refresh every dashboardPipelineRefreshInterval;
// the heatmap every dashboardHeatmapRefreshInterval.
// Both views are also refreshed once immediately on startup.
func runRefresher(ctx context.Context, log logger.Logger, r dashboardsvc.StatsRefresher) {
	if err := r.RefreshPipelineStats(ctx); err != nil {
		log.Error("refresh pipeline stats failed", err)
	}
	if err := r.RefreshHeatmap(ctx); err != nil {
		log.Error("refresh heatmap failed", err)
	}

	pipelineTicker := time.NewTicker(dashboardPipelineRefreshInterval)
	heatmapTicker := time.NewTicker(dashboardHeatmapRefreshInterval)
	defer pipelineTicker.Stop()
	defer heatmapTicker.Stop()

	for {
		select {
		case <-pipelineTicker.C:
			if err := r.RefreshPipelineStats(ctx); err != nil {
				log.Error("refresh pipeline stats failed", err)
			}
		case <-heatmapTicker.C:
			if err := r.RefreshHeatmap(ctx); err != nil {
				log.Error("refresh heatmap failed", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
