package storage

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Store 持久化导出图片并返回可访问的下载 URL。
// key 约定为相对路径，形如 2026/616/0.png（season/zoneID/partIndex.png）。
type Store interface {
	Save(ctx context.Context, key string, data []byte) (url string, err error)
}

const (
	envStorageBackend   = "SCHEDULE_EXPORT_STORAGE_BACKEND"
	envStorageDir       = "SCHEDULE_EXPORT_STORAGE_DIR"
	envPublicBaseURL    = "SCHEDULE_EXPORT_PUBLIC_BASE_URL"
	envCOSBucket        = "SCHEDULE_EXPORT_COS_BUCKET"
	envCOSRegion        = "SCHEDULE_EXPORT_COS_REGION"
	envCOSSecretID      = "SCHEDULE_EXPORT_COS_SECRET_ID"
	envCOSSecretKey     = "SCHEDULE_EXPORT_COS_SECRET_KEY"
	envCOSDomain        = "SCHEDULE_EXPORT_COS_DOMAIN"

	defaultStorageDir    = "./data/export_images"
	defaultStorageBackend = "local"
)

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
