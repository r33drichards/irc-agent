package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// URLStorage is an interface for storing URL mappings
type URLStorage interface {
	// Set stores a mapping from short ID to original URL
	Set(ctx context.Context, shortID, url string) error
	// Get retrieves the original URL for a given short ID
	Get(ctx context.Context, shortID string) (string, bool, error)
	// Close releases any resources held by the storage
	Close() error
}

// InMemoryStorage implements URLStorage using an in-memory map
type InMemoryStorage struct {
	mu     sync.RWMutex
	urlMap map[string]string
}

// NewInMemoryStorage creates a new in-memory storage backend
func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		urlMap: make(map[string]string),
	}
}

// Set stores a mapping in memory
func (s *InMemoryStorage) Set(ctx context.Context, shortID, url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.urlMap[shortID] = url
	return nil
}

// Get retrieves a URL from memory
func (s *InMemoryStorage) Get(ctx context.Context, shortID string) (string, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	url, exists := s.urlMap[shortID]
	return url, exists, nil
}

// Close is a no-op for in-memory storage
func (s *InMemoryStorage) Close() error {
	return nil
}

// RedisStorage implements URLStorage using Redis
type RedisStorage struct {
	client *redis.Client
	ttl    time.Duration // TTL for keys (0 means no expiration)
}

// RedisStorageConfig contains configuration for Redis storage
type RedisStorageConfig struct {
	// Redis server address (e.g., "localhost:6379")
	Addr string
	// Password for Redis authentication (empty if no password)
	Password string
	// Database number (0-15)
	DB int
	// TTL for URL mappings (0 means no expiration)
	TTL time.Duration
}

// NewRedisStorage creates a new Redis storage backend
func NewRedisStorage(ctx context.Context, config RedisStorageConfig) (*RedisStorage, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
	})

	// Test the connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisStorage{
		client: client,
		ttl:    config.TTL,
	}, nil
}

// Set stores a mapping in Redis with the configured TTL
func (s *RedisStorage) Set(ctx context.Context, shortID, url string) error {
	key := fmt.Sprintf("url:%s", shortID)
	return s.client.Set(ctx, key, url, s.ttl).Err()
}

// Get retrieves a URL from Redis
func (s *RedisStorage) Get(ctx context.Context, shortID string) (string, bool, error) {
	key := fmt.Sprintf("url:%s", shortID)
	url, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return url, true, nil
}

// Close closes the Redis connection
func (s *RedisStorage) Close() error {
	return s.client.Close()
}
