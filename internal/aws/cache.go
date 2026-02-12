package aws

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileCache provides file-based caching for AWS API responses.
type FileCache struct {
	dir string
}

// NewFileCache creates a new file cache in the given directory.
func NewFileCache(dir string) *FileCache {
	return &FileCache{dir: dir}
}

// Get retrieves a cached value if it exists and hasn't expired.
func (fc *FileCache) Get(key string, ttl time.Duration, dest interface{}) bool {
	path := fc.path(key)
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	// Check TTL
	if time.Since(info.ModTime()) > ttl {
		return false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return false
	}

	return true
}

// Set stores a value in the cache.
func (fc *FileCache) Set(key string, value interface{}) error {
	if err := os.MkdirAll(fc.dir, 0755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshaling cache value: %w", err)
	}

	path := fc.path(key)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing cache file: %w", err)
	}

	return nil
}

// Clear removes all cached data.
func (fc *FileCache) Clear() error {
	entries, err := os.ReadDir(fc.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, e := range entries {
		if err := os.Remove(filepath.Join(fc.dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (fc *FileCache) path(key string) string {
	return filepath.Join(fc.dir, key+".json")
}
