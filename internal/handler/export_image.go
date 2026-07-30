package handler

import (
	"errors"
	"strconv"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/render"
	"github.com/sirupsen/logrus"
)

func exportImageFields(season, zone, group int, scale float64) logrus.Fields {
	return logrus.Fields{
		"season": season,
		"zone":   zone,
		"group":  group,
		"scale":  scale,
	}
}

func ExportImageHandler(c iris.Context) {
	start := time.Now()

	writeBadRequest := func(msg string, fields logrus.Fields) {
		logrus.WithFields(fields).WithField("reason", msg).Error("export_image failed")
		c.StatusCode(400)
		_, _ = c.WriteString(msg)
	}

	seasonStr := c.URLParam("season")
	zoneStr := c.URLParam("zone")
	if seasonStr == "" || zoneStr == "" {
		writeBadRequest("season and zone are required", logrus.Fields{
			"season": seasonStr,
			"zone":   zoneStr,
		})
		return
	}

	season, err := strconv.Atoi(seasonStr)
	if err != nil {
		writeBadRequest("season must be an integer", logrus.Fields{"season": seasonStr})
		return
	}

	zone, err := strconv.Atoi(zoneStr)
	if err != nil {
		writeBadRequest("zone must be an integer", logrus.Fields{"zone": zoneStr})
		return
	}

	group := 0
	if groupStr := c.URLParam("group"); groupStr != "" {
		group, err = strconv.Atoi(groupStr)
		if err != nil {
			writeBadRequest("group must be an integer", exportImageFields(season, zone, 0, 0))
			return
		}
	}

	scale := 2.0
	if scaleStr := c.URLParam("scale"); scaleStr != "" {
		scale, err = strconv.ParseFloat(scaleStr, 64)
		if err != nil || scale < 1 || scale > 8 {
			writeBadRequest("scale must be a number between 1 and 8", exportImageFields(season, zone, group, 0))
			return
		}
	}

	fields := exportImageFields(season, zone, group, scale)
	logrus.WithFields(fields).Info("export_image start")

	img, cached, err := render.RenderScheduleImage(c.Request().Context(), season, zone, group, scale)
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
		}).WithError(err).Error("export_image failed")
		c.StatusCode(statusCode)
		_, _ = c.WriteString(err.Error())
		return
	}

	c.Header("Content-Type", "image/png")
	c.Header("Cache-Control", "public, max-age=60")
	_, _ = c.Write(img)
	logrus.WithFields(fields).WithFields(logrus.Fields{
		"duration": time.Since(start).Truncate(time.Millisecond),
		"bytes":    len(img),
		"cached":   cached,
	}).Info("export_image success")
}
