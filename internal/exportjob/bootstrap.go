package exportjob

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/scutrobotlab/rm-schedule/internal/static"
	"github.com/scutrobotlab/rm-schedule/internal/storage"
	"github.com/sirupsen/logrus"
)

// Bootstrap 扫描本地 meta 文件恢复内存状态，并对归档赛区缺失的图片执行一次性渲染。
func Bootstrap(store storage.Store) {
	cfg := loadConfig()
	if !cfg.Enabled {
		logrus.Info("schedule export disabled, skip bootstrap")
		return
	}

	initManager(cfg)
	restoreFromDisk(cfg)

	ctx := context.Background()
	for _, zone := range static.CurrentSeasonZones {
		if !isArchivedZone(zone.ID) {
			continue
		}
		for _, part := range zone.Parts {
			key := partKey(static.CurrentSeason, zone.ID, part.Index)
			defaultManager.mu.RLock()
			st, exists := defaultManager.parts[key]
			ready := exists && st.Status == StatusReady && st.ImageURL != ""
			defaultManager.mu.RUnlock()
			if ready {
				continue
			}
			logrus.WithFields(logrus.Fields{
				"season": static.CurrentSeason,
				"zone":   zone.ID,
				"group":  part.Index,
			}).Info("export bootstrap: render archived zone part")
			renderPart(ctx, store, cfg, zone, part, staticScheduleHash, true)
		}
	}
}

func restoreFromDisk(cfg Config) {
	err := filepath.WalkDir(cfg.StorageDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".meta.json") {
			return nil
		}

		meta, err := readMetaFile(path)
		if err != nil {
			logrus.WithField("path", path).WithError(err).Warn("export bootstrap: skip invalid meta")
			return nil
		}

		imageURL := imageURLFromMeta(cfg, meta)
		defaultManager.mu.Lock()
		defaultManager.restoreFromMeta(meta, imageURL)
		defaultManager.mu.Unlock()
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		logrus.WithError(err).Warn("export bootstrap: scan storage dir failed")
	}
}
