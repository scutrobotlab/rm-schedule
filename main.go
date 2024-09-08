package main

import (
	"embed"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/job"
	"github.com/scutrobotlab/rm-schedule/internal/router"
)

//go:embed public/*
var frontend embed.FS

func main() {
	cron := job.InitCronJob()
	cron.Start()
	defer cron.Stop()

	r := iris.Default()
	router.Router(r, &frontend)

	if err := r.Listen(":8080"); err != nil {
		panic(err)
	}
}
