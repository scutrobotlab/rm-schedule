package render

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/patrickmn/go-cache"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"golang.org/x/sync/singleflight"
)

const (
	forecastPosterID      = "forecast-poster"
	forecastDeviceScale   = 2.0
	forecastErrorCacheTTL = 5 * time.Second
	forecastMaxStale      = 5 * time.Minute
	// 与 handler.MatchForecast 共用：截图页请求同源 /api/match_forecast，
	// 故 Mock 场次只需设置此环境变量，无需额外查询参数。
	envForecastDebugMatchID = "SCHEDULE_FORECAST_DEBUG_MATCH_ID"
)

var (
	forecastImages       = newForecastImageStore()
	forecastErrorCache   = cache.New(forecastErrorCacheTTL, time.Minute)
	forecastSfGroup      singleflight.Group
	forecastNow          = time.Now
	runForecastRender    = renderForecastOnce
	forecastFlightJoined = func(int64) {}
)

// forecastImage 保存一张已经完整生成并可对外发布的预测图。
// Data 发布后不可修改，因此请求可以在释放 store 锁后安全读取。
type forecastImage struct {
	Data       []byte
	Version    int64
	RenderedAt time.Time
}

type forecastImageEntry struct {
	image      *forecastImage
	lastAccess time.Time
}

type forecastImageStore struct {
	mu      sync.RWMutex
	entries map[string]forecastImageEntry
}

func newForecastImageStore() *forecastImageStore {
	return &forecastImageStore{entries: make(map[string]forecastImageEntry)}
}

func (s *forecastImageStore) get(matchID string, now time.Time, maxAge time.Duration) (*forecastImage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[matchID]
	if !ok || entry.image == nil || now.Sub(entry.image.RenderedAt) > maxAge {
		return nil, false
	}
	entry.lastAccess = now
	s.entries[matchID] = entry
	return entry.image, true
}

func (s *forecastImageStore) version(matchID string, now time.Time) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[matchID]
	if !ok || entry.image == nil || now.Sub(entry.image.RenderedAt) > forecastMaxStale {
		return 0, false
	}
	entry.lastAccess = now
	s.entries[matchID] = entry
	return entry.image.Version, true
}

func (s *forecastImageStore) publish(matchID string, img []byte, version int64, renderedAt time.Time) *forecastImage {
	published := &forecastImage{
		Data:       img,
		Version:    version,
		RenderedAt: renderedAt,
	}
	s.mu.Lock()
	s.entries[matchID] = forecastImageEntry{image: published, lastAccess: renderedAt}
	s.mu.Unlock()
	return published
}

func (s *forecastImageStore) cleanup(targets map[string]struct{}, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for matchID, entry := range s.entries {
		if _, keep := targets[matchID]; keep {
			continue
		}
		if now.Sub(entry.lastAccess) > forecastMaxStale {
			delete(s.entries, matchID)
		}
	}
}

func forecastDebugMatchID() string {
	return strings.TrimSpace(os.Getenv(envForecastDebugMatchID))
}

func forecastCacheKey(scale float64, matchID string) string {
	if matchID != "" {
		return fmt.Sprintf("forecast:%g:match:%s", scale, matchID)
	}
	// 纳入 DEBUG match_id，避免「无比赛空海报」与 Mock 场次在 5s 缓存内互相污染。
	if debugID := forecastDebugMatchID(); debugID != "" {
		return fmt.Sprintf("forecast:%g:debug:%s", scale, debugID)
	}
	return fmt.Sprintf("forecast:%g", scale)
}

// RenderForecastImage 打开前端 /forecast?render=1，等待 #forecast-poster 就绪后截取元素 PNG。
// 固定 deviceScaleFactor=2，CSS 画幅 1920×1080，成品为 3840×2160。
// matchID 非空时透传给前端；否则由 /api/match_forecast 选择当前比赛或 Mock 场次。
// 已发布图片在后台刷新期间继续服务，最多允许陈旧 5 分钟；并发渲染复用全局 sem（上限 3）。
func RenderForecastImage(ctx context.Context, matchID string) ([]byte, bool, error) {
	now := forecastNow()
	if active, ok := forecastImages.get(matchID, now, forecastMaxStale); ok {
		return active.Data, true, nil
	}
	key := forecastCacheKey(forecastDeviceScale, matchID)
	if err, ok := forecastCachedError(key); ok {
		return nil, false, err
	}

	img, cached, err := refreshForecastImage(ctx, matchID, now.Truncate(time.Minute).Unix())
	if err != nil {
		forecastErrorCache.Set(key, err, forecastErrorCacheTTL)
		return nil, false, err
	}
	forecastErrorCache.Delete(key)
	return img, cached, nil
}

// RefreshForecastImage 为后台预热强制生成当前分钟版本。已有当前分钟版本时快速返回；
// 失败不会写入 HTTP 路径使用的错误缓存，也不会替换仍可服务的 active 图片。
func RefreshForecastImage(ctx context.Context, matchID string) ([]byte, bool, error) {
	now := forecastNow()
	return refreshForecastImage(ctx, matchID, now.Truncate(time.Minute).Unix())
}

func refreshForecastImage(ctx context.Context, matchID string, requestedVersion int64) ([]byte, bool, error) {
	if active, ok := forecastImages.get(matchID, forecastNow(), forecastMaxStale); ok && active.Version >= requestedVersion {
		return active.Data, true, nil
	}

	// 同一场次跨分钟也只允许一个渲染任务；完成时按实际发布时间生成版本，
	// 避免分钟边界的 HTTP 兜底与后台预热各自启动 Chromium。
	flightKey := forecastCacheKey(forecastDeviceScale, matchID)
	ch := forecastSfGroup.DoChan(flightKey, func() (interface{}, error) {
		if active, ok := forecastImages.get(matchID, forecastNow(), forecastMaxStale); ok && active.Version >= requestedVersion {
			return renderCacheResult{active.Data, true}, nil
		}

		renderCtx, cancel := context.WithTimeout(context.Background(), maxRenderTimeout)
		defer cancel()

		img, err := runForecastRender(renderCtx, forecastDeviceScale, matchID)
		if err != nil {
			return renderCacheResult{}, err
		}
		published := forecastImages.publish(matchID, img, requestedVersion, forecastNow())
		return renderCacheResult{published.Data, false}, nil
	})
	forecastFlightJoined(requestedVersion)

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

func forecastCachedError(key string) (error, bool) {
	cached, found := forecastErrorCache.Get(key)
	if !found {
		return nil, false
	}
	err, ok := cached.(error)
	return err, ok
}

// ForecastImageVersion 返回当前仍可服务的已发布图片版本。
func ForecastImageVersion(matchID string) (int64, bool) {
	return forecastImages.version(matchID, forecastNow())
}

// CleanupForecastImages 清理不再属于预热目标且已超过 5 分钟未访问的图片。
func CleanupForecastImages(targetMatchIDs []string) {
	targets := make(map[string]struct{}, len(targetMatchIDs))
	for _, matchID := range targetMatchIDs {
		targets[matchID] = struct{}{}
	}
	forecastImages.cleanup(targets, forecastNow())
}

// renderForecastOnce 执行一次 chromedp 元素截图，共享全局 sem，不含 TTL 缓存。
func renderForecastOnce(ctx context.Context, scale float64, matchID string) ([]byte, error) {
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

	renderURL := forecastRenderURL(matchID)

	var status, errText string
	var img []byte
	err := chromedp.Run(tabCtx,
		chromedp.EmulateViewport(defaultViewportW, defaultViewportH, chromedp.EmulateScale(scale)),
		chromedp.Navigate(renderURL),
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

func forecastRenderURL(matchID string) string {
	renderURL := fmt.Sprintf("%s/forecast?render=1", svc.RenderBaseURL)
	if matchID == "" {
		return renderURL
	}
	return renderURL + "&match_id=" + url.QueryEscape(matchID)
}
