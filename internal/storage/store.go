package storage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// Store 持久化导出图片并返回可访问的下载 URL。
// key 约定为相对路径，形如 2026/616/0.png（season/zoneID/partIndex.png）。
// version 由调用方统一生成，用于 URL cache busting（如 ?v=）并与 meta.updated_at 保持一致。
type Store interface {
	Save(ctx context.Context, key string, data []byte, version time.Time) (url string, err error)
}

const (
	envStorageBackend = "SCHEDULE_EXPORT_STORAGE_BACKEND"
	envStorageDir     = "SCHEDULE_EXPORT_STORAGE_DIR"
	// envPublicBaseURL 为本服务公网域名前缀，导出图片 URL 与 college_logo 绝对化共用同一变量。
	envPublicBaseURL = "SCHEDULE_PUBLIC_BASE_URL"
	envCOSBucket     = "SCHEDULE_EXPORT_COS_BUCKET"
	envCOSRegion     = "SCHEDULE_EXPORT_COS_REGION"
	envCOSSecretID   = "SCHEDULE_EXPORT_COS_SECRET_ID"
	envCOSSecretKey  = "SCHEDULE_EXPORT_COS_SECRET_KEY"
	envCOSDomain     = "SCHEDULE_EXPORT_COS_DOMAIN"

	defaultStorageDir     = "./data/export_images"
	defaultStorageBackend = "local"
)

// EnvStorageDir 返回本地存储目录（SCHEDULE_EXPORT_STORAGE_DIR，默认 ./data/export_images）。
func EnvStorageDir() string {
	if dir := strings.TrimSpace(os.Getenv(envStorageDir)); dir != "" {
		return dir
	}
	return defaultStorageDir
}

// EnvStorageBackend 返回存储后端名称（SCHEDULE_EXPORT_STORAGE_BACKEND，默认 local）。
func EnvStorageBackend() string {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv(envStorageBackend)))
	if backend == "" {
		return defaultStorageBackend
	}
	return backend
}

// EnvPublicBaseURL 返回本服务公网域名前缀（SCHEDULE_PUBLIC_BASE_URL，默认空即相对路径）。
func EnvPublicBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv(envPublicBaseURL)), "/")
}

// NewStoreFromEnv 按 SCHEDULE_EXPORT_STORAGE_BACKEND 选择存储实现（local | cos，默认 local）。
func NewStoreFromEnv() (Store, error) {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv(envStorageBackend)))
	if backend == "" {
		backend = defaultStorageBackend
	}

	switch backend {
	case "local":
		dir := os.Getenv(envStorageDir)
		if dir == "" {
			dir = defaultStorageDir
		}
		baseURL := os.Getenv(envPublicBaseURL)
		return NewLocalStore(dir, baseURL), nil
	case "cos":
		return NewCosStore(CosConfig{
			Bucket:    os.Getenv(envCOSBucket),
			Region:    os.Getenv(envCOSRegion),
			SecretID:  os.Getenv(envCOSSecretID),
			SecretKey: os.Getenv(envCOSSecretKey),
			Domain:    os.Getenv(envCOSDomain),
		}), nil
	default:
		return nil, fmt.Errorf("unknown storage backend %q", backend)
	}
}
