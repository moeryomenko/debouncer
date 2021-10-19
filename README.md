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
	suppressor, err := debouncer.NewDebouncer(
		debouncer.Redis,   // driver.
		"localhost:6379",  // host and port to redis.
		time.Second,       // expiration duration for instance cache.
		3*time.Second,     // expiration duration for Redis.
	)
	if err != nil {
		panic("could not create suppressor")
	}

	...

	result, err := suppressor.Do(key /* token for acquire fn */, 300 * time.Millisecond /* time of execute fn */, fn)
	if err != nil {
		panic("something gone wrong")
	}

	...
}
```

## License

Debouncer is primarily distributed under the terms of both the MIT license and the Apache License (Version 2.0).

See [LICENSE-APACHE](LICENSE-APACHE) and/or [LICENSE-MIT](LICENSE-MIT) for details.
