package debouncer

import (
	"context"
	"time"

	"github.com/moeryomenko/debouncer/adapters"
	"github.com/moeryomenko/suppressor"
	cache "github.com/moeryomenko/ttlcache"
)

type CacheDriver int

const (
	// Memcached indicates use Memcached like distributed cache and locker.
	Memcached CacheDriver = iota
	// Redis indicates use Redis like distributed cache and locker.
	Redis
)

type Serializer interface {
	Serialize(any) ([]byte, error)
	Deserilize([]byte) (any, error)
}

// Debouncer represents distributed suppressor duplicated calls.
type Debouncer struct {
	localTTL time.Duration

	localGroup       *suppressor.Suppressor
	distributedGroup *DistributedGroup
}

type Closure func() (any, error)

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
	Locker     adapters.LockFactory
	Cache      adapters.Cache
	Retry      time.Duration
	TTL        time.Duration
	Serializer Serializer
}

// NewDebouncer returns new instance of Debouncer.
func NewDebouncer(cfg Config) (*Debouncer, error) {
	return &Debouncer{
		localTTL:   cfg.Local.TTL,
		localGroup: suppressor.New(cfg.Capacity, cfg.Local.TTL, cfg.Local.Policy),
		distributedGroup: &DistributedGroup{
			cache: cfg.Distributed.Cache,
			ttl:   cfg.Distributed.TTL,
			mu:    cfg.Distributed.Locker,
			retry: cfg.Distributed.Retry,
			conv:  cfg.Distributed.Serializer,
		},
	}, nil
}

// Do executes and returns the results of the given function, making
// sure that only one execution is in-flight for a given key at a
// time. If a duplicate comes in from same instance, the duplicate
// caller waits for the original to complete and receives the same results.
// The return a channel that will receive the
// results when they are ready.
func (d *Debouncer) Do(key string, closure Closure) (any, error) {
	result := d.localGroup.Do(key, func() (any, error) {
		return d.distributedGroup.Do(key, closure)
	})

	return result.Val, result.Err
}

// DistributedGroup suppress duplicated calls.
type DistributedGroup struct {
	mu    adapters.LockFactory
	cache adapters.Cache
	ttl   time.Duration
	retry time.Duration
	conv  Serializer
}

// Do executes and returns the results of the given function, making
// sure that only one execution is in-flight for a given key at a
// time. If a duplicate comes in from intances, the duplicate caller
// waits for the only once instance to complete and receives the same results.
// The return a channel that will receive the
// results when they are ready.
func (g *DistributedGroup) Do(key string, closure Closure) (any, error) {
	val, err := g.cache.Get(key)
	if err == nil {
		return g.conv.Deserilize(val)
	}

	lock := g.mu(key, g.ttl)
	if lockErr := lock.Lock(); lockErr != nil {
		ctx, cancel := context.WithTimeout(context.Background(), g.ttl)
		defer cancel()

		tries := int(g.ttl / g.retry)

		return pollResult(ctx, g.retry, tries, func() (any, error) {
			val, err := g.cache.Get(key)
			if err != nil {
				return nil, err
			}
			return g.conv.Deserilize(val)
		})
	}

	result, err := closure()
	if err != nil {
		return nil, err
	}

	binary, err := g.conv.Serialize(result)
	if err != nil {
		return result, nil
	}

	err = g.cache.Set(key, binary, g.ttl)
	if err != nil {
		return result, nil
	}

	return result, nil
}

func pollResult(ctx context.Context, retry time.Duration, tries int, action Closure) (any, error) {
	ticker := time.NewTicker(retry)
	defer ticker.Stop()

	counter := 0

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			counter++
			result, err := action()
			if err != nil && counter != tries {
				continue
			}
			return result, err
		}
	}
}
