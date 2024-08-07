package job

import (
	"github.com/robfig/cron/v3"
	"log"
)

func InitCronJob() *cron.Cron {
	c := cron.New()

	_, err := c.AddFunc("@every 2s", UpdateSchedule)
	if err != nil {
		log.Fatalf("cron add func failed: %v", err)
	}
	UpdateSchedule()

	return c
}
