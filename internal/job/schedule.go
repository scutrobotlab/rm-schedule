package job

import (
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/patrickmn/go-cache"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
)

const ScheduleUrl = "https://pro-robomasters-hz-n5i3.oss-cn-hangzhou.aliyuncs.com/live_json/schedule.json"

func UpdateSchedule() {
	resp, err := http.Get(ScheduleUrl)
	if err != nil {
		log.Printf("Failed to get schedule: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Failed to get schedule: status code %d\n", resp.StatusCode)
		return
	}

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read schedule: %v\n", err)
		return
	}
	bytes = replaceRMStatic(bytes)
	svc.Cache.Set("schedule", bytes, cache.DefaultExpiration)

	log.Println("Schedule updated")
}

func replaceRMStatic(data []byte) []byte {
	str := string(data)
	str = strings.ReplaceAll(str, "https://rm-static.djicdn.com", "/api/static/rm-static_djicdn_com")
	str = strings.ReplaceAll(str, "https://terra-cn-oss-cdn-public-pro.oss-cn-hangzhou.aliyuncs.com", "/api/static/terra-cn-oss-cdn-public-pro_oss-cn-hangzhou_aliyuncs_com")
	str = strings.ReplaceAll(str, "https://pro-robomasters-hz-n5i3.oss-cn-hangzhou.aliyuncs.com", "/api/static/pro-robomasters-hz-n5i3_oss-cn-hangzhou_aliyuncs_com")
	return []byte(str)
}
