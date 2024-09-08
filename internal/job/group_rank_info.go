package job

import (
	"io"
	"log"
	"net/http"

	"github.com/patrickmn/go-cache"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
)

const GroupRankInfoUrl = "https://pro-robomasters-hz-n5i3.oss-cn-hangzhou.aliyuncs.com/live_json/group_rank_info.json"

func UpdateGroupRankInfo() {
	resp, err := http.Get(GroupRankInfoUrl)
	if err != nil {
		log.Printf("Failed to get group rank info: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Failed to get group rank info: status code %d\n", resp.StatusCode)
		return
	}

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read group rank info: %v\n", err)
		return
	}
	svc.Cache.Set("group_rank_info", bytes, cache.DefaultExpiration)

	log.Println("Group rank info updated")
}
