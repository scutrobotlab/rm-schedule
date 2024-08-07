package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
)

const (
	groupRankInfoCacheControl = "public, max-age=5"
)

func GroupRankInfoHandler(c *gin.Context) {
	if cached, b := svc.Cache.Get("group_rank_info"); b {
		c.Header("Cache-Control", groupRankInfoCacheControl)
		c.Data(200, "application/json", cached.([]byte))
		return
	}

	c.Header("Cache-Control", groupRankInfoCacheControl)
	c.JSON(500, gin.H{"code": -1, "msg": "Failed to get group rank info"})
}
