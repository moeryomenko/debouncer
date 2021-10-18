package debouncer

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/orlangure/gnomock"
	"github.com/orlangure/gnomock/preset/memcached"
	nomockredis "github.com/orlangure/gnomock/preset/redis"
)

const property = `regardless of the number of instances and the number of concurrent requests, only one request will be made to a third-party service`

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

	testcases := map[string]struct {
		driver CacheDriver
		dsn    string
	}{
		"Memcached": {
			driver: Memcached,
			dsn:    mem.DefaultAddress(),
		},
		"Redis": {
			driver: Redis,
			dsn:    red.DefaultAddress(),
		},
	}

	keyStart := int32(0)
	genKey := func() string {
		return fmt.Sprintf("test_key_%d", atomic.AddInt32(&keyStart, 1))
	}

	for name, testcase := range testcases {
		name := name
		testcase := testcase
		t.Run(name, func(t *testing.T) {
			parameters := gopter.DefaultTestParameters()
			properties := gopter.NewProperties(parameters)

			properties.Property(property, prop.ForAll(
				func(requests int) bool {
					key := genKey()
					counter := 0
					testService := func() (interface{}, error) {
						counter++
						return counter, nil
					}
					// run instances.
					wait := sync.WaitGroup{}
					wait.Add(3)
					for i := 0; i < 3; i++ {
						go func(instance int) {
							defer wait.Done()
							d, err := NewDebouncer(10, testcase.driver, testcase.dsn, 2*time.Second, 15*time.Second)
							if err != nil {
								t.Fatalf("failed create debouncer: %s", err)
							}

							// do concurrent waitRequests.
							waitRequests := sync.WaitGroup{}
							waitRequests.Add(requests)
							for i := 0; i < requests; i++ {
								go func() {
									defer waitRequests.Done()
									_, err := d.Do(key, time.Second, testService)
									if err != nil {
										t.Fatalf("failed create concurrent request: %s", err)
									}
								}()
							}

							waitRequests.Wait()
						}(i)
					}

					wait.Wait()
					return counter == 1
				},
				gen.IntRange(5, 10),
			))

			properties.TestingRun(t)
		})
	}
}
