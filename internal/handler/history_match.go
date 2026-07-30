package handler

import (
	"encoding/json"
	"github.com/kataras/iris/v12"
	"github.com/patrickmn/go-cache"
	"github.com/scutrobotlab/rm-schedule/internal/static"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/sirupsen/logrus"
	"sort"
	"strconv"
)

const HistoryMatchKey = "history_match"

type HistoryMatchMap map[string]HistorySeason

type HistorySeason struct {
	Season  string         `json:"season"`
	Matches []HistoryMatch `json:"matches"`
}

type HistoryMatchList []HistoryMatch

type HistoryMatch struct {
	BlueCollegeName      string `json:"blueCollegeName"`
	BlueSideWinGameCount int64  `json:"blueSideWinGameCount"`
	BlueTeamName         string `json:"blueTeamName"`
	Group                string `json:"group"`
	Order                int64  `json:"order"`
	OrderNumber          int64  `json:"orderNumber"`
	RedCollegeName       string `json:"redCollegeName"`
	RedSideWinGameCount  int64  `json:"redSideWinGameCount"`
	RedTeamName          string `json:"redTeamName"`
	Season               int64  `json:"season"`
	Zone                 string `json:"zone"`
}

func HistoryMatchHandler(c iris.Context) {
	primaryCollegeName := c.URLParam("primary_college_name")
	secondaryCollegeName := c.URLParam("secondary_college_name")
	if primaryCollegeName == "" || secondaryCollegeName == "" {
		c.StatusCode(400)
		c.JSON(iris.Map{"code": -1, "msg": "primary_college_name and secondary_college_name are required"})
		return
	}

	var historyMatchMap HistoryMatchMap
	if cached, ok := svc.Cache.Get(HistoryMatchKey); ok {
		// 如果缓存中有历史比赛数据，则直接使用缓存
		historyMatchMap = cached.(HistoryMatchMap)
	} else {
		// 如果缓存中没有历史比赛数据，则从静态文件中加载
		historyMatchMap = make(HistoryMatchMap)
		var historyMatchList HistoryMatchList
		err := json.Unmarshal(static.HistoryMatchBytes, &historyMatchList)
		if err != nil {
			logrus.Errorf("history match unmarshal err: %v", err)
			c.StatusCode(500)
			c.JSON(iris.Map{"code": -1, "msg": "Failed to parse history match data"})
			return
		}
		// 将历史比赛数据按赛季分组
		for _, match := range historyMatchList {
			seasonStr := strconv.FormatInt(match.Season, 10)
			if _, ok := historyMatchMap[seasonStr]; !ok {
				// 如果当前赛季不存在，则创建一个新的赛季
				historyMatchMap[seasonStr] = HistorySeason{
					Season:  seasonStr,
					Matches: []HistoryMatch{},
				}
			}
			// 获取当前赛季
			season := historyMatchMap[seasonStr]
			// 将当前比赛添加到对应赛季的比赛列表中
			season.Matches = append(season.Matches, match)
			// 更新赛季信息
			historyMatchMap[seasonStr] = season
		}
		// 将历史比赛数据存入缓存
		svc.Cache.Set(HistoryMatchKey, historyMatchMap, cache.NoExpiration)
	}

	// 查找匹配的比赛
	// 时间复杂度为 O(n)，其中 n 是历史比赛的数量
	// 后续可以考虑使用更高效的数据结构或算法来优化查询性能
	var hits []HistoryMatch
	for _, season := range historyMatchMap {
		for _, match := range season.Matches {
			// 检查比赛的蓝队和红队是否匹配
			if (match.BlueCollegeName == primaryCollegeName && match.RedCollegeName == secondaryCollegeName) ||
				(match.BlueCollegeName == secondaryCollegeName && match.RedCollegeName == primaryCollegeName) {
				hits = append(hits, match)
			}
		}
	}
	// 按照 Order 字段对匹配的比赛进行排序
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].Order < hits[j].Order
	})

	c.Header("Cache-Control", longLivedStaticCacheControl)
	c.JSON(iris.Map{
		"total": len(hits),
		"hits":  hits,
	})
}
