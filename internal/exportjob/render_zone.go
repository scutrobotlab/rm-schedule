package exportjob

import (
	"context"
	"fmt"
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

	key := imageKey(static.CurrentSeason, zone.ID, part.Index)
	if _, err := store.Save(ctx, key, img); err != nil {
		return fail("save", err)
	}

	// updatedAt 同时作为 meta 落盘时间与图片 URL 的 ?v= 版本号，二者必须一致，
	// 因此不采用 store.Save 返回的 URL（其内部另行生成时间戳），而是统一由 imageURLFromMeta 构造。
	now := time.Now()
	meta := MetaFile{
		Season:       static.CurrentSeason,
		ZoneID:       zone.ID,
		Group:        part.Index,
		ScheduleHash: scheduleHash,
		UpdatedAt:    now,
		Scale:        cfg.Scale,
		Static:       isStatic,
	}
	if err := writeMeta(cfg.StorageDir, meta); err != nil {
		return fail("meta write", err)
	}

	imageURL := imageURLFromMeta(cfg, meta)

	defaultManager.mu.Lock()
	defaultManager.setPartReady(static.CurrentSeason, zone.ID, part, imageURL, scheduleHash, now)
	defaultManager.mu.Unlock()

	logrus.WithFields(fields).WithField("bytes", len(img)).Info("export render success")
	return true
}

func imageURLFromMeta(cfg Config, meta MetaFile) string {
	key := imageKey(meta.Season, meta.ZoneID, meta.Group)
	urlPath := "/api/export_static/" + key + fmt.Sprintf("?v=%d", meta.UpdatedAt.Unix())
	if cfg.PublicBaseURL != "" {
		return cfg.PublicBaseURL + urlPath
	}
	return urlPath
}

func scheduleBytesFromCache() ([]byte, bool) {
	cached, ok := svc.Cache.Get(common.UpstreamNameSchedule)
	if !ok {
		return nil, false
	}
	data, ok := cached.([]byte)
	return data, ok
}
