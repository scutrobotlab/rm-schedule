package svc

import (
	"github.com/patrickmn/go-cache"
	"time"
)

var Cache = cache.New(cache.NoExpiration, 1*time.Minute)
