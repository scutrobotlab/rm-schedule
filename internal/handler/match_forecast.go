package handler

import (
	"encoding/json"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/common"
	"github.com/scutrobotlab/rm-schedule/internal/render"
	"github.com/scutrobotlab/rm-schedule/internal/storage"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/scutrobotlab/rm-schedule/internal/types"
	"github.com/sirupsen/logrus"
)

const (
	// forecastTimeLayout 与 current_match_operator.json 的 publish_time 一致，精确到秒。
	forecastTimeLayout = "2006-01-02 15:04:05"
	// forecastDeadlineLayout 为支持率截止时间格式。预测数据按分钟更新，不下发无意义的秒。
	forecastDeadlineLayout = "2006-01-02 15:04"
	// matchStatusStarted 表示比赛「直播中 / 进行中」，与前端 MatchGraph 判定一致。
	matchStatusStarted = "STARTED"
	// envForecastDebugMatchID 调试用：手动指定「进行中」的 match_id（按 schedule 中 MatchNode.id
	// 匹配，不限 status）。设置后覆盖 STARTED 自动探测，便于无正式进行中比赛时联调。
	envForecastDebugMatchID = "SCHEDULE_FORECAST_DEBUG_MATCH_ID"
	// forecastImagePath 为比赛预测图的下载接口路径。
	forecastImagePath = "/api/match_forecast_image"
)

// forecastLocation 为东八区（CST），与 current_match_operator.json 中的时间保持一致。
var forecastLocation = time.FixedZone("CST", 8*3600)

var forecastImageVersion = render.ForecastImageVersion

// MatchForecastResp 同时下发当前场次及其下一场的竞猜预测。
type MatchForecastResp struct {
	PublishTime         string        `json:"publish_time"`
	SupportRateDeadline string        `json:"support_rate_deadline"`
	ZoneName            string        `json:"zone_name"`
	ZoneID              int           `json:"zone_id"`
	Current             MatchForecast `json:"current"`
	Next                MatchForecast `json:"next"`
}

// MatchForecast 下发单场比赛的竞猜预测。
// 变量命名与结构尽量参考 current_match_operator.json（red_side/blue_side/team_info 等）。
type MatchForecast struct {
	// HasMatch 是否成功选中比赛；未传 match_id 时表示是否存在进行中比赛。
	HasMatch bool `json:"has_match"`
	// OrderNumber 场次号。
	OrderNumber int `json:"order_number"`
	// Slug 比赛 slug（可能为 null 或字符串），透传自 schedule.json。
	Slug interface{} `json:"slug"`
	// MatchID 比赛 ID，即 /mp/match 使用的 matchID。
	MatchID int `json:"match_id"`
	// ImageURL 比赛预测图的下载地址。
	ImageURL string `json:"image_url"`
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

// MatchForecastHandler 下发指定比赛或当前进行中比赛的竞猜预测。
// 显式 match_id 可选择当前赛季任意状态的比赛；未传时取第一场 status == STARTED 的比赛。
func MatchForecastHandler(c iris.Context) {
	requestedMatchID, explicit, err := parseForecastMatchID(c)
	if err != nil {
		writeForecastError(c, iris.StatusBadRequest, err.Error())
		return
	}

	baseURL := storage.EnvPublicBaseURL()
	resp := MatchForecastResp{
		PublishTime: time.Now().In(forecastLocation).Format(forecastTimeLayout),
		Current:     emptyMatchForecast(),
		Next:        emptyMatchForecast(),
	}

	schedule, ok := loadCachedSchedule()
	if !ok {
		if explicit {
			writeForecastError(c, iris.StatusServiceUnavailable, "schedule unavailable")
			return
		}
		c.Header("Cache-Control", "public, max-age=1")
		c.JSON(resp)
		return
	}

	zone, match, found := selectForecastMatch(schedule, requestedMatchID, explicit)
	if !found {
		if explicit {
			writeForecastError(c, iris.StatusNotFound, "match_id not found")
			return
		}
		if nextZone, nextMatch, nextFound := findFirstUpcomingMatch(schedule); nextFound {
			resp.ZoneName = nextZone.Name
			resp.ZoneID, _ = strconv.Atoi(nextZone.ID)
			resp.Next, _ = buildMatchForecast(nextMatch, baseURL)
		}
		c.Header("Cache-Control", "public, max-age=1")
		c.JSON(resp)
		return
	}

	resp.ZoneName = zone.Name
	resp.ZoneID, _ = strconv.Atoi(zone.ID)
	var currentQueriedAt, nextQueriedAt time.Time
	resp.Current, currentQueriedAt = buildMatchForecast(match, baseURL)
	if nextMatch, nextFound := findNextMatch(zone, match); nextFound {
		resp.Next, nextQueriedAt = buildMatchForecast(nextMatch, baseURL)
	}
	deadline := currentQueriedAt
	if nextQueriedAt.After(deadline) {
		deadline = nextQueriedAt
	}
	if !deadline.IsZero() {
		resp.SupportRateDeadline = deadline.In(forecastLocation).Format(forecastDeadlineLayout)
	}

	c.Header("Cache-Control", "public, max-age=1")
	c.JSON(resp)
}

func emptyMatchForecast() MatchForecast {
	return MatchForecast{
		HasMatch: false,
		Slug:     nil,
		ImageURL: "",
		// 无进行中比赛时也走同一装配路径，保证 support_rate 与 support_rate_percent 均为 -1，
		// 避免 support_rate_percent 默认成 0 被误读为真实的 0%。
		// nil player 的 logo 恒为空，baseURL 不影响结果，故此处传 ""。
		RedSide:  forecastSide(nil, -1, -1, ""),
		BlueSide: forecastSide(nil, -1, -1, ""),
	}
}

func buildMatchForecast(match types.MatchNode, baseURL string) (MatchForecast, time.Time) {
	matchID, _ := strconv.Atoi(match.ID)
	// 走 1s 短缓存的实时取数，尽量降低当前进行中比赛的支持率延迟。
	mp := resolveMpMatchRealtime(match.ID, matchID)

	resp := emptyMatchForecast()
	resp.HasMatch = true
	resp.OrderNumber = match.OrderNumber
	resp.Slug = match.Slug
	resp.MatchID = matchID
	version, _ := forecastImageVersion(match.ID)
	resp.ImageURL = forecastImageURL(baseURL, match.ID, version)
	// 排除平局票后归一化，保证红蓝 support_rate 之和为 1.000、百分数之和为 100。
	red, blue := forecastRates(mp)
	resp.RedSide = forecastSide(match.RedSide.Player, red.rate, red.percent, baseURL)
	resp.BlueSide = forecastSide(match.BlueSide.Player, blue.rate, blue.percent, baseURL)
	return resp, mp.QueriedAt
}

// parseForecastMatchID 读取可选 match_id；显式参数必须为正整数。
func parseForecastMatchID(c iris.Context) (matchID string, explicit bool, err error) {
	if _, explicit = c.Request().URL.Query()["match_id"]; !explicit {
		return "", false, nil
	}
	raw := strings.TrimSpace(c.URLParam("match_id"))
	id, parseErr := strconv.Atoi(raw)
	if parseErr != nil || id <= 0 {
		return "", true, &forecastParamError{message: "match_id must be a positive integer"}
	}
	return strconv.Itoa(id), true, nil
}

type forecastParamError struct {
	message string
}

func (e *forecastParamError) Error() string {
	return e.message
}

func writeForecastError(c iris.Context, status int, message string) {
	c.StatusCode(status)
	c.JSON(iris.Map{"error": message})
}

// forecastImageURL 根据公网域名前缀生成预测图下载地址；match_id 必定写入地址。
// version 来自已经成功发布的内存图片；尚无图片时不写 v，避免 CDN 长缓存未就绪版本。
func forecastImageURL(baseURL, matchID string, version int64) string {
	imageURL := strings.TrimRight(baseURL, "/") + forecastImagePath
	query := url.Values{}
	query.Set("match_id", matchID)
	if version > 0 {
		query.Set("v", strconv.FormatInt(version, 10))
	}
	return imageURL + "?" + query.Encode()
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
		logrus.Errorf("match_forecast: unmarshal schedule failed: %v", err)
		return types.ScheduleResp{}, false
	}
	return schedule, true
}

// selectForecastMatch 选出用于竞猜下发的比赛：
// 显式 match_id 优先；否则沿用调试环境变量，最后取第一场 status == STARTED 的比赛。
func selectForecastMatch(
	schedule types.ScheduleResp,
	requestedMatchID string,
	explicit bool,
) (types.ZoneNode, types.MatchNode, bool) {
	if explicit {
		return findMatchByID(schedule, requestedMatchID)
	}
	if debugID := strings.TrimSpace(os.Getenv(envForecastDebugMatchID)); debugID != "" {
		zone, match, found := findMatchByID(schedule, debugID)
		if !found {
			logrus.Warnf("match_forecast: debug match_id %q not found in schedule", debugID)
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

// findNextMatch 返回同一赛区内 current 之后最早且尚未产生比分的比赛。
func findNextMatch(zone types.ZoneNode, current types.MatchNode) (types.MatchNode, bool) {
	var next types.MatchNode
	found := false
	for _, match := range append(zone.GroupMatches.Nodes, zone.KnockoutMatches.Nodes...) {
		if match.OrderNumber <= current.OrderNumber || hasMatchScore(match) {
			continue
		}
		if !found || match.OrderNumber < next.OrderNumber {
			next = match
			found = true
		}
	}
	return next, found
}

// findFirstUpcomingMatch 在没有进行中比赛时返回当前赛季最早的未结束比赛。
func findFirstUpcomingMatch(schedule types.ScheduleResp) (types.ZoneNode, types.MatchNode, bool) {
	var selectedZone types.ZoneNode
	var selectedMatch types.MatchNode
	found := false
	for _, zone := range schedule.Data.Event.Zones.Nodes {
		for _, match := range append(zone.GroupMatches.Nodes, zone.KnockoutMatches.Nodes...) {
			if match.Status == "DONE" || match.Status == matchStatusStarted || hasMatchScore(match) {
				continue
			}
			if !found || matchBefore(match, selectedMatch) {
				selectedZone = zone
				selectedMatch = match
				found = true
			}
		}
	}
	return selectedZone, selectedMatch, found
}

// hasMatchScore 检查红蓝双方的获胜局数。上游偶尔会先更新局分、稍后才把
// status 推进到 DONE；这段窗口内不能再把该场比赛作为 next 下发。
func hasMatchScore(match types.MatchNode) bool {
	return match.RedSideWinGameCount != 0 || match.BlueSideWinGameCount != 0
}

func matchBefore(left, right types.MatchNode) bool {
	if left.PlanStartedAt != "" && right.PlanStartedAt != "" && left.PlanStartedAt != right.PlanStartedAt {
		return left.PlanStartedAt < right.PlanStartedAt
	}
	return left.OrderNumber < right.OrderNumber
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
