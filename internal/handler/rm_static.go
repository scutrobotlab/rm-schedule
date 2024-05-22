package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/scutrobotlab/RMSituationBackend/internal/svc"
	"io"
	"log"
	"net/http"
)

func RMStaticHandler(c *gin.Context) {
	uuid := c.Param("uuid")
	cached, b := svc.Cache.Get("static/" + uuid)
	if b {
		c.Header("Cache-Control", "public, max-age=31536000")
		c.Data(200, "image/png", cached.([]byte))
		return
	}

	resp, err := http.Get("https://rm-static.djicdn.com/games-backend/" + uuid)
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
	svc.Cache.Set("static/"+uuid, bytes, cache.DefaultExpiration)

	c.Header("Cache-Control", "public, max-age=31536000")
	c.Data(200, "image/png", bytes)
}
