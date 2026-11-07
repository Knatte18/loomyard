// bench_test.go holds the permanent probes behind the numbers in
// docs/benchmarks/fixture-copy.md. Run them with:
//
//	go test -tags integration -bench BenchmarkCopyRepo -run '^$' ./internal/gitkit
//
// b.TempDir() cleanup accumulates to the end of the benchmark:
// Go removes temp dirs only when the benchmark function returns.
// This matches how real tests defer fixture cleanup to test end rather than per iteration.
//
// The benchmarks of the real hub's fixture copy live in internal/hubforge/bench_test.go.
// docs/benchmarks/fixture-copy.md records the reproducing trail.

package gitkit

import "testing"

import "github.com/Knatte18/loomyard/internal/testkit/tiergate"

// BenchmarkCopyRepo measures the serial cost of a CopyRepo call.
func BenchmarkCopyRepo(b *testing.B) {
	tiergate.Require(b, 1)
	for b.Loop() {
		CopyRepo(b)
	}
}
