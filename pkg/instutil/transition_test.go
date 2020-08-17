package instutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransitionCollector(t *testing.T) {
	ctx := context.Background()
	ctx = NewTransitionCollector(ctx, "test-a")
	ctx = NewTransitionCollector(ctx, "test-b")

	// Note: The sub tests depend on each other.

	t.Run("AddInitial", func(t *testing.T) {
		tc := GetTransitionCollector(ctx, "test-a")
		tc.Observe("i-001", "pending", nil)
		tc.Observe("i-002", "running", nil)
		tc.Observe("i-003", "terminated", nil)
		result := tc.Finish()

		require.Len(t, result, 3)

		assert.Equal(t, Transition{Name: "i-001", From: "", To: "pending"}, result[0])
		assert.Equal(t, Transition{Name: "i-002", From: "", To: "running"}, result[1])
		assert.Equal(t, Transition{Name: "i-003", From: "", To: "terminated"}, result[2])
	})

	t.Run("ChangeSingleState", func(t *testing.T) {
		tc := GetTransitionCollector(ctx, "test-a")
		tc.Observe("i-001", "running", nil)
		tc.Observe("i-002", "running", nil)
		tc.Observe("i-003", "terminated", nil)
		result := tc.Finish()

		require.Len(t, result, 1)

		assert.Equal(t, Transition{Name: "i-001", From: "pending", To: "running"}, result[0])
	})

	t.Run("RemoveState", func(t *testing.T) {
		tc := GetTransitionCollector(ctx, "test-a")
		tc.Observe("i-001", "running", nil)
		tc.Observe("i-002", "running", nil)
		result := tc.Finish()

		require.Len(t, result, 1)

		assert.Equal(t, Transition{Name: "i-003", From: "terminated", To: ""}, result[0])
	})

	t.Run("EmptyStateRemovesToo", func(t *testing.T) {
		tc := GetTransitionCollector(ctx, "test-a")
		tc.Observe("i-001", "running", nil)
		tc.Observe("i-002", "", nil)
		result := tc.Finish()

		require.Len(t, result, 1)

		assert.Equal(t, Transition{Name: "i-002", From: "running", To: ""}, result[0])
	})

	t.Run("CachedToNotInterfere", func(t *testing.T) {
		tc := GetTransitionCollector(ctx, "test-b")
		result := tc.Finish()

		// The `test-a` cache still have states in it. Therefore it would want
		// to remove it, when they access the same cache.
		require.Len(t, result, 0)
	})

	t.Run("MissingShouldNotCrash", func(t *testing.T) {
		tc := GetTransitionCollector(ctx, "missing")
		tc.Observe("i-001", "pending", nil)
		tc.Observe("i-002", "running", nil)
		tc.Observe("i-003", "terminated", nil)
		result := tc.Finish()

		require.Len(t, result, 0)
	})
}
