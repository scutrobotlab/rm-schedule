package exportjob

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/scutrobotlab/rm-schedule/internal/common"
	"github.com/scutrobotlab/rm-schedule/internal/render"
	"github.com/scutrobotlab/rm-schedule/internal/static"
	"github.com/scutrobotlab/rm-schedule/internal/storage"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/sirupsen/logrus"
)

// archivedRetryBaseBackoff 归档赛区渲染重试的线性退避基数：第 n 次失败后等待 n*base（2s、4s...）。
const archivedRetryBaseBackoff = 2 * time.Second

// renderZoneParts 渲染 zone 下所有 part，返回是否全部成功。
// 调用方（watcher）据此决定是否推进 zone 的 lastHash：只有全部成功才推进，
// 否则保留旧 hash 以便下次 tick（冷却期结束后）自动重试失败的 part。
func renderZoneParts(ctx context.Context, store storage.Store, cfg Config, zone static.ZoneManifest, scheduleHash string, isStatic bool) bool {
	allSucceeded := true
	for _, part := range zone.Parts {
		if !renderPart(ctx, store, cfg, zone, part, scheduleHash, isStatic) {
			allSucceeded = false
		}
	}
	return allSucceeded
}

// renderPart 渲染单个 part 并落盘，返回是否成功；失败时置 error 状态。
// 供 watcher 的 renderZoneParts 使用（非归档赛区，失败由 cron 冷却期后自愈，不在此处重试）。
func renderPart(ctx context.Context, store storage.Store, cfg Config, zone static.ZoneManifest, part static.PartManifest, scheduleHash string, isStatic bool) bool {
	if err := renderPartOnce(ctx, store, cfg, zone, part, scheduleHash, isStatic); err != nil {
		logrus.WithFields(partLogFields(zone, part, isStatic)).WithError(err).Error("export render failed")
		defaultManager.mu.Lock()
		defaultManager.setPartError(static.CurrentSeason, zone.ID, part, err.Error())
		defaultManager.mu.Unlock()
		return false
	}
	return true
}

// renderArchivedPartWithRetry 渲染归档赛区单个 part，对瞬时错误（网络/超时）退避重试。
// 归档赛区不由 watcher 监听，若首次因启动竞态（如 ERR_CONNECTION_REFUSED）失败将永久卡住，
// 故在此处补一层重试；ParamError 或存储/meta 错误重试无益，直接终止。
// 全部尝试失败后才置 error 状态，重试期间保持 pending，避免 manifest 状态抖动。
func renderArchivedPartWithRetry(ctx context.Context, store storage.Store, cfg Config, zone static.ZoneManifest, part static.PartManifest, scheduleHash string) bool {
	attempts := cfg.RenderMaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	fields := partLogFields(zone, part, true)

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			lastErr = err
			break
		}

		lastErr = renderPartOnce(ctx, store, cfg, zone, part, scheduleHash, true)
		if lastErr == nil {
			return true
		}
		if !isTransientRenderError(lastErr) {
			break // ParamError / 存储 / meta 错误，重试无益
		}
		if attempt >= attempts {
			break
		}

		backoff := time.Duration(attempt) * archivedRetryBaseBackoff
		logrus.WithFields(fields).WithError(lastErr).Warnf("export archived render attempt %d/%d failed, retry in %s", attempt, attempts, backoff)
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			lastErr = ctx.Err()
		case <-timer.C:
		}
	}

	logrus.WithFields(fields).WithError(lastErr).Error("export archived render failed after retries")
	defaultManager.mu.Lock()
	defaultManager.setPartError(static.CurrentSeason, zone.ID, part, lastErr.Error())
	defaultManager.mu.Unlock()
	return false
}

// renderPartOnce 执行一次「渲染 + 落盘 + 写 meta + 置 ready」，成功返回 nil，失败返回带阶段前缀的错误
// （用 %w 包裹原始错误，便于调用方用 errors.As 判定 render 错误类型）；本函数不负责置 error 状态。
func renderPartOnce(ctx context.Context, store storage.Store, cfg Config, zone static.ZoneManifest, part static.PartManifest, scheduleHash string, isStatic bool) error {
	fields := partLogFields(zone, part, isStatic)

	renderCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	img, err := render.RenderOnce(renderCtx, static.CurrentSeason, zone.ID, part.Index, cfg.Scale)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}

	now := time.Now()
	key := imageKey(static.CurrentSeason, zone.ID, part.Index)
	imageURL, err := store.Save(ctx, key, img, now)
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}

	meta := MetaFile{
		Season:       static.CurrentSeason,
		ZoneID:       zone.ID,
		Group:        part.Index,
		ScheduleHash: scheduleHash,
		UpdatedAt:    now,
		Scale:        cfg.Scale,
		Static:       isStatic,
		ImageURL:     imageURL,
	}
	if err := writeMetaWithRetry(cfg.StorageDir, meta, metaWriteAttempts); err != nil {
		if rmErr := removeSavedImage(cfg.StorageDir, meta); rmErr != nil {
			logrus.WithFields(fields).WithError(rmErr).Warn("export rollback image failed")
		}
		return fmt.Errorf("meta write: %w", err)
	}

	defaultManager.mu.Lock()
	defaultManager.setPartReady(static.CurrentSeason, zone.ID, part, imageURL, scheduleHash, now)
	defaultManager.mu.Unlock()

	logrus.WithFields(fields).WithField("bytes", len(img)).Info("export render success")
	return nil
}

func partLogFields(zone static.ZoneManifest, part static.PartManifest, isStatic bool) logrus.Fields {
	return logrus.Fields{
		"season": static.CurrentSeason,
		"zone":   zone.ID,
		"group":  part.Index,
		"static": isStatic,
	}
}

// isTransientRenderError 判定错误是否为可重试的瞬时渲染错误（页面加载失败、超时等）。
// render.ParamError（页面自身报参数错误）与存储/meta 错误视为非瞬时，不重试。
func isTransientRenderError(err error) bool {
	var re *render.RenderError
	var te *render.TimeoutError
	return errors.As(err, &re) || errors.As(err, &te)
}

func scheduleBytesFromCache() ([]byte, bool) {
	cached, ok := svc.Cache.Get(common.UpstreamNameSchedule)
	if !ok {
		return nil, false
	}
	data, ok := cached.([]byte)
	return data, ok
}
