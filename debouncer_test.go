package debouncer

import (
	"context"
	"testing"
	"time"
)

func TestDebouncer(t *testing.T) {
	testcases := []struct {
		name          string
		duration      time.Duration
		pause         time.Duration
		fn            Closure
		expectedCalls int
	}{
		{
			name:          "two calls in same time interval",
			duration:      time.Second,
			pause:         time.Millisecond,
			expectedCalls: 1,
			fn: func() Closure {
				counter := 0
				return func(_ context.Context) (interface{}, error) {
					counter++
					return counter, nil
				}
			}(),
		},
		{
			name:          "second call after threshold",
			duration:      100 * time.Microsecond,
			pause:         110 * time.Microsecond,
			expectedCalls: 2,
			fn: func() Closure {
				counter := 0
				return func(_ context.Context) (interface{}, error) {
					counter++
					return counter, nil
				}
			}(),
		},
	}

	for _, testcase := range testcases {
		testcase := testcase

		t.Run(testcase.name, func(t *testing.T) {
			d := NewDebouncer(testcase.fn, testcase.duration)

			d.Debounce(context.TODO())
			<-time.After(testcase.pause)
			result, _ := d.Debounce(context.TODO())

			counter := result.(int)

			if counter != testcase.expectedCalls {
				t.Errorf("expected fn calls %d times, but actualy %d", testcase.expectedCalls, counter)
			}
		})
	}
}
