package router

import (
	"github.com/gin-gonic/gin"
	"github.com/scutrobotlab/rm-schedule/internal/handler"
)

// Router defines the router for this service
func Router(r *gin.Engine) {
	api := r.Group("/api")
	api.GET("/schedule", handler.ScheduleHandler)
	api.GET("/group_rank_info", handler.GroupRankInfoHandler)
	api.GET("/static/*path", handler.RMStaticHandler)
	api.GET("/mp/match", handler.MpMatchHandler)
	api.GET("/rank", handler.RankListHandler)
}
