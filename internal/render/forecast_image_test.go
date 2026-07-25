package render

import (
	"strings"
	"testing"
	"time"
)

func TestForecastCacheKeyIncludesExplicitMatchID(t *testing.T) {
	first := forecastCacheKey(2, "30988")
	second := forecastCacheKey(2, "31056")
	if first == second {
		t.Fatalf("cache keys collide: %q", first)
	}
	if !strings.Contains(first, "match:30988") {
		t.Fatalf("cache key %q does not contain match ID", first)
	}
}

func TestForecastRenderURLIncludesExplicitMatchID(t *testing.T) {
	if got := forecastRenderURL(""); strings.Contains(got, "match_id=") {
		t.Fatalf("current render URL unexpectedly has match_id: %q", got)
	}
	if got := forecastRenderURL("30988"); !strings.HasSuffix(got, "/forecast?render=1&match_id=30988") {
		t.Fatalf("explicit render URL = %q", got)
	}
}

func TestForecastCacheTTLExpiresAtNextMinute(t *testing.T) {
	now := time.Date(2026, 7, 25, 17, 8, 12, 345_000_000, time.FixedZone("CST", 8*3600))
	if got, want := forecastCacheTTL(now), 47*time.Second+655*time.Millisecond; got != want {
		t.Fatalf("forecastCacheTTL() = %v, want %v", got, want)
	}

	nearBoundary := time.Date(2026, 7, 25, 17, 8, 59, 999_000_000, time.UTC)
	if got, want := forecastCacheTTL(nearBoundary), time.Millisecond; got != want {
		t.Fatalf("forecastCacheTTL() near boundary = %v, want %v", got, want)
	}
}
