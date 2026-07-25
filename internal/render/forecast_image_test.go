package render

import (
	"strings"
	"testing"
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
