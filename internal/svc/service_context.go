package svc

import (
	"context"
	"os"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/patrickmn/go-cache"
)

var Cache = cache.New(cache.NoExpiration, 1*time.Minute)

var (
	chromeAllocator context.Context
	chromeCancel    context.CancelFunc
	RenderBaseURL   string
)

func InitChrome() {
	RenderBaseURL = os.Getenv("SCHEDULE_RENDER_BASE_URL")
	if RenderBaseURL == "" {
		RenderBaseURL = "http://127.0.0.1:8080"
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
	)

	chromeAllocator, chromeCancel = chromedp.NewExecAllocator(context.Background(), opts...)
}

func CloseChrome() {
	if chromeCancel != nil {
		chromeCancel()
	}
}

func ChromeAllocator() context.Context {
	return chromeAllocator
}
