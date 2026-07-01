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
	resultCacheTTL   = 15 * time.Second
	maxRenderTimeout = 90 * time.Second
	pollInterval     = 100 * time.Millisecond
	defaultViewportW = 1920
	defaultViewportH = 1080
)

var (
	sem         = make(chan struct{}, maxConcurrent)
	resultCache = cache.New(resultCacheTTL, time.Minute)
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

func RenderScheduleImage(ctx context.Context, season, zoneID, group int, scale float64) ([]byte, error) {
	key := cacheKey(season, zoneID, group, scale)
	if cached, found := resultCache.Get(key); found {
		return cached.([]byte), nil
	}

	// singleflight 的 fn 由第一个到达的调用者触发执行，因此用它的 ctx 派生渲染
	// ctx：该请求断开/超时会真正取消共享的 Chrome tab，而不仅仅是让当前
	// select 提前返回（若改用 context.Background()，即便所有等待者都已
	// 断开，后台渲染仍会跑满 maxRenderTimeout，白白占着信号量与 tab）。
	renderCtx, cancel := context.WithTimeout(ctx, maxRenderTimeout)
	defer cancel()

	ch := sfGroup.DoChan(key, func() (interface{}, error) {
		if cached, found := resultCache.Get(key); found {
			return cached.([]byte), nil
		}

		img, err := renderScheduleImage(renderCtx, season, zoneID, group, scale)
		if err == nil {
			resultCache.Set(key, img, resultCacheTTL)
		}
		return img, err
	})

	select {
	case result := <-ch:
		if result.Err != nil {
			return nil, result.Err
		}
		img, ok := result.Val.([]byte)
		if !ok {
			return nil, &RenderError{Msg: "empty render result"}
		}
		return img, nil
	case <-ctx.Done():
		return nil, &TimeoutError{Msg: ctx.Err().Error()}
	}
}

func renderScheduleImage(ctx context.Context, season, zoneID, group int, scale float64) ([]byte, error) {
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
