package debouncer

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/go-redis/redis/v8"
	"github.com/moeryomenko/debouncer/adapters"
	cache "github.com/moeryomenko/ttlcache"
	"github.com/orlangure/gnomock"
	"github.com/orlangure/gnomock/preset/memcached"
	nomockredis "github.com/orlangure/gnomock/preset/redis"
)

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

	testcases := map[string]Distributed{
		"Memcached": {
			Cache:  memCache,
			Locker: memLock,
			Retry:  20 * time.Millisecond,
			TTL:    3 * time.Second,
		},
		"Redis": {
			Cache:  redisCache,
			Locker: redisLock,
			Retry:  20 * time.Millisecond,
			TTL:    3 * time.Second,
		},
	}

	localCache := Local{
		TTL:      time.Second,
		Capacity: 100,
		Policy:   cache.LFU,
	}

	keyStart := int32(0)
	genKey := func() string {
		return fmt.Sprintf("test_key_%d", atomic.AddInt32(&keyStart, 1))
	}

	t.Parallel()
	for name, testcase := range testcases {
		name := name
		testcase := testcase

		t.Run(name, func(t *testing.T) {
			key := genKey()
			counter := int32(0)
			testService := func() ([]byte, error) {
				atomic.AddInt32(&counter, 1)
				return nil, nil
			}
			// run instances.
			instances := 3
			wait := sync.WaitGroup{}
			wait.Add(instances)
			for i := 0; i < 3; i++ {
				go func(instance int) {
					defer wait.Done()
					d, err := NewDebouncer(Config{
						Local:       localCache,
						Distributed: testcase,
					})
					if err != nil {
						t.Errorf("failed create debouncer: %s", err)
					}

					// do concurrent waitRequests.
					requests := 3
					waitRequests := sync.WaitGroup{}
					waitRequests.Add(requests)
					for i := 0; i < requests; i++ {
						go func() {
							defer waitRequests.Done()
							_, err := d.Do(key, testService)
							if err != nil {
								t.Errorf("failed create concurrent request: %s", err)
							}
						}()
					}

					waitRequests.Wait()
				}(i)
			}

			wait.Wait()
			if counter != 1 {
				t.Fatal("call's more than once")
			}
		})
	}
}
