package router

import (
	"embed"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/handler"
)

// Router defines the router for this service
func Router(r *iris.Application, frontend *embed.FS) {
	api := r.Party("/api")
	api.Get("/schedule", handler.ScheduleHandler)
	api.Get("/group_rank_info", handler.GroupRankInfoHandler)
	api.Get("/static/*path", handler.RMStaticHandler)
	api.Get("/mp/match", handler.MpMatchHandler)
	api.Get("/rank", handler.RankListHandler)

	r.HandleDir("/", *frontend, iris.DirOptions{
		IndexName: "index.html",
		ShowList:  true,
		Compress:  true,
	})

	// on 404, redirect to the index.html
	r.OnErrorCode(iris.StatusNotFound, func(ctx iris.Context) {
		ctx.Redirect("/", iris.StatusTemporaryRedirect)
	})
}
