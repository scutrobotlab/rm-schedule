package exportjob

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scutrobotlab/rm-schedule/internal/static"
	"github.com/scutrobotlab/rm-schedule/internal/storage"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/sirupsen/logrus"
)

const (
	// readyProbeInterval 渲染目标就绪探测的轮询间隔。
	readyProbeInterval = 1 * time.Second
	// readyProbeTimeout 单次就绪探测请求的超时。
	readyProbeTimeout = 5 * time.Second
	// renderSlotAcquireTimeout Bootstrap 等待渲染互斥的最长时间；
	// 就绪探测在锁外进行，回来抢锁时可能偶发撞上 cron 的某轮渲染，故有界重试而非直接放弃。
	renderSlotAcquireTimeout = 2 * time.Minute
	// renderSlotPollInterval 抢锁失败后的重试间隔（cron 每轮持锁通常很短，很快即可插空拿到）。
	renderSlotPollInterval = 200 * time.Millisecond
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
	ctx := context.Background()

	// 先等待渲染目标（默认即本进程 :8080，在 main.go 末尾才 Listen）就绪，避免启动竞态下
	// chromedp 立即拿到 ERR_CONNECTION_REFUSED；放在获取渲染互斥之前，避免等待期间饿死 cron。
	waitRenderTargetReady(ctx, cfg.RenderTargetReadyTimeout, readyProbeInterval)

	// 与 CheckAndRender 共享渲染互斥，避免启动阶段与 cron 并发占用 chromedp。
	// 就绪探测在锁外进行，回来抢锁时可能撞上 cron 某轮渲染，故有界重试插空获取，
	// 而不是直接放弃——归档赛区不被 watcher 监听，一旦跳过将永久无人补渲染。
	if !acquireRenderSlot(ctx, renderSlotAcquireTimeout) {
		logrus.Warn("export bootstrap: render skipped, could not acquire render slot in time")
		return
	}
	defer checkAndRenderRunning.Store(false)

	for _, zone := range static.CurrentSeasonZones {
		if !isArchivedZone(zone.ID) {
			continue
		}

		// 归档赛区赛程已定格在内嵌快照，用其子树 hash 记录图片对应的赛程版本（便于审计）。
		// watcher 通过 isArchivedZone 跳过归档赛区，不依赖该值触发渲染，故 hash 仅作标识。
		scheduleHash := archivedZoneHash(zone.ID)

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
			renderArchivedPartWithRetry(ctx, store, cfg, zone, part, scheduleHash)
		}
	}
}

// acquireRenderSlot 有界重试获取渲染互斥（与 CheckAndRender 共享的 checkAndRenderRunning）。
// cron 每轮持锁通常很短，插空即可拿到；在 maxWait 内始终拿不到才放弃。返回是否获取成功。
func acquireRenderSlot(ctx context.Context, maxWait time.Duration) bool {
	deadline := time.Now().Add(maxWait)
	for {
		if checkAndRenderRunning.CompareAndSwap(false, true) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(renderSlotPollInterval):
		}
	}
}

// waitRenderTargetReady 轮询 svc.RenderBaseURL，直到能建立连接拿到任意 HTTP 响应（含 4xx/5xx）
// 即视为渲染目标已就绪；连接被拒绝则按 interval 重试直到超过 timeout。返回是否在超时前就绪。
// timeout <= 0 表示不等待，直接返回；超时未就绪时也返回（交由后续渲染重试兜底），不阻断启动。
func waitRenderTargetReady(ctx context.Context, timeout, interval time.Duration) bool {
	if timeout <= 0 {
		return true
	}
	if interval <= 0 {
		interval = readyProbeInterval
	}

	target := svc.RenderBaseURL
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: readyProbeTimeout}

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			logrus.WithField("target", target).WithError(err).Warn("export bootstrap: build readiness request failed")
			return false
		}

		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			logrus.WithField("target", target).Info("export bootstrap: render target ready")
			return true
		}

		if time.Now().After(deadline) {
			logrus.WithField("target", target).WithError(err).Warn("export bootstrap: render target not ready within timeout, proceed anyway")
			return false
		}

		select {
		case <-ctx.Done():
			return false
		case <-time.After(interval):
		}
	}
}

// archivedZoneHash 计算归档赛区在当前赛季内嵌快照中的子树 hash；解析失败时返回空串。
func archivedZoneHash(zoneID int) string {
	hash, err := zoneHashFromSchedule(static.CurrentSeasonScheduleBytes, zoneID)
	if err != nil {
		logrus.WithField("zone", zoneID).WithError(err).Warn("export bootstrap: compute archived zone hash failed")
		return ""
	}
	return hash
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
