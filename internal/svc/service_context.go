package svc

import (
	"time"

	"github.com/patrickmn/go-cache"
)

var Cache = cache.New(cache.NoExpiration, 1*time.Minute)
