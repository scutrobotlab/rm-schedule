package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"golang.org/x/sync/singleflight"
)

const (
	MpMatchCacheRefreshTime        = 10 * time.Second // 缓存即将过期时，异步刷新
	MpMatchCacheExpiration         = 60 * time.Second // 缓存过期时间
	MpMatchFailureCacheExpiration  = 30 * time.Second // 单场拉取失败的占位缓存时间
	MpMatchRealtimeCacheExpiration = 1 * time.Second  // 实时查询（match_forecast）的短缓存，兼顾实时性与限流
	MpMatchUpstreamTimeout         = 5 * time.Second  // 上游拉取超时，避免 singleflight leader 卡住拖垮所有 follower
	MpMatchDisabled                = false            // 是否禁用
)

// mpMatchSFGroup 合并同一 match_id 的并发上游拉取，避免冷启动与刷新窗口内的惊群。
var mpMatchSFGroup singleflight.Group

// mpMatchHTTPClient 带超时，防止上游无响应时请求 goroutine 无限期挂起。
var mpMatchHTTPClient = &http.Client{Timeout: MpMatchUpstreamTimeout}

type MpMatchSrcResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		RedCount  int `json:"redCount"`
		BlueCount int `json:"blueCount"`
		TieCount  int `json:"tieCount"`
	} `json:"data"`
}

type MpMatchDstResp struct {
	List []MpMatchData `json:"list"`
}

type MpMatchData struct {
	MatchId    int     `json:"matchId"`
	RedCount   int     `json:"redCount"`
	BlueCount  int     `json:"blueCount"`
	TieCount   int     `json:"tieCount"`
	TotalCount int     `json:"totalCount"`
	RedRate    float64 `json:"redRate"`
	BlueRate   float64 `json:"blueRate"`
	TieRate    float64 `json:"tieRate"`
	// QueriedAt 记录本条支持率数据从 mp.robomaster.com 拉取的时刻。
	// 不参与 /api/mp/match 序列化（json:"-"），仅供 match_forecast 计算「支持率查询截止时间」。
	QueriedAt time.Time `json:"-"`
}

func MpMatchHandler(c iris.Context) {
	if MpMatchDisabled {
		// 禁用时，返回空数据
		c.Header("Cache-Control", "public, max-age=10")
		c.JSON(MpMatchDstResp{List: make([]MpMatchData, 0)})
		return
	}

	// matchIds := c.Query("match_ids")
	matchIds := c.URLParam("match_ids")
	if matchIds == "" {
		c.StatusCode(400)
		c.JSON(iris.Map{"error": "match_ids is required"})
		return
	}

	matchIdList := strings.Split(matchIds, ",")
	var mpMatchRespList []MpMatchData
	for _, id := range matchIdList {
		_id, err := strconv.Atoi(id)
		if err != nil {
			logrus.Errorf("invalid match_id: %v", id)
			c.StatusCode(400)
			c.JSON(iris.Map{"error": "invalid match_id"})
			return
		}

		mpMatchRespList = append(mpMatchRespList, resolveMpMatch(id, _id))
	}

	c.Header("Cache-Control", "public, max-age=10")
	c.JSON(MpMatchDstResp{List: mpMatchRespList})
}

// resolveMpMatch 按 match_id 取一条支持率数据：命中缓存直接返回（临近过期时异步刷新），
// 未命中则同步拉取；拉取失败时写入短时占位缓存并返回不可用数据。
// idStr 与 id 为同一 match_id 的字符串/整数形式，避免重复转换。
func resolveMpMatch(idStr string, id int) MpMatchData {
	mpMatch, expiration, b := svc.Cache.GetWithExpiration("mp_match:" + idStr)
	if !b {
		data, err := loadMpMatchShared(id)
		if err != nil {
			logrus.Errorf("Failed to get mp match %d: %v", id, err)
			data = unavailableMpMatchData(id)
			svc.Cache.Set("mp_match:"+idStr, *data, MpMatchFailureCacheExpiration)
		}
		return *data
	}

	// 如果缓存即将过期，异步刷新
	if expiration.Sub(time.Now()) < MpMatchCacheRefreshTime {
		go func(id int) {
			_, err := loadMpMatchShared(id)
			if err != nil {
				logrus.Errorf("Failed to get mp match: %v", err)
			}
		}(id)
	}

	return mpMatch.(MpMatchData)
}

// resolveMpMatchRealtime 为 match_forecast 提供更实时的支持率：
// 使用独立的 1s 短缓存（mp_match_rt:），命中即返回，未命中则同步拉取上游。
// 相比 resolveMpMatch 的 60s 缓存，可将当前进行中比赛的支持率延迟控制在 ~1s，
// 同时借助 loadMpMatchShared 的 singleflight 合并并发拉取、避免打爆上游。
func resolveMpMatchRealtime(idStr string, id int) MpMatchData {
	key := "mp_match_rt:" + idStr
	if cached, b := svc.Cache.Get(key); b {
		return cached.(MpMatchData)
	}

	data, err := loadMpMatchShared(id)
	if err != nil {
		logrus.Errorf("Failed to get mp match %d (realtime): %v", id, err)
		data = unavailableMpMatchData(id)
	}
	svc.Cache.Set(key, *data, MpMatchRealtimeCacheExpiration)
	return *data
}

func unavailableMpMatchData(id int) *MpMatchData {
	return &MpMatchData{
		MatchId:   id,
		RedRate:   -1.0,
		BlueRate:  -1.0,
		TieRate:   -1.0,
		QueriedAt: time.Now(),
	}
}

// loadMpMatchShared 对 loadMpMatch 做 singleflight 去重：同一 match_id 的并发拉取
// （冷启动同步加载 + 刷新窗口内的异步刷新）会合并为一次上游请求，共享同一结果。
func loadMpMatchShared(id int) (*MpMatchData, error) {
	key := "mp_match:" + strconv.Itoa(id)
	v, err, _ := mpMatchSFGroup.Do(key, func() (interface{}, error) {
		return loadMpMatch(id)
	})
	if err != nil {
		return nil, err
	}
	data, ok := v.(*MpMatchData)
	if !ok || data == nil {
		return nil, fmt.Errorf("unexpected mp match result for id %d", id)
	}
	return data, nil
}

func loadMpMatch(id int) (*MpMatchData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), MpMatchUpstreamTimeout)
	defer cancel()

	reqURL := "https://mp.robomaster.com/api/v1/match?matchID=" + strconv.Itoa(id)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build mp match request: %v", err)
	}
	request.Header.Set("Referer", "https://servicewechat.com/wx449772ad6960c39f/34/page-frame.html")

	response, err := mpMatchHTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failed to get mp match: %v", err)
	}
	defer response.Body.Close()

	bytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read mp match response: %v", err)
	}

	if response.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get mp match, http status code: %v, response: %v", response.StatusCode, string(bytes))
	}

	var _mpMatchResp MpMatchSrcResp
	err = json.Unmarshal(bytes, &_mpMatchResp)
	if err != nil {
		logrus.Errorf("failed to unmarshal bytes: %v", string(bytes))
		return nil, fmt.Errorf("failed to unmarshal mp match response: %v", err)
	}

	data := MpMatchData{
		MatchId:   id,
		RedCount:  _mpMatchResp.Data.RedCount,
		BlueCount: _mpMatchResp.Data.BlueCount,
		TieCount:  _mpMatchResp.Data.TieCount,
		QueriedAt: time.Now(),
	}
	data.TotalCount = data.RedCount + data.BlueCount + data.TieCount
	if data.TotalCount != 0 {
		data.RedRate = float64(data.RedCount) / float64(data.TotalCount)
		data.BlueRate = float64(data.BlueCount) / float64(data.TotalCount)
		data.TieRate = float64(data.TieCount) / float64(data.TotalCount)
	} else {
		data.RedRate = -1.0
		data.BlueRate = -1.0
		data.TieRate = -1.0
	}
	svc.Cache.Set("mp_match:"+strconv.Itoa(id), data, MpMatchCacheExpiration)

	return &data, nil
}
