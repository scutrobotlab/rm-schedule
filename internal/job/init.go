package job

import (
	"github.com/robfig/cron/v3"
	"log"
)

func InitCronJob() *cron.Cron {
	c := cron.New()

	_, err := c.AddFunc("@every 5s", UpdateGroupRankInfo)
	if err != nil {
		log.Fatalf("cron add func failed: %v", err)
	}

	_, err = c.AddFunc("@every 5s", UpdateSchedule)
	if err != nil {
		log.Fatalf("cron add func failed: %v", err)
	}

	UpdateGroupRankInfo()
	UpdateSchedule()

	return c
}
