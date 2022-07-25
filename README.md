# Debouncer

Library for suppress duplicated calls in distributed systems.

## Supported distributed cache and locker

- Redis
- Memcached
- Tarantool(coming soon)

## Usage

```go
package main

import (
	"time"

	"github.com/moeryomenko/debouncer"
)

func main() {
	// create distributed suppressor.
	redisCache, redisLocker := adapters.NewRedisDriver(redis.NewClient(&redis.Options{Addr: "<ip:port>"}))

	suppressor, err := debouncer.NewDebouncer(debouncer.Config{
		Local: {
			TTL:      time.Second,
			Capacity: 100,
			Policy:   cache.LFU,
		},
		Distributed: {
			Cache:  redisCache,
			Locker: redisLocker,
			Retry:  20 * time.Millisecond,
			TTL:    3 * time.Second,
		},
	})
	if err != nil {
		panic("could not create suppressor")
	}

	...

	result, err := suppressor.Do(key /* token for acquire fn */, fn)
	if err != nil {
		panic("something gone wrong")
	}

	...
}
```

## License

Debouncer is primarily distributed under the terms of both the MIT license and the Apache License (Version 2.0).

See [LICENSE-APACHE](LICENSE-APACHE) and/or [LICENSE-MIT](LICENSE-MIT) for details.
