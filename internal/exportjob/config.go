// Package exportjob 负责当前赛季赛程图的预渲染、持久化与状态查询。
// watcher 周期性对比 schedule hash 触发渲染；Bootstrap 从本地 meta 恢复状态，
// 并对归档赛区补做一次性渲染。
package exportjob

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/scutrobotlab/rm-schedule/internal/storage"
)

const (
	envEnabled        = "SCHEDULE_EXPORT_ENABLED"
	envRenderCooldown = "SCHEDULE_EXPORT_RENDER_COOLDOWN"
	envScale          = "SCHEDULE_EXPORT_SCALE"

	defaultRenderCooldown = 20 * time.Second
	defaultScale          = 2.0
)

type Config struct {
	Enabled        bool          // SCHEDULE_EXPORT_ENABLED，默认 true
	StorageDir     string        // 本地图片与 meta 目录
	PublicBaseURL  string        // 图片 URL 域名前缀，空则返回相对路径
	RenderCooldown time.Duration // 同一 zone 两次渲染的最小间隔
	Scale          float64       // chromedp 渲染缩放倍数
}

func loadConfig() Config {
	enabled := true
	if v := strings.TrimSpace(os.Getenv(envEnabled)); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err == nil {
			enabled = parsed
		}
	}

	cooldown := defaultRenderCooldown
	if v := strings.TrimSpace(os.Getenv(envRenderCooldown)); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil && parsed > 0 {
			cooldown = parsed
		}
	}

	scale := defaultScale
	if v := strings.TrimSpace(os.Getenv(envScale)); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed >= 1 && parsed <= 8 {
			scale = parsed
		}
	}

	return Config{
		Enabled:        enabled,
		StorageDir:     storage.EnvStorageDir(),
		PublicBaseURL:  storage.EnvPublicBaseURL(),
		RenderCooldown: cooldown,
		Scale:          scale,
	}
}
