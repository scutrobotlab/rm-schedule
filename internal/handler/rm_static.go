package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"io"
	"log"
	"net/http"
	"strings"
)

func RMStaticHandler(c *gin.Context) {
	path := c.Param("path")

	cached, b := svc.Cache.Get("static" + path)
	if b {
		c.Header("Cache-Control", "public, max-age=3600")
		c.Data(200, "image/png", cached.([]byte))
		return
	}

	url := strings.ReplaceAll(path, "/rm-static_djicdn_com", "https://rm-static.djicdn.com")
	url = strings.ReplaceAll(url, "/terra-cn-oss-cdn-public-pro_oss-cn-hangzhou_aliyuncs_com", "https://terra-cn-oss-cdn-public-pro.oss-cn-hangzhou.aliyuncs.com")
	url = strings.ReplaceAll(url, "/pro-robomasters-hz-n5i3_oss-cn-hangzhou_aliyuncs_com", "https://pro-robomasters-hz-n5i3.oss-cn-hangzhou.aliyuncs.com")

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Failed to get static file: %v\n", err)
		c.JSON(500, gin.H{"code": -1, "msg": "Failed to get static file"})
		return
	}
	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read static file: %v\n", err)
		c.JSON(500, gin.H{"code": -1, "msg": "Failed to read static file"})
		return
	}
	svc.Cache.Set("static"+path, bytes, cache.DefaultExpiration)

	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(200, "image/png", bytes)
}
