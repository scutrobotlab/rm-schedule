package router

import (
	"strings"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/common"
	"github.com/scutrobotlab/rm-schedule/internal/handler"
	"github.com/scutrobotlab/rm-schedule/internal/storage"
)

func apiCacheDefaults(ctx iris.Context) {
	ctx.Next()

	// Handlers own the cache policy for successful responses. Only provide a
	// default when the handler (including an early error path) did not set one.
	if ctx.ResponseWriter().Header().Get("Cache-Control") != "" {
		return
	}

	requestPath := ctx.Request().URL.Path
	if strings.HasPrefix(requestPath, "/api/export_static/") {
		if ctx.URLParam("v") != "" {
			ctx.Header("Cache-Control", "public, max-age=3600, s-maxage=86400")
		} else {
			ctx.Header("Cache-Control", "public, max-age=60")
		}
	} else if strings.HasPrefix(requestPath, "/api/") {
		ctx.Header("Cache-Control", "no-store")
	}
}

// Router defines the router for this service
func Router(r *iris.Application, frontend string) {
	// 所有 API 默认禁止缓存，确保参数错误、未找到和上游失败等提前返回路径
	// 不会被 CDN 缓存。允许缓存的成功响应由各处理器显式覆盖。
	r.UseRouter(apiCacheDefaults)

	api := r.Party("/api")
	api.Get("/config", handler.ConfigHandler)
	api.Get("/static/*path", handler.RMStaticHandler)
	api.Get("/mp/match", handler.MpMatchHandler)
	api.Get("/match_forecast", handler.MatchForecastHandler)
	api.Get("/match_forecast_image", handler.MatchForecastImageHandler)
	api.Get("/rank", handler.RankListHandler)
	api.Get("/group_rank_info", handler.RedirectRouteHandlerFactory(RedirectParams[common.UpstreamNameGroupRankInfo]))
	api.Get("/robot_data", handler.RedirectRouteHandlerFactory(RedirectParams[common.UpstreamNameRobotData]))
	api.Get("/schedule", handler.RedirectRouteHandlerFactory(RedirectParams[common.UpstreamNameSchedule]))
	api.Get("/match_id_to_video", handler.MatchIDHandler)
	api.Get("/match_order_to_video", handler.MatchOrderHandler)
	api.Get("/team_info", handler.TeamInfoHandler)
	api.Get("/team_abbreviations", handler.TeamAbbreviationsHandler)
	api.Get("/history_match", handler.HistoryMatchHandler)
	api.Get("/live_json/*path", handler.ProxyLiveJsonHandler)
	api.Get("/export_image", handler.ExportImageHandler)
	api.Get("/export_manifest", handler.ExportManifestHandler)

	if storage.EnvStorageBackend() == "local" {
		// cos 后端时图片 URL 由 CosStore 直接返回，不走本地静态托管。
		r.HandleDir("/api/export_static", iris.Dir(storage.EnvStorageDir()), iris.DirOptions{
			IndexName: "",
			ShowList:  false,
			Compress:  false,
		})
	}

	r.HandleDir("/", iris.Dir(frontend), iris.DirOptions{
		IndexName: "index.html",
		ShowList:  false,
		Compress:  true,
	})

	// 404 时回退到 index.html（而不是 Redirect 到 "/"），保留原始路径与 query，
	// 交给前端 vue-router 处理 client-side 路由（如 /2026/616/export?group=0）。
	r.OnErrorCode(iris.StatusNotFound, func(ctx iris.Context) {
		if strings.HasPrefix(ctx.Path(), "/api/") {
			return
		}
		_ = ctx.ServeFile(frontend + "/index.html")
	})
}
