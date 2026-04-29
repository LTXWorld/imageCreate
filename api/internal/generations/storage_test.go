package generations

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImageStorageSaveOpenDelete(t *testing.T) {
	root := t.TempDir()
	storage := ImageStorage{Root: root}
	now := time.Date(2026, 4, 29, 10, 30, 0, 0, time.UTC)

	relativePath, err := storage.Save(context.Background(), "task-123", []byte("png bytes"), now)
	if err != nil {
		t.Fatalf("save image: %v", err)
	}
	if relativePath != filepath.Join("2026", "04", "29", "task-123.png") {
		t.Fatalf("relative path = %q, want dated png path", relativePath)
	}

	file, err := storage.Open(context.Background(), relativePath)
	if err != nil {
		t.Fatalf("open image: %v", err)
	}
	defer file.Close()

	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatalf("read image: %v", err)
	}
	if string(data) != "png bytes" {
		t.Fatalf("data = %q, want saved bytes", string(data))
	}

	if err := storage.Delete(context.Background(), relativePath); err != nil {
		t.Fatalf("delete image: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "storage", "images", relativePath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat deleted file error = %v, want not exist", err)
	}
}

func TestImageStorageRejectsTraversal(t *testing.T) {
	storage := ImageStorage{Root: t.TempDir()}

	if _, err := storage.Open(context.Background(), filepath.Join("..", "secret.png")); err == nil {
		t.Fatal("open traversal error = nil, want error")
	}
	if err := storage.Delete(context.Background(), "2026/../secret.png"); err == nil {
		t.Fatal("delete traversal error = nil, want error")
	}
}
