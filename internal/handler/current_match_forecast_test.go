package handler

import (
	"testing"

	"github.com/scutrobotlab/rm-schedule/internal/common"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
)

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

func TestCurrentForecastImageFilenameIncludesMatchID(t *testing.T) {
	const schedule = `{"data":{"event":{"zones":{"nodes":[{"id":"615","groupMatches":{"nodes":[{"id":"31056","status":"STARTED"}]},"knockoutMatches":{"nodes":[]}}]}}}}`
	svc.Cache.SetDefault(common.UpstreamNameSchedule, []byte(schedule))
	t.Cleanup(func() {
		svc.Cache.Delete(common.UpstreamNameSchedule)
	})

	if got := currentForecastImageFilename(); got != "current-match-forecast-31056.png" {
		t.Fatalf("filename = %q, want %q", got, "current-match-forecast-31056.png")
	}
}

func TestForecastImageURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "relative", want: "/api/current_match_forecast_image"},
		{name: "absolute", baseURL: "https://schedule.scutbot.cn/", want: "https://schedule.scutbot.cn/api/current_match_forecast_image"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := forecastImageURL(tt.baseURL); got != tt.want {
				t.Fatalf("forecastImageURL(%q) = %q, want %q", tt.baseURL, got, tt.want)
			}
		})
	}
}
