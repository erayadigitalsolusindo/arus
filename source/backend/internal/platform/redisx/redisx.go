// Package redisx membungkus klien Redis.
package redisx

import (
	"fmt"

	"github.com/redis/go-redis/v9"
)

func Connect(url string) (*redis.Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	return redis.NewClient(opt), nil
}
