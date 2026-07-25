package handler

import (
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/common"
	"github.com/scutrobotlab/rm-schedule/internal/storage"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/scutrobotlab/rm-schedule/internal/types"
	"github.com/sirupsen/logrus"
)

const (
	// forecastTimeLayout 与 current_match_operator.json 的 publish_time 一致，精确到秒。
	forecastTimeLayout = "2006-01-02 15:04:05"
	// matchStatusStarted 表示比赛「直播中 / 进行中」，与前端 MatchGraph 判定一致。
	matchStatusStarted = "STARTED"
	// envForecastDebugMatchID 调试用：手动指定「进行中」的 match_id（按 schedule 中 MatchNode.id
	// 匹配，不限 status）。设置后覆盖 STARTED 自动探测，便于无正式进行中比赛时联调。
	envForecastDebugMatchID = "SCHEDULE_FORECAST_DEBUG_MATCH_ID"
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
	// SupportRate 该侧支持率（0~1，已排除平局票、红蓝之和恒为 1.000），保留 3 位小数；不可用时为 -1。
	SupportRate float64 `json:"support_rate"`
	// SupportRatePercent 支持率百分数（support_rate * 100，红蓝之和恒为 100），取整到 1%；不可用时为 -1。
	SupportRatePercent int `json:"support_rate_percent"`
}

// ForecastTeamInfo 参考 current_match_operator.json 的 team_info 命名。
// zone_id / match_id 已在顶层给出，此处不再重复。
type ForecastTeamInfo struct {
	TeamID   string `json:"team_id"`
	TeamName string `json:"team_name"`
	// CollegeLogo 为绝对 URL：已是绝对地址原样下发；原始相对路径会拼上
	// SCHEDULE_PUBLIC_BASE_URL（未配置则保持相对路径），不做上游 CDN 还原。
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
		// nil player 的 logo 恒为空，baseURL 不影响结果，故此处传 ""。
		RedSide:  forecastSide(nil, -1, -1, ""),
		BlueSide: forecastSide(nil, -1, -1, ""),
	}

	schedule, ok := loadCachedSchedule()
	if !ok {
		c.Header("Cache-Control", "public, max-age=1")
		c.JSON(resp)
		return
	}

	zone, match, found := selectForecastMatch(schedule)
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
	// 排除平局票后归一化，保证红蓝 support_rate 之和为 1.000、百分数之和为 100。
	red, blue := forecastRates(mp)
	baseURL := storage.EnvPublicBaseURL()
	resp.RedSide = forecastSide(match.RedSide.Player, red.rate, red.percent, baseURL)
	resp.BlueSide = forecastSide(match.BlueSide.Player, blue.rate, blue.percent, baseURL)

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

// selectForecastMatch 选出用于竞猜下发的「当前比赛」：
// 若设置了 SCHEDULE_FORECAST_DEBUG_MATCH_ID，则按该 match_id 定位（不限 status，供调试）；
// 否则按约定取第一场 status == STARTED 的比赛。
func selectForecastMatch(schedule types.ScheduleResp) (types.ZoneNode, types.MatchNode, bool) {
	if debugID := strings.TrimSpace(os.Getenv(envForecastDebugMatchID)); debugID != "" {
		zone, match, found := findMatchByID(schedule, debugID)
		if !found {
			logrus.Warnf("current_match_forecast: debug match_id %q not found in schedule", debugID)
		}
		return zone, match, found
	}
	return findStartedMatch(schedule)
}

// findMatchByID 遍历所有赛区，按 MatchNode.id 精确匹配（不限 status）。
func findMatchByID(schedule types.ScheduleResp, id string) (types.ZoneNode, types.MatchNode, bool) {
	for _, zone := range schedule.Data.Event.Zones.Nodes {
		for _, m := range zone.GroupMatches.Nodes {
			if m.ID == id {
				return zone, m, true
			}
		}
		for _, m := range zone.KnockoutMatches.Nodes {
			if m.ID == id {
				return zone, m, true
			}
		}
	}
	return types.ZoneNode{}, types.MatchNode{}, false
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

// sideRate 保存单侧最终的支持率与百分数。
type sideRate struct {
	rate    float64
	percent int
}

// forecastRates 由 mp 支持率数据计算红蓝双方的支持率与百分数：
// 排除平局票、按红蓝票数归一化，并让一侧四舍五入、另一侧取补，
// 从而保证 red.rate + blue.rate == 1.000、red.percent + blue.percent == 100。
// 无有效红蓝票（redCount+blueCount<=0，含拉取失败或零票）时两侧均为 -1。
func forecastRates(mp MpMatchData) (red, blue sideRate) {
	denom := mp.RedCount + mp.BlueCount
	if denom <= 0 {
		return sideRate{-1, -1}, sideRate{-1, -1}
	}
	redRate := roundTo(float64(mp.RedCount)/float64(denom), 3)
	redPct := int(math.Round(redRate * 100))
	return sideRate{redRate, redPct}, sideRate{roundTo(1.0-redRate, 3), 100 - redPct}
}

// resolveCollegeLogo 输出绝对 logo URL：已是绝对 URL 原样返回；
// 相对路径拼上 baseURL（本服务公网域名）；baseURL 为空或 raw 为空时按原值返回。
func resolveCollegeLogo(raw, baseURL string) string {
	if raw == "" || strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	if baseURL == "" {
		return raw
	}
	if !strings.HasPrefix(raw, "/") {
		return baseURL + "/" + raw
	}
	return baseURL + raw
}

// forecastSide 组装单侧队伍信息与支持率；player 或 team 缺失时字段留空。
func forecastSide(player *types.Player, rate float64, percent int, baseURL string) ForecastSide {
	var info ForecastTeamInfo
	if player != nil && player.Team != nil {
		info.TeamID = player.Team.ID
		info.TeamName = player.Team.Name
		info.CollegeLogo = resolveCollegeLogo(player.Team.CollegeLogo, baseURL)
		info.CollegeName = player.Team.CollegeName
	}
	return ForecastSide{TeamInfo: info, SupportRate: rate, SupportRatePercent: percent}
}

// roundTo 将 v 四舍五入到 decimals 位小数。
func roundTo(v float64, decimals int) float64 {
	p := math.Pow(10, float64(decimals))
	return math.Round(v*p) / p
}
