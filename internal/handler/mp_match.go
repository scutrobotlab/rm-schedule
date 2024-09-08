package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
)

const (
	MpMatchCacheRefreshTime = 10 * time.Second // 缓存即将过期时，异步刷新
	MpMatchCacheExpiration  = 60 * time.Second // 缓存过期时间
	MpMatchDisabled         = false            // 是否禁用
)

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
			log.Printf("invalid match_id: %v", id)
			c.StatusCode(400)
			c.JSON(iris.Map{"error": "invalid match_id"})
			return
		}

		mpMatch, expiration, b := svc.Cache.GetWithExpiration("mp_match:" + id)
		if !b {
			data, err := loadMpMatch(_id)
			if err != nil {
				log.Printf("Failed to get mp match: %v\n", err)
				c.StatusCode(500)
				c.JSON(iris.Map{"code": -1, "msg": "Failed to get mp match"})
				return
			}
			mpMatchRespList = append(mpMatchRespList, *data)
		} else {
			// 如果缓存即将过期，异步刷新
			if expiration.Sub(time.Now()) < MpMatchCacheRefreshTime {
				go func(id int) {
					_, err := loadMpMatch(id)
					if err != nil {
						log.Printf("Failed to get mp match: %v\n", err)
					}
				}(_id)
			}

			mpMatchRespList = append(mpMatchRespList, mpMatch.(MpMatchData))
		}
	}

	c.Header("Cache-Control", "public, max-age=10")
	c.JSON(MpMatchDstResp{List: mpMatchRespList})
}

func loadMpMatch(id int) (*MpMatchData, error) {
	_url, err := url.Parse("https://mp.robomaster.com/api/v1/match?matchID=" + strconv.Itoa(id))
	request := http.Request{
		Method: http.MethodGet,
		URL:    _url,
		Header: http.Header{"Referer": []string{"https://servicewechat.com/wx449772ad6960c39f/34/page-frame.html"}},
	}

	response, err := http.DefaultClient.Do(&request)
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
		log.Printf("failed to unmarshal bytes: %v\n", string(bytes))
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
