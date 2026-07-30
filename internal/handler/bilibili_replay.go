package handler

import (
	"encoding/json"
	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/scutrobotlab/rm-schedule/internal/types"
	"github.com/sirupsen/logrus"
	"strconv"
)

func MatchIDHandler(c iris.Context) {
	all := c.URLParam("all")
	if all != "" {
		_ret, _ := svc.Cache.Get("match_id_to_video")
		ret := _ret.(map[string]types.BiliBiliVideoMetaData)
		jsonData, err := json.Marshal(ret)
		if err != nil {
			logrus.Error(err)
		} else {
			c.Header("Cache-Control", "public, max-age=60, s-maxage=300, stale-while-revalidate=60")
			c.JSON(string(jsonData))
			return
		}
	}

	matchId := c.URLParam("match_id")
	if matchId == "" {
		c.StatusCode(400)
		c.JSON(iris.Map{"error": "match_id is required"})
		return
	}

	_ret, ok := svc.Cache.Get("match_id_to_video")
	if !ok {
		c.StatusCode(500)
		c.JSON(iris.Map{"code": -1, "msg": "Failed to get videos"})
		return
	}

	ret := _ret.(map[string]types.BiliBiliVideoMetaData)
	video, ok := ret[matchId]
	if !ok {
		c.StatusCode(404)
		c.JSON(iris.Map{"code": -1, "msg": "Match not found"})
		return
	}
	c.Header("Cache-Control", "public, max-age=60, s-maxage=300, stale-while-revalidate=60")
	c.JSON(video)
	return
}

func MatchOrderHandler(c iris.Context) {
	all := c.URLParam("all")
	if all != "" {
		_ret, _ := svc.Cache.Get("match_order_to_video")
		ret := _ret.(types.MatchOrderToVideoType)
		jsonData, err := json.Marshal(ret)
		if err != nil {
			logrus.Error(err)
		} else {
			c.Header("Cache-Control", "public, max-age=60, s-maxage=300, stale-while-revalidate=60")
			c.JSON(string(jsonData))
			return
		}
	}

	season := c.URLParam("season")
	zone := c.URLParam("zone")
	_orderNumber := c.URLParam("order_number")
	if season == "" || zone == "" || _orderNumber == "" {
		c.StatusCode(400)
		c.JSON(iris.Map{"error": "season & zone & order_number is required"})
		return
	}
	orderNumber, err := strconv.Atoi(_orderNumber)
	if err != nil {
		c.StatusCode(400)
		c.JSON(iris.Map{"error": "order_number should be int"})
		return
	}

	_ret, ok := svc.Cache.Get("match_order_to_video")
	if !ok {
		c.StatusCode(500)
		c.JSON(iris.Map{"code": -1, "msg": "Failed to get videos"})
		return
	}
	ret := _ret.(types.MatchOrderToVideoType)

	selectedSeason, ok := ret[season]
	if !ok {
		c.StatusCode(404)
		c.JSON(iris.Map{"code": -1, "msg": "Season not found"})
		return
	}
	selectedZone, ok := selectedSeason[zone]
	if !ok {
		c.StatusCode(404)
		c.JSON(iris.Map{"code": -1, "msg": "Zone not found"})
		return
	}
	video, ok := selectedZone[orderNumber]
	if !ok {
		c.StatusCode(404)
		c.JSON(iris.Map{"code": -1, "msg": "Match not found"})
		return
	}
	c.Header("Cache-Control", "public, max-age=60, s-maxage=300, stale-while-revalidate=60")
	c.JSON(video)
	return
}
