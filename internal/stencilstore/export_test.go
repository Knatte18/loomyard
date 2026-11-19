// export_test.go exposes the process-global skew state to the external tests of package stencilstore.

package stencilstore

import "testing"

// SetSkewStateForTest sets the shipped registry, the running writer and the writer ordering for the calling test.
// It restores the previous state when the test ends.
// It changes process-global state, so a test that calls it must not call t.Parallel.
func SetSkewStateForTest(t *testing.T, registry Registry, running Writer, older func(recorded Writer) Ordering) {
	t.Helper()
	previousShipped, previousRunning, previousOlder := shipped, runningWriter, shippedOrdering.Load()
	shipped = registry
	runningWriter = func() Writer { return running }
	RegisterOrdering(older)
	t.Cleanup(func() {
		shipped = previousShipped
		runningWriter = previousRunning
		shippedOrdering.Store(previousOlder)
	})
}
