// Package redis provides a Redis-backed cache for slug-to-file_id mappings.
package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	// DefaultTTL is the default TTL for cached slug mappings.
	DefaultTTL = 5 * time.Minute

	// keyPrefix is the Redis key prefix for slug mappings.
	keyPrefix = "slug:"
)

// Cache provides Redis operations for the redirect cache.
type Cache struct {
	client *goredis.Client
}

// NewCache creates a new Redis cache client.
func NewCache(redisURL string) (*Cache, error) {
	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("redis: parse url: %w", err)
	}

	client := goredis.NewClient(opts)

	// Verify connectivity.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis: ping: %w", err)
	}

	return &Cache{client: client}, nil
}

// Close closes the Redis connection.
func (c *Cache) Close() error {
	return c.client.Close()
}

// Get retrieves the file_id for a slug from the cache.
// Returns empty string if the slug is not cached.
func (c *Cache) Get(ctx context.Context, slug string) (string, error) {
	val, err := c.client.Get(ctx, keyPrefix+slug).Result()
	if err == goredis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("redis: get: %w", err)
	}
	return val, nil
}

// Set stores a slug-to-file_id mapping in the cache with the given TTL.
func (c *Cache) Set(ctx context.Context, slug, fileID string, ttl time.Duration) error {
	if err := c.client.Set(ctx, keyPrefix+slug, fileID, ttl).Err(); err != nil {
		return fmt.Errorf("redis: set: %w", err)
	}
	return nil
}

// Delete removes a slug mapping from the cache.
func (c *Cache) Delete(ctx context.Context, slug string) error {
	if err := c.client.Del(ctx, keyPrefix+slug).Err(); err != nil {
		return fmt.Errorf("redis: del: %w", err)
	}
	return nil
}
