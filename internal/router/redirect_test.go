package router

import (
	"testing"

	"github.com/scutrobotlab/rm-schedule/internal/common"
)

func TestCurrentScheduleSeasonIsNotStatic(t *testing.T) {
	param := RedirectParams[common.UpstreamNameSchedule]

	if _, ok := param.SeasonMap["2026"]; ok {
		t.Fatal("2026 schedule must not use a static season snapshot")
	}
	if _, ok := param.StaticZoneSeasonMap["2026"]; ok {
		t.Fatal("2026 schedule must not use static zone merging")
	}
}
