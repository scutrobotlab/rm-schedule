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

// Bootstrap 扫描本地 meta 文件恢复内存状态（同步、快速），并在后台异步对归档赛区中
// 缺失的图片执行一次性渲染（可能耗时较久，不阻塞进程启动/HTTP 监听）。
func Bootstrap(store storage.Store) {
	cfg := loadConfig()
	if !cfg.Enabled {
		logrus.Info("schedule export disabled, skip bootstrap")
		return
	}

	initManager(cfg)
	restoreFromDisk(cfg)

	go renderMissingArchivedZones(store, cfg)
}

// renderMissingArchivedZones 对归档赛区（static.ArchivedZoneIDs）中磁盘尚无图片的 part
// 各渲染一次并永久保留；不再监听后续 schedule 变化。
func renderMissingArchivedZones(store storage.Store, cfg Config) {
	// 与 CheckAndRender 共享渲染互斥，避免启动阶段与 cron 并发占用 chromedp。
	if !checkAndRenderRunning.CompareAndSwap(false, true) {
		logrus.Warn("export bootstrap: render skipped, another export job is running")
		return
	}
	defer checkAndRenderRunning.Store(false)

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
		if meta.Season != static.CurrentSeason {
			// 赛季切换后遗留的旧 meta 文件，跳过，不纳入当前状态表。
			return nil
		}
		if _, ok := static.FindCurrentSeasonZone(meta.ZoneID); !ok {
			logrus.WithField("path", path).Warn("export bootstrap: zone not in current manifest, skip")
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
