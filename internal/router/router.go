package router

import (
	"github.com/gin-gonic/gin"
	"github.com/scutrobotlab/RMSituationBackend/internal/handler"
)

// Router defines the router for this service
func Router(r *gin.Engine) {
	api := r.Group("/api")
	api.GET("/schedule", handler.ScheduleHandler)
	api.GET("/group_rank_info", handler.GroupRankInfoHandler)
	api.GET("/static/rm-static_djicdn_com/games-backend/:uuid", handler.RMStaticHandler)
}
