// Package config loads static configuration from environment variables /
// the config/.env file. Static config (tokens, DB URIs, credentials)
// lives here. Dynamic, owner-editable settings (worker count, queue
// size, limits) live in MongoDB via internal/database and are read
// through internal/models.Settings instead of this package.
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all static, file-based configuration for the bot.
type Config struct {
	BotToken string
	APIID    int64
	APIHash  string
	OwnerID  int64

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string

	MongoURI    string
	MongoDBName string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	RcloneConfigPath string
	RclonePath       string

	DefaultWorkers         int
	DefaultQueueSize       int
	DefaultDownloadLimitMB int64
	DefaultUploadLimitMB   int64

	DownloadDir string
	LogDir      string
	LogLevel    string
}

// Load reads config/.env (if present) and then environment variables,
// validating that every required field is set. Environment variables
// always take precedence over the .env file, which makes container /
// systemd deployments easy to override without editing files.
func Load(envPath string) (*Config, error) {
	// It's fine if the .env file doesn't exist; real envs may be
	// injected by systemd/docker instead.
	_ = godotenv.Load(envPath)

	cfg := &Config{
		BotToken:           os.Getenv("BOT_TOKEN"),
		APIHash:            os.Getenv("API_HASH"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURI:  getEnvDefault("GOOGLE_REDIRECT_URI", "http://127.0.0.1:1/"),
		MongoURI:           getEnvDefault("MONGO_URI", "mongodb://localhost:27017"),
		MongoDBName:        getEnvDefault("MONGO_DB_NAME", "gdrive_bot"),
		RedisAddr:          os.Getenv("REDIS_ADDR"),
		RedisPassword:      os.Getenv("REDIS_PASSWORD"),
		RcloneConfigPath:   getEnvDefault("RCLONE_CONFIG_PATH", "./config/rclone.conf"),
		RclonePath:         getEnvDefault("RCLONE_PATH", "rclone"),
		DownloadDir:        getEnvDefault("DOWNLOAD_DIR", "./downloads"),
		LogDir:             getEnvDefault("LOG_DIR", "./logs"),
		LogLevel:           getEnvDefault("LOG_LEVEL", "info"),
	}

	var err error
	if cfg.APIID, err = parseInt64Env("API_ID"); err != nil {
		return nil, err
	}
	if cfg.OwnerID, err = parseInt64Env("OWNER_ID"); err != nil {
		return nil, err
	}
	cfg.RedisDB = parseIntDefault("REDIS_DB", 0)
	cfg.DefaultWorkers = parseIntDefault("DEFAULT_WORKERS", 4)
	cfg.DefaultQueueSize = parseIntDefault("DEFAULT_QUEUE_SIZE", 50)
	cfg.DefaultDownloadLimitMB = parseInt64Default("DEFAULT_DOWNLOAD_LIMIT_MB", 0)
	cfg.DefaultUploadLimitMB = parseInt64Default("DEFAULT_UPLOAD_LIMIT_MB", 0)

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	missing := []string{}
	if c.BotToken == "" {
		missing = append(missing, "BOT_TOKEN")
	}
	if c.APIHash == "" {
		missing = append(missing, "API_HASH")
	}
	if c.OwnerID == 0 {
		missing = append(missing, "OWNER_ID")
	}
	if c.GoogleClientID == "" {
		missing = append(missing, "GOOGLE_CLIENT_ID")
	}
	if c.GoogleClientSecret == "" {
		missing = append(missing, "GOOGLE_CLIENT_SECRET")
	}
	if len(missing) > 0 {
		return fmt.Errorf("config: missing required environment variables: %v", missing)
	}
	return nil
}

func getEnvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseIntDefault(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func parseInt64Default(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func parseInt64Env(key string) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return 0, fmt.Errorf("config: %s is required", key)
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be an integer: %w", key, err)
	}
	return n, nil
}
