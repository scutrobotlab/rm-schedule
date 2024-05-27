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

type RankScoreItem struct {
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

	_rankScoreMap, b := svc.Cache.Get("rank_score")
	if b {
		rankScoreMap := _rankScoreMap.(map[string]RankScoreItem)
		if _, ok := rankScoreMap[schoolName]; !ok {
			c.JSON(404, gin.H{"code": -1, "msg": "School not found"})
			return
		}
		c.Header("Cache-Control", "public, max-age=3600")
		c.JSON(200, rankScoreMap[schoolName])
		return
	}

	rankScoreJson := make([]RankScoreItem, 0)
	err := json.Unmarshal(static.RankScoreBytes, &rankScoreJson)
	if err != nil {
		log.Printf("Failed to parse rank list: %v\n", err)
		c.JSON(500, gin.H{"code": -1, "msg": "Failed to parse rank list"})
		return
	}

	rankScoreMap := lo.SliceToMap(rankScoreJson, func(item RankScoreItem) (string, RankScoreItem) { return item.SchoolChinese, item })
	svc.Cache.Set("rank_list", rankScoreMap, cache.NoExpiration)
	if _, ok := rankScoreMap[schoolName]; !ok {
		c.JSON(404, gin.H{"code": -1, "msg": "School not found"})
		return
	}

	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(200, rankScoreMap[schoolName])
}
