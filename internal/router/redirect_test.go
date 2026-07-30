package router

import (
	"testing"

	"github.com/scutrobotlab/rm-schedule/internal/common"
)

func TestCurrentSeasonRedirectRoutesAreNotStatic(t *testing.T) {
	names := []string{
		common.UpstreamNameGroupRankInfo,
		common.UpstreamNameRobotData,
		common.UpstreamNameSchedule,
	}

	for _, name := range names {
		param := RedirectParams[name]
		if _, ok := param.SeasonMap["2026"]; ok {
			t.Fatalf("2026 %s must not use a static season snapshot", name)
		}
		if _, ok := param.StaticZoneSeasonMap["2026"]; ok {
			t.Fatalf("2026 %s must not use static zone merging", name)
		}
	}
}
