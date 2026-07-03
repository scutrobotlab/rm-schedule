package exportjob

import (
	"context"
	"time"

	"github.com/scutrobotlab/rm-schedule/internal/common"
	"github.com/scutrobotlab/rm-schedule/internal/render"
	"github.com/scutrobotlab/rm-schedule/internal/static"
	"github.com/scutrobotlab/rm-schedule/internal/storage"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/sirupsen/logrus"
)

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

// renderPart 渲染单个 part 并落盘，返回是否成功。
func renderPart(ctx context.Context, store storage.Store, cfg Config, zone static.ZoneManifest, part static.PartManifest, scheduleHash string, isStatic bool) bool {
	fields := logrus.Fields{
		"season": static.CurrentSeason,
		"zone":   zone.ID,
		"group":  part.Index,
		"static": isStatic,
	}

	fail := func(stage string, err error) bool {
		logrus.WithFields(fields).WithError(err).Errorf("export %s failed", stage)
		defaultManager.mu.Lock()
		defaultManager.setPartError(static.CurrentSeason, zone.ID, part, err.Error())
		defaultManager.mu.Unlock()
		return false
	}

	renderCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	img, err := render.RenderOnce(renderCtx, static.CurrentSeason, zone.ID, part.Index, cfg.Scale)
	if err != nil {
		return fail("render", err)
	}

	now := time.Now()
	key := imageKey(static.CurrentSeason, zone.ID, part.Index)
	imageURL, err := store.Save(ctx, key, img, now)
	if err != nil {
		return fail("save", err)
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
	if err := writeMeta(cfg.StorageDir, meta); err != nil {
		return fail("meta write", err)
	}

	defaultManager.mu.Lock()
	defaultManager.setPartReady(static.CurrentSeason, zone.ID, part, imageURL, scheduleHash, now)
	defaultManager.mu.Unlock()

	logrus.WithFields(fields).WithField("bytes", len(img)).Info("export render success")
	return true
}

func scheduleBytesFromCache() ([]byte, bool) {
	cached, ok := svc.Cache.Get(common.UpstreamNameSchedule)
	if !ok {
		return nil, false
	}
	data, ok := cached.([]byte)
	return data, ok
}
