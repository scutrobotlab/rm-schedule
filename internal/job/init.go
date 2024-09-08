package job

import (
	"log"

	"github.com/robfig/cron/v3"
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
