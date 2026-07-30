package router

import (
	"github.com/scutrobotlab/rm-schedule/internal/common"
	"github.com/scutrobotlab/rm-schedule/internal/handler"
	"github.com/scutrobotlab/rm-schedule/internal/static"
)

// 当前赛季不要加入 SeasonMap 或 StaticZoneSeasonMap。
// 腾讯 CDN 回源请求需要继续进入 RedirectRouteHandlerFactory 的 301 分支，
// 直接使用上游 OSS 数据；相比由后端解析、合并并返回大 JSON，响应速度显著更快。
// 只有历史赛季才应在这里配置静态快照。
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
	},
}
