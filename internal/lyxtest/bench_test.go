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

// BenchmarkCopyPaired measures the serial cost of CopyPaired: a full
// byte-copy of the paired-Add fixture (hub + bare + weft-prime + weft-bare).
func BenchmarkCopyPaired(b *testing.B) {
	for b.Loop() {
		CopyPaired(b)
	}
}

// BenchmarkCopyPairedLocal measures the serial cost of CopyPairedLocal: the
// SkipPush:true-optimized fixture that omits the weft-bare copy.
func BenchmarkCopyPairedLocal(b *testing.B) {
	for b.Loop() {
		CopyPairedLocal(b)
	}
}

// BenchmarkCopyPairedParallel measures CopyPaired under contention
// (b.RunParallel), matching the concurrency real test binaries impose when
// go test runs many packages/tests in parallel — this is the number that
// matters for suite wall-clock, since contended cost (~500 ms) dwarfs serial
// cost (~128 ms) on the reference machine.
func BenchmarkCopyPairedParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			CopyPaired(b)
		}
	})
}

// BenchmarkCopyPairedLocalParallel measures CopyPairedLocal under
// contention (b.RunParallel); see BenchmarkCopyPairedParallel.
func BenchmarkCopyPairedLocalParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			CopyPairedLocal(b)
		}
	})
}
