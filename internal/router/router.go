package router

import (
	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/common"
	"github.com/scutrobotlab/rm-schedule/internal/handler"
)

// Router defines the router for this service
func Router(r *iris.Application, frontend string) {
	api := r.Party("/api")
	api.Get("/static/*path", handler.RMStaticHandler)
	api.Get("/mp/match", handler.MpMatchHandler)
	api.Get("/rank", handler.RankListHandler)
	api.Get("/group_rank_info", handler.RedirectRouteHandlerFactory(RedirectParams[common.UpstreamNameGroupRankInfo]))
	api.Get("/robot_data", handler.RedirectRouteHandlerFactory(RedirectParams[common.UpstreamNameRobotData]))
	api.Get("/schedule", handler.RedirectRouteHandlerFactory(RedirectParams[common.UpstreamNameSchedule]))
	api.Get("/match_id_to_video", handler.MatchIDHandler)
	api.Get("/match_order_to_video", handler.MatchOrderHandler)
	api.Get("/team_info", handler.TeamInfoHandler)
	api.Get("/history_match", handler.HistoryMatchHandler)
	api.Get("/live_json/*path", handler.ProxyLiveJsonHandler)
	api.Get("/export_image", handler.ExportImageHandler)

	r.HandleDir("/", iris.Dir(frontend), iris.DirOptions{
		IndexName: "index.html",
		ShowList:  false,
		Compress:  true,
	})

	// on 404, redirect to the index.html
	r.OnErrorCode(iris.StatusNotFound, func(ctx iris.Context) {
		ctx.Redirect("/", iris.StatusTemporaryRedirect)
	})
}
