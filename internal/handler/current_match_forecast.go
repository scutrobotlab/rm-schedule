package handler

import (
	"encoding/json"
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
	// SupportRate 该侧支持率（0~1）；不可用时为 -1。
	SupportRate float64 `json:"support_rate"`
}

// ForecastTeamInfo 参考 current_match_operator.json 的 team_info 命名。
type ForecastTeamInfo struct {
	TeamID      string `json:"team_id"`
	TeamName    string `json:"team_name"`
	CollegeLogo string `json:"college_logo"`
	CollegeName string `json:"college_name"`
	ZoneID      int    `json:"zone_id"`
	MatchID     int    `json:"match_id"`
}

// CurrentMatchForecastHandler 下发当前进行中比赛的竞猜预测。
// 进行中比赛取自 svc.Cache 中的实时 schedule（status == STARTED），多场并行时取第一场。
func CurrentMatchForecastHandler(c iris.Context) {
	resp := CurrentMatchForecastResp{
		PublishTime: time.Now().In(forecastLocation).Format(forecastTimeLayout),
		HasMatch:    false,
		Slug:        nil,
		RedSide:     ForecastSide{SupportRate: -1.0},
		BlueSide:    ForecastSide{SupportRate: -1.0},
	}

	schedule, ok := loadCachedSchedule()
	if !ok {
		c.Header("Cache-Control", "public, max-age=5")
		c.JSON(resp)
		return
	}

	zone, match, found := findStartedMatch(schedule)
	if !found {
		c.Header("Cache-Control", "public, max-age=5")
		c.JSON(resp)
		return
	}

	matchID, _ := strconv.Atoi(match.ID)
	zoneID, _ := strconv.Atoi(zone.ID)
	mp := resolveMpMatch(match.ID, matchID)

	resp.HasMatch = true
	resp.ZoneName = zone.Name
	resp.ZoneID = zoneID
	resp.OrderNumber = match.OrderNumber
	resp.Slug = match.Slug
	resp.MatchID = matchID
	if !mp.QueriedAt.IsZero() {
		resp.SupportRateDeadline = mp.QueriedAt.In(forecastLocation).Format(forecastTimeLayout)
	}
	resp.RedSide = forecastSide(match.RedSide.Player, zoneID, matchID, mp.RedRate)
	resp.BlueSide = forecastSide(match.BlueSide.Player, zoneID, matchID, mp.BlueRate)

	c.Header("Cache-Control", "public, max-age=5")
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

// findStartedMatch 遍历所有赛区，返回第一场 status == STARTED 的比赛及其所在赛区。
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
func forecastSide(player *types.Player, zoneID, matchID int, rate float64) ForecastSide {
	info := ForecastTeamInfo{ZoneID: zoneID, MatchID: matchID}
	if player != nil && player.Team != nil {
		info.TeamID = player.Team.ID
		info.TeamName = player.Team.Name
		info.CollegeLogo = player.Team.CollegeLogo
		info.CollegeName = player.Team.CollegeName
	}
	return ForecastSide{TeamInfo: info, SupportRate: rate}
}
