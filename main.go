package main

import (
	"os"
	"strings"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/cors"
	"github.com/kataras/iris/v12/middleware/recover"
	"github.com/kataras/iris/v12/middleware/requestid"
	"github.com/scutrobotlab/rm-schedule/internal/exportjob"
	"github.com/scutrobotlab/rm-schedule/internal/job"
	"github.com/scutrobotlab/rm-schedule/internal/router"
	"github.com/scutrobotlab/rm-schedule/internal/storage"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/sirupsen/logrus"
)

// irisLogLevel 返回 Iris 日志级别（SCHEDULE_LOG_LEVEL，默认 debug）。
// iris.Default() 会强制设为 debug，生产镜像通过将其设为 info 关闭 Iris 的 debug 输出。
func irisLogLevel() string {
	if lvl := strings.TrimSpace(os.Getenv("SCHEDULE_LOG_LEVEL")); lvl != "" {
		return lvl
	}
	return "debug"
}

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

	r := iris.New()
	r.Logger().SetLevel(irisLogLevel())
	// 复刻 iris.Default() 默认注册的中间件（requestid -> recover -> cors）。
	r.UseRouter(requestid.New())
	r.UseRouter(recover.New())
	r.UseRouter(cors.New().
		ExtractOriginFunc(cors.DefaultOriginExtractor).
		ReferrerPolicy(cors.NoReferrerWhenDowngrade).
		AllowOriginFunc(cors.AllowAnyOrigin).
		Handler())
	router.Router(r, "./public")

	if err := r.Listen(":8080"); err != nil {
		panic(err)
	}
}
