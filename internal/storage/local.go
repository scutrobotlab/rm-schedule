package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalStore 将图片写入本地目录，URL 指向 /api/export_static/ 静态路由。
type LocalStore struct {
	dir     string // SCHEDULE_EXPORT_STORAGE_DIR
	baseURL string // SCHEDULE_EXPORT_PUBLIC_BASE_URL，为空时返回相对路径
}

func NewLocalStore(dir, baseURL string) *LocalStore {
	return &LocalStore{
		dir:     dir,
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

func (s *LocalStore) Save(_ context.Context, key string, data []byte, version time.Time) (string, error) {
	destPath, err := resolveDestPath(s.dir, key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", fmt.Errorf("create storage dir: %w", err)
	}

	// 先写临时文件再 rename，保证任意时刻目标文件存在即内容完整（与 meta 写入顺序配合）。
	tmpPath := destPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("rename temp file: %w", err)
	}

	urlPath := "/api/export_static/" + key + fmt.Sprintf("?v=%d", version.Unix())
	if s.baseURL != "" {
		return s.baseURL + urlPath, nil
	}
	return urlPath, nil
}

// validateKey 校验 key 格式；仅允许正斜杠分隔的相对路径（如 2026/616/0.png）。
func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("storage key is empty")
	}
	if strings.Contains(key, "..") {
		return fmt.Errorf("storage key contains ..")
	}
	if strings.Contains(key, `\`) {
		return fmt.Errorf("storage key must use forward slashes")
	}
	if strings.HasPrefix(key, "/") {
		return fmt.Errorf("storage key must be relative")
	}
	rel := filepath.FromSlash(key)
	if filepath.IsAbs(rel) {
		return fmt.Errorf("storage key must be relative")
	}
	return nil
}

// resolveDestPath 将 key 解析为 baseDir 下的绝对路径，并确认结果不逃逸存储根目录。
func resolveDestPath(baseDir, key string) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}

	destPath := filepath.Join(baseDir, filepath.FromSlash(key))

	// filepath.Join 在部分平台上遇到绝对路径 key 会丢弃 baseDir，此处用 Rel 二次确认边界。
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve storage dir: %w", err)
	}
	destAbs, err := filepath.Abs(destPath)
	if err != nil {
		return "", fmt.Errorf("resolve storage path: %w", err)
	}

	relPath, err := filepath.Rel(baseAbs, destAbs)
	if err != nil {
		return "", fmt.Errorf("storage key escapes base dir: %w", err)
	}
	if relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("storage key escapes base dir")
	}

	return destPath, nil
}

// ResolveDestPath 将相对 storage key 解析为 baseDir 下的绝对路径，并校验不会逃逸出 baseDir。
func ResolveDestPath(baseDir, key string) (string, error) {
	return resolveDestPath(baseDir, key)
}
