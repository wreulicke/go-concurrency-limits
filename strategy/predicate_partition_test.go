package strategy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/platinummonkey/go-concurrency-limits/strategy/matchers"
)

func makeTestPartitions() []*PredicatePartition {
	batchPartition := NewPredicatePartitionWithMetricRegistry(
		"batch",
		0.3,
		matchers.StringPredicateMatcher("batch", false),
	)

	livePartition := NewPredicatePartitionWithMetricRegistry(
		"live",
		0.7,
		matchers.StringPredicateMatcher("live", false),
	)

	return []*PredicatePartition{batchPartition, livePartition}
}

func TestPredicatePartition(t *testing.T) {
	t.Parallel()

	t.Run("partitions", func(t2 *testing.T) {
		t2.Parallel()
		asrt := assert.New(t2)
		partitions := makeTestPartitions()
		asrt.Equal("batch", partitions[0].Name())
		asrt.Equal("PredicatePartition{name=batch, percent=0.300000, limit=1, busy=0}", partitions[0].String())
	})

	t.Run("NewPredicatePartition", func(t2 *testing.T) {
		t2.Parallel()
		asrt := assert.New(t2)
		strategy, err := NewPredicatePartitionStrategyWithMetricRegistry(
			makeTestPartitions(),
			1,
		)
		asrt.NoError(err, "failed to create strategy")
		asrt.NotNil(strategy)
		asrt.Equal(
			"PredicatePartitionStrategy{partitions=[PredicatePartition{name=batch, percent=0.300000, limit=1, busy=0} PredicatePartition{name=live, percent=0.700000, limit=1, busy=0}], limit=1, busy=0}",
			strategy.String(),
		)
	})

	t.Run("NewPredicatePartitionError", func(t2 *testing.T) {
		t2.Parallel()
		asrt := assert.New(t2)
		strategy, err := NewPredicatePartitionStrategyWithMetricRegistry(
			make([]*PredicatePartition, 0),
			1,
		)
		asrt.Errorf(err, "expected error instead")
		asrt.Nil(strategy)

		partitions := make([]*PredicatePartition, 0)
		partitions = append(partitions, NewPredicatePartitionWithMetricRegistry(
			"foo", 0.9, nil))
		partitions = append(partitions, NewPredicatePartitionWithMetricRegistry(
			"bar", 0.2, nil))
		strategy, err = NewPredicatePartitionStrategyWithMetricRegistry(
			partitions,
			1,
		)
		asrt.Errorf(err, "expected error instead")
		asrt.Nil(strategy)
	})

	t.Run("LimitAllocatedToBins", func(t2 *testing.T) {
		t2.Parallel()
		asrt := assert.New(t2)
		strategy, err := NewPredicatePartitionStrategyWithMetricRegistry(
			makeTestPartitions(),
			1,
		)
		asrt.NoError(err, "failed to create strategy")
		asrt.NotNil(strategy)

		// negative limit uses 1
		strategy.SetLimit(-10)
		asrt.Equal(1, strategy.Limit(), "expected limit to be set to 1")

		strategy.SetLimit(10)
		asrt.Equal(10, strategy.Limit(), "expected limit to be set to 10")

		limit, err := strategy.BinLimit(0)
		asrt.NoError(err)
		asrt.Equal(3, limit)

		limit, err = strategy.BinLimit(1)
		asrt.NoError(err)
		asrt.Equal(7, limit)
	})

	t.Run("UseExcessCapacityUntilTotalLimit", func(t2 *testing.T) {
		t2.Parallel()
		asrt := assert.New(t2)
		strategy, err := NewPredicatePartitionStrategyWithMetricRegistry(
			makeTestPartitions(),
			1,
		)
		asrt.NoError(err, "failed to create strategy")
		asrt.NotNil(strategy)

		strategy.SetLimit(10)

		ctx := context.WithValue(context.Background(), matchers.StringPredicateContextKey, "batch")

		for i := 0; i < 10; i++ {
			token, ok := strategy.TryAcquire(ctx)
			asrt.True(ok && token != nil)
			asrt.True(token.IsAcquired())
			busyCount, err := strategy.BinBusyCount(0)
			asrt.NoError(err)
			asrt.Equal(i+1, busyCount)
		}

		// should be exceeded
		token, ok := strategy.TryAcquire(ctx)
		asrt.False(ok)
		if token != nil {
			asrt.False(token.IsAcquired())
		}
	})

	t.Run("ExceedTotalLimitForUnusedBin", func(t2 *testing.T) {
		t2.Parallel()
		asrt := assert.New(t2)
		strategy, err := NewPredicatePartitionStrategyWithMetricRegistry(
			makeTestPartitions(),
			1,
		)
		asrt.NoError(err, "failed to create strategy")
		asrt.NotNil(strategy)

		strategy.SetLimit(10)

		ctxBatch := context.WithValue(context.Background(), matchers.StringPredicateContextKey, "batch")
		ctxLive := context.WithValue(context.Background(), matchers.StringPredicateContextKey, "live")

		for i := 0; i < 10; i++ {
			token, ok := strategy.TryAcquire(ctxBatch)
			asrt.True(ok && token != nil)
			asrt.True(token.IsAcquired())
			busyCount, err := strategy.BinBusyCount(0)
			asrt.NoError(err)
			asrt.Equal(i+1, busyCount)
		}

		// should be exceeded
		token, ok := strategy.TryAcquire(ctxBatch)
		asrt.False(ok)
		if token != nil {
			asrt.False(token.IsAcquired())
		}

		// now try live
		token, ok = strategy.TryAcquire(ctxLive)
		asrt.True(ok && token != nil)
		asrt.True(token.IsAcquired())
	})

	t.Run("RejectOnceAllLimitsReached", func(t2 *testing.T) {
		t2.Parallel()
		asrt := assert.New(t2)
		strategy, err := NewPredicatePartitionStrategyWithMetricRegistry(
			makeTestPartitions(),
			1,
		)
		asrt.NoError(err, "failed to create strategy")
		asrt.NotNil(strategy)

		strategy.SetLimit(10)

		ctxBatch := context.WithValue(context.Background(), matchers.StringPredicateContextKey, "batch")
		ctxLive := context.WithValue(context.Background(), matchers.StringPredicateContextKey, "live")

		for i := 0; i < 3; i++ {
			token, ok := strategy.TryAcquire(ctxBatch)
			asrt.True(ok && token != nil)
			asrt.True(token.IsAcquired())
			busyCount, err := strategy.BinBusyCount(0)
			asrt.NoError(err)
			asrt.Equal(i+1, busyCount)
			asrt.Equal(i+1, strategy.BusyCount())
		}

		for i := 0; i < 7; i++ {
			token, ok := strategy.TryAcquire(ctxLive)
			asrt.True(ok && token != nil)
			asrt.True(token.IsAcquired())
			busyCount, err := strategy.BinBusyCount(1)
			asrt.NoError(err)
			asrt.Equal(i+1, busyCount)
			asrt.Equal(i+4, strategy.BusyCount())
		}

		// should be exceeded
		token, ok := strategy.TryAcquire(ctxBatch)
		asrt.False(ok)
		if token != nil {
			asrt.False(token.IsAcquired())
		}
		// should be exceeded
		token, ok = strategy.TryAcquire(ctxLive)
		asrt.False(ok)
		if token != nil {
			asrt.False(token.IsAcquired())
		}

	})

	t.Run("ReleaseLimit", func(t2 *testing.T) {
		t2.Parallel()
		asrt := assert.New(t2)
		strategy, err := NewPredicatePartitionStrategyWithMetricRegistry(
			makeTestPartitions(),
			1,
		)
		asrt.NoError(err, "failed to create strategy")
		asrt.NotNil(strategy)

		strategy.SetLimit(10)

		ctxBatch := context.WithValue(context.Background(), matchers.StringPredicateContextKey, "batch")

		token, ok := strategy.TryAcquire(ctxBatch)
		asrt.True(ok && token != nil)
		asrt.True(token.IsAcquired())

		for i := 1; i < 10; i++ {
			token2, ok := strategy.TryAcquire(ctxBatch)
			asrt.True(ok && token2 != nil)
			asrt.True(token2.IsAcquired())
			busyCount, err := strategy.BinBusyCount(0)
			asrt.NoError(err)
			asrt.Equal(i+1, busyCount)
		}

		// should be exceeded
		token2, ok := strategy.TryAcquire(ctxBatch)
		asrt.False(ok)
		if token2 != nil {
			asrt.False(token2.IsAcquired())
		}

		token.Release()
		busyCount, err := strategy.BinBusyCount(0)
		asrt.NoError(err)
		asrt.Equal(9, busyCount)
		asrt.Equal(9, strategy.BusyCount())

		// can re-acquire
		token2, ok = strategy.TryAcquire(ctxBatch)
		asrt.True(ok && token2 != nil)
		asrt.True(token2.IsAcquired())

		busyCount, err = strategy.BinBusyCount(0)
		asrt.NoError(err)
		asrt.Equal(10, busyCount)
		asrt.Equal(10, strategy.BusyCount())
	})

	t.Run("SetLimitReservesBusy", func(t2 *testing.T) {
		t2.Parallel()
		asrt := assert.New(t2)
		strategy, err := NewPredicatePartitionStrategyWithMetricRegistry(
			makeTestPartitions(),
			1,
		)
		asrt.NoError(err, "failed to create strategy")
		asrt.NotNil(strategy)

		strategy.SetLimit(10)

		binLimit, err := strategy.BinLimit(0)
		asrt.NoError(err)
		asrt.Equal(3, binLimit)

		ctxBatch := context.WithValue(context.Background(), matchers.StringPredicateContextKey, "batch")
		// should be exceeded
		token, ok := strategy.TryAcquire(ctxBatch)
		asrt.True(ok && token != nil)
		asrt.True(token.IsAcquired())

		busyCount, err := strategy.BinBusyCount(0)
		asrt.NoError(err)
		asrt.Equal(1, busyCount)
		asrt.Equal(1, strategy.BusyCount())

		strategy.SetLimit(20)

		binLimit, err = strategy.BinLimit(0)
		asrt.NoError(err)
		asrt.Equal(6, binLimit)

		busyCount, err = strategy.BinBusyCount(0)
		asrt.NoError(err)
		asrt.Equal(1, busyCount)
		asrt.Equal(1, strategy.BusyCount())
	})

	t.Run("AddRemoveDynamically", func(t2 *testing.T) {
		t2.Parallel()
		asrt := assert.New(t2)

		testPartitions := makeTestPartitions()
		strategy, err := NewPredicatePartitionStrategyWithMetricRegistry(
			testPartitions,
			1,
		)
		asrt.NoError(err, "failed to create strategy")
		asrt.NotNil(strategy)
		strategy.SetLimit(10)

		// add a partition
		testPartition := NewPredicatePartitionWithMetricRegistry(
			"test1",
			0.7,
			matchers.StringPredicateMatcher("test1", false),
		)
		strategy.AddPartition(testPartition)
		ctxTest := context.WithValue(context.Background(), matchers.StringPredicateContextKey, "test1")
		token, ok := strategy.TryAcquire(ctxTest)
		asrt.True(ok)
		asrt.NotNil(token)

		// remove a partition
		strategy.RemovePartitionsMatching(ctxTest)
		// we get the default token now
		token, ok = strategy.TryAcquire(ctxTest)
		asrt.False(ok)
		asrt.False(token.IsAcquired())
	})
}
