package debouncer

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	redigo "github.com/gomodule/redigo/redis"
	"github.com/moeryomenko/debouncer/adapters"
	"github.com/moeryomenko/synx"
	cache "github.com/moeryomenko/ttlcache"
	"github.com/orlangure/gnomock"
	"github.com/orlangure/gnomock/preset/memcached"
	nomockredis "github.com/orlangure/gnomock/preset/redis"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type Data struct {
	IntValue    int    `json:"int_value"`
	StringValue string `json:"string_value"`
}

var value = map[string]Data{
	`key1`: {IntValue: 10, StringValue: `test`},
	`key2`: {IntValue: 15, StringValue: `test`},
}

type testDeserilizer[V any] struct{}

func (testDeserilizer[V]) Serialize(v V) ([]byte, error) {
	return json.Marshal(v)
}

func (testDeserilizer[V]) Deserilize(b []byte) (V, error) {
	var v V
	err := json.Unmarshal(b, &v)
	return v, err
}

func TestDebouncer(t *testing.T) {
	mem, err := gnomock.Start(memcached.Preset())
	if err != nil {
		t.Fatalf("could not start memcached: %s", err)
	}
	defer func() {
		gnomock.Stop(mem)
	}()

	red, err := gnomock.Start(nomockredis.Preset())
	if err != nil {
		t.Fatalf("could not start redis : %s", err)
	}
	defer func() {
		gnomock.Stop(red)
	}()

	memCache, memLock := adapters.NewMemcachedDriver(memcache.New(mem.DefaultAddress()))

	redisCache, redisLock := adapters.NewRedisDriver(redis.NewClient(&redis.Options{Addr: red.DefaultAddress()}))

	redigoCache, redigoLock := adapters.NewRedigoDriver(&redigo.Pool{
		MaxIdle:     3,
		IdleTimeout: time.Second,
		Dial:        func() (redigo.Conn, error) { return redigo.Dial("tcp", red.DefaultAddress()) },
	})

	testcases := map[string]Distributed[map[string]Data]{
		"Memcached": {
			Cache:      memCache,
			Locker:     memLock,
			Retry:      20 * time.Millisecond,
			TTL:        3 * time.Second,
			Serializer: testDeserilizer[map[string]Data]{},
		},
		"Redis": {
			Cache:      redisCache,
			Locker:     redisLock,
			Retry:      20 * time.Millisecond,
			TTL:        3 * time.Second,
			Serializer: testDeserilizer[map[string]Data]{},
		},
		"Redigo": {
			Cache:      redigoCache,
			Locker:     redigoLock,
			Retry:      20 * time.Millisecond,
			TTL:        3 * time.Second,
			Serializer: testDeserilizer[map[string]Data]{},
		},
	}

	t.Parallel()
	for name, testcase := range testcases {
		name := name
		testcase := testcase

		t.Run(name, func(t *testing.T) {
			key := `test` + name
			counter := int32(0)
			testService := func(context.Context) (map[string]Data, error) {
				<-time.After(time.Second)
				atomic.AddInt32(&counter, 1)
				return value, nil
			}

			// run instances.
			instanceGroup := synx.NewCtxGroup(context.Background())
			for instance := 0; instance < 3; instance++ {
				instance := instance + 1

				instanceGroup.Go(func(ctx context.Context) error {
					ctx, cancel := context.WithCancel(ctx)
					defer cancel()

					localCache := Local[map[string]Data]{
						TTL:   time.Second,
						Cache: cache.NewCache[string, map[string]Data](ctx, 100),
					}

					d, err := NewDebouncer(Config[map[string]Data]{
						Local:       localCache,
						Distributed: testcase,
					})
					require.NoError(t, err, `create debouncer failed`)

					// do concurrent waitRequests.
					requests := 10
					group := synx.NewCtxGroup(context.Background())
					for requestID := 0; requestID < requests; requestID++ {
						requestID := requestID + 1

						group.Go(func(ctx context.Context) error {
							timedRun(t, fmt.Sprintf(`instance%d_request%d`, instance, requestID), func(t *testing.T) {
								result, err := d.Do(ctx, key, testService)
								require.NoError(t, err)
								require.Equal(t, value, result)
							})

							// take from local cache.
							<-time.After(100 * time.Millisecond)
							timedRun(t, fmt.Sprintf(`instance%d_request%d_after_first_request`, instance, requestID), func(t *testing.T) {
								result, err := d.Do(ctx, key, testService)
								require.NoError(t, err)
								require.Equal(t, value, result)
							})

							// take from distributed cache.
							<-time.After(1 * time.Second)
							timedRun(t, fmt.Sprintf(`instance%d_request%d_distributed_cache`, instance, requestID), func(t *testing.T) {
								result, err := d.Do(ctx, key, testService)
								require.NoError(t, err)
								require.Equal(t, value, result)
							})

							return nil
						})
					}

					return group.Wait()
				})
			}

			err = instanceGroup.Wait()
			require.NoError(t, err)
			if counter != 1 {
				t.Fatal("call's more than once")
			}
		})
	}
}

func timedRun(t *testing.T, name string, fn func(t *testing.T)) {
	start := time.Now()
	defer func() {
		t.Logf(`execution time of request %s: %s`, name, time.Since(start).String())
	}()

	fn(t)
}
