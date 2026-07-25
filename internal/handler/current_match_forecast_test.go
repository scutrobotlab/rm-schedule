package handler

import "testing"

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
