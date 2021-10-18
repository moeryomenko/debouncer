package adapters

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/moeryomenko/memsync"
)

func NewMemcachedDriver(dsn string) (Cache, LockFactory, error) {
	client := memcache.New(dsn)
	if err := client.Ping(); err != nil {
		return nil, nil, err
	}
	return &Memcached{client: client}, MemcacheLockFactory(client), nil
}

type Memcached struct {
	client *memcache.Client
}

// Get returns the value for the specified key if it is present in the cache.
func (m *Memcached) Get(key string) (interface{}, error) {
	item, err := m.client.Get(key)
	if err != nil {
		return nil, err
	}
	return item.Value, nil
}

// Set inserts or updates the specified key-value pair with an expiration time.
func (m *Memcached) Set(key string, value interface{}, expiry time.Duration) error {
	val, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return m.client.Add(&memcache.Item{
		Key:        key,
		Value:      val,
		Expiration: int32(expiry / time.Second),
	})
}

func MemcacheLockFactory(cache *memcache.Client) LockFactory {
	locker := memsync.New(cache)
	return func(key string, duration time.Duration) DistributedLock {
		mutex := locker.NewMutex(
			fmt.Sprintf("lock_%s", key),
			memsync.WithTries(1),
			memsync.WithExpiry(duration/2),
		)
		return mutex
	}
}
