package handler

import (
	"github.com/gin-gonic/gin"
	"io"
	"log"
	"net/http"
)

func GroupRankInfoHandler(c *gin.Context) {
	resp, err := http.Get("https://pro-robomasters-hz-n5i3.oss-cn-hangzhou.aliyuncs.com/live_json/group_rank_info.json")
	if err != nil {
		log.Printf("Failed to get group rank info: %v\n", err)
		c.JSON(500, gin.H{
			"code": -1,
			"msg":  "Failed to get group rank info",
		})
		return
	}

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read group rank info: %v\n", err)
		c.JSON(500, gin.H{
			"code": -1,
			"msg":  "Failed to read group rank info",
		})
		return
	}

	c.Data(200, "application/json", bytes)
}
