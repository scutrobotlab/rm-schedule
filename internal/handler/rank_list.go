package handler

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/x/errors"
	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	"github.com/scutrobotlab/rm-schedule/internal/static"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/sirupsen/logrus"
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
	Rank                  int    `json:"rank"`
	School                string `json:"school"`
	Team                  string `json:"team"`
	Score                 int    `json:"score"`
	InitialCoinDocument   int    `json:"initialCoinDocument"`
	LevelDocument         string `json:"levelDocument"`
	InitialCoinTechnology int    `json:"initialCoinTechnology"`
	LevelTechnology       string `json:"levelTechnology"`
	InitialCoinTotal      int    `json:"initialCoinTotal"`
}

type CompleteFormRank struct {
	Rank   int    `json:"rank"`
	School string `json:"school"`
	Team   string `json:"team"`
}

var SeasonCompleteFormMap = map[string][]byte{
	"2024": static.CompleteFormBytes2024,
	"2025": static.CompleteFormBytes2025,
}

var SeasonCompleteFormRankMap = map[string][]byte{
	"2024": {},
	"2025": static.CompleteFormRankBytes2025,
}

var SeasonRankScoreMap = map[string][]byte{
	"2024": static.RankScoreBytes2024,
	"2025": static.RankScoreBytes2025,
}

var schoolNameReplacer = strings.NewReplacer("（", "(", "）", ")")

// normalizeSchoolName 将全角括号统一转换为半角，兼容数据源中的括号差异
func normalizeSchoolName(name string) string {
	return schoolNameReplacer.Replace(name)
}

func RankListHandler(c iris.Context) {
	season := c.URLParam("season")
	rankScoreKey := fmt.Sprintf("rank_score_%s", season)
	var rankScoreBytes []byte
	_, historicalSeason := SeasonRankScoreMap[season]
	if data, ok := SeasonRankScoreMap[season]; ok {
		rankScoreBytes = data
	} else {
		rankScoreBytes = static.RankScoreBytes
	}

	schoolName := normalizeSchoolName(c.URLParam("school_name"))
	if schoolName == "" {
		c.StatusCode(400)
		c.JSON(iris.Map{"code": -1, "msg": "School name is empty"})
		return
	}

	completeFormMap, err := GetCompleteFormMap(season)
	if err != nil {
		c.StatusCode(500)
		c.JSON(iris.Map{"code": -1, "msg": "Failed to get complete form"})
		return
	}
	completeForm, ok := completeFormMap[schoolName]
	if !ok {
		c.StatusCode(404)
		c.JSON(iris.Map{"code": -1, "msg": "School not found"})
		return
	}

	rankScoreMap, ok := svc.Cache.Get(rankScoreKey)
	if !ok {
		rankScoreJson := make([]RankScoreItem, 0)
		err := json.Unmarshal(rankScoreBytes, &rankScoreJson)
		if err != nil {
			logrus.Errorf("Failed to parse rank list: %v", err)
			c.StatusCode(500)
			c.JSON(iris.Map{"code": -1, "msg": "Failed to parse rank list"})
			return
		}

		rankScoreMap = lo.SliceToMap(rankScoreJson, func(item RankScoreItem) (string, RankScoreItem) {
			return normalizeSchoolName(item.SchoolChinese), item
		})
		svc.Cache.Set(rankScoreKey, rankScoreMap, cache.NoExpiration)
	}

	rankScore, ok := rankScoreMap.(map[string]RankScoreItem)[schoolName]
	if !ok {
		c.StatusCode(404)
		c.JSON(iris.Map{"code": -1, "msg": "School not found"})
		return
	}

	if historicalSeason {
		c.Header("Cache-Control", longLivedStaticCacheControl)
	} else {
		c.Header("Cache-Control", "public, max-age=3600")
	}
	c.JSON(RankListItem{
		RankScoreItem: rankScore,
		CompleteForm:  completeForm,
	})
}

// GetCompleteFormMap 获取完整形态
func GetCompleteFormMap(season string) (map[string]CompleteForm, error) {
	completeFormKey := fmt.Sprintf("complete_form_%s", season)
	ret, ok := svc.Cache.Get(completeFormKey)
	if ok {
		return ret.(map[string]CompleteForm), nil
	}

	var completeFormBytes []byte
	var completeFormRankBytes []byte
	if data, ok := SeasonCompleteFormMap[season]; ok {
		completeFormBytes = data
	} else {
		completeFormBytes = static.CompleteFormBytes
	}
	if data, ok := SeasonCompleteFormRankMap[season]; ok {
		completeFormRankBytes = data
	} else {
		completeFormRankBytes = static.CompleteFormRankBytes
	}

	completeFormJson := make([]CompleteForm, 0)
	err := json.Unmarshal(completeFormBytes, &completeFormJson)
	if err != nil {
		logrus.Errorf("Failed to parse complete form: %v", err)
		return nil, errors.New("Failed to parse complete form")
	}

	if len(completeFormRankBytes) != 0 {
		// 有完整形态排名
		completeFormRankJson := make([]CompleteFormRank, 0)
		err = json.Unmarshal(completeFormRankBytes, &completeFormRankJson)
		if err != nil {
			logrus.Errorf("Failed to parse complete form rank: %v", err)
			return nil, errors.New("Failed to parse complete form rank")
		}
		completeFormRankMap := make(map[string]CompleteFormRank)
		for _, item := range completeFormRankJson {
			completeFormRankMap[normalizeSchoolName(item.School)] = item
		}
		for i, item := range completeFormJson {
			if rankItem, ok := completeFormRankMap[normalizeSchoolName(item.School)]; ok {
				completeFormJson[i].Rank = rankItem.Rank
			}
		}
	} else {
		// 无完整形态排名 按照金币数量计算
		// 并列名次处理
		var rank int
		var lastCoinTotal int
		for i := range completeFormJson {
			if completeFormJson[i].InitialCoinTotal != lastCoinTotal {
				rank = i + 1
			}
			completeFormJson[i].Rank = rank
			lastCoinTotal = completeFormJson[i].InitialCoinTotal
		}
	}
	completeFormMap := lo.SliceToMap(completeFormJson, func(item CompleteForm) (string, CompleteForm) {
		return normalizeSchoolName(item.School), item
	})
	svc.Cache.Set(completeFormKey, completeFormMap, cache.NoExpiration)

	return completeFormMap, nil
}
