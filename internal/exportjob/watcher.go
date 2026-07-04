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

	ctx := context.Background()
	now := time.Now()

	// 归档赛区长期兜底：用内嵌快照渲染、走前端导出页取数，均不依赖 live schedule 缓存，
	// 故放在缓存检查之前——即使 OSS 拉取尚未就绪/持续失败，也不影响归档缺图的补渲染。
	// 正常情况下归档图由 Bootstrap 一次性渲染；此处覆盖「bootstrap 有界重试耗尽仍缺图」的长期场景，
	// 按冷却期节奏补渲染仍未 ready 的 part，成功后永久保留、不再重试。
	for _, zone := range static.CurrentSeasonZones {
		if isArchivedZone(zone.ID) {
			backfillArchivedZone(ctx, store, cfg, now, zone)
		}
	}

	scheduleData, ok := scheduleBytesFromCache()
	if !ok || len(scheduleData) == 0 {
		return
	}

	for _, zone := range static.CurrentSeasonZones {
		if isArchivedZone(zone.ID) {
			continue // 归档赛区已在上方独立兜底处理
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

// backfillArchivedZone 对归档赛区中仍未 ready 的 part 做长期兜底补渲染。
// 与非归档赛区自愈一致：每冷却期最多尝试一次、单次尝试快速失败（不长时间占用渲染槽），
// 由 cron 的周期性调用充当重试节奏；全部 part ready 后走快速返回、不再产生渲染。
func backfillArchivedZone(ctx context.Context, store storage.Store, cfg Config, now time.Time, zone static.ZoneManifest) {
	defaultManager.mu.Lock()
	missing := make([]static.PartManifest, 0, len(zone.Parts))
	for _, part := range zone.Parts {
		st, ok := defaultManager.parts[partKey(static.CurrentSeason, zone.ID, part.Index)]
		if !ok || st.Status != StatusReady || st.ImageURL == "" {
			missing = append(missing, part)
		}
	}
	if len(missing) == 0 {
		defaultManager.mu.Unlock()
		return
	}
	// 复用 zoneWatch.lastRenderAt 作为归档补渲染的冷却计时（归档赛区不参与 hash 比较，仅借用节流）。
	zs := defaultManager.zoneWatch(static.CurrentSeason, zone.ID)
	if !zs.lastRenderAt.IsZero() && now.Sub(zs.lastRenderAt) < cfg.RenderCooldown {
		defaultManager.mu.Unlock()
		return
	}
	zs.lastRenderAt = now
	defaultManager.mu.Unlock()

	scheduleHash := archivedZoneHash(zone.ID)
	logrus.WithFields(logrus.Fields{
		"season":  static.CurrentSeason,
		"zone":    zone.ID,
		"missing": len(missing),
	}).Warn("export watcher: backfill archived zone parts")

	for _, part := range missing {
		renderPart(ctx, store, cfg, zone, part, scheduleHash, true)
	}
}
