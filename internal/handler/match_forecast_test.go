package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/common"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/scutrobotlab/rm-schedule/internal/types"
)

const forecastTestSchedule = `{"data":{"event":{"zones":{"nodes":[{
	"id":"615",
	"name":"东部赛区",
	"groupMatches":{"nodes":[
		{"id":"30988","status":"DONE","orderNumber":1,"redSide":{"player":{"team":{"id":"1","name":"Born of Fire","collegeName":"南京航空航天大学金城学院"}}},"blueSide":{"player":{"team":{"id":"2","name":"Helios","collegeName":"西南交通大学"}}}}
	]},
	"knockoutMatches":{"nodes":[
		{"id":"31056","status":"STARTED","orderNumber":68,"redSide":{"player":{"team":{"id":"3","name":"毅恒","collegeName":"北京理工大学（珠海）"}}},"blueSide":{"player":{"team":{"id":"1","name":"Born of Fire","collegeName":"南京航空航天大学金城学院"}}}}
	]}
}]}}}}`

func TestForecastRatesRoundsPercentToInteger(t *testing.T) {
	red, blue := forecastRates(MpMatchData{RedCount: 623, BlueCount: 377})

	if red.percent != 62 || blue.percent != 38 {
		t.Fatalf("percent = %d/%d, want 62/38", red.percent, blue.percent)
	}
	if red.percent+blue.percent != 100 {
		t.Fatalf("percent sum = %d, want 100", red.percent+blue.percent)
	}
}

func TestForecastRatesUnavailablePercent(t *testing.T) {
	red, blue := forecastRates(MpMatchData{})

	if red.percent != -1 || blue.percent != -1 {
		t.Fatalf("percent = %d/%d, want -1/-1", red.percent, blue.percent)
	}
}

func TestMatchForecastImageFilenameIncludesMatchID(t *testing.T) {
	setForecastTestSchedule(t)

	if got := matchForecastImageFilename("30988", true); got != "match-forecast-30988.png" {
		t.Fatalf("explicit filename = %q, want %q", got, "match-forecast-30988.png")
	}
}

func TestSelectForecastMatch(t *testing.T) {
	t.Setenv(envForecastDebugMatchID, "")
	var schedule types.ScheduleResp
	if err := json.Unmarshal([]byte(forecastTestSchedule), &schedule); err != nil {
		t.Fatal(err)
	}

	_, explicit, found := selectForecastMatch(schedule, "30988", true)
	if !found || explicit.ID != "30988" {
		t.Fatalf("explicit match = %q, found=%v; want 30988", explicit.ID, found)
	}
	_, current, found := selectForecastMatch(schedule, "", false)
	if !found || current.ID != "31056" {
		t.Fatalf("current match = %q, found=%v; want 31056", current.ID, found)
	}

	t.Setenv(envForecastDebugMatchID, "31056")
	_, explicit, found = selectForecastMatch(schedule, "30988", true)
	if !found || explicit.ID != "30988" {
		t.Fatalf("explicit match did not override debug ID: %q", explicit.ID)
	}
}

func TestMatchForecastHandlerValidatesExplicitMatchID(t *testing.T) {
	setForecastTestSchedule(t)

	app := iris.New()
	app.Get("/api/match_forecast", MatchForecastHandler)
	if err := app.Build(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{name: "empty", query: "?match_id=", wantStatus: http.StatusBadRequest},
		{name: "not integer", query: "?match_id=abc", wantStatus: http.StatusBadRequest},
		{name: "not positive", query: "?match_id=0", wantStatus: http.StatusBadRequest},
		{name: "not found", query: "?match_id=99999", wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/match_forecast"+tt.query, nil)
			app.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestMatchForecastImageHandlerValidatesExplicitMatchID(t *testing.T) {
	setForecastTestSchedule(t)

	app := iris.New()
	app.Get("/api/match_forecast_image", MatchForecastImageHandler)
	if err := app.Build(); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		query      string
		wantStatus int
	}{
		{query: "", wantStatus: http.StatusBadRequest},
		{query: "?match_id=abc", wantStatus: http.StatusBadRequest},
		{query: "?match_id=99999", wantStatus: http.StatusNotFound},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/match_forecast_image"+tt.query, nil)
		app.ServeHTTP(rec, req)
		if rec.Code != tt.wantStatus {
			t.Fatalf("query %q status = %d, want %d", tt.query, rec.Code, tt.wantStatus)
		}
	}
}

func TestMatchForecastHandlerReturnsExplicitCompletedMatch(t *testing.T) {
	setForecastTestSchedule(t)
	queriedAt := time.Date(2026, 7, 25, 12, 34, 56, 0, time.FixedZone("CST", 8*3600))
	svc.Cache.SetDefault("mp_match_rt:30988", MpMatchData{
		RedCount:  2,
		BlueCount: 1,
		QueriedAt: queriedAt,
	})
	svc.Cache.SetDefault("mp_match_rt:31056", MpMatchData{})
	t.Cleanup(func() {
		svc.Cache.Delete("mp_match_rt:30988")
		svc.Cache.Delete("mp_match_rt:31056")
	})

	app := iris.New()
	app.Get("/api/match_forecast", MatchForecastHandler)
	if err := app.Build(); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/match_forecast?match_id=30988", nil)
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp MatchForecastResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Current.HasMatch || resp.Current.MatchID != 30988 {
		t.Fatalf("current match = has_match:%v id:%d, want true/30988", resp.Current.HasMatch, resp.Current.MatchID)
	}
	if resp.SupportRateDeadline != "2026-07-25 12:34" {
		t.Fatalf("support_rate_deadline = %q", resp.SupportRateDeadline)
	}
	if resp.ZoneName != "东部赛区" || resp.ZoneID != 615 {
		t.Fatalf("zone = %q/%d, want 东部赛区/615", resp.ZoneName, resp.ZoneID)
	}
	wantImageURL := "/api/match_forecast_image?match_id=30988&v=" +
		strconv.FormatInt(queriedAt.Truncate(time.Minute).Unix(), 10)
	if resp.Current.ImageURL != wantImageURL {
		t.Fatalf("image_url = %q", resp.Current.ImageURL)
	}
	if !resp.Next.HasMatch || resp.Next.MatchID != 31056 {
		t.Fatalf("next match = has_match:%v id:%d, want true/31056", resp.Next.HasMatch, resp.Next.MatchID)
	}
	if resp.Next.ImageURL != "/api/match_forecast_image?match_id=31056" {
		t.Fatalf("next image_url = %q", resp.Next.ImageURL)
	}
}

func TestForecastImageURL(t *testing.T) {
	queriedAt := time.Date(2026, 7, 25, 12, 34, 56, 0, time.FixedZone("CST", 8*3600))
	version := strconv.FormatInt(queriedAt.Truncate(time.Minute).Unix(), 10)
	tests := []struct {
		name      string
		baseURL   string
		matchID   string
		queriedAt time.Time
		want      string
	}{
		{
			name:      "relative versioned by minute",
			matchID:   "30988",
			queriedAt: queriedAt,
			want:      "/api/match_forecast_image?match_id=30988&v=" + version,
		},
		{
			name:    "absolute",
			baseURL: "https://schedule.scutbot.cn/",
			matchID: "30988",
			want:    "https://schedule.scutbot.cn/api/match_forecast_image?match_id=30988",
		},
		{
			name:      "absolute versioned by minute",
			baseURL:   "https://schedule.scutbot.cn/",
			matchID:   "30988",
			queriedAt: queriedAt.Add(3 * time.Second),
			want:      "https://schedule.scutbot.cn/api/match_forecast_image?match_id=30988&v=" + version,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := forecastImageURL(tt.baseURL, tt.matchID, tt.queriedAt); got != tt.want {
				t.Fatalf("forecastImageURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMatchForecastImageCacheControl(t *testing.T) {
	setForecastTestSchedule(t)

	originalRender := renderForecastImage
	renderForecastImage = func(context.Context, string) ([]byte, bool, error) {
		return []byte("png"), false, nil
	}
	t.Cleanup(func() { renderForecastImage = originalRender })

	app := iris.New()
	app.Get("/api/match_forecast_image", MatchForecastImageHandler)
	if err := app.Build(); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		query string
		want  string
	}{
		{query: "?match_id=30988", want: "public, max-age=1"},
		{query: "?match_id=30988&v=1784954040", want: "public, max-age=3600"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/match_forecast_image"+tt.query, nil)
		app.ServeHTTP(rec, req)
		if got := rec.Header().Get("Cache-Control"); got != tt.want {
			t.Fatalf("query %q Cache-Control = %q, want %q", tt.query, got, tt.want)
		}
	}
}

func setForecastTestSchedule(t *testing.T) {
	t.Helper()
	svc.Cache.SetDefault(common.UpstreamNameSchedule, []byte(forecastTestSchedule))
	t.Cleanup(func() {
		svc.Cache.Delete(common.UpstreamNameSchedule)
	})
}
