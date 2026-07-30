package render

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/patrickmn/go-cache"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"golang.org/x/sync/singleflight"
)

const (
	exportOutputID   = "schedule-export-output"
	maxConcurrent    = 3
	resultCacheTTL   = 60 * time.Second
	errorCacheTTL    = 10 * time.Second
	maxRenderTimeout = 90 * time.Second
	pollInterval     = 100 * time.Millisecond
	defaultViewportW = 1920
	defaultViewportH = 1080
)

var (
	sem         = make(chan struct{}, maxConcurrent)
	resultCache = cache.New(resultCacheTTL, time.Minute)
	errorCache  = cache.New(errorCacheTTL, time.Minute)
	sfGroup     singleflight.Group
)

type ParamError struct {
	Msg string
}

func (e *ParamError) Error() string { return e.Msg }

type TimeoutError struct {
	Msg string
}

func (e *TimeoutError) Error() string { return e.Msg }

type RenderError struct {
	Msg string
}

func (e *RenderError) Error() string { return e.Msg }

func cacheKey(season, zoneID, group int, scale float64) string {
	return fmt.Sprintf("%d:%d:%d:%g", season, zoneID, group, scale)
}

func RenderScheduleImage(ctx context.Context, season, zoneID, group int, scale float64) ([]byte, bool, error) {
	key := cacheKey(season, zoneID, group, scale)
	if img, ok := cachedImage(key); ok {
		return img, true, nil
	}
	if err, ok := cachedError(key); ok {
		return nil, false, err
	}

	ch := sfGroup.DoChan(key, func() (interface{}, error) {
		if img, ok := cachedImage(key); ok {
			return renderCacheResult{img, true}, nil
		}
		if err, ok := cachedError(key); ok {
			return nil, err
		}

		renderCtx, cancel := context.WithTimeout(context.Background(), maxRenderTimeout)
		defer cancel()

		img, err := RenderOnce(renderCtx, season, zoneID, group, scale)
		if err == nil {
			errorCache.Delete(key)
			resultCache.Set(key, img, resultCacheTTL)
			return renderCacheResult{img, false}, nil
		}
		errorCache.Set(key, err, errorCacheTTL)
		return renderCacheResult{}, err
	})

	select {
	case result := <-ch:
		if result.Err != nil {
			return nil, false, result.Err
		}
		val, ok := result.Val.(renderCacheResult)
		if !ok {
			return nil, false, &RenderError{Msg: "empty render result"}
		}
		return val.img, val.cached, nil
	case <-ctx.Done():
		return nil, false, &TimeoutError{Msg: ctx.Err().Error()}
	}
}

type renderCacheResult struct {
	img    []byte
	cached bool
}

// cachedImage returns a cache hit. The slice is shared across callers; do not mutate it.
func cachedImage(key string) ([]byte, bool) {
	cached, found := resultCache.Get(key)
	if !found {
		return nil, false
	}
	img, ok := cached.([]byte)
	return img, ok
}

func cachedError(key string) (error, bool) {
	cached, found := errorCache.Get(key)
	if !found {
		return nil, false
	}
	err, ok := cached.(error)
	return err, ok
}

// RenderOnce 执行一次 chromedp 渲染，共享全局 sem 并发限制，不含 TTL 缓存。
// HTTP 路径经 RenderScheduleImage 的 singleflight 调用；后台落盘任务应直接调用本函数，
// 避免走 15s 内存缓存（结果需长期保存而非短期复用）。
func RenderOnce(ctx context.Context, season, zoneID, group int, scale float64) ([]byte, error) {
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

	url := fmt.Sprintf("%s/%d/%d/export?group=%d&live=1", svc.RenderBaseURL, season, zoneID, group)

	var status, content string
	err := chromedp.Run(tabCtx,
		chromedp.EmulateViewport(defaultViewportW, defaultViewportH, chromedp.EmulateScale(scale)),
		chromedp.Navigate(url),
		chromedp.ActionFunc(func(c context.Context) error {
			for {
				if err := c.Err(); err != nil {
					return err
				}

				if err := chromedp.Evaluate(
					fmt.Sprintf(`document.getElementById(%q)?.dataset.status ?? ""`, exportOutputID),
					&status,
				).Do(c); err != nil {
					return err
				}

				if status == "ready" || status == "error" {
					return chromedp.Evaluate(
						fmt.Sprintf(`document.getElementById(%q)?.textContent ?? ""`, exportOutputID),
						&content,
					).Do(c)
				}

				select {
				case <-c.Done():
					return c.Err()
				case <-time.After(pollInterval):
				}
			}
		}),
	)
	if err != nil {
		return nil, classifyRunError(err)
	}

	if status == "error" {
		return nil, &ParamError{Msg: strings.TrimSpace(content)}
	}
	if status != "ready" {
		return nil, &RenderError{Msg: "unexpected export status: " + status}
	}

	return decodeBase64PNG(content)
}

func decodeBase64PNG(content string) ([]byte, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, &RenderError{Msg: "empty export output"}
	}

	if idx := strings.Index(content, ","); strings.HasPrefix(content, "data:") && idx >= 0 {
		content = content[idx+1:]
	}

	img, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return nil, &RenderError{Msg: fmt.Sprintf("base64 decode: %v", err)}
	}
	return img, nil
}

func timeoutFromContext(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
		return &TimeoutError{Msg: ctx.Err().Error()}
	}
	return &RenderError{Msg: ctx.Err().Error()}
}

func classifyRunError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &TimeoutError{Msg: err.Error()}
	}
	return &RenderError{Msg: err.Error()}
}
