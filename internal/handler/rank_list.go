package handler

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	"github.com/scutrobotlab/RMSituationBackend/internal/static"
	"github.com/scutrobotlab/RMSituationBackend/internal/svc"
	"log"
)

type RankListItem struct {
	Rank          int     `json:"Rank"`
	SchoolChinese string  `json:"SchoolChinese"`
	SchoolEnglish string  `json:"SchoolEnglish"`
	Score         float64 `json:"Score"`
}

func RankListHandler(c *gin.Context) {
	schoolName := c.Query("school_name")
	if schoolName == "" {
		c.JSON(400, gin.H{"code": -1, "msg": "School name is empty"})
		return
	}

	_rankListMap, b := svc.Cache.Get("rank_list")
	if b {
		rankListMap := _rankListMap.(map[string]RankListItem)
		if _, ok := rankListMap[schoolName]; !ok {
			c.JSON(404, gin.H{"code": -1, "msg": "School not found"})
			return
		}
		c.Header("Cache-Control", "public, max-age=3600")
		c.JSON(200, rankListMap[schoolName])
		return
	}

	rankListJson := make([]RankListItem, 0)
	err := json.Unmarshal(static.RankListBytes, &rankListJson)
	if err != nil {
		log.Printf("Failed to parse rank list: %v\n", err)
		c.JSON(500, gin.H{"code": -1, "msg": "Failed to parse rank list"})
		return
	}

	rankListMap := lo.SliceToMap(rankListJson, func(item RankListItem) (string, RankListItem) { return item.SchoolChinese, item })
	svc.Cache.Set("rank_list", rankListMap, cache.NoExpiration)
	if _, ok := rankListMap[schoolName]; !ok {
		c.JSON(404, gin.H{"code": -1, "msg": "School not found"})
		return
	}

	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(200, rankListMap[schoolName])
}
