package aws

import (
	"testing"
	"time"
)

func TestFileCache_SetAndGet(t *testing.T) {
	dir := t.TempDir()
	cache := NewFileCache(dir)

	type testData struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	original := testData{Name: "test", Value: 42}
	if err := cache.Set("test-key", original); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	var loaded testData
	if !cache.Get("test-key", time.Hour, &loaded) {
		t.Fatal("Get returned false for valid cache entry")
	}
	if loaded.Name != "test" || loaded.Value != 42 {
		t.Errorf("got %+v, want %+v", loaded, original)
	}
}

func TestFileCache_Expired(t *testing.T) {
	dir := t.TempDir()
	cache := NewFileCache(dir)

	if err := cache.Set("expired", "value"); err != nil {
		t.Fatal(err)
	}

	var result string
	// TTL of 0 means always expired
	if cache.Get("expired", 0, &result) {
		t.Error("expected expired cache miss")
	}
}

func TestFileCache_Missing(t *testing.T) {
	dir := t.TempDir()
	cache := NewFileCache(dir)

	var result string
	if cache.Get("nonexistent", time.Hour, &result) {
		t.Error("expected cache miss for nonexistent key")
	}
}

func TestFileCache_Clear(t *testing.T) {
	dir := t.TempDir()
	cache := NewFileCache(dir)

	_ = cache.Set("key1", "val1")
	_ = cache.Set("key2", "val2")

	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	var result string
	if cache.Get("key1", time.Hour, &result) {
		t.Error("expected cache miss after clear")
	}
}
