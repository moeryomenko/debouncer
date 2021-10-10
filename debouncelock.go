package debouncer

import (
	"runtime"
	"sync/atomic"
	"time"
)

const (
	unlocked = int32(0)
	locked   = -1

	threshold = 2
	timeout   = 50 * time.Nanosecond
)

// DebounceLock is a spinlock-based implementation of sync.Locker.
type DebounceLock struct {
	state     int32
	threshold *time.Time
	duration  time.Duration
}

func New(duration time.Duration) DebounceLock {
	return DebounceLock{
		state:    unlocked,
		duration: duration,
	}
}

// TryLock performs a non-blocking attempt to lock the locker and returns true if successful.
func (l *DebounceLock) TryLock() bool {
	if l.threshold != nil {
		if time.Now().Before(*l.threshold) {
			return false
		}
	}
	return atomic.CompareAndSwapInt32(&l.state, unlocked, locked)
}

// Lock waits until the locker with be unlocked (if it is not) and then locks it.
func (l *DebounceLock) Lock() {
	wait(l.TryLock)
}

// Unlock unlocks the locker.
func (l *DebounceLock) Unlock() {
	l.threshold = refTime(time.Now().Add(l.duration))

	if !atomic.CompareAndSwapInt32(&l.state, locked, unlocked) {
		panic(`Unlock()-ing non-locked locker`)
	}
}

func refTime(t time.Time) *time.Time {
	return &t
}

// IsUnlocked returns true if the locker is currently unlocked.
func (l *DebounceLock) IsUnlocked() bool {
	return atomic.LoadInt32(&l.state) == unlocked
}

func (l *DebounceLock) WaitUnlocked() {
	wait(l.IsUnlocked)
}

func wait(slowFn func() bool) {
	for i := 0; !slowFn(); {
		if i < threshold {
			time.Sleep(timeout)
			i++
			continue
		}

		// NOTE: if after trying a short timeout,
		// it was not possible to take true,
		// then release the scheduler resources.
		runtime.Gosched()
	}
}
