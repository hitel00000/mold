package fsblob

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hitel00000/mold/storage"
)

// FSBlobStore implements storage.BlobStore for local filesystem storage.
type FSBlobStore struct {
	rootDir string
	mu      sync.RWMutex
}

var _ storage.BlobStore = (*FSBlobStore)(nil)

// New creates a new FSBlobStore instance storing blobs under rootDir.
func New(rootDir string) (*FSBlobStore, error) {
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create blob root dir: %w", err)
	}
	return &FSBlobStore{rootDir: rootDir}, nil
}

func (s *FSBlobStore) Put(ctx context.Context, key string, data io.Reader, size int64, contentType string) error {
	if key == "" {
		return fmt.Errorf("blob key cannot be empty")
	}
	cleanKey := filepath.FromSlash(strings.TrimPrefix(key, "/"))
	fullPath := filepath.Join(s.rootDir, cleanKey)

	dir := filepath.Dir(fullPath)
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for blob '%s': %w", key, err)
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create blob file '%s': %w", key, err)
	}
	defer f.Close()

	if data != nil {
		if _, err := io.Copy(f, data); err != nil {
			return fmt.Errorf("failed to write blob data for '%s': %w", key, err)
		}
	}

	metaPath := fullPath + ".meta"
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_ = os.WriteFile(metaPath, []byte(contentType), 0644)

	return nil
}

func (s *FSBlobStore) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	if key == "" {
		return nil, "", fmt.Errorf("blob key cannot be empty")
	}
	cleanKey := filepath.FromSlash(strings.TrimPrefix(key, "/"))
	fullPath := filepath.Join(s.rootDir, cleanKey)

	s.mu.RLock()
	defer s.mu.RUnlock()

	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", storage.ErrNotFound
		}
		return nil, "", fmt.Errorf("failed to open blob file '%s': %w", key, err)
	}

	contentType := "application/octet-stream"
	metaBytes, err := os.ReadFile(fullPath + ".meta")
	if err == nil && len(metaBytes) > 0 {
		contentType = string(metaBytes)
	}

	return f, contentType, nil
}

func (s *FSBlobStore) Delete(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	cleanKey := filepath.FromSlash(strings.TrimPrefix(key, "/"))
	fullPath := filepath.Join(s.rootDir, cleanKey)

	s.mu.Lock()
	defer s.mu.Unlock()

	_ = os.Remove(fullPath + ".meta")
	err := os.Remove(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete blob file '%s': %w", key, err)
	}
	return nil
}
