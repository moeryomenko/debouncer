package debouncer

import (
	"time"

	"github.com/moeryomenko/debouncer/adapters"
	"github.com/moeryomenko/synx"
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
	distributedGroup *DistributedGroup
}

type Closure func() ([]byte, error)

// NewDebouncer returns new instance of Debouncer.
func NewDebouncer(
	impl CacheDriver, dsn string,
	localCacheTTL, distributedCacheTTL time.Duration,
) (*Debouncer, error) {
	var (
		err                         error
		distributedCache            adapters.Cache
		distributedGroupLockFactory adapters.LockFactory
	)
	switch impl {
	case Memcached:
		distributedCache, distributedGroupLockFactory, err = adapters.NewMemcachedDriver(dsn)
	case Redis:
		distributedCache, distributedGroupLockFactory, err = adapters.NewRedisDriver(dsn)
	default:
		panic("unkown driver")
	}
	if err != nil {
		return nil, err
	}

	return &Debouncer{
		localTTL:   localCacheTTL,
		localGroup: synx.NewSuppressor(),
		distributedGroup: &DistributedGroup{
			cache: distributedCache,
			ttl:   distributedCacheTTL,
			mu:    distributedGroupLockFactory,
		},
	}, nil
}

// Do executes and returns the results of the given function, making
// sure that only one execution is in-flight for a given key at a
// time. If a duplicate comes in from same instance, the duplicate
// caller waits for the original to complete and receives the same results.
// The return a channel that will receive the
// results when they are ready.
func (d *Debouncer) Do(key string, duration time.Duration, closure Closure) (interface{}, error) {
	result := <-d.localGroup.Do(key, d.localTTL, func() (interface{}, error) {
		return d.distributedGroup.Do(key, duration, closure)
	})

	return result.Val, result.Err
}

// DistributedGroup suppress duplicated calls.
type DistributedGroup struct {
	mu    adapters.LockFactory
	cache adapters.Cache
	ttl   time.Duration
}

// Do executes and returns the results of the given function, making
// sure that only one execution is in-flight for a given key at a
// time. If a duplicate comes in from intances, the duplicate caller
// waits for the only once instance to complete and receives the same results.
// The return a channel that will receive the
// results when they are ready.
func (g *DistributedGroup) Do(key string, duration time.Duration, closure Closure) ([]byte, error) {
	val, err := g.cache.Get(key)
	if err == nil {
		return val, nil
	}

	lock := g.mu(key, g.ttl)
	if err := lock.Lock(); err != nil {
		<-time.After(duration)
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
