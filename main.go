package main

import (
	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/exportjob"
	"github.com/scutrobotlab/rm-schedule/internal/job"
	"github.com/scutrobotlab/rm-schedule/internal/router"
	"github.com/scutrobotlab/rm-schedule/internal/storage"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/sirupsen/logrus"
)

func main() {
	svc.InitChrome()
	defer svc.CloseChrome()

	exportStore, err := storage.NewStoreFromEnv()
	if err != nil {
		logrus.Fatalf("init export storage failed: %v", err)
	}
	exportjob.Bootstrap(exportStore) // 同步恢复 meta，归档区缺图渲染在后台进行

	cron := job.InitCronJob()

	// 与 OSS schedule 拉取同频（5s），对比 hash 后按需触发后台渲染。
	checkAndRenderExport := func() { exportjob.CheckAndRender(exportStore) }
	if _, err := cron.AddFunc("@every 5s", checkAndRenderExport); err != nil {
		logrus.Fatalf("cron add func failed: %v", err)
	}

	cron.Start()
	defer cron.Stop()

	r := iris.Default()
	router.Router(r, "./public")

	if err := r.Listen(":8080"); err != nil {
		panic(err)
	}
}
