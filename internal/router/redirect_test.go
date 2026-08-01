package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/common"
)

func TestCurrentSeasonRedirectRoutesAreNotStatic(t *testing.T) {
	names := []string{
		common.UpstreamNameGroupRankInfo,
		common.UpstreamNameRobotData,
		common.UpstreamNameSchedule,
	}

	for _, name := range names {
		param := RedirectParams[name]
		if param.CacheControl != "public, max-age=1, s-maxage=5" {
			t.Fatalf("%s current-season redirect Cache-Control = %q, want public, max-age=1, s-maxage=5", name, param.CacheControl)
		}
		if _, ok := param.SeasonMap["2026"]; ok {
			t.Fatalf("2026 %s must not use a static season snapshot", name)
		}
		if _, ok := param.StaticZoneSeasonMap["2026"]; ok {
			t.Fatalf("2026 %s must not use static zone merging", name)
		}
	}
}

func TestAPICacheDefaults(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "API defaults to no-store", path: "/api/missing", want: "no-store"},
		{name: "static proxy owns cache policy", path: "/api/static/image.png", want: ""},
		{name: "unversioned export", path: "/api/export_static/image.png", want: "public, max-age=60"},
		{name: "versioned export", path: "/api/export_static/image.png?v=123", want: "public, max-age=3600, s-maxage=86400"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := iris.New()
			app.UseRouter(apiCacheDefaults)
			app.Get("/api/{path:path}", func(ctx iris.Context) {
				ctx.StatusCode(http.StatusOK)
			})
			if err := app.Build(); err != nil {
				t.Fatal(err)
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			app.ServeHTTP(rec, req)

			if got := rec.Header().Get("Cache-Control"); got != tt.want {
				t.Fatalf("Cache-Control = %q, want %q", got, tt.want)
			}
		})
	}
}
