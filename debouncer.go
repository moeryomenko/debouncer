package debouncer

import (
	"context"
	"time"
)

type Debouncer struct {
	result  interface{}
	err     error
	closure Closure
	locker  DebounceLock
}

type Closure func(ctx context.Context) (interface{}, error)

func NewDebouncer(closure Closure, duration time.Duration) *Debouncer {
	return &Debouncer{
		locker:  New(duration),
		closure: closure,
	}
}

func (d *Debouncer) Debounce(ctx context.Context) (interface{}, error) {
	if d.locker.TryLock() {
		defer d.locker.Unlock()

		d.result, d.err = d.closure(ctx)
		return d.result, d.err
	}

	d.locker.WaitUnlocked()
	return d.result, d.err
}
