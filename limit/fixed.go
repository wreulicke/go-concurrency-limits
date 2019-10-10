package limit

import (
	"fmt"

	"github.com/platinummonkey/go-concurrency-limits/core"
)

// FixedLimit is a non dynamic limit with fixed value.
type FixedLimit struct {
	limit int
}

// NewFixedLimit will return a new FixedLimit
func NewFixedLimit(name string, limit int) *FixedLimit {
	if limit < 0 {
		// force to a positive value
		limit = 10
	}

	l := &FixedLimit{
		limit: limit,
	}
	return l
}

// EstimatedLimit will return the current limit.
func (l *FixedLimit) EstimatedLimit() int {
	return l.limit
}

// NotifyOnChange will register a callback to receive notification whenever the limit is updated to a new value.
func (l *FixedLimit) NotifyOnChange(consumer core.LimitChangeListener) {
	// noop for fixed limit
}

// OnSample will update the limit with the sample.
func (l *FixedLimit) OnSample(startTime int64, rtt int64, inFlight int, didDrop bool) {
	// noop for fixed limit, just record metrics
}

func (l FixedLimit) String() string {
	return fmt.Sprintf("FixedLimit{limit=%d}", l.limit)
}
