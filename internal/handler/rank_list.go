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
	RankScoreItem RankScoreItem `json:"rankScoreItem"`
	CompleteForm  CompleteForm  `json:"completeForm"`
}

type RankScoreItem struct {
	Rank          int     `json:"rank"`
	SchoolChinese string  `json:"schoolChinese"`
	SchoolEnglish string  `json:"schoolEnglish"`
	Score         float64 `json:"score"`
}

type CompleteForm struct {
	School                string `json:"school"`
	Team                  string `json:"team"`
	Score                 int    `json:"score"`
	InitialCoinDocument   int    `json:"initialCoinDocument"`
	InitialCoinTechnology int    `json:"initialCoinTechnology"`
	InitialCoinTotal      int    `json:"initialCoinTotal"`
}

func RankListHandler(c *gin.Context) {
	schoolName := c.Query("school_name")
	if schoolName == "" {
		c.JSON(400, gin.H{"code": -1, "msg": "School name is empty"})
		return
	}

	completedFormMap, ok := svc.Cache.Get("completed_form")
	if !ok {
		completedFormJson := make([]CompleteForm, 0)
		err := json.Unmarshal(static.CompleteFormBytes, &completedFormJson)
		if err != nil {
			log.Printf("Failed to parse completed form: %v\n", err)
			c.JSON(500, gin.H{"code": -1, "msg": "Failed to parse completed form"})
			return
		}

		completedFormMap = lo.SliceToMap(completedFormJson, func(item CompleteForm) (string, CompleteForm) { return item.School, item })
		svc.Cache.Set("completed_form", completedFormMap, cache.NoExpiration)
	}

	completedForm, ok := completedFormMap.(map[string]CompleteForm)[schoolName]
	if !ok {
		c.JSON(404, gin.H{"code": -1, "msg": "School not found"})
		return
	}

	rankScoreMap, ok := svc.Cache.Get("rank_score")
	if !ok {
		rankScoreJson := make([]RankScoreItem, 0)
		err := json.Unmarshal(static.RankScoreBytes, &rankScoreJson)
		if err != nil {
			log.Printf("Failed to parse rank list: %v\n", err)
			c.JSON(500, gin.H{"code": -1, "msg": "Failed to parse rank list"})
			return
		}

		rankScoreMap = lo.SliceToMap(rankScoreJson, func(item RankScoreItem) (string, RankScoreItem) { return item.SchoolChinese, item })
		svc.Cache.Set("rank_list", rankScoreMap, cache.NoExpiration)
	}

	rankScore, ok := rankScoreMap.(map[string]RankScoreItem)[schoolName]
	if !ok {
		c.JSON(404, gin.H{"code": -1, "msg": "School not found"})
		return
	}

	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(200, RankListItem{
		RankScoreItem: rankScore,
		CompleteForm:  completedForm,
	})
}
