//go:build integration

// bench_test.go holds the permanent probes behind the numbers in
// docs/benchmarks/fixture-copy.md. Run them with:
//
//	go test -tags integration -bench BenchmarkCopy -run '^$' ./internal/lyxtest
//
// Note that b.TempDir() cleanup accumulates to the end of the benchmark
// (Go only cleans up temp dirs when the benchmark function returns), which
// matches how real tests defer fixture cleanup to test end rather than
// per-iteration — the same accumulation any test suite using these fixtures
// already pays.

package lyxtest

import "testing"

// BenchmarkCopyPaired measures the serial cost of CopyPaired.
func BenchmarkCopyPaired(b *testing.B) {
	for b.Loop() {
		CopyPaired(b)
	}
}

// BenchmarkCopyPairedLocal measures the serial cost of CopyPairedLocal.
func BenchmarkCopyPairedLocal(b *testing.B) {
	for b.Loop() {
		CopyPairedLocal(b)
	}
}

// BenchmarkCopyPairedParallel measures CopyPaired under contention (b.RunParallel).
func BenchmarkCopyPairedParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			CopyPaired(b)
		}
	})
}

// BenchmarkCopyPairedLocalParallel measures CopyPairedLocal under contention.
func BenchmarkCopyPairedLocalParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			CopyPairedLocal(b)
		}
	})
}
