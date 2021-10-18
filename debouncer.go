package debouncer

import (
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/moeryomenko/debouncer/adapters"
	"github.com/moeryomenko/ttlcache"
)

type CacheDriver int

const (
	Memcached CacheDriver = iota
	Redis
)

// Debouncer
type Debouncer struct {
	localCache       adapters.Cache
	distributedCache adapters.Cache

	localTTL time.Duration

	localGroup       singleflight.Group
	distributedGroup *DistributedGroup
}

// Closure
type Closure func() (interface{}, error)

// NewDebouncer
func NewDebouncer(
	localCapacity int,
	impl CacheDriver, dsn string,
	localCacheTTL, distributedCacheTTL time.Duration,
) (*Debouncer, error) {
	d := &Debouncer{
		localCache: cache.NewCache(localCapacity, cache.LRU),
		localTTL:   localCacheTTL,
		distributedGroup: &DistributedGroup{
			ttl: distributedCacheTTL,
		},
	}

	var (
		err                         error
		distributedCache            adapters.Cache
		distributedGroupLockFactory adapters.LockFactory
	)
	switch impl {
	case Memcached:
		distributedCache, distributedGroupLockFactory, err = adapters.NewMemcachedDriver(dsn)
		if err != nil {
			return nil, err
		}
	case Redis:
		distributedCache, distributedGroupLockFactory, err = adapters.NewRedisDriver(dsn)
		if err != nil {
			return nil, err
		}
	default:
		panic("unkown driver")
	}

	d.distributedCache = distributedCache
	d.distributedGroup.mu = distributedGroupLockFactory
	d.distributedGroup.cache = d.distributedCache
	return d, err
}

// Do
func (d *Debouncer) Do(key string, duration time.Duration, closure Closure) (interface{}, error) {
	val, err := d.localCache.Get(key)
	if err == nil {
		return val, nil
	}

	result := <-d.localGroup.DoChan(key, d.takeFromDistributedCache(key, duration, closure))

	return result.Val, result.Err
}

func (d *Debouncer) takeFromDistributedCache(key string, duration time.Duration, closure Closure) Closure {
	return func() (val interface{}, err error) {
		defer func() {
			if err == nil {
				d.localCache.Set(key, val, d.localTTL)

				// deferred release local lock.
				go func() {
					<-time.After(d.localTTL / 2)
					d.localGroup.Forget(key)
				}()
			}
		}()

		val, err = d.distributedCache.Get(key)
		if err == nil {
			return val, nil
		}

		return d.distributedGroup.Do(key, duration, closure)
	}
}

// DistributedGroup
type DistributedGroup struct {
	mu    adapters.LockFactory
	cache adapters.Cache
	ttl   time.Duration
}

// Do
func (g *DistributedGroup) Do(key string, duration time.Duration, closure Closure) (interface{}, error) {
	lock := g.mu(key, g.ttl)
	if err := lock.Lock(); err != nil {
		<-time.After(duration)
		return g.cache.Get(key)
	}

	result, err := closure()

	g.cache.Set(key, result, g.ttl)

	return result, err
}
