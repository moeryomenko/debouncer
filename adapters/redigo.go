package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/redigo"
	"github.com/gomodule/redigo/redis"
)

func NewRedigoDriver(pool *redis.Pool) (Cache, LockFactory) {
	return &Redigo{pool: pool}, RedisLockFactory(redigoRedLock(pool))
}

type Redigo struct {
	pool *redis.Pool
}

func (r *Redigo) query(ctx context.Context, fn func(conn redis.Conn) error) (err error) {
	conn, err := r.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := conn.Close()
		if closeErr != nil {
			err = errors.Join(closeErr)
		}
	} ()

	return fn(conn)
}

// Get returns the value for the specified key if it is present in the cache.
func (r *Redigo) Get(ctx context.Context, key string) (value []byte, err error) {
	err = r.query(ctx, func(conn redis.Conn) (err error) {
		v, err := redis.String(conn.Do(`GET`, key))
		if err != nil {
			return err
		}
		if v == "" {
			return redis.ErrNil
		}
		value = []byte(v)
		return nil
	})
	return value, err
}

// Set inserts or updates the specified key-value pair with an expiration time.
func (r *Redigo) Set(ctx context.Context, key string, value []byte, expiry time.Duration) error {
	return r.query(ctx, func(conn redis.Conn) error {
		_, err := conn.Do(`SET`, key, string(value))
		return err
	})
}

func redigoRedLock(cache *redis.Pool) *redsync.Redsync {
	pool := redigo.NewPool(cache)
	return redsync.New(pool)
}
