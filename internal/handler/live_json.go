package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/kataras/iris/v12"
)

func ProxyLiveJsonHandler(c iris.Context) {
	p := c.Params().Get("path")
	if p == "" {
		c.StatusCode(400)
		return
	}

	u, err := url.Parse(fmt.Sprintf("https://rm-static.djicdn.com/live_json/%s", p))
	if err != nil {
		c.StatusCode(400)
		return
	}

	resp, err := http.Get(u.String())
	if err != nil {
		c.StatusCode(500)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		if strings.Contains(k, "Access-Control") ||
			strings.EqualFold(k, "Cache-Control") ||
			strings.EqualFold(k, "Expires") ||
			strings.EqualFold(k, "Set-Cookie") {
			continue
		}
		c.Header(k, v[0])
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		c.Header("Cache-Control", "public, max-age=5")
	} else {
		c.Header("Cache-Control", "no-store")
	}
	c.StatusCode(resp.StatusCode)

	if _, err := io.Copy(c.ResponseWriter(), resp.Body); err != nil {
		c.StatusCode(500)
		return
	}
}
