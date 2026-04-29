package config

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppBaseURL           string
	DatabaseURL          string
	SessionSecret        string
	AdminUsername        string
	AdminPassword        string
	OpenAIBaseURL        string
	OpenAIAPIKey         string
	OpenAIImageModel     string
	OpenAIRequestTimeout time.Duration
	ImageStorageDir      string
	ImageRetentionDays   int
	ImageSizePresets     map[string]string
}

func Load() (Config, error) {
	cfg := Config{
		AppBaseURL:         strings.TrimRight(os.Getenv("APP_BASE_URL"), "/"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		SessionSecret:      os.Getenv("SESSION_SECRET"),
		AdminUsername:      os.Getenv("ADMIN_USERNAME"),
		AdminPassword:      os.Getenv("ADMIN_PASSWORD"),
		OpenAIBaseURL:      strings.TrimRight(os.Getenv("OPENAI_BASE_URL"), "/"),
		OpenAIAPIKey:       os.Getenv("OPENAI_API_KEY"),
		OpenAIImageModel:   getenv("OPENAI_IMAGE_MODEL", "gpt-image-2"),
		ImageStorageDir:    getenv("IMAGE_STORAGE_DIR", "./storage/images"),
		ImageRetentionDays: getenvInt("IMAGE_RETENTION_DAYS", 30),
		ImageSizePresets: map[string]string{
			"1:1":  "1024x1024",
			"3:4":  "768x1024",
			"4:3":  "1024x768",
			"9:16": "720x1280",
			"16:9": "1280x720",
		},
	}

	timeoutSeconds := getenvInt("OPENAI_REQUEST_TIMEOUT_SECONDS", 120)
	cfg.OpenAIRequestTimeout = time.Duration(timeoutSeconds) * time.Second

	if raw := os.Getenv("IMAGE_SIZE_PRESETS"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg.ImageSizePresets); err != nil {
			return Config{}, err
		}
	}

	if cfg.AppBaseURL == "" || cfg.DatabaseURL == "" || cfg.SessionSecret == "" ||
		cfg.AdminUsername == "" || cfg.AdminPassword == "" ||
		cfg.OpenAIBaseURL == "" || cfg.OpenAIAPIKey == "" {
		return Config{}, errors.New("missing required environment variables")
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
