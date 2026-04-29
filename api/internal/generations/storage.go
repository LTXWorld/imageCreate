package generations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ImageStorage struct {
	Root string
}

func (s ImageStorage) Save(ctx context.Context, taskID string, data []byte, now time.Time) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if invalidPathPart(taskID) {
		return "", errors.New("invalid task ID")
	}

	relativePath := filepath.Join(
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", int(now.Month())),
		fmt.Sprintf("%02d", now.Day()),
		taskID+".png",
	)
	fullPath := s.fullPath(relativePath)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", fmt.Errorf("create image directory: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.WriteFile(fullPath, data, 0o600); err != nil {
		return "", fmt.Errorf("write image: %w", err)
	}
	return relativePath, nil
}

func (s ImageStorage) Open(ctx context.Context, relativePath string) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if invalidRelativePath(relativePath) {
		return nil, errors.New("invalid image path")
	}
	return os.Open(s.fullPath(relativePath))
}

func (s ImageStorage) Delete(ctx context.Context, relativePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if invalidRelativePath(relativePath) {
		return errors.New("invalid image path")
	}
	return os.Remove(s.fullPath(relativePath))
}

func (s ImageStorage) fullPath(relativePath string) string {
	return filepath.Join(s.Root, "storage", "images", relativePath)
}

func invalidRelativePath(relativePath string) bool {
	return relativePath == "" ||
		filepath.IsAbs(relativePath) ||
		strings.Contains(relativePath, "..")
}

func invalidPathPart(part string) bool {
	return part == "" ||
		strings.Contains(part, "..") ||
		strings.ContainsAny(part, `/\`)
}
