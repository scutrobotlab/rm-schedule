package bilibili

import (
	"encoding/json"
	"fmt"
	"github.com/scutrobotlab/rm-schedule/internal/common"
	"github.com/scutrobotlab/rm-schedule/internal/router"
	"github.com/scutrobotlab/rm-schedule/internal/static"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/scutrobotlab/rm-schedule/internal/types"
	"github.com/sirupsen/logrus"
	"strconv"
	"strings"
)

type Matches map[string]map[string][]types.MatchNode

func intToChinese(num int) string {
	if num == 0 {
		return "零"
	}

	digits := []string{"零", "一", "二", "三", "四", "五", "六", "七", "八", "九"}
	units := []string{"", "十", "百", "千"}
	parts := make([]string, 0, 8)
	needZero := false

	for unitPos := 0; num > 0; unitPos++ {
		digit := num % 10
		if digit == 0 {
			if len(parts) > 0 {
				needZero = true
			}
		} else {
			part := digits[digit] + units[unitPos]
			if needZero {
				parts = append(parts, "零")
				needZero = false
			}
			parts = append(parts, part)
		}
		num /= 10
	}

	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
	}

	result := strings.Join(parts, "")
	if strings.HasPrefix(result, "一十") {
		return strings.TrimPrefix(result, "一")
	}
	return result
}

func getMatches() Matches {
	//返回赛季、赛区、比赛三级嵌套的map,列出从日程中获取的所有比赛信息
	matches := Matches{}

	schedules := map[string][]byte{
		"2024": static.ScheduleBytes2024,
		"2025": static.ScheduleBytes2025,
	}

	if !router.RedirectParams[common.UpstreamNameSchedule].Static {
		liveScheduleData, ok := svc.Cache.Get(router.RedirectParams[common.UpstreamNameSchedule].Name)
		if ok {
			schedules["2026"] = liveScheduleData.([]byte)
		}
	}

	for season, scheduleBytes := range schedules {
		var scheduleData types.ScheduleResp
		err := json.Unmarshal(scheduleBytes, &scheduleData)
		if err != nil {
			logrus.Errorf("failed to unmarshal schedule: %v", err)
			return nil
		}

		//遍历整理比赛
		zones := make(map[string][]types.MatchNode)
		for _, currentZone := range scheduleData.Data.Event.Zones.Nodes {
			var currentMatches []types.MatchNode
			for _, match := range currentZone.GroupMatches.Nodes {
				currentMatches = append(currentMatches, match)
			}
			for _, match := range currentZone.KnockoutMatches.Nodes {
				currentMatches = append(currentMatches, match)
			}
			zones[currentZone.Name] = currentMatches
		}
		matches[season] = zones
	}

	return matches
}

func findCollection(season string, zone string, collectionList *[]types.BiliBiliCollectionMetaData) (types.BiliBiliCollectionMetaData, bool) {
	if season == "2025" {
		switch zone {
		case "复活赛第一赛段":
			return types.BiliBiliCollectionMetaData{Name: "2025复活赛第一赛段", CollectionId: 5947992}, true
		}
	}

	for _, collection := range *collectionList {
		collectionName := collection.Name
		isReplay := strings.Contains(collectionName, "比赛回放") || strings.Contains(collectionName, "直播回放")
		isRMUC := strings.Contains(collectionName, "RMUC") || strings.Contains(collectionName, "超级对抗赛")
		isCorrectSeason := strings.Contains(collectionName, season)
		var isCorrectZone bool
		if len([]rune(zone)) > 3 {
			//港澳台及海外赛区&复活赛在赛程中被拆分为两段，回放中属于同一合集
			isCorrectZone = strings.Contains(collectionName, string([]rune(zone)[:3]))
		} else {
			isCorrectZone = strings.Contains(collectionName, zone)
		}
		if isReplay && isRMUC && isCorrectSeason && isCorrectZone {
			return collection, true
		}
	}
	return types.BiliBiliCollectionMetaData{Name: "not found", CollectionId: 0}, false
}

func checkStringInclude(fullStr string, subStr string) bool {
	var isInclusion bool
	if len([]rune(subStr)) > 3 {
		isInclusion = strings.Contains(fullStr, string([]rune(subStr)[:3])) || strings.Contains(fullStr, string([]rune(subStr)[3:]))
	} else {
		isInclusion = strings.Contains(fullStr, subStr)
	}
	return isInclusion
}

func findMatchVideo(match *types.MatchNode, collection *types.BiliBiliCollectionInfo) (types.BiliBiliVideoMetaData, bool) {
	for _, video := range collection.Data.Archives {
		if match.RedSide.Player == nil || match.RedSide.Player.Team == nil {
			//如果比赛信息中没有红蓝双方的选手信息，则跳过
			logrus.Warnf("match %s has no player info, skipping", match.ID)
			continue
		}
		if match.BlueSide.Player == nil || match.BlueSide.Player.Team == nil {
			//如果比赛信息中没有红蓝双方的选手信息，则跳过
			logrus.Warnf("match %s has no player info, skipping", match.ID)
			continue
		}
		videoTitle := video.Title
		isCorrectOrderNum := strings.Contains(videoTitle, fmt.Sprintf("第%d场", match.OrderNumber)) ||
			strings.Contains(videoTitle, fmt.Sprintf("第 %d 场", match.OrderNumber)) ||
			strings.Contains(videoTitle, fmt.Sprintf("第%s场", intToChinese(match.OrderNumber)))
		isCorrectBlueTeam := checkStringInclude(videoTitle, match.BlueSide.Player.Team.Name)
		isCorrectBlueSchool := checkStringInclude(videoTitle, match.BlueSide.Player.Team.CollegeName)
		isCorrectRedTeam := checkStringInclude(videoTitle, match.RedSide.Player.Team.Name)
		isCorrectRedSchool := checkStringInclude(videoTitle, match.RedSide.Player.Team.CollegeName)
		if isCorrectOrderNum && isCorrectBlueTeam && isCorrectBlueSchool && isCorrectRedTeam && isCorrectRedSchool {
			return video, true
		}
	}
	return types.BiliBiliVideoMetaData{}, false
}
func stringKeyToIntKey[T any](stringKeyMap map[string]T) map[int]T {
	//int作为键的数据存入json时会被变为string，该函数将键转换回去
	intKeyMap := make(map[int]T)
	for key, value := range stringKeyMap {
		intKey, err := strconv.Atoi(key)
		if err == nil {
			intKeyMap[intKey] = value
		}
	}
	return intKeyMap
}
