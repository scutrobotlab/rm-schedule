package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/scutrobotlab/RMSituationBackend/internal/svc"
	"io"
	"log"
	"net/http"
	"time"
)

func GroupRankInfoHandler(c *gin.Context) {
	cached, b := svc.Cache.Get("group_rank_info")
	if b {
		c.Data(200, "application/json", cached.([]byte))
		return
	}

	resp, err := http.Get("https://pro-robomasters-hz-n5i3.oss-cn-hangzhou.aliyuncs.com/live_json/group_rank_info.json")
	if err != nil {
		log.Printf("Failed to get group rank info: %v\n", err)
		c.JSON(500, gin.H{"code": -1, "msg": "Failed to get group rank info"})
		return
	}
	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read group rank info: %v\n", err)
		c.JSON(500, gin.H{"code": -1, "msg": "Failed to read group rank info"})
		return
	}
	svc.Cache.Set("group_rank_info", bytes, 5*time.Second)

	c.Data(200, "application/json", bytes)
}
