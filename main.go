package main

import (
	"github.com/gin-gonic/gin"
	"github.com/scutrobotlab/RMSituationBackend/internal/router"
)

func main() {
	r := gin.Default()
	router.Router(r)
	err := r.Run(":8080")
	if err != nil {
		panic(err)
	}
}
