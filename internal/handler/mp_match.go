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
	MpMatchCacheRefreshTime       = 10 * time.Second // 缓存即将过期时，异步刷新
	MpMatchCacheExpiration        = 60 * time.Second // 缓存过期时间
	MpMatchFailureCacheExpiration = 30 * time.Second // 单场拉取失败的占位缓存时间
	MpMatchUpstreamTimeout        = 5 * time.Second  // 上游拉取超时，避免 singleflight leader 卡住拖垮所有 follower
	MpMatchDisabled               = false            // 是否禁用
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
}

func MpMatchHandler(c iris.Context) {
	if MpMatchDisabled {
		// 禁用时，返回空数据
		c.Header("Cache-Control", "public, max-age=60")
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

		mpMatch, expiration, b := svc.Cache.GetWithExpiration("mp_match:" + id)
		if !b {
			data, err := loadMpMatchShared(_id)
			if err != nil {
				logrus.Errorf("Failed to get mp match %d: %v", _id, err)
				data = unavailableMpMatchData(_id)
				svc.Cache.Set("mp_match:"+id, *data, MpMatchFailureCacheExpiration)
			}
			mpMatchRespList = append(mpMatchRespList, *data)
		} else {
			// 如果缓存即将过期，异步刷新
			if expiration.Sub(time.Now()) < MpMatchCacheRefreshTime {
				go func(id int) {
					_, err := loadMpMatchShared(id)
					if err != nil {
						logrus.Errorf("Failed to get mp match: %v", err)
					}
				}(_id)
			}

			mpMatchRespList = append(mpMatchRespList, mpMatch.(MpMatchData))
		}
	}

	c.Header("Cache-Control", "public, max-age=10")
	c.JSON(MpMatchDstResp{List: mpMatchRespList})
}

func unavailableMpMatchData(id int) *MpMatchData {
	return &MpMatchData{
		MatchId:  id,
		RedRate:  -1.0,
		BlueRate: -1.0,
		TieRate:  -1.0,
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
