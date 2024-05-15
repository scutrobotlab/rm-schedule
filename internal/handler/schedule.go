package handler

import (
	"github.com/gin-gonic/gin"
	"io"
	"log"
	"net/http"
)

func ScheduleHandler(c *gin.Context) {
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

	c.Data(200, "application/json", bytes)
}
