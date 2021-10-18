package debouncer

import (
	"context"
	"time"

	"golang.org/x/sync/singleflight"
)

type DistributedLock interface {
	Lock() error
	Unlock() error
}

type Cache interface {
	// Get returns the value for the specified key if it is present in the cache.
	Get(key string) (interface{}, error)
	// Set inserts or updates the specified key-value pair with an expiration time.
	Set(key string, value interface{}, expiry time.Duration) error
}

type LockFactory func(key string) DistributedLock

type Debouncer struct {
	localCache       Cache
	distributedCache Cache

	localTTL time.Duration

	localGroup       singleflight.Group
	distributedGroup DistributedGroup
}

type Closure func() (interface{}, error)

func NewDebouncer(duration time.Duration) *Debouncer {
	return &Debouncer{}
}

func (d *Debouncer) takeFromDistributedCache(key string, closure Closure) func() (interface{}, error) {
	return func() (interface{}, error) {
		val, err := d.distributedCache.Get(key)
		if err == nil {
			return val, nil
		}

		return d.distributedGroup.Do(key, closure)
	}
}

func (d *Debouncer) Do(key string, closure Closure) (interface{}, error) {
	val, err := d.localCache.Get(key)
	if err == nil {
		return val, nil
	}

	result := <-d.localGroup.DoChan(key, d.takeFromDistributedCache(key, closure))

	d.localCache.Set(key, result.Val, d.localTTL)

	return result.Val, result.Err
}

// DistributedGroup
type DistributedGroup struct {
	mu    LockFactory
	cache Cache
	ttl   time.Duration
}

func (g *DistributedGroup) Do(key string, closure Closure) (v interface{}, err error) {
	lock := g.mu(key)
	if err := lock.Lock(); err != nil {
		<-time.After(time.Second)
		return g.cache.Get(key)
	}
	defer lock.Unlock()

	result, err := closure()

	g.cache.Set(key, result, g.ttl)

	return result, err
}
