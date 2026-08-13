//go:build integration

// bench_test.go holds the permanent probes behind the numbers in
// docs/benchmarks/fixture-copy.md, re-established on the real hub this batch introduces. Run them
// with:
//
//	go test -tags integration -bench 'BenchmarkNewHub|BenchmarkCopyBares' -run '^$' ./internal/hubforge
//
// Note that b.TempDir() cleanup accumulates to the end of the benchmark (Go only cleans up temp dirs
// when the benchmark function returns), which matches how real tests defer fixture cleanup to test end
// rather than per-iteration — the same accumulation any test suite using these fixtures already pays.
//
// These replace gitkit's own BenchmarkCopyPaired, BenchmarkCopyPairedLocal,
// BenchmarkCopyPairedParallel, and BenchmarkCopyPairedLocalParallel, which measured the paired
// warp+weft fixture that package built before this migration — see
// internal/gitkit/bench_test.go's own header for the reproducing trail across both files.

package hubforge

import "testing"

// BenchmarkNewHub measures the full fixture — bare copy plus clone plus wire — serially.
func BenchmarkNewHub(b *testing.B) {
	for b.Loop() {
		NewHub(b, ".")
	}
}

// BenchmarkNewHubParallel measures the full fixture under b.RunParallel, exercising NewHub's
// structural parallel safety: a sync.Once template read followed by a per-call tb.TempDir().
func BenchmarkNewHubParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			NewHub(b, ".")
		}
	})
}

// BenchmarkCopyBares measures the bare-copy step alone, isolating it from the clone-and-wire cost
// BenchmarkNewHub measures, so the clone-versus-copy comparison the whole design rests on stays
// reproducible from inside the repo.
func BenchmarkCopyBares(b *testing.B) {
	for b.Loop() {
		copyBares(b)
	}
}
