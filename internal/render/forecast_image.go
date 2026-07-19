package render

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/patrickmn/go-cache"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"golang.org/x/sync/singleflight"
)

const (
	forecastPosterID       = "forecast-poster"
	forecastDeviceScale    = 2.0
	forecastResultCacheTTL = 1 * time.Second
	forecastErrorCacheTTL  = 3 * time.Second
	// 与 handler.CurrentMatchForecast 共用：截图页请求同源 /api/current_match_forecast，
	// 故 Mock 场次只需设置此环境变量，无需额外查询参数。
	envForecastDebugMatchID = "SCHEDULE_FORECAST_DEBUG_MATCH_ID"
)

var (
	forecastResultCache = cache.New(forecastResultCacheTTL, time.Minute)
	forecastErrorCache  = cache.New(forecastErrorCacheTTL, time.Minute)
	forecastSfGroup     singleflight.Group
)

func forecastDebugMatchID() string {
	return strings.TrimSpace(os.Getenv(envForecastDebugMatchID))
}

func forecastCacheKey(scale float64) string {
	// 纳入 DEBUG match_id，避免「无比赛空海报」与 Mock 场次在 1s 缓存内互相污染。
	if debugID := forecastDebugMatchID(); debugID != "" {
		return fmt.Sprintf("forecast:%g:debug:%s", scale, debugID)
	}
	return fmt.Sprintf("forecast:%g", scale)
}

// RenderForecastImage 打开前端 /forecast?render=1，等待 #forecast-poster 就绪后截取元素 PNG。
// 固定 deviceScaleFactor=2，CSS 画幅 1920×1080，成品为 3840×2160。
// Mock 场次复用 SCHEDULE_FORECAST_DEBUG_MATCH_ID（由 /api/current_match_forecast 生效）。
// 结果带 1s 内存缓存；并发渲染复用全局 sem（上限 3）。
func RenderForecastImage(ctx context.Context) ([]byte, bool, error) {
	scale := forecastDeviceScale
	key := forecastCacheKey(scale)
	if img, ok := forecastCachedImage(key); ok {
		return img, true, nil
	}
	if err, ok := forecastCachedError(key); ok {
		return nil, false, err
	}

	ch := forecastSfGroup.DoChan(key, func() (interface{}, error) {
		if img, ok := forecastCachedImage(key); ok {
			return renderCacheResult{img, true}, nil
		}
		if err, ok := forecastCachedError(key); ok {
			return nil, err
		}

		renderCtx, cancel := context.WithTimeout(context.Background(), maxRenderTimeout)
		defer cancel()

		img, err := renderForecastOnce(renderCtx, scale)
		if err == nil {
			forecastErrorCache.Delete(key)
			forecastResultCache.Set(key, img, forecastResultCacheTTL)
			return renderCacheResult{img, false}, nil
		}
		forecastErrorCache.Set(key, err, forecastErrorCacheTTL)
		return renderCacheResult{}, err
	})

	select {
	case result := <-ch:
		if result.Err != nil {
			return nil, false, result.Err
		}
		val, ok := result.Val.(renderCacheResult)
		if !ok {
			return nil, false, &RenderError{Msg: "empty forecast render result"}
		}
		return val.img, val.cached, nil
	case <-ctx.Done():
		return nil, false, &TimeoutError{Msg: ctx.Err().Error()}
	}
}

func forecastCachedImage(key string) ([]byte, bool) {
	cached, found := forecastResultCache.Get(key)
	if !found {
		return nil, false
	}
	img, ok := cached.([]byte)
	return img, ok
}

func forecastCachedError(key string) (error, bool) {
	cached, found := forecastErrorCache.Get(key)
	if !found {
		return nil, false
	}
	err, ok := cached.(error)
	return err, ok
}

// renderForecastOnce 执行一次 chromedp 元素截图，共享全局 sem，不含 TTL 缓存。
func renderForecastOnce(ctx context.Context, scale float64) ([]byte, error) {
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		return nil, timeoutFromContext(ctx)
	}

	tabCtx, tabCancel := chromedp.NewContext(svc.ChromeAllocator())
	defer tabCancel()

	go func() {
		select {
		case <-ctx.Done():
			tabCancel()
		case <-tabCtx.Done():
		}
	}()

	url := fmt.Sprintf("%s/forecast?render=1", svc.RenderBaseURL)

	var status, errText string
	var img []byte
	err := chromedp.Run(tabCtx,
		chromedp.EmulateViewport(defaultViewportW, defaultViewportH, chromedp.EmulateScale(scale)),
		chromedp.Navigate(url),
		chromedp.ActionFunc(func(c context.Context) error {
			for {
				if err := c.Err(); err != nil {
					return err
				}

				if err := chromedp.Evaluate(
					fmt.Sprintf(`document.getElementById(%q)?.dataset.status ?? ""`, forecastPosterID),
					&status,
				).Do(c); err != nil {
					return err
				}

				if status == "ready" || status == "error" {
					if status == "error" {
						return chromedp.Evaluate(
							fmt.Sprintf(`document.getElementById(%q)?.dataset.error ?? ""`, forecastPosterID),
							&errText,
						).Do(c)
					}
					return nil
				}

				select {
				case <-c.Done():
					return c.Err()
				case <-time.After(pollInterval):
				}
			}
		}),
		chromedp.ActionFunc(func(c context.Context) error {
			if status == "error" {
				return nil
			}
			return chromedp.Screenshot("#"+forecastPosterID, &img, chromedp.NodeVisible).Do(c)
		}),
	)
	if err != nil {
		return nil, classifyRunError(err)
	}

	if status == "error" {
		msg := strings.TrimSpace(errText)
		if msg == "" {
			msg = "forecast poster reported error"
		}
		return nil, &RenderError{Msg: msg}
	}
	if status != "ready" {
		return nil, &RenderError{Msg: "unexpected forecast status: " + status}
	}
	if len(img) == 0 {
		return nil, &RenderError{Msg: "empty forecast screenshot"}
	}
	return img, nil
}
