package handler

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/render"
	"github.com/sirupsen/logrus"
)

// CurrentMatchForecastImageHandler 导出「王牌预言家」海报 PNG。
// 无查询参数，固定输出 3840×2160；无进行中比赛时仍返回海报「暂无进行中比赛」状态图（非 HTTP 错误）。
// Mock 场次复用 SCHEDULE_FORECAST_DEBUG_MATCH_ID（与 /api/current_match_forecast 相同）。
func CurrentMatchForecastImageHandler(c iris.Context) {
	start := time.Now()
	debugMatchID := strings.TrimSpace(os.Getenv(envForecastDebugMatchID))
	fields := logrus.Fields{}
	if debugMatchID != "" {
		fields["debug_match_id"] = debugMatchID
	}
	logrus.WithFields(fields).Info("current_match_forecast_image start")

	img, cached, err := render.RenderForecastImage(c.Request().Context())
	if err != nil {
		statusCode := 502
		var paramErr *render.ParamError
		var timeoutErr *render.TimeoutError
		switch {
		case errors.As(err, &paramErr):
			statusCode = 400
		case errors.As(err, &timeoutErr):
			statusCode = 504
		}
		logrus.WithFields(fields).WithFields(logrus.Fields{
			"duration":    time.Since(start).Truncate(time.Millisecond),
			"status_code": statusCode,
		}).WithError(err).Error("current_match_forecast_image failed")
		c.StatusCode(statusCode)
		_, _ = c.WriteString(err.Error())
		return
	}

	c.Header("Content-Type", "image/png")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, currentForecastImageFilename()))
	c.Header("Cache-Control", "public, max-age=1")
	_, _ = c.Write(img)
	logrus.WithFields(fields).WithFields(logrus.Fields{
		"duration": time.Since(start).Truncate(time.Millisecond),
		"bytes":    len(img),
		"cached":   cached,
	}).Info("current_match_forecast_image success")
}

// currentForecastImageFilename 将当前预测场次的 match ID 放入下载文件名。
// 无比赛或 match ID 非法时回退到不带 ID 的文件名。
func currentForecastImageFilename() string {
	schedule, ok := loadCachedSchedule()
	if !ok {
		return "current-match-forecast.png"
	}
	_, match, found := selectForecastMatch(schedule)
	if !found {
		return "current-match-forecast.png"
	}
	matchID, err := strconv.Atoi(match.ID)
	if err != nil || matchID < 0 {
		return "current-match-forecast.png"
	}
	return fmt.Sprintf("current-match-forecast-%d.png", matchID)
}
