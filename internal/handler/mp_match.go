package handler

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/scutrobotlab/RMSituationBackend/internal/svc"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
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

func MpMatchHandler(c *gin.Context) {
	matchIds := c.Query("match_ids")
	if matchIds == "" {
		c.JSON(400, gin.H{"error": "match_ids is required"})
		return
	}

	matchIdList := strings.Split(matchIds, ",")
	var mpMatchRespList []MpMatchData
	for _, id := range matchIdList {
		_id, err := strconv.Atoi(id)
		if err != nil {
			log.Printf("invalid match_id: %v", id)
			c.JSON(400, gin.H{"error": "invalid match_id"})
			return
		}

		mpMatch, b := svc.Cache.Get("mp_match:" + id)
		if !b {
			_url, err := url.Parse("https://mp.robomaster.com/api/v1/match?matchID=" + id)
			request := http.Request{
				Method: http.MethodGet,
				URL:    _url,
				Header: http.Header{
					"Referer": []string{"https://servicewechat.com/wx449772ad6960c39f/34/page-frame.html"},
				},
			}

			response, err := http.DefaultClient.Do(&request)
			if err != nil {
				log.Printf("failed to fetch data from mp.robomaster.com: %v", err)
				c.JSON(500, gin.H{"error": "failed to fetch data from mp.robomaster.com"})
				return
			}

			bytes, err := io.ReadAll(response.Body)
			if err != nil {
				log.Printf("failed to read response body: %v", err)
				c.JSON(500, gin.H{"error": "failed to read response body"})
				response.Body.Close()
				return
			}
			response.Body.Close()

			var _mpMatchResp MpMatchSrcResp
			err = json.Unmarshal(bytes, &_mpMatchResp)
			if err != nil {
				log.Printf("failed to unmarshal response body: %v", err)
				fmt.Printf("%s", bytes)
				c.JSON(500, gin.H{"error": "failed to unmarshal response body"})
				return
			}

			data := MpMatchData{
				MatchId:   _id,
				RedCount:  _mpMatchResp.Data.RedCount,
				BlueCount: _mpMatchResp.Data.BlueCount,
				TieCount:  _mpMatchResp.Data.TieCount,
			}
			data.TotalCount = data.RedCount + data.BlueCount + data.TieCount
			if data.TotalCount != 0 {
				data.RedRate = float64(data.RedCount) / float64(data.TotalCount)
				data.BlueRate = float64(data.BlueCount) / float64(data.TotalCount)
				data.TieRate = float64(data.TieCount) / float64(data.TotalCount)
			}
			svc.Cache.Set("mp_match:"+id, data, 30*time.Second)
			mpMatchRespList = append(mpMatchRespList, data)
		} else {
			mpMatchRespList = append(mpMatchRespList, mpMatch.(MpMatchData))
		}
	}

	c.JSON(200, MpMatchDstResp{List: mpMatchRespList})
}
