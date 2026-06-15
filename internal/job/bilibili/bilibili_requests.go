package bilibili

import (
	"encoding/json"
	"fmt"
	"github.com/patrickmn/go-cache"
	"github.com/scutrobotlab/rm-schedule/internal/common"
	"github.com/scutrobotlab/rm-schedule/internal/static"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/scutrobotlab/rm-schedule/internal/types"
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
)

func processRequest(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:139.0) Gecko/20100101 Firefox/139.0")
	req.Header.Set("Host", "api.bilibili.com")
	req.Header.Set("Referer", "https://space.bilibili.com/")
	req.Header.Set("Origin", "https://space.bilibili.com/")
}

func getCollectionList() ([]types.BiliBiliCollectionMetaData, error) {
	//获取组委会b站空间中的所有合集
	pageNum := 1
	moreCollections := true
	var collections []types.BiliBiliCollectionMetaData
	for pageNum <= 10 && moreCollections {
		req, err := http.NewRequest("GET", fmt.Sprintf(common.UpstreamUrlBilibiliCollections, pageNum), nil)
		if err != nil {
			return []types.BiliBiliCollectionMetaData{}, fmt.Errorf("Failed to get collections page %d: %v\n", pageNum, err)
		}
		processRequest(req)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return []types.BiliBiliCollectionMetaData{}, fmt.Errorf("Failed to get collections page %d: %v\n", pageNum, err)
		}

		bytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return []types.BiliBiliCollectionMetaData{}, fmt.Errorf("Failed to read collections page %d: %v\n", pageNum, err)
		}

		var data types.BiliBiliCollectionResponse
		err = json.Unmarshal(bytes, &data)
		if err != nil {
			return []types.BiliBiliCollectionMetaData{}, fmt.Errorf("Failed to read collections page %d: %v\n", pageNum, err)
		}
		if data.Code != 0 || data.Message != "OK" {
			return []types.BiliBiliCollectionMetaData{}, fmt.Errorf("Failed to read collections page %d: %s\n", pageNum, data.Message)
		}

		if data.Data.ItemsLists.Page.Total <= data.Data.ItemsLists.Page.PageSize*pageNum {
			moreCollections = false
		} else {
			pageNum += 1
		}

		for _, collectionMeta := range data.Data.ItemsLists.CollectionList {
			collections = append(collections, collectionMeta.Meta)
		}

		resp.Body.Close()
	}
	return collections, nil
}

func getCollectionInfo(id int) (types.BiliBiliCollectionInfo, error) {
	//获取合集中所有视频的信息
	//b站限制每次最多获取100个视频，但同时每赛区不会超过100场，因此不循环
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.bilibili.com/x/polymer/web-space/seasons_archives_list?season_id=%d&page_size=100&page_num=1", id), nil)
	if err != nil {
		return types.BiliBiliCollectionInfo{}, fmt.Errorf("Failed to get collection %d: %v\n", id, err)
	}
	processRequest(req)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return types.BiliBiliCollectionInfo{}, fmt.Errorf("Failed to get collection %d: %v\n", id, err)
	}
	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.BiliBiliCollectionInfo{}, fmt.Errorf("Failed to get collection %d: %v\n", id, err)
	}

	var data types.BiliBiliCollectionInfo
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		return types.BiliBiliCollectionInfo{}, fmt.Errorf("Failed to get collection %d: %v\n", id, err)
	}
	if data.Code != 0 || data.Message != "OK" {
		return types.BiliBiliCollectionInfo{}, fmt.Errorf("Failed to get collection %d: %v\n", id, data.Message)
	}
	return data, nil
}

func FetchBiliBiliReplayVideos() {
	previousSeasons := map[string][]byte{
		"2024": static.BilibiliVideosBytes2024,
		"2025": static.BilibiliVideosBytes2025,
	}
	matchIDToVideo := make(map[string]types.BiliBiliVideoMetaData)
	matchOrderToVideoAll := make(types.MatchOrderToVideoType)
	matchesList := getMatches()
	for season, zones := range matchesList {
		skipCurrentSeason := false

		for previousSeason, previousDataBytes := range previousSeasons {
			if season == previousSeason {
				//若该赛季属于历史数据
				var previousDataRaw map[string]map[string]types.BiliBiliVideoMetaData
				_ = json.Unmarshal(previousDataBytes, &previousDataRaw)
				previousData := make(map[string]map[int]types.BiliBiliVideoMetaData)
				for zone, matches := range previousDataRaw {
					previousData[zone] = stringKeyToIntKey(matches)
				}

				skipCurrentSeason = true
				matchOrderToVideoAll[season] = previousData
				for zone, matches := range zones {
					for _, match := range matches {
						videoData, ok := previousData[zone][match.OrderNumber]
						if ok {
							matchIDToVideo[match.ID] = videoData
						}
					}
				}
			}
		}
		if skipCurrentSeason {
			//跳过从b站获取该赛季相关内容的代码
			continue
		}

		matchOrderToVideoSingleSeason := make(map[string]map[int]types.BiliBiliVideoMetaData)
		collectionList, err := getCollectionList()
		if err != nil {
			logrus.Error(err)
			collectionList = []types.BiliBiliCollectionMetaData{}
		}
		var staticDataRaw map[string]map[string]types.BiliBiliVideoMetaData
		_ = json.Unmarshal(static.BilibiliVideosBytes, &staticDataRaw)
		staticData := make(map[string]map[int]types.BiliBiliVideoMetaData)
		for zone, matches := range staticDataRaw {
			staticData[zone] = stringKeyToIntKey(matches)
		}
		for zone, matches := range zones {
			matchOrderToVideoSingleZone := make(map[int]types.BiliBiliVideoMetaData)
			skipCurrentZone := false

			for staticZone, staticMatches := range staticData {
				if zone == staticZone {
					//若当前赛季不是历史赛季，但当前赛区属于历史数据
					skipCurrentZone = true
					matchOrderToVideoSingleSeason[zone] = staticMatches
					for _, match := range matches {
						videoData, ok := staticMatches[match.OrderNumber]
						if ok {
							matchIDToVideo[match.ID] = videoData
						}
					}
				}
			}
			if skipCurrentZone {
				//跳过从b站获取该赛区相关内容的代码
				continue
			}

			targetCollection, ok := findCollection(season, zone, &collectionList)
			if ok {
				collectionInfo, err := getCollectionInfo(targetCollection.CollectionId)
				if err != nil {
					logrus.Error(err)
					continue
				}
				for _, match := range matches {
					correspondingVideo, ok := findMatchVideo(&match, &collectionInfo)
					if !ok {
						logrus.Errorf("video for match %s not found", match.ID)
						continue
					}
					matchIDToVideo[match.ID] = correspondingVideo
					matchOrderToVideoSingleZone[match.OrderNumber] = correspondingVideo
				}
			}
			if len(matchOrderToVideoSingleZone) > 0 {
				matchOrderToVideoSingleSeason[zone] = matchOrderToVideoSingleZone
			}
		}
		if len(matchOrderToVideoSingleSeason) > 0 {
			matchOrderToVideoAll[season] = matchOrderToVideoSingleSeason
		}
	}
	svc.Cache.Set("match_id_to_video", matchIDToVideo, cache.NoExpiration)
	if len(matchOrderToVideoAll) > 0 {
		svc.Cache.Set("match_order_to_video", matchOrderToVideoAll, cache.NoExpiration)
	}
	logrus.Info("Bilibili collections updated")
}
