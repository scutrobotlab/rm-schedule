package router

import (
	"github.com/gin-gonic/gin"
	"github.com/scutrobotlab/RMSituationBackend/internal/handler"
)

// Router defines the router for this service
func Router(r *gin.Engine) {
	api := r.Group("/api")
	api.GET("/schedule", handler.ScheduleHandler)
}
