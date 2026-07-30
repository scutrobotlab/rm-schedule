package handler

import (
	"fmt"
	"github.com/scutrobotlab/rm-schedule/internal/common"
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
	"strings"

	"github.com/kataras/iris/v12"
	"github.com/patrickmn/go-cache"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
)

func RMStaticHandler(c iris.Context) {
	path := c.Params().Get("path")
	process := c.URLParam("process")
	cacheKey := fmt.Sprintf("Static_%s_%s", path, process)

	cached, b := svc.Cache.Get(cacheKey)
	if b {
		c.Header("Cache-Control", longLivedStaticCacheControl)
		c.ContentType("image/png")
		c.Write(cached.([]byte))
		return
	}

	url := strings.Replace(path, "rm-static_djicdn_com", "https://rm-static.djicdn.com", 1)
	url = strings.Replace(url, "terra-cn-oss-cdn-public-pro_oss-cn-hangzhou_aliyuncs_com", "https://terra-cn-oss-cdn-public-pro.oss-cn-hangzhou.aliyuncs.com", 1)
	url = strings.Replace(url, "pro-robomasters-hz-n5i3_oss-cn-hangzhou_aliyuncs_com", "https://pro-robomasters-hz-n5i3.oss-cn-hangzhou.aliyuncs.com", 1)
	url = strings.Replace(url, "hz-rm-bbs-web-prod_oss-cn-hangzhou_aliyuncs_com", "https://hz-rm-bbs-web-prod.oss-cn-hangzhou.aliyuncs.com", 1)
	url = strings.Replace(url, "terra-us-pro-rm-prod-pub-us_s3_amazonaws_com", "https://terra-us-pro-rm-prod-pub-us.s3.amazonaws.com", 1)
	url = strings.Replace(url, "sz-rm-rmua-dispatch-prod_oss-cn-shenzhen_aliyuncs_com", "https://sz-rm-rmua-dispatch-prod.oss-cn-shenzhen.aliyuncs.com", 1)
	// auto add scheme
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}

	resp, err := http.Get(url)
	if err != nil {
		logrus.Errorf("Failed to get static file: %v", err)
		c.StatusCode(500)
		c.JSON(iris.Map{"code": -1, "msg": "Failed to get static file"})
		return
	}
	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logrus.Errorf("Failed to read static file: %v", err)
		c.StatusCode(500)
		c.JSON(iris.Map{"code": -1, "msg": "Failed to read static file"})
		return
	}

	if process != "" {
		switch process {
		case "bg_white":
			// Convert transparent PNG to white background
			bytes, err = common.ConvertTransparentToWhite(bytes)
			if err != nil {
				logrus.Errorf("ConvertTransparentToWhite failed: %v", err)
				c.StatusCode(500)
				c.JSON(iris.Map{"code": -1, "msg": "Failed to process image"})
				return
			}
		default:
			c.StatusCode(400)
			c.JSON(iris.Map{"code": -1, "msg": "Unknown process type"})
			return
		}
	}

	svc.Cache.Set(cacheKey, bytes, cache.DefaultExpiration)

	c.Header("Cache-Control", longLivedStaticCacheControl)
	c.ContentType(resp.Header.Get("Content-Type"))
	c.Write(bytes)
}
