package handler

import (
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/kataras/iris/v12"
	"github.com/patrickmn/go-cache"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
)

func RMStaticHandler(c iris.Context) {
	path := c.Params().Get("path")

	cached, b := svc.Cache.Get("static" + path)
	if b {
		c.Header("Cache-Control", "public, max-age=3600")
		c.ContentType("image/png")
		c.Write(cached.([]byte))
		return
	}

	url := strings.Replace(path, "rm-static_djicdn_com", "https://rm-static.djicdn.com", 1)
	url = strings.Replace(url, "terra-cn-oss-cdn-public-pro_oss-cn-hangzhou_aliyuncs_com", "https://terra-cn-oss-cdn-public-pro.oss-cn-hangzhou.aliyuncs.com", 1)
	url = strings.Replace(url, "pro-robomasters-hz-n5i3_oss-cn-hangzhou_aliyuncs_com", "https://pro-robomasters-hz-n5i3.oss-cn-hangzhou.aliyuncs.com", 1)
	// auto add scheme
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Failed to get static file: %v\n", err)
		c.StatusCode(500)
		c.JSON(iris.Map{"code": -1, "msg": "Failed to get static file"})
		return
	}
	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read static file: %v\n", err)
		c.StatusCode(500)
		c.JSON(iris.Map{"code": -1, "msg": "Failed to read static file"})
		return
	}
	svc.Cache.Set("static"+path, bytes, cache.DefaultExpiration)

	c.Header("Cache-Control", "public, max-age=3600")
	c.ContentType("image/png")
	c.Write(bytes)
}
