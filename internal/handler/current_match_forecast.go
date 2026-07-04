package handler

import (
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/common"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/scutrobotlab/rm-schedule/internal/types"
	"github.com/sirupsen/logrus"
)

const (
	// forecastTimeLayout 与 current_match_operator.json 的 publish_time 一致，精确到秒。
	forecastTimeLayout = "2006-01-02 15:04:05"
	// matchStatusStarted 表示比赛「直播中 / 进行中」，与前端 MatchGraph 判定一致。
	matchStatusStarted = "STARTED"
)

// forecastLocation 为东八区（CST），与 current_match_operator.json 中的时间保持一致。
var forecastLocation = time.FixedZone("CST", 8*3600)

// CurrentMatchForecastResp 下发当前进行中比赛的竞猜预测。
// 变量命名与结构尽量参考 current_match_operator.json（red_side/blue_side/team_info 等）。
type CurrentMatchForecastResp struct {
	// PublishTime 本次下发时间，精确到秒。
	PublishTime string `json:"publish_time"`
	// HasMatch 当前是否存在进行中（STARTED）的比赛。
	HasMatch bool `json:"has_match"`
	// ZoneName 赛区名称。
	ZoneName string `json:"zone_name"`
	// ZoneID 赛区 ID。
	ZoneID int `json:"zone_id"`
	// OrderNumber 场次号。
	OrderNumber int `json:"order_number"`
	// Slug 比赛 slug（可能为 null 或字符串），透传自 schedule.json。
	Slug interface{} `json:"slug"`
	// MatchID 比赛 ID，即 /mp/match 使用的 matchID。
	MatchID int `json:"match_id"`
	// SupportRateDeadline 支持率查询的截止时间（该 match 从 mp.robomaster.com 查询的时刻），精确到秒。
	SupportRateDeadline string `json:"support_rate_deadline"`
	// RedSide / BlueSide 红蓝双方信息与支持率。
	RedSide  ForecastSide `json:"red_side"`
	BlueSide ForecastSide `json:"blue_side"`
}

// ForecastSide 单侧（红/蓝）的队伍信息与支持率。
type ForecastSide struct {
	TeamInfo ForecastTeamInfo `json:"team_info"`
	// SupportRate 该侧支持率（0~1），保留 3 位小数；不可用时为 -1。
	SupportRate float64 `json:"support_rate"`
	// SupportRatePercent 支持率百分数（support_rate * 100），保留 1 位小数；不可用时为 -1。
	SupportRatePercent float64 `json:"support_rate_percent"`
}

// ForecastTeamInfo 参考 current_match_operator.json 的 team_info 命名。
// zone_id / match_id 已在顶层给出，此处不再重复。
type ForecastTeamInfo struct {
	TeamID      string `json:"team_id"`
	TeamName    string `json:"team_name"`
	CollegeLogo string `json:"college_logo"`
	CollegeName string `json:"college_name"`
}

// CurrentMatchForecastHandler 下发当前进行中比赛的竞猜预测。
// 约定同一时刻只有一场比赛（不同赛区不并行开赛），取 svc.Cache 中实时 schedule
// 里第一场 status == STARTED 的比赛即可。
func CurrentMatchForecastHandler(c iris.Context) {
	resp := CurrentMatchForecastResp{
		PublishTime: time.Now().In(forecastLocation).Format(forecastTimeLayout),
		HasMatch:    false,
		Slug:        nil,
		// 无进行中比赛时也走同一装配路径，保证 support_rate 与 support_rate_percent 均为 -1，
		// 避免 support_rate_percent 默认成 0 被误读为真实的 0%。
		RedSide:  forecastSide(nil, -1.0),
		BlueSide: forecastSide(nil, -1.0),
	}

	schedule, ok := loadCachedSchedule()
	if !ok {
		c.Header("Cache-Control", "public, max-age=1")
		c.JSON(resp)
		return
	}

	zone, match, found := findStartedMatch(schedule)
	if !found {
		c.Header("Cache-Control", "public, max-age=1")
		c.JSON(resp)
		return
	}

	matchID, _ := strconv.Atoi(match.ID)
	zoneID, _ := strconv.Atoi(zone.ID)
	// 走 1s 短缓存的实时取数，尽量降低当前进行中比赛的支持率延迟。
	mp := resolveMpMatchRealtime(match.ID, matchID)

	resp.HasMatch = true
	resp.ZoneName = zone.Name
	resp.ZoneID = zoneID
	resp.OrderNumber = match.OrderNumber
	resp.Slug = match.Slug
	resp.MatchID = matchID
	if !mp.QueriedAt.IsZero() {
		resp.SupportRateDeadline = mp.QueriedAt.In(forecastLocation).Format(forecastTimeLayout)
	}
	resp.RedSide = forecastSide(match.RedSide.Player, mp.RedRate)
	resp.BlueSide = forecastSide(match.BlueSide.Player, mp.BlueRate)

	c.Header("Cache-Control", "public, max-age=1")
	c.JSON(resp)
}

// loadCachedSchedule 从 svc.Cache 读取实时 schedule 并解析。
func loadCachedSchedule() (types.ScheduleResp, bool) {
	cached, ok := svc.Cache.Get(common.UpstreamNameSchedule)
	if !ok {
		return types.ScheduleResp{}, false
	}
	scheduleBytes, ok := cached.([]byte)
	if !ok {
		return types.ScheduleResp{}, false
	}
	var schedule types.ScheduleResp
	if err := json.Unmarshal(scheduleBytes, &schedule); err != nil {
		logrus.Errorf("current_match_forecast: unmarshal schedule failed: %v", err)
		return types.ScheduleResp{}, false
	}
	return schedule, true
}

// findStartedMatch 遍历所有赛区，返回 status == STARTED 的比赛及其所在赛区。
// 按约定同一时刻只有一场进行中比赛，遇到第一场即返回。
func findStartedMatch(schedule types.ScheduleResp) (types.ZoneNode, types.MatchNode, bool) {
	for _, zone := range schedule.Data.Event.Zones.Nodes {
		for _, m := range zone.GroupMatches.Nodes {
			if m.Status == matchStatusStarted {
				return zone, m, true
			}
		}
		for _, m := range zone.KnockoutMatches.Nodes {
			if m.Status == matchStatusStarted {
				return zone, m, true
			}
		}
	}
	return types.ZoneNode{}, types.MatchNode{}, false
}

// forecastSide 组装单侧队伍信息与支持率；player 或 team 缺失时字段留空。
// support_rate 保留 3 位小数，support_rate_percent 为其 ×100 后保留 1 位小数。
// 支持率不可用（rate<0，含无进行中比赛）时，两者统一返回 -1。
func forecastSide(player *types.Player, rate float64) ForecastSide {
	var info ForecastTeamInfo
	if player != nil && player.Team != nil {
		info.TeamID = player.Team.ID
		info.TeamName = player.Team.Name
		info.CollegeLogo = player.Team.CollegeLogo
		info.CollegeName = player.Team.CollegeName
	}
	if rate < 0 {
		return ForecastSide{TeamInfo: info, SupportRate: -1, SupportRatePercent: -1}
	}
	return ForecastSide{
		TeamInfo:           info,
		SupportRate:        roundTo(rate, 3),
		SupportRatePercent: roundTo(rate*100, 1),
	}
}

// roundTo 将 v 四舍五入到 decimals 位小数。
func roundTo(v float64, decimals int) float64 {
	p := math.Pow(10, float64(decimals))
	return math.Round(v*p) / p
}
