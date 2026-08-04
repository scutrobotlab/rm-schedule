package handler

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/scutrobotlab/rm-schedule/internal/render"
	"github.com/scutrobotlab/rm-schedule/internal/types"
	"github.com/sirupsen/logrus"
)

const forecastPrewarmRetryInterval = 15 * time.Second

type forecastPrewarmAttempt struct {
	attemptedAt time.Time
}

var (
	forecastPrewarmRunning  atomic.Bool
	forecastPrewarmMu       sync.Mutex
	forecastPrewarmAttempts = make(map[string]forecastPrewarmAttempt)
	forecastPrewarmNow      = time.Now
	refreshForecastImage    = render.RefreshForecastImage
	prewarmedImageVersion   = render.ForecastImageVersion
	cleanupForecastImages   = render.CleanupForecastImages
)

// CheckAndPrewarmForecastImages 由短周期 cron 调用。检查周期为 5 秒，但同一场次
// 正常每分钟只渲染一次；场次切换会立即预热，失败最早 15 秒后重试。
func CheckAndPrewarmForecastImages() {
	if !forecastPrewarmRunning.CompareAndSwap(false, true) {
		return
	}
	defer forecastPrewarmRunning.Store(false)

	schedule, ok := loadCachedSchedule()
	if !ok {
		return
	}

	matchIDs := forecastPrewarmMatchIDs(schedule)
	cleanupForecastImages(matchIDs)
	if len(matchIDs) == 0 {
		return
	}

	pruneForecastPrewarmAttempts(matchIDs)

	for _, matchID := range matchIDs {
		// 每场次单独取当前分钟，避免前一场 chromedp 跨分钟后仍用过期 version 误跳过。
		now := forecastPrewarmNow()
		version := now.Truncate(time.Minute).Unix()
		if activeVersion, ok := prewarmedImageVersion(matchID); ok && activeVersion >= version {
			continue
		}
		if !shouldAttemptForecastPrewarm(matchID, now) {
			continue
		}

		img, cached, err := refreshForecastImage(context.Background(), matchID)
		if err != nil {
			logrus.WithField("match_id", matchID).WithError(err).Error("match forecast prewarm failed")
			continue
		}
		logrus.WithFields(logrus.Fields{
			"match_id": matchID,
			"bytes":    len(img),
			"cached":   cached,
		}).Info("match forecast prewarm success")
	}
}

func forecastPrewarmMatchIDs(schedule types.ScheduleResp) []string {
	var selected []string
	zone, current, found := selectForecastMatch(schedule, "", false)
	if found {
		selected = appendUniqueMatchID(selected, current.ID)
		if next, ok := findNextMatch(zone, current); ok {
			selected = appendUniqueMatchID(selected, next.ID)
		}
		return selected
	}

	if _, upcoming, ok := findFirstUpcomingMatch(schedule); ok {
		selected = appendUniqueMatchID(selected, upcoming.ID)
	}
	return selected
}

func appendUniqueMatchID(ids []string, matchID string) []string {
	if matchID == "" {
		return ids
	}
	for _, existing := range ids {
		if existing == matchID {
			return ids
		}
	}
	return append(ids, matchID)
}

func shouldAttemptForecastPrewarm(matchID string, now time.Time) bool {
	forecastPrewarmMu.Lock()
	defer forecastPrewarmMu.Unlock()
	last, ok := forecastPrewarmAttempts[matchID]
	if ok && now.Sub(last.attemptedAt) < forecastPrewarmRetryInterval {
		return false
	}
	forecastPrewarmAttempts[matchID] = forecastPrewarmAttempt{attemptedAt: now}
	return true
}

func pruneForecastPrewarmAttempts(targets []string) {
	keep := make(map[string]struct{}, len(targets))
	for _, matchID := range targets {
		keep[matchID] = struct{}{}
	}
	forecastPrewarmMu.Lock()
	defer forecastPrewarmMu.Unlock()
	for matchID := range forecastPrewarmAttempts {
		if _, ok := keep[matchID]; !ok {
			delete(forecastPrewarmAttempts, matchID)
		}
	}
}
