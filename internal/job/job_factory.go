package job

import (
	"github.com/patrickmn/go-cache"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"io"
	"net/http"
	"strings"
)

func CronJobFactory(param CronJobParam) func() {
	return func() {
		resp, err := http.Get(param.Url)
		if err != nil {
			logrus.Errorf("Failed to get %s: %v", param.Name, err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			logrus.Errorf("Failed to get %s: status code %d", param.Name, resp.StatusCode)
			return
		}

		bytes, err := io.ReadAll(resp.Body)
		if err != nil {
			logrus.Errorf("Failed to read %s: %v", param.Name, err)
			return
		}

		if param.ReplaceRMStatic {
			bytes = replaceRMStatic(bytes)
		}

		svc.Cache.Set(param.Name, bytes, cache.DefaultExpiration)

		logrus.Infof("%s updated", strings.ReplaceAll(cases.Title(language.English).String(param.Name), "_", " "))
	}
}

func replaceRMStatic(data []byte) []byte {
	str := string(data)
	str = strings.ReplaceAll(str, "https://rm-static.djicdn.com", "/api/static/rm-static_djicdn_com")
	str = strings.ReplaceAll(str, "https://terra-cn-oss-cdn-public-pro.oss-cn-hangzhou.aliyuncs.com", "/api/static/terra-cn-oss-cdn-public-pro_oss-cn-hangzhou_aliyuncs_com")
	str = strings.ReplaceAll(str, "https://pro-robomasters-hz-n5i3.oss-cn-hangzhou.aliyuncs.com", "/api/static/pro-robomasters-hz-n5i3_oss-cn-hangzhou_aliyuncs_com")
	str = strings.ReplaceAll(str, "https://hz-rm-bbs-web-prod.oss-cn-hangzhou.aliyuncs.com", "/api/static/hz-rm-bbs-web-prod_oss-cn-hangzhou_aliyuncs_com")
	str = strings.ReplaceAll(str, "https://terra-us-pro-rm-prod-pub-us.s3.amazonaws.com", "/api/static/terra-us-pro-rm-prod-pub-us_s3_amazonaws_com")
	str = strings.ReplaceAll(str, "https://sz-rm-rmua-dispatch-prod.oss-cn-shenzhen.aliyuncs.com", "/api/static/sz-rm-rmua-dispatch-prod_oss-cn-shenzhen_aliyuncs_com")
	return []byte(str)
}
