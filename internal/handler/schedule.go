package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/scutrobotlab/rm-schedule/internal/static"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const scheduleDebug = false

func ScheduleHandler(c *gin.Context) {
	if scheduleDebug {
		c.Header("Cache-Control", "no-cache")
		c.Data(200, "application/json", static.ScheduleBytes)
		return
	}

	cached, b := svc.Cache.Get("schedule")
	if b {
		c.Header("Cache-Control", "public, max-age=5")
		c.Data(200, "application/json", cached.([]byte))
		return
	}

	resp, err := http.Get("https://pro-robomasters-hz-n5i3.oss-cn-hangzhou.aliyuncs.com/live_json/schedule.json")
	if err != nil {
		log.Printf("Failed to get schedule: %v\n", err)
		c.JSON(500, gin.H{"code": -1, "msg": "Failed to get schedule"})
		return
	}
	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read schedule: %v\n", err)
		c.JSON(500, gin.H{"code": -1, "msg": "Failed to read schedule"})
		return
	}
	bytes = replaceRMStatic(bytes)
	svc.Cache.Set("schedule", bytes, 5*time.Second)

	c.Header("Cache-Control", "public, max-age=5")
	c.Data(200, "application/json", bytes)
}

func replaceRMStatic(data []byte) []byte {
	str := string(data)
	str = strings.ReplaceAll(str, "https://rm-static.djicdn.com", "/api/static/rm-static_djicdn_com")
	str = strings.ReplaceAll(str, "https://terra-cn-oss-cdn-public-pro.oss-cn-hangzhou.aliyuncs.com", "/api/static/terra-cn-oss-cdn-public-pro_oss-cn-hangzhou_aliyuncs_com")
	str = strings.ReplaceAll(str, "https://pro-robomasters-hz-n5i3.oss-cn-hangzhou.aliyuncs.com", "/api/static/pro-robomasters-hz-n5i3_oss-cn-hangzhou_aliyuncs_com")
	return []byte(str)
}
