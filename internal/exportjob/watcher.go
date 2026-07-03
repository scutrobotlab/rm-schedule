package exportjob

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/scutrobotlab/rm-schedule/internal/static"
	"github.com/scutrobotlab/rm-schedule/internal/storage"
	"github.com/sirupsen/logrus"
)

var checkAndRenderRunning atomic.Bool

// CheckAndRender 由 cron 周期性调用：对比 schedule hash，在冷却期过后触发后台渲染。
// 若上一轮尚未结束则直接跳过，避免 chromedp 渲染耗时超过 tick 间隔时并发重叠。
func CheckAndRender(store storage.Store) {
	if !checkAndRenderRunning.CompareAndSwap(false, true) {
		return
	}
	defer checkAndRenderRunning.Store(false)

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
			continue // 归档赛区由 Bootstrap 一次性渲染，不由 watcher 监听
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
			// 冷却期内又检测到变化：先合并标记为 pending，冷却结束后统一补渲染一次，
			// 避免同一 zone 在短时间内被连续渲染多次。
			zs.pending = true
			for _, part := range zone.Parts {
				defaultManager.setPartPending(static.CurrentSeason, zone.ID, part)
			}
			defaultManager.mu.Unlock()
			continue
		}

		shouldRender := !inCooldown && (hashChanged || zs.pending)
		if !shouldRender {
			defaultManager.mu.Unlock()
			continue
		}
		defaultManager.mu.Unlock()

		logrus.WithFields(logrus.Fields{
			"season": static.CurrentSeason,
			"zone":   zone.ID,
			"hash":   hash,
		}).Info("export watcher: render zone")

		allSucceeded := renderZoneParts(ctx, store, cfg, zone, hash, false)

		defaultManager.mu.Lock()
		// lastRenderAt 无论成败都推进，用于限制重试节奏（每个冷却期最多重试一次）；
		// lastHash/pending 只有全部渲染成功后才更新，失败的 part 会在下一次冷却期结束后自动重试。
		zs.lastRenderAt = now
		if allSucceeded {
			zs.lastHash = hash
			zs.pending = false
		} else {
			zs.pending = true
		}
		defaultManager.mu.Unlock()
	}
}
