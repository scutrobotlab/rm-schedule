package handler

import (
	"errors"
	"strconv"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/render"
)

func ExportImageHandler(c iris.Context) {
	seasonStr := c.URLParam("season")
	zoneStr := c.URLParam("zone")
	if seasonStr == "" || zoneStr == "" {
		c.StatusCode(400)
		_, _ = c.WriteString("season and zone are required")
		return
	}

	season, err := strconv.Atoi(seasonStr)
	if err != nil {
		c.StatusCode(400)
		_, _ = c.WriteString("season must be an integer")
		return
	}

	zone, err := strconv.Atoi(zoneStr)
	if err != nil {
		c.StatusCode(400)
		_, _ = c.WriteString("zone must be an integer")
		return
	}

	group := 0
	if groupStr := c.URLParam("group"); groupStr != "" {
		group, err = strconv.Atoi(groupStr)
		if err != nil {
			c.StatusCode(400)
			_, _ = c.WriteString("group must be an integer")
			return
		}
	}

	scale := 2.0
	if scaleStr := c.URLParam("scale"); scaleStr != "" {
		scale, err = strconv.ParseFloat(scaleStr, 64)
		if err != nil || scale < 1 || scale > 8 {
			c.StatusCode(400)
			_, _ = c.WriteString("scale must be a number between 1 and 8")
			return
		}
	}

	img, err := render.RenderScheduleImage(c.Request().Context(), season, zone, group, scale)
	if err != nil {
		var paramErr *render.ParamError
		var timeoutErr *render.TimeoutError
		switch {
		case errors.As(err, &paramErr):
			c.StatusCode(400)
			_, _ = c.WriteString(paramErr.Error())
		case errors.As(err, &timeoutErr):
			c.StatusCode(504)
			_, _ = c.WriteString(timeoutErr.Error())
		default:
			c.StatusCode(502)
			_, _ = c.WriteString(err.Error())
		}
		return
	}

	c.Header("Content-Type", "image/png")
	_, _ = c.Write(img)
}
