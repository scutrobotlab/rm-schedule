package exportjob

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/scutrobotlab/rm-schedule/internal/common"
	"github.com/scutrobotlab/rm-schedule/internal/render"
	"github.com/scutrobotlab/rm-schedule/internal/static"
	"github.com/scutrobotlab/rm-schedule/internal/storage"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/sirupsen/logrus"
)

func renderZoneParts(ctx context.Context, store storage.Store, cfg Config, zone static.ZoneManifest, scheduleHash string, isStatic bool) {
	for _, part := range zone.Parts {
		renderPart(ctx, store, cfg, zone, part, scheduleHash, isStatic)
	}
}

func renderPart(ctx context.Context, store storage.Store, cfg Config, zone static.ZoneManifest, part static.PartManifest, scheduleHash string, isStatic bool) {
	fields := logrus.Fields{
		"season": static.CurrentSeason,
		"zone":   zone.ID,
		"group":  part.Index,
		"static": isStatic,
	}

	renderCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	img, err := render.RenderOnce(renderCtx, static.CurrentSeason, zone.ID, part.Index, cfg.Scale)
	if err != nil {
		logrus.WithFields(fields).WithError(err).Error("export render failed")
		defaultManager.mu.Lock()
		defaultManager.setPartError(static.CurrentSeason, zone.ID, part, err.Error())
		defaultManager.mu.Unlock()
		return
	}

	key := imageKey(static.CurrentSeason, zone.ID, part.Index)
	imageURL, err := store.Save(ctx, key, img)
	if err != nil {
		logrus.WithFields(fields).WithError(err).Error("export save failed")
		defaultManager.mu.Lock()
		defaultManager.setPartError(static.CurrentSeason, zone.ID, part, err.Error())
		defaultManager.mu.Unlock()
		return
	}

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
		logrus.WithFields(fields).WithError(err).Error("export meta write failed")
		defaultManager.mu.Lock()
		defaultManager.setPartError(static.CurrentSeason, zone.ID, part, err.Error())
		defaultManager.mu.Unlock()
		return
	}

	defaultManager.mu.Lock()
	defaultManager.setPartReady(static.CurrentSeason, zone.ID, part, imageURL, scheduleHash, now)
	defaultManager.mu.Unlock()

	logrus.WithFields(fields).WithField("bytes", len(img)).Info("export render success")
}

func imageURLFromMeta(cfg Config, meta MetaFile) string {
	key := imageKey(meta.Season, meta.ZoneID, meta.Group)
	v := meta.UpdatedAt.Unix()
	urlPath := "/api/export_static/" + key + fmt.Sprintf("?v=%d", v)

	baseURL := strings.TrimRight(cfg.PublicBaseURL, "/")
	if baseURL != "" {
		return baseURL + urlPath
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
