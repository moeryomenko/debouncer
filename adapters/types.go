package adapters

import (
	"context"
	"time"
)

// DistributedLock
type DistributedLock interface {
	Lock() error
	Unlock() error
}

// LockFactory
type LockFactory func(key string, duration time.Duration) DistributedLock

// Cache
type Cache interface {
	// Get returns the value for the specified key if it is present in the cache.
	Get(ctx context.Context, key string) ([]byte, error)
	// Set inserts or updates the specified key-value pair with an expiration time.
	Set(ctx context.Context, key string, value []byte, expiry time.Duration) error
}
