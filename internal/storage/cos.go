package storage

import (
	"context"
	"errors"
)

var ErrCosNotImplemented = errors.New("cos storage backend is not implemented yet")

// CosConfig 腾讯云 COS 配置，当前为占位骨架。
type CosConfig struct {
	Bucket    string
	Region    string
	SecretID  string
	SecretKey string
	Domain    string
}

// CosStore 预留的腾讯云 COS 实现，尚未接入 SDK。
type CosStore struct {
	cfg CosConfig
}

func NewCosStore(cfg CosConfig) *CosStore {
	return &CosStore{cfg: cfg}
}

// Save 尚未实现；配置 cos 后端时进程可正常启动，首次写入会返回 ErrCosNotImplemented。
func (s *CosStore) Save(_ context.Context, _ string, _ []byte) (string, error) {
	return "", ErrCosNotImplemented
}
