package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestInMemoryStorage(t *testing.T) {
	storage := NewInMemoryStorage()
	defer storage.Close()

	ctx := context.Background()
	testURL := "https://example.com/test"
	shortID := "abcd1234"

	// Test Set
	err := storage.Set(ctx, shortID, testURL)
	if err != nil {
		t.Errorf("Set failed: %v", err)
	}

	// Test Get - exists
	url, exists, err := storage.Get(ctx, shortID)
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
	if !exists {
		t.Errorf("Expected URL to exist")
	}
	if url != testURL {
		t.Errorf("Expected URL %s, got %s", testURL, url)
	}

	// Test Get - not exists
	url, exists, err = storage.Get(ctx, "nonexistent")
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
	if exists {
		t.Errorf("Expected URL to not exist")
	}
	if url != "" {
		t.Errorf("Expected empty URL, got %s", url)
	}
}

func TestRedisStorage(t *testing.T) {
	// Skip if Redis is not available
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	ctx := context.Background()
	config := RedisStorageConfig{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
		TTL:      time.Hour,
	}

	storage, err := NewRedisStorage(ctx, config)
	if err != nil {
		t.Skipf("Skipping Redis test: %v", err)
	}
	defer storage.Close()

	testURL := "https://example.com/test"
	shortID := "redis1234"

	// Test Set
	err = storage.Set(ctx, shortID, testURL)
	if err != nil {
		t.Errorf("Set failed: %v", err)
	}

	// Test Get - exists
	url, exists, err := storage.Get(ctx, shortID)
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
	if !exists {
		t.Errorf("Expected URL to exist")
	}
	if url != testURL {
		t.Errorf("Expected URL %s, got %s", testURL, url)
	}

	// Test Get - not exists
	url, exists, err = storage.Get(ctx, "nonexistent")
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
	if exists {
		t.Errorf("Expected URL to not exist")
	}
	if url != "" {
		t.Errorf("Expected empty URL, got %s", url)
	}
}
