package limiter

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/platinummonkey/go-concurrency-limits/core"
	"github.com/platinummonkey/go-concurrency-limits/limit"
	"github.com/platinummonkey/go-concurrency-limits/strategy"
)

type testLifoQueueContextKey int

func TestLifoQueue(t *testing.T) {
	t.Parallel()
	asrt := assert.New(t)
	q := lifoQueue{}

	asrt.Equal(uint64(0), q.len())
	size, ctx := q.peek()
	asrt.Equal(uint64(0), size)
	asrt.Nil(ctx)
	asrt.Nil(q.pop())

	ctx1 := context.WithValue(context.Background(), testLifoQueueContextKey(1), 1)
	q.push(ctx1)

	size, ctx = q.peek()
	asrt.Equal(uint64(1), q.len())
	asrt.Equal(uint64(1), size)
	asrt.NotNil(ctx)
	asrt.Equal(ctx1, ctx)

	// add a 2nd
	ctx2 := context.WithValue(context.Background(), testLifoQueueContextKey(2), 2)
	q.push(ctx2)

	// make sure it's still LIFO
	size, ctx = q.peek()
	asrt.Equal(uint64(2), q.len())
	asrt.Equal(uint64(2), size)
	asrt.NotNil(ctx)
	asrt.Equal(ctx2, ctx)
	asrt.Equal(ctx2, q.top.ctx)

	// pop off
	element := q.pop()
	asrt.NotNil(element)
	asrt.Equal(ctx2, element.ctx)

	// check that we only have one again
	size, ctx = q.peek()
	asrt.Equal(uint64(1), q.len())
	asrt.Equal(uint64(1), size)
	asrt.NotNil(ctx)
	asrt.Equal(ctx1, ctx)

	// add a 2nd & 3rd
	ctx3 := context.WithValue(context.Background(), testLifoQueueContextKey(3), 3)
	q.push(ctx3)
	ctx4 := context.WithValue(context.Background(), testLifoQueueContextKey(4), 4)
	q.push(ctx4)

	// remove the middle
	asrt.Equal(uint64(3), q.top.id)
	q.remove(2)
	size, ctx = q.peek()
	asrt.Equal(uint64(2), q.len())
	asrt.Equal(uint64(2), size)
	asrt.NotNil(ctx)
	asrt.Equal(ctx4, ctx)
	asrt.Equal(ctx4, q.top.ctx)

	// check sanity on id's for regression
	for i := 2; i > 0; i-- {
		element := q.pop()
		asrt.Equal(uint64(i), element.id)
	}
}

func TestLifoBlockingListener(t *testing.T) {
	t.Parallel()
	delegateLimiter, _ := NewDefaultLimiterWithDefaults(
		"",
		strategy.NewSimpleStrategy(20),
		limit.NoopLimitLogger{},
	)
	limiter := NewLifoBlockingLimiterWithDefaults(delegateLimiter)
	delegateListener, _ := delegateLimiter.Acquire(context.Background())
	listener := LifoBlockingListener{
		delegateListener: delegateListener,
		limiter:          limiter,
	}
	listener.OnSuccess()
	listener.OnIgnore()
	listener.OnDropped()
}

type acquiredListenerLifo struct {
	id       int
	listener core.Listener
}

func TestLifoBlockingLimiter(t *testing.T) {
	t.Parallel()

	t.Run("NewLifoBlockingLimiterWithDefaults", func(t2 *testing.T) {
		t2.Parallel()
		asrt := assert.New(t2)
		delegateLimiter, _ := NewDefaultLimiterWithDefaults(
			"",
			strategy.NewSimpleStrategy(20),
			limit.NoopLimitLogger{},
		)
		limiter := NewLifoBlockingLimiterWithDefaults(delegateLimiter)
		asrt.NotNil(limiter)
		asrt.True(strings.Contains(limiter.String(), "LifoBlockingLimiter{delegate=DefaultLimiter{"))
	})

	t.Run("NewLifoBlockingLimiter", func(t2 *testing.T) {
		t2.Parallel()
		asrt := assert.New(t2)
		delegateLimiter, _ := NewDefaultLimiterWithDefaults(
			"",
			strategy.NewSimpleStrategy(20),
			limit.NoopLimitLogger{},
		)
		limiter := NewLifoBlockingLimiter(delegateLimiter, -1, 0)
		asrt.NotNil(limiter)
		asrt.True(strings.Contains(limiter.String(), "LifoBlockingLimiter{delegate=DefaultLimiter{"))
	})

	t.Run("Acquire", func(t2 *testing.T) {
		t2.Parallel()
		asrt := assert.New(t2)
		delegateLimiter, _ := NewDefaultLimiter(
			limit.NewFixedLimit("test", 10),
			defaultMinWindowTime,
			defaultMaxWindowTime,
			defaultMinRTTThreshold,
			defaultWindowSize,
			strategy.NewSimpleStrategy(10),
			limit.NoopLimitLogger{},
		)
		limiter := NewLifoBlockingLimiterWithDefaults(delegateLimiter)
		asrt.NotNil(limiter)

		// acquire all tokens first
		listeners := make([]core.Listener, 0)
		for i := 0; i < 10; i++ {
			listener, ok := limiter.Acquire(context.Background())
			asrt.True(ok)
			asrt.NotNil(listener)
			listeners = append(listeners, listener)
		}

		// queue up 10 more waiting
		waitingListeners := make([]acquiredListenerLifo, 0)
		mu := sync.Mutex{}
		startupReady := make(chan bool, 1)
		wg := sync.WaitGroup{}
		wg.Add(10)
		for i := 0; i < 10; i++ {
			if i > 0 {
				select {
				case <-startupReady:
					// proceed
				}
			}
			go func(j int) {
				startupReady <- true
				listener, ok := limiter.Acquire(context.Background())
				asrt.True(ok)
				asrt.NotNil(listener)
				mu.Lock()
				waitingListeners = append(waitingListeners, acquiredListenerLifo{id: j, listener: listener})
				mu.Unlock()
				wg.Done()
			}(i)
		}

		// release all other listeners, so we can continue
		for _, listener := range listeners {
			listener.OnSuccess()
		}

		// wait for others
		wg.Wait()

		// check all eventually required. Note: due to scheduling, it's not entirely LIFO as scheduling will allow
		// some non-determinism
		asrt.Len(waitingListeners, 10)
		// release all
		for _, acquired := range waitingListeners {
			acquired.listener.OnSuccess()
		}
	})
}
