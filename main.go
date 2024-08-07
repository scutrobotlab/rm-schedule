package main

import (
	"github.com/gin-gonic/gin"
	"github.com/scutrobotlab/rm-schedule/internal/job"
	"github.com/scutrobotlab/rm-schedule/internal/router"
)

func main() {
	cron := job.InitCronJob()
	cron.Start()
	defer cron.Stop()

	r := gin.Default()
	router.Router(r)
	err := r.Run(":8080")
	if err != nil {
		panic(err)
	}
}
