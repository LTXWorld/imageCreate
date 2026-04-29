package config

import "testing"

func TestLoadDefaultsImagePresets(t *testing.T) {
	t.Setenv("APP_BASE_URL", "https://img.example.com")
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/imagecreate?sslmode=disable")
	t.Setenv("SESSION_SECRET", "test-secret-with-32-characters")
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "admin-password")
	t.Setenv("OPENAI_BASE_URL", "https://proxy.example.com")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_IMAGE_MODEL", "gpt-image-2")
	t.Setenv("OPENAI_REQUEST_TIMEOUT_SECONDS", "")
	t.Setenv("IMAGE_STORAGE_DIR", "")
	t.Setenv("IMAGE_RETENTION_DAYS", "")
	t.Setenv("IMAGE_SIZE_PRESETS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.ImageSizePresets["1:1"] != "1024x1024" {
		t.Fatalf("expected 1:1 preset 1024x1024, got %q", cfg.ImageSizePresets["1:1"])
	}
	if cfg.ImageRetentionDays != 30 {
		t.Fatalf("expected default retention 30, got %d", cfg.ImageRetentionDays)
	}
}

func TestLoadRequiresSecrets(t *testing.T) {
	t.Setenv("APP_BASE_URL", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("ADMIN_USERNAME", "")
	t.Setenv("ADMIN_PASSWORD", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected missing required environment variables to fail")
	}
}
