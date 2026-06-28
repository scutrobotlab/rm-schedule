package router

import (
	"github.com/scutrobotlab/rm-schedule/internal/common"
	"github.com/scutrobotlab/rm-schedule/internal/handler"
	"github.com/scutrobotlab/rm-schedule/internal/static"
)

var archived2026RegionalZoneIDs = map[string]struct{}{
	"614": {},
	"615": {},
	"616": {},
}

// RedirectParams 定义重定向路由的参数
var RedirectParams = map[string]handler.RedirectRouteHandlerParam{
	common.UpstreamNameGroupRankInfo: {
		Name:         common.UpstreamNameGroupRankInfo,
		Static:       false,
		CacheControl: "public, max-age=5",
		OriginalUrl:  common.UpstreamUrlGroupRankInfo,
		Data:         static.GroupRankInfoBytes,
		SeasonMap: map[string][]byte{
			"2024": static.GroupRankInfoBytes2024,
			"2025": static.GroupRankInfoBytes2025,
		},
		StaticZoneSeasonMap: map[string]handler.StaticZoneSeason{
			"2026": {
				Data:        static.GroupRankInfoBytes2026,
				ZoneIDs:     archived2026RegionalZoneIDs,
				ZonePath:    []string{"zones"},
				ZoneIDField: "zoneId",
			},
		},
	},
	common.UpstreamNameRobotData: {
		Name:         common.UpstreamNameRobotData,
		Static:       false,
		CacheControl: "public, max-age=5",
		OriginalUrl:  common.UpstreamUrlRobotData,
		Data:         static.RobotDataBytes,
		SeasonMap: map[string][]byte{
			"2025": static.RobotDataBytes2025,
		},
		StaticZoneSeasonMap: map[string]handler.StaticZoneSeason{
			"2026": {
				Data:        static.RobotDataBytes2026,
				ZoneIDs:     archived2026RegionalZoneIDs,
				ZonePath:    []string{"zones"},
				ZoneIDField: "zoneId",
			},
		},
	},
	common.UpstreamNameSchedule: {
		Name:         common.UpstreamNameSchedule,
		Static:       false,
		CacheControl: "public, max-age=5",
		OriginalUrl:  common.UpstreamUrlSchedule,
		Data:         static.ScheduleBytes,
		SeasonMap: map[string][]byte{
			"2024": static.ScheduleBytes2024,
			"2025": static.ScheduleBytes2025,
		},
		StaticZoneSeasonMap: map[string]handler.StaticZoneSeason{
			"2026": {
				Data:        static.ScheduleBytes2026,
				ZoneIDs:     archived2026RegionalZoneIDs,
				ZonePath:    []string{"data", "event", "zones", "nodes"},
				ZoneIDField: "id",
			},
		},
	},
}
