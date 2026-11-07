// bench_test.go holds the hub fixture benchmarks behind the numbers in docs/benchmarks/fixture-copy.md.
// b.TempDir() cleanup accumulates until the benchmark returns, as it does in real tests until each test ends.
// Run them with: go test -tags integration -bench 'BenchmarkNewHub|BenchmarkCopyBares' -run '^$' ./internal/hubforge

package hubforge

import "testing"

import "github.com/Knatte18/loomyard/internal/testkit/tiergate"

// BenchmarkNewHub measures the full fixture — bare copy plus clone plus wire — serially.
func BenchmarkNewHub(b *testing.B) {
	tiergate.Require(b, 1)
	for b.Loop() {
		NewHub(b, ".")
	}
}

// BenchmarkNewHubParallel measures the full fixture under b.RunParallel.
// It exercises the parallel safety of NewHub: a sync.Once template read followed by a per-call tb.TempDir().
func BenchmarkNewHubParallel(b *testing.B) {
	tiergate.Require(b, 1)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			NewHub(b, ".")
		}
	})
}

// BenchmarkCopyBares measures the bare-copy step alone, without the clone and wire steps of the full fixture.
// That isolation lets the clone-versus-copy comparison be reproduced from inside the repository.
func BenchmarkCopyBares(b *testing.B) {
	tiergate.Require(b, 1)
	for b.Loop() {
		copyBares(b)
	}
}
