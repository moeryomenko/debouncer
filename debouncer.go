package debouncer

import "context"

type Debouncer struct {
	result interface{}
	err    error
	locker DebounceLock
}

type Closure func(ctx context.Context) (interface{}, error)

func (d *Debouncer) Debounce(ctx context.Context, closure Closure) (interface{}, error) {
	if d.locker.TryLock() {
		defer d.locker.Unlock()

		d.result, d.err = closure(ctx)
		return d.result, d.err
	}

	d.locker.WaitUnlocked()
	return d.result, d.err
}
