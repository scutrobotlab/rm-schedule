package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/scutrobotlab/rm-schedule/internal/job"
	"github.com/scutrobotlab/rm-schedule/internal/static"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
)

const (
	scheduleDebug        = false
	scheduleCacheControl = "public, max-age=5"
)

func ScheduleHandler(c *gin.Context) {
	// 是否存在 Tencent-Acceleration-Domain-Name
	if c.GetHeader("Tencent-Acceleration-Domain-Name") != "" {
		c.Header("Cache-Control", scheduleCacheControl)
		c.Redirect(301, job.ScheduleUrl)
		return
	}

	if scheduleDebug {
		c.Header("Cache-Control", "no-cache")
		c.Data(200, "application/json", static.ScheduleBytes)
		return
	}

	if cached, b := svc.Cache.Get("schedule"); b {
		c.Header("Cache-Control", scheduleCacheControl)
		c.Data(200, "application/json", cached.([]byte))
		return
	}

	c.Header("Cache-Control", scheduleCacheControl)
	c.JSON(500, gin.H{"code": -1, "msg": "Failed to get schedule"})
}
