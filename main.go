package main

import (
	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/job"
	"github.com/scutrobotlab/rm-schedule/internal/router"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
)

func main() {
	svc.InitChrome()
	defer svc.CloseChrome()

	cron := job.InitCronJob()
	cron.Start()
	defer cron.Stop()

	r := iris.Default()
	router.Router(r, "./public")

	if err := r.Listen(":8080"); err != nil {
		panic(err)
	}
}
