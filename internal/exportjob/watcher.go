package exportjob

import (
	"context"
	"time"

	"github.com/scutrobotlab/rm-schedule/internal/static"
	"github.com/scutrobotlab/rm-schedule/internal/storage"
	"github.com/sirupsen/logrus"
)

// CheckAndRender 由 cron 周期性调用：对比 schedule hash，在冷却期过后触发后台渲染。
func CheckAndRender(store storage.Store) {
	cfg := ensureConfig()
	if !cfg.Enabled {
		return
	}

	scheduleData, ok := scheduleBytesFromCache()
	if !ok || len(scheduleData) == 0 {
		return
	}

	ctx := context.Background()
	now := time.Now()

	for _, zone := range static.CurrentSeasonZones {
		if isArchivedZone(zone.ID) {
			continue
		}

		hash, err := zoneHashFromSchedule(scheduleData, zone.ID)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"zone": zone.ID,
			}).WithError(err).Warn("export watcher: hash failed")
			continue
		}

		defaultManager.mu.Lock()
		zs := defaultManager.zoneWatch(static.CurrentSeason, zone.ID)
		inCooldown := !zs.lastRenderAt.IsZero() && now.Sub(zs.lastRenderAt) < cfg.RenderCooldown
		hashChanged := hash != zs.lastHash

		if inCooldown && hashChanged {
			zs.pending = true
			for _, part := range zone.Parts {
				defaultManager.setPartPending(static.CurrentSeason, zone.ID, part)
			}
			defaultManager.mu.Unlock()
			continue
		}

		shouldRender := (!inCooldown && hashChanged) || (!inCooldown && zs.pending)
		if !shouldRender {
			defaultManager.mu.Unlock()
			continue
		}

		zs.lastHash = hash
		zs.lastRenderAt = now
		zs.pending = false
		defaultManager.mu.Unlock()

		logrus.WithFields(logrus.Fields{
			"season": static.CurrentSeason,
			"zone":   zone.ID,
			"hash":   hash,
		}).Info("export watcher: render zone")
		renderZoneParts(ctx, store, cfg, zone, hash, false)
	}
}
