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

const staticScheduleHash = "static"

type MetaFile struct {
	Season       int       `json:"season"`
	ZoneID       int       `json:"zone_id"`
	Group        int       `json:"group"`
	ScheduleHash string    `json:"schedule_hash"`
	UpdatedAt    time.Time `json:"updated_at"`
	Scale        float64   `json:"scale"`
	Static       bool      `json:"static"`
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
