package handler

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/scutrobotlab/rm-schedule/internal/types"
)

func TestForecastPrewarmMatchIDsSelectsCurrentAndNext(t *testing.T) {
	t.Setenv(envForecastDebugMatchID, "")
	schedule := types.ScheduleResp{}
	schedule.Data.Event.Zones.Nodes = []types.ZoneNode{{
		ID: "618",
		GroupMatches: types.Matches{Nodes: []types.MatchNode{
			{ID: "31451", OrderNumber: 1, Status: matchStatusStarted},
			{ID: "31452", OrderNumber: 2, Status: "WAITING"},
			{ID: "31453", OrderNumber: 3, Status: "WAITING"},
		}},
	}}

	if got, want := forecastPrewarmMatchIDs(schedule), []string{"31451", "31452"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("match IDs = %v, want %v", got, want)
	}
}

func TestForecastPrewarmMatchIDsSelectsUpcomingWithoutCurrent(t *testing.T) {
	t.Setenv(envForecastDebugMatchID, "")
	schedule := types.ScheduleResp{}
	schedule.Data.Event.Zones.Nodes = []types.ZoneNode{{
		ID: "618",
		GroupMatches: types.Matches{Nodes: []types.MatchNode{
			{ID: "31452", OrderNumber: 2, PlanStartedAt: "2026-08-05T04:00:00Z", Status: "WAITING"},
			{ID: "31451", OrderNumber: 1, PlanStartedAt: "2026-08-05T03:00:00Z", Status: "DONE"},
		}},
	}}

	if got, want := forecastPrewarmMatchIDs(schedule), []string{"31452"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("match IDs = %v, want %v", got, want)
	}
}

func TestForecastPrewarmMatchIDsHonorsDebugMatch(t *testing.T) {
	t.Setenv(envForecastDebugMatchID, "31450")
	schedule := types.ScheduleResp{}
	schedule.Data.Event.Zones.Nodes = []types.ZoneNode{{
		ID: "618",
		GroupMatches: types.Matches{Nodes: []types.MatchNode{
			{ID: "31450", OrderNumber: 1, Status: "DONE"},
			{ID: "31451", OrderNumber: 2, Status: matchStatusStarted},
			{ID: "31452", OrderNumber: 3, Status: "WAITING"},
		}},
	}}

	if got, want := forecastPrewarmMatchIDs(schedule), []string{"31450", "31451"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("match IDs = %v, want %v", got, want)
	}
}

func TestCheckAndPrewarmForecastImagesWarmsOnceAndRetriesAfterInterval(t *testing.T) {
	setForecastTestSchedule(t)
	resetForecastPrewarmGlobals(t)
	t.Setenv(envForecastDebugMatchID, "")
	now := time.Date(2026, 8, 5, 6, 35, 2, 0, time.UTC)
	forecastPrewarmNow = func() time.Time { return now }

	var mu sync.Mutex
	versions := make(map[string]int64)
	var calls []string
	prewarmedImageVersion = func(matchID string) (int64, bool) {
		mu.Lock()
		defer mu.Unlock()
		version, ok := versions[matchID]
		return version, ok
	}
	refreshForecastImage = func(_ context.Context, matchID string) ([]byte, bool, error) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, matchID)
		versions[matchID] = now.Truncate(time.Minute).Unix()
		return []byte("png"), false, nil
	}

	CheckAndPrewarmForecastImages()
	CheckAndPrewarmForecastImages()
	mu.Lock()
	got := append([]string(nil), calls...)
	mu.Unlock()
	if want := []string{"31056"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("render calls = %v, want %v", got, want)
	}

	mu.Lock()
	delete(versions, "31056")
	mu.Unlock()
	now = now.Add(10 * time.Second)
	CheckAndPrewarmForecastImages()
	mu.Lock()
	callCount := len(calls)
	mu.Unlock()
	if callCount != 1 {
		t.Fatalf("render calls before retry interval = %d, want 1", callCount)
	}

	now = now.Add(6 * time.Second)
	CheckAndPrewarmForecastImages()
	mu.Lock()
	callCount = len(calls)
	mu.Unlock()
	if callCount != 2 {
		t.Fatalf("render calls after retry interval = %d, want 2", callCount)
	}
}

func TestCheckAndPrewarmForecastImagesSkipsOverlappingRun(t *testing.T) {
	setForecastTestSchedule(t)
	resetForecastPrewarmGlobals(t)
	forecastPrewarmRunning.Store(true)
	var calls int
	refreshForecastImage = func(context.Context, string) ([]byte, bool, error) {
		calls++
		return nil, false, nil
	}

	CheckAndPrewarmForecastImages()
	if calls != 0 {
		t.Fatalf("render calls = %d, want 0", calls)
	}
}

func TestForecastPrewarmRetryIntervalAppliesAcrossMinuteBoundary(t *testing.T) {
	resetForecastPrewarmGlobals(t)
	first := time.Date(2026, 8, 5, 6, 35, 59, 0, time.UTC)
	if !shouldAttemptForecastPrewarm("31056", first) {
		t.Fatal("first attempt was rejected")
	}
	if shouldAttemptForecastPrewarm("31056", first.Add(5*time.Second)) {
		t.Fatal("retry was allowed before 15 seconds across minute boundary")
	}
	if !shouldAttemptForecastPrewarm("31056", first.Add(forecastPrewarmRetryInterval)) {
		t.Fatal("retry was not allowed at 15 seconds")
	}
}

func resetForecastPrewarmGlobals(t *testing.T) {
	t.Helper()
	originalNow := forecastPrewarmNow
	originalRefresh := refreshForecastImage
	originalVersion := prewarmedImageVersion
	originalCleanup := cleanupForecastImages
	forecastPrewarmMu.Lock()
	originalAttempts := forecastPrewarmAttempts
	forecastPrewarmAttempts = make(map[string]forecastPrewarmAttempt)
	forecastPrewarmMu.Unlock()
	forecastPrewarmRunning.Store(false)
	cleanupForecastImages = func([]string) {}
	t.Cleanup(func() {
		forecastPrewarmNow = originalNow
		refreshForecastImage = originalRefresh
		prewarmedImageVersion = originalVersion
		cleanupForecastImages = originalCleanup
		forecastPrewarmMu.Lock()
		forecastPrewarmAttempts = originalAttempts
		forecastPrewarmMu.Unlock()
		forecastPrewarmRunning.Store(false)
	})
}
