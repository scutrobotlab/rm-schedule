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

var renderForecastImage = render.RenderForecastImage

// MatchForecastImageHandler 导出「王牌预言家」海报 PNG。
// match_id 必填，用于指定当前赛季任意场次。
func MatchForecastImageHandler(c iris.Context) {
	start := time.Now()
	requestedMatchID, explicit, err := parseForecastMatchID(c)
	if err != nil {
		writeForecastError(c, iris.StatusBadRequest, err.Error())
		return
	}
	if !explicit {
		writeForecastError(c, iris.StatusBadRequest, "match_id is required")
		return
	}
	schedule, ok := loadCachedSchedule()
	if !ok {
		writeForecastError(c, iris.StatusServiceUnavailable, "schedule unavailable")
		return
	}
	if _, _, found := selectForecastMatch(schedule, requestedMatchID, true); !found {
		writeForecastError(c, iris.StatusNotFound, "match_id not found")
		return
	}

	debugMatchID := strings.TrimSpace(os.Getenv(envForecastDebugMatchID))
	fields := logrus.Fields{}
	if explicit {
		fields["match_id"] = requestedMatchID
	}
	if debugMatchID != "" {
		fields["debug_match_id"] = debugMatchID
	}
	logrus.WithFields(fields).Info("match_forecast_image start")

	img, cached, err := renderForecastImage(c.Request().Context(), requestedMatchID)
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
		}).WithError(err).Error("match_forecast_image failed")
		c.StatusCode(statusCode)
		_, _ = c.WriteString(err.Error())
		return
	}

	requestedVersion, versionedRequest := parseForecastImageVersion(c.URLParam("v"))
	servedVersion, hasServedVersion := forecastImageVersion(requestedMatchID)
	if versionedRequest && hasServedVersion && requestedVersion != servedVersion {
		location := forecastImageURL("", requestedMatchID, servedVersion)
		c.Header("Cache-Control", "no-store")
		logrus.WithFields(fields).WithFields(logrus.Fields{
			"requested_version": requestedVersion,
			"served_version":    servedVersion,
			"location":          location,
		}).Info("match_forecast_image redirect to published version")
		c.Redirect(location, iris.StatusTemporaryRedirect)
		return
	}

	c.Header("Content-Type", "image/png")
	c.Header("Content-Disposition", fmt.Sprintf(
		`attachment; filename="%s"`,
		matchForecastImageFilename(requestedMatchID, explicit),
	))
	if versionedRequest && hasServedVersion && requestedVersion == servedVersion {
		c.Header("Cache-Control", "public, max-age=3600")
	} else {
		c.Header("Cache-Control", "public, max-age=1")
	}
	_, _ = c.Write(img)
	logrus.WithFields(fields).WithFields(logrus.Fields{
		"duration": time.Since(start).Truncate(time.Millisecond),
		"bytes":    len(img),
		"cached":   cached,
	}).Info("match_forecast_image success")
}

func parseForecastImageVersion(raw string) (int64, bool) {
	version, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return version, err == nil && version > 0
}

// matchForecastImageFilename 将预测场次的 match ID 放入下载文件名。
// 无比赛或 match ID 非法时回退到不带 ID 的文件名。
func matchForecastImageFilename(requestedMatchID string, explicit bool) string {
	if explicit {
		return "match-forecast-" + requestedMatchID + ".png"
	}
	schedule, ok := loadCachedSchedule()
	if !ok {
		return "match-forecast.png"
	}
	_, match, found := selectForecastMatch(schedule, "", false)
	if !found {
		return "match-forecast.png"
	}
	matchID, err := strconv.Atoi(match.ID)
	if err != nil || matchID < 0 {
		return "match-forecast.png"
	}
	return fmt.Sprintf("match-forecast-%d.png", matchID)
}
