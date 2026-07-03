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
	dir     string
	baseURL string
}

func NewLocalStore(dir, baseURL string) *LocalStore {
	return &LocalStore{
		dir:     dir,
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

func (s *LocalStore) Save(_ context.Context, key string, data []byte) (string, error) {
	destPath, err := resolveDestPath(s.dir, key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", fmt.Errorf("create storage dir: %w", err)
	}

	tmpPath := destPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("rename temp file: %w", err)
	}

	v := time.Now().Unix()
	urlPath := "/api/export_static/" + key + fmt.Sprintf("?v=%d", v)
	if s.baseURL != "" {
		return s.baseURL + urlPath, nil
	}
	return urlPath, nil
}

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

func resolveDestPath(baseDir, key string) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}

	destPath := filepath.Join(baseDir, filepath.FromSlash(key))

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
