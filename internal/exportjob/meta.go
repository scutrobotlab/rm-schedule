package exportjob

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scutrobotlab/rm-schedule/internal/storage"
)

const staticScheduleHash = "static" // 归档赛区占位 hash，不参与 watcher 变化检测

// MetaFile 与图片同目录同名，后缀 .meta.json；存在即表示对应 png 已完整落盘。
type MetaFile struct {
	Season       int       `json:"season"`
	ZoneID       int       `json:"zone_id"`
	Group        int       `json:"group"`
	ScheduleHash string    `json:"schedule_hash"`
	UpdatedAt    time.Time `json:"updated_at"`
	Scale        float64   `json:"scale"`
	Static       bool      `json:"static"` // true 表示归档赛区，渲染一次后不再监听变化
	ImageURL     string    `json:"image_url,omitempty"`
}

// imageURLFromMeta 从 meta 取 URL；新 meta 直接读 image_url，旧 meta 无该字段时回退到本地静态路由格式。
func imageURLFromMeta(cfg Config, meta MetaFile) string {
	if meta.ImageURL != "" {
		return meta.ImageURL
	}
	key := imageKey(meta.Season, meta.ZoneID, meta.Group)
	urlPath := "/api/export_static/" + key + fmt.Sprintf("?v=%d", meta.UpdatedAt.Unix())
	if cfg.PublicBaseURL != "" {
		return cfg.PublicBaseURL + urlPath
	}
	return urlPath
}

func imageKey(season, zoneID, partIndex int) string {
	return fmt.Sprintf("%d/%d/%d.png", season, zoneID, partIndex)
}

func metaPath(storageDir, imageKey string) (string, error) {
	destPath, err := storage.ResolveDestPath(storageDir, imageKey)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(destPath, ".png") + ".meta.json", nil
}

func readMetaFile(path string) (MetaFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MetaFile{}, err
	}
	var meta MetaFile
	if err := json.Unmarshal(data, &meta); err != nil {
		return MetaFile{}, err
	}
	return meta, nil
}

const metaWriteAttempts = 3

// writeMeta 必须在图片 Save 成功之后调用；采用 tmp + rename 保证原子写入。
func writeMeta(storageDir string, meta MetaFile) error {
	key := imageKey(meta.Season, meta.ZoneID, meta.Group)
	path, err := metaPath(storageDir, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create meta dir: %w", err)
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write temp meta: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp meta: %w", err)
	}
	return nil
}

func writeMetaWithRetry(storageDir string, meta MetaFile, attempts int) error {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for range attempts {
		if err := writeMeta(storageDir, meta); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}

// removeSavedImage 在 meta 写入失败时回滚刚落盘的 png，避免「新图已写、meta 仍指向旧版」的不一致。
func removeSavedImage(storageDir string, meta MetaFile) error {
	key := imageKey(meta.Season, meta.ZoneID, meta.Group)
	path, err := storage.ResolveDestPath(storageDir, key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove saved image: %w", err)
	}
	return nil
}
