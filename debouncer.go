package debouncer

import (
	"context"
	"time"

	"github.com/moeryomenko/debouncer/adapters"
	"github.com/moeryomenko/suppressor"
)

type Serializer[V any] interface {
	Serialize(V) ([]byte, error)
	Deserilize([]byte) (V, error)
}

// Debouncer represents distributed suppressor duplicated calls.
type Debouncer[V any] struct {
	localGroup       *suppressor.Suppressor[string, V]
	distributedGroup *DistributedGroup[V]
}

type Closure[V any] func(context.Context) (V, error)

type Config[V any] struct {
	Local[V]
	Distributed[V]
}

type Local[V any] struct {
	TTL   time.Duration
	Cache suppressor.Cache[string, V]
}

type Distributed[V any] struct {
	Locker     adapters.LockFactory
	Cache      adapters.Cache
	Retry      time.Duration
	TTL        time.Duration
	Serializer Serializer[V]
}

// NewDebouncer returns new instance of Debouncer.
func NewDebouncer[V any](cfg Config[V]) (*Debouncer[V], error) {
	return &Debouncer[V]{
		localGroup: suppressor.New[string, V](cfg.Local.TTL, cfg.Local.Cache),
		distributedGroup: &DistributedGroup[V]{
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
func (d *Debouncer[V]) Do(ctx context.Context, key string, closure Closure[V]) (V, error) {
	result, err := d.localGroup.Do(key, func() (V, error) {
		return d.distributedGroup.Do(ctx, key, closure)
	})

	return result, err
}

// DistributedGroup suppress duplicated calls.
type DistributedGroup[V any] struct {
	mu    adapters.LockFactory
	cache adapters.Cache
	ttl   time.Duration
	retry time.Duration
	conv  Serializer[V]
}

// Do executes and returns the results of the given function, making
// sure that only one execution is in-flight for a given key at a
// time. If a duplicate comes in from intances, the duplicate caller
// waits for the only once instance to complete and receives the same results.
// The return a channel that will receive the
// results when they are ready.
func (g *DistributedGroup[V]) Do(ctx context.Context, key string, closure Closure[V]) (V, error) {
	val, err := g.cache.Get(ctx, key)
	if err == nil {
		return g.conv.Deserilize(val)
	}

	lock := g.mu(key, g.ttl)
	if lockErr := lock.Lock(); lockErr != nil {
		ctx, cancel := context.WithTimeout(context.Background(), g.ttl)
		defer cancel()

		tries := int(g.ttl / g.retry)

		return pollResult(ctx, g.retry, tries, func() (V, error) {
			val, err := g.cache.Get(ctx, key)
			if err != nil {
				var v V
				return v, err
			}
			return g.conv.Deserilize(val)
		})
	}

	result, err := closure(ctx)
	if err != nil {
		var v V
		return v, err
	}

	binary, err := g.conv.Serialize(result)
	if err != nil {
		return result, nil
	}

	_ = g.cache.Set(ctx, key, binary, g.ttl)

	return result, nil
}

func pollResult[V any](ctx context.Context, retry time.Duration, tries int, action func() (V, error)) (V, error) {
	ticker := time.NewTicker(retry)
	defer ticker.Stop()

	counter := 0

	for {
		select {
		case <-ctx.Done():
			var v V
			return v, ctx.Err()
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
