package render

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
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

func TestForecastImageStoreServesAcrossMinuteBoundary(t *testing.T) {
	store := newForecastImageStore()
	renderedAt := time.Date(2026, 7, 25, 17, 8, 58, 0, time.UTC)
	published := store.publish("30988", []byte("old"), renderedAt.Truncate(time.Minute).Unix(), renderedAt)

	active, ok := store.get("30988", renderedAt.Add(5*time.Second), forecastMaxStale)
	if !ok || string(active.Data) != "old" {
		t.Fatalf("active = %q, ok=%v; want old/true", active.Data, ok)
	}
	if active.Version != renderedAt.Truncate(time.Minute).Unix() || published.Version != active.Version {
		t.Fatalf("version = %d, want %d", active.Version, renderedAt.Truncate(time.Minute).Unix())
	}
}

func TestForecastImageStoreStopsServingAfterFiveMinutes(t *testing.T) {
	store := newForecastImageStore()
	renderedAt := time.Date(2026, 7, 25, 17, 8, 0, 0, time.UTC)
	store.publish("30988", []byte("old"), renderedAt.Truncate(time.Minute).Unix(), renderedAt)

	if _, ok := store.get("30988", renderedAt.Add(forecastMaxStale), forecastMaxStale); !ok {
		t.Fatal("image at exact stale limit should still be serviceable")
	}
	if _, ok := store.get("30988", renderedAt.Add(forecastMaxStale+time.Nanosecond), forecastMaxStale); ok {
		t.Fatal("image older than stale limit should not be serviceable")
	}
}

func TestRefreshForecastImageKeepsActiveUntilSuccessfulSwap(t *testing.T) {
	resetForecastImageGlobals(t)
	now := time.Date(2026, 7, 25, 17, 9, 3, 0, time.UTC)
	forecastNow = func() time.Time { return now }
	oldAt := now.Add(-time.Minute)
	forecastImages.publish("30988", []byte("old"), oldAt.Truncate(time.Minute).Unix(), oldAt)

	started := make(chan struct{})
	release := make(chan struct{})
	runForecastRender = func(context.Context, float64, string) ([]byte, error) {
		close(started)
		<-release
		return []byte("new"), nil
	}
	done := make(chan error, 1)
	go func() {
		_, _, err := RefreshForecastImage(context.Background(), "30988")
		done <- err
	}()
	<-started

	img, cached, err := RenderForecastImage(context.Background(), "30988")
	if err != nil || !cached || string(img) != "old" {
		t.Fatalf("during refresh img=%q cached=%v err=%v; want old/true/nil", img, cached, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	img, cached, err = RenderForecastImage(context.Background(), "30988")
	if err != nil || !cached || string(img) != "new" {
		t.Fatalf("after refresh img=%q cached=%v err=%v; want new/true/nil", img, cached, err)
	}
}

func TestRefreshForecastImageFailureKeepsActive(t *testing.T) {
	resetForecastImageGlobals(t)
	now := time.Date(2026, 7, 25, 17, 9, 3, 0, time.UTC)
	forecastNow = func() time.Time { return now }
	oldAt := now.Add(-time.Minute)
	old := forecastImages.publish("30988", []byte("old"), oldAt.Truncate(time.Minute).Unix(), oldAt)
	runForecastRender = func(context.Context, float64, string) ([]byte, error) {
		return nil, errors.New("render failed")
	}

	if _, _, err := RefreshForecastImage(context.Background(), "30988"); err == nil {
		t.Fatal("RefreshForecastImage() error = nil, want failure")
	}
	active, ok := forecastImages.get("30988", now, forecastMaxStale)
	if !ok || active != old || string(active.Data) != "old" {
		t.Fatalf("active changed after failed refresh: %#v, ok=%v", active, ok)
	}
}

func TestRenderForecastImageRefreshesActiveOlderThanFiveMinutes(t *testing.T) {
	resetForecastImageGlobals(t)
	now := time.Date(2026, 7, 25, 17, 9, 3, 0, time.UTC)
	forecastNow = func() time.Time { return now }
	staleAt := now.Add(-forecastMaxStale - time.Second)
	forecastImages.publish("30988", []byte("stale"), staleAt.Truncate(time.Minute).Unix(), staleAt)
	var calls atomic.Int32
	runForecastRender = func(context.Context, float64, string) ([]byte, error) {
		calls.Add(1)
		return []byte("fresh"), nil
	}

	img, cached, err := RenderForecastImage(context.Background(), "30988")
	if err != nil || cached || string(img) != "fresh" {
		t.Fatalf("img=%q cached=%v err=%v; want fresh/false/nil", img, cached, err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("render calls = %d, want 1", got)
	}
	active, ok := forecastImages.get("30988", now, forecastMaxStale)
	if !ok || active.Version != now.Truncate(time.Minute).Unix() {
		t.Fatalf("published version = %d, ok=%v; want leader version %d", active.Version, ok, now.Truncate(time.Minute).Unix())
	}
}

func TestForecastRefreshKeepsTargetVersionWhenRenderCrossesMinute(t *testing.T) {
	resetForecastImageGlobals(t)
	now := time.Date(2026, 7, 25, 17, 9, 58, 0, time.UTC)
	forecastNow = func() time.Time { return now }
	leaderVersion := now.Truncate(time.Minute).Unix()
	var calls atomic.Int32
	runForecastRender = func(context.Context, float64, string) ([]byte, error) {
		calls.Add(1)
		now = now.Add(time.Minute)
		return []byte("png"), nil
	}

	if _, _, err := refreshForecastImage(context.Background(), "30988", leaderVersion); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("render calls = %d, want 1", got)
	}
	active, ok := forecastImages.get("30988", now, forecastMaxStale)
	if !ok || active.Version != leaderVersion {
		t.Fatalf("published version = %d, ok=%v; want target-minute %d", active.Version, ok, leaderVersion)
	}
}

func TestForecastRefreshRetriesWhenCoalescedFlightPublishesOlderVersion(t *testing.T) {
	resetForecastImageGlobals(t)
	now := time.Date(2026, 7, 25, 17, 9, 58, 0, time.UTC)
	forecastNow = func() time.Time { return now }
	started := make(chan struct{})
	release := make(chan struct{})
	secondJoined := make(chan struct{})
	var secondJoinedOnce sync.Once
	leaderVersion := now.Truncate(time.Minute).Unix()
	nextVersion := now.Add(time.Minute).Truncate(time.Minute).Unix()
	forecastFlightJoined = func(version int64) {
		if version == nextVersion {
			secondJoinedOnce.Do(func() { close(secondJoined) })
		}
	}
	var calls atomic.Int32
	runForecastRender = func(context.Context, float64, string) ([]byte, error) {
		n := calls.Add(1)
		if n == 1 {
			close(started)
			<-release
			return []byte("old-minute"), nil
		}
		return []byte("new-minute"), nil
	}

	done := make(chan error, 2)
	go func() {
		_, _, err := refreshForecastImage(context.Background(), "30988", leaderVersion)
		done <- err
	}()
	<-started
	// 新分钟的调用并入旧 flight；旧图发布后应自动再刷一次新分钟版本。
	now = now.Add(time.Minute)
	go func() {
		_, _, err := refreshForecastImage(context.Background(), "30988", nextVersion)
		done <- err
	}()
	<-secondJoined
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("render calls = %d, want 2 (retry after stale coalesce)", got)
	}
	active, ok := forecastImages.get("30988", now, forecastMaxStale)
	if !ok || active.Version != nextVersion || string(active.Data) != "new-minute" {
		t.Fatalf("active=%q version=%d ok=%v; want new-minute/%d", active.Data, active.Version, ok, nextVersion)
	}
}

func TestForecastImageStoreCleansOnlyIdleNonTargets(t *testing.T) {
	store := newForecastImageStore()
	now := time.Date(2026, 7, 25, 17, 9, 0, 0, time.UTC)
	oldAt := now.Add(-10 * time.Minute)
	store.publish("current", []byte("a"), oldAt.Truncate(time.Minute).Unix(), oldAt)
	store.publish("old", []byte("b"), oldAt.Truncate(time.Minute).Unix(), oldAt)
	store.cleanup(map[string]struct{}{"current": {}}, now)

	if _, ok := store.entries["current"]; !ok {
		t.Fatal("target image was removed")
	}
	if _, ok := store.entries["old"]; ok {
		t.Fatal("idle non-target image was not removed")
	}
}

func resetForecastImageGlobals(t *testing.T) {
	t.Helper()
	originalImages := forecastImages
	originalErrors := forecastErrorCache
	originalNow := forecastNow
	originalRender := runForecastRender
	originalFlightJoined := forecastFlightJoined
	forecastImages = newForecastImageStore()
	forecastErrorCache = cache.New(forecastErrorCacheTTL, time.Minute)
	t.Cleanup(func() {
		forecastImages = originalImages
		forecastErrorCache = originalErrors
		forecastNow = originalNow
		runForecastRender = originalRender
		forecastFlightJoined = originalFlightJoined
	})
}
