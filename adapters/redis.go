package adapters

import (
	"context"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/go-redsync/redsync/v4"
	goredis "github.com/go-redsync/redsync/v4/redis/goredis/v9"
)

func NewRedisDriver(client *redis.Client) (Cache, LockFactory) {
	return &Redis{client: client}, RedisLockFactory(goredisRedLock(client))
}

type Redis struct {
	client *redis.Client
}

// Get returns the value for the specified key if it is present in the cache.
func (r *Redis) Get(ctx context.Context, key string) ([]byte, error) {
	return r.client.Get(ctx, key).Bytes()
}

// Set inserts or updates the specified key-value pair with an expiration time.
func (r *Redis) Set(ctx context.Context, key string, value []byte, expiry time.Duration) error {
	return r.client.SetNX(ctx, key, value, expiry).Err()
}

type adapterLock struct {
	mu *redsync.Mutex
}

func (a *adapterLock) Lock() error {
	return a.mu.Lock()
}

func (a *adapterLock) Unlock() error {
	_, err := a.mu.Unlock()
	return err
}

func goredisRedLock(cache *redis.Client) *redsync.Redsync {
	pool := goredis.NewPool(cache)
	return redsync.New(pool)
}

func RedisLockFactory(locker *redsync.Redsync) LockFactory {
	return func(key string, duration time.Duration) DistributedLock {
		mutex := locker.NewMutex(
			fmt.Sprintf("lock_%s", key),
			redsync.WithTries(1),
			redsync.WithExpiry(duration/2),
		)
		return &adapterLock{mutex}
	}
}
