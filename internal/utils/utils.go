package utils

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"

	"github.com/redis/go-redis/v9"
)

func RandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

func NewRedisClient() (*redis.Client, error) {
	Addr := os.Getenv("REDIS_ADDR")
	if Addr == "" {
		return nil, fmt.Errorf("REDIS_ADDR environment variable not set")
	}
	Password := os.Getenv("REDIS_PW")
	if Password == "" {
		return nil, fmt.Errorf("REDIS_PW environment variable not set")
	}
	DB, err := strconv.Atoi(os.Getenv("REDIS_DB"))
	if err != nil {
		return nil, fmt.Errorf("REDIS_DB environment variable not set or is invalid")
	}
	return redis.NewClient(
		&redis.Options{
			Addr:     Addr,
			Password: Password,
			DB:       DB,
		}), nil
}
