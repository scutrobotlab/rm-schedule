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
	Enabled        bool
	StorageDir     string
	PublicBaseURL  string
	RenderCooldown time.Duration
	Scale          float64
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
		PublicBaseURL:  strings.TrimSpace(os.Getenv("SCHEDULE_EXPORT_PUBLIC_BASE_URL")),
		RenderCooldown: cooldown,
		Scale:          scale,
	}
}
