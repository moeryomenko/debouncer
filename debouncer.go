package debouncer

import (
	"time"

	"github.com/moeryomenko/debouncer/adapters"
	"github.com/moeryomenko/synx"
	cache "github.com/moeryomenko/ttlcache"
)

type CacheDriver int

const (
	// Memcached indicates use Memcached like distributed cache and locker.
	Memcached CacheDriver = iota
	// Redis indicates use Redis like distributed cache and locker.
	Redis
)

// Debouncer represents distributed suppressor duplicated calls.
type Debouncer struct {
	localTTL time.Duration

	localGroup       *synx.Suppressor
	localCache       cache.Cache
	distributedGroup *DistributedGroup
}

type Closure func() ([]byte, error)

type Config struct {
	Local
	Distributed
}

type Local struct {
	TTL      time.Duration
	Capacity int
	Policy   cache.EvictionPolicy
}

type Distributed struct {
	Locker adapters.LockFactory
	Cache  adapters.Cache
	Retry  time.Duration
	TTL    time.Duration
}

// NewDebouncer returns new instance of Debouncer.
func NewDebouncer(cfg Config) (*Debouncer, error) {
	return &Debouncer{
		localTTL:   cfg.Local.TTL,
		localCache: cache.NewCache(cfg.Local.Capacity, cfg.Local.Policy),
		localGroup: synx.NewSuppressor(cfg.Local.TTL),
		distributedGroup: &DistributedGroup{
			cache: cfg.Distributed.Cache,
			ttl:   cfg.Distributed.TTL,
			mu:    cfg.Distributed.Locker,
			retry: cfg.Distributed.Retry,
		},
	}, nil
}

// Do executes and returns the results of the given function, making
// sure that only one execution is in-flight for a given key at a
// time. If a duplicate comes in from same instance, the duplicate
// caller waits for the original to complete and receives the same results.
// The return a channel that will receive the
// results when they are ready.
func (d *Debouncer) Do(key string, closure Closure) (interface{}, error) {
	val, err := d.localCache.Get(key)
	if err == nil {
		return val, nil
	}

	result := <-d.localGroup.Do(key, func() (interface{}, error) {
		return d.distributedGroup.Do(key, closure)
	})

	if result.Err == nil {
		_ = d.localCache.Set(key, result.Val, d.localTTL)
	}

	return result.Val, result.Err
}

// DistributedGroup suppress duplicated calls.
type DistributedGroup struct {
	mu    adapters.LockFactory
	cache adapters.Cache
	ttl   time.Duration
	retry time.Duration
}

// Do executes and returns the results of the given function, making
// sure that only one execution is in-flight for a given key at a
// time. If a duplicate comes in from intances, the duplicate caller
// waits for the only once instance to complete and receives the same results.
// The return a channel that will receive the
// results when they are ready.
func (g *DistributedGroup) Do(key string, closure Closure) ([]byte, error) {
	val, err := g.cache.Get(key)
	if err == nil {
		return val, nil
	}

	lock := g.mu(key, g.ttl)
	if err := lock.Lock(); err != nil {
		<-time.After(g.retry)
		val, err := g.cache.Get(key)
		if err != nil {
			return closure()
		}
		return val, nil
	}

	result, err := closure()

	g.cache.Set(key, result, g.ttl)

	return result, err
}
