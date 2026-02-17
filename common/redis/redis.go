package redis

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var Client *redis.Client

type Config struct {
   Host         string
   Port         string
   Password     string
   DB           int
   DialTimeout  time.Duration
   ReadTimeout  time.Duration
   WriteTimeout time.Duration
   PoolSize     int
   MinIdleConns int
}

func InitRedis() error {
   config := Config{
      Host:         getEnv("REDIS_HOST", "127.0.0.1"),
      Port:         getEnv("REDIS_PORT", "6379"),
      Password:     getEnv("REDIS_PASSWORD", ""),
      DB:           getEnvInt("REDIS_DB", 0),
      DialTimeout:  5 * time.Second,
      ReadTimeout:  3 * time.Second,
      WriteTimeout: 3 * time.Second,
      PoolSize:     10,
      MinIdleConns: 5,
   }
   return InitRedisWithConfig(config)
}

func InitRedisWithConfig(config Config) error {
   Client = redis.NewClient(&redis.Options{
      Addr:         fmt.Sprintf("%s:%s", config.Host, config.Port),
      Password:     config.Password,
      DB:           config.DB,
      DialTimeout:  config.DialTimeout,
      ReadTimeout:  config.ReadTimeout,
      WriteTimeout: config.WriteTimeout,
      PoolSize:     config.PoolSize,
      MinIdleConns: config.MinIdleConns,
   })

   ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
   defer cancel()

   if err := Client.Ping(ctx).Err(); err != nil {
      return fmt.Errorf("redis ping failed: %w", err)
   }

   fmt.Println("✅ Redis connected successfully")
   return nil
}

func GetClient() *redis.Client {
   return Client
}

func Close() error {
   if Client != nil {
      return Client.Close()
   }
   return nil
}

func getEnv(key, defaultValue string) string {
   if value := os.Getenv(key); value != "" {
      return value
   }
   return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
   if value := os.Getenv(key); value != "" {
      if intValue, err := strconv.Atoi(value); err == nil {
         return intValue
      }
   }
   return defaultValue
}
