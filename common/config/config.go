package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
   DBHost        string
   DBPort        string
   DBUser        string
   DBPassword    string
   DBName        string
   RedisHost     string
   RedisPort     string
   RedisPassword string
   RedisDB       string
   RPCPort       string
   HTTPPort      string
   EtcdEndpoints string
   ServiceName   string
   StoragePath   string
   ChunkSize     string
   JWTSecret     string
   Environment   string
}

func Load() (*Config, error) {
   godotenv.Load()
   return &Config{
      DBHost:        getEnv("DB_HOST", "127.0.0.1"),
      DBPort:        getEnv("DB_PORT", "3306"),
      DBUser:        getEnv("DB_USER", "root"),
      DBPassword:    getEnv("DB_PASSWORD", ""),
      DBName:        getEnv("DB_NAME", "video_platform"),
      RedisHost:     getEnv("REDIS_HOST", "127.0.0.1"),
      RedisPort:     getEnv("REDIS_PORT", "6379"),
      RedisPassword: getEnv("REDIS_PASSWORD", ""),
      RedisDB:       getEnv("REDIS_DB", "0"),
      RPCPort:       getEnv("RPC_PORT", "8888"),
      HTTPPort:      getEnv("HTTP_PORT", "8080"),
      EtcdEndpoints: getEnv("ETCD_ENDPOINTS", "127.0.0.1:2379"),
      ServiceName:   getEnv("SERVICE_NAME", "unknown"),
      StoragePath:   getEnv("STORAGE_PATH", "/tmp/video-platform"),
      ChunkSize:     getEnv("CHUNK_SIZE", "2097152"),
      JWTSecret:     getEnv("JWT_SECRET", "your-secret-key"),
      Environment:   getEnv("ENVIRONMENT", "dev"),
   }, nil
}

func getEnv(key, defaultValue string) string {
   if value := os.Getenv(key); value != "" {
      return value
   }
   return defaultValue
}

func (c *Config) IsProd() bool {
   return c.Environment == "prod"
}
