package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/scutrobotlab/RMSituationBackend/internal/svc"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func ScheduleHandler(c *gin.Context) {
	cached, b := svc.Cache.Get("schedule")
	if b {
		c.Data(200, "application/json", cached.([]byte))
		return
	}

	resp, err := http.Get("https://pro-robomasters-hz-n5i3.oss-cn-hangzhou.aliyuncs.com/live_json/schedule.json")
	if err != nil {
		log.Printf("Failed to get schedule: %v\n", err)
		c.JSON(500, gin.H{
			"code": -1,
			"msg":  "Failed to get schedule",
		})
		return
	}

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read schedule: %v\n", err)
		c.JSON(500, gin.H{
			"code": -1,
			"msg":  "Failed to read schedule",
		})
		return
	}
	bytes = replaceRMStatic(bytes)
	svc.Cache.Set("schedule", bytes, 5*time.Second)

	c.Data(200, "application/json", bytes)
}

func replaceRMStatic(data []byte) []byte {
	str := string(data)
	str = strings.ReplaceAll(str, "https://rm-static.djicdn.com/games-backend/", "/api/static/rm-static_djicdn_com/games-backend/")
	return []byte(str)
}
