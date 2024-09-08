package handler

import (
	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/job"
	"github.com/scutrobotlab/rm-schedule/internal/static"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
)

const (
	scheduleDebug        = false
	scheduleCacheControl = "public, max-age=5"
)

func ScheduleHandler(c iris.Context) {
	// 是否存在 Tencent-Acceleration-Domain-Name
	if c.GetHeader("Tencent-Acceleration-Domain-Name") != "" {
		c.Header("Cache-Control", scheduleCacheControl)
		c.Redirect(job.ScheduleUrl, 301)
		return
	}

	if scheduleDebug {
		c.Header("Cache-Control", "no-cache")
		c.ContentType("application/json")
		c.Write(static.ScheduleBytes)
		return
	}

	if cached, b := svc.Cache.Get("schedule"); b {
		c.Header("Cache-Control", scheduleCacheControl)
		c.ContentType("application/json")
		c.Write(cached.([]byte))
		return
	}

	c.Header("Cache-Control", scheduleCacheControl)
	c.StatusCode(500)
	c.JSON(iris.Map{"code": -1, "msg": "Failed to get schedule"})
}
