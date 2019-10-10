package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/platinummonkey/go-concurrency-limits/core"
	"github.com/platinummonkey/go-concurrency-limits/limit"
	"github.com/platinummonkey/go-concurrency-limits/limiter"
	"github.com/platinummonkey/go-concurrency-limits/strategy"
)

type contextKey uint8

const testContextKey = contextKey(1)

type resource struct {
	limiter *rate.Limiter
}

func (r *resource) poll(ctx context.Context) (bool, error) {
	id := ctx.Value(testContextKey).(int)
	log.Printf("request started for id=%d\n", id)
	if !r.limiter.Allow() {
		time.Sleep(time.Millisecond * 10)
		return false, fmt.Errorf("limit exceeded for id=%d", id)
	}
	// sleep some time
	time.Sleep(time.Millisecond * time.Duration(rand.Intn(90)+10))
	log.Printf("request succeeded for id=%d\n", id)
	return true, nil
}

type protectedResource struct {
	external *resource
	guard    core.Limiter
}

func (r *protectedResource) poll(ctx context.Context) (bool, error) {
	id := ctx.Value(testContextKey).(int)
	log.Printf("guarded request started for id=%d\n", id)
	token, ok := r.guard.Acquire(ctx)
	if !ok {
		// short circuit no need to try
		log.Printf("guarded request short circuited for id=%d\n", id)
		if token != nil {
			token.OnDropped()
		}
		return false, fmt.Errorf("short circuited request id=%d", id)
	}

	// try to make request
	_, err := r.external.poll(ctx)
	if err != nil {
		token.OnDropped()
		log.Printf("guarded request failed for id=%d err=%v\n", id, err)
		return false, fmt.Errorf("request failed err=%v", err)
	}
	token.OnSuccess()
	log.Printf("guarded request succeeded for id=%d\n", id)
	return true, nil
}

func main() {
	limitStrategy := strategy.NewSimpleStrategy(10)
	externalResourceLimiter, err := limiter.NewDefaultLimiterWithDefaults(
		"example_single_limit",
		limitStrategy,
		limit.BuiltinLimitLogger{},
	)
	if err != nil {
		log.Fatalf("Error creating limiter err=%v\n", err)
		os.Exit(-1)
	}

	fakeExternalResource := &resource{
		limiter: rate.NewLimiter(5, 15),
	}

	guardedResource := &protectedResource{
		external: fakeExternalResource,
		guard:    externalResourceLimiter,
	}

	endOfExampleTimer := time.NewTimer(time.Second * 10)
	ticker := time.NewTicker(time.Millisecond * 100)
	wg := sync.WaitGroup{}
	counter := 0
	for {
		select {
		case <-endOfExampleTimer.C:
			log.Printf("Waiting for goroutines to finish...")
			wg.Wait()
			return
		case <-ticker.C:
			// make a few requests
			wg.Add(5)
			go func(c int) {
				for i := 0; i < 5; i++ {
					defer wg.Done()
					ctx := context.WithValue(context.Background(), testContextKey, c+i)
					guardedResource.poll(ctx)
				}
			}(counter)
		}
		counter += 5
	}
}
