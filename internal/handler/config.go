package handler

import (
	"os"
	"strconv"
	"strings"

	"github.com/kataras/iris/v12"
)

const envIsTestEnvironment = "SCHEDULE_IS_TEST_ENVIRONMENT"

type globalConfig struct {
	IsTestEnvironment bool `json:"isTestEnvironment"`
}

func isTestEnvironment() bool {
	value, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(envIsTestEnvironment)))
	return err == nil && value
}

// ConfigHandler 下发由运行时环境变量决定的前端全局配置。
func ConfigHandler(c iris.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(globalConfig{IsTestEnvironment: isTestEnvironment()})
}
