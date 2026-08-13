//go:build integration

// bench_test.go holds the permanent probes behind the numbers in
// docs/benchmarks/fixture-copy.md. Run them with:
//
//	go test -tags integration -bench BenchmarkCopyRepo -run '^$' ./internal/gitkit
//
// Note that b.TempDir() cleanup accumulates to the end of the benchmark
// (Go only cleans up temp dirs when the benchmark function returns), which
// matches how real tests defer fixture cleanup to test end rather than
// per-iteration — the same accumulation any test suite using these fixtures
// already pays.
//
// This file used to carry the four benchmarks measuring the paired warp+weft fixture this package
// built, before that fixture's deletion. Those four are re-created, retargeted onto the real hub,
// as batch 3's own benchmark file in the hubforge package (internal/hubforge/bench_test.go) —
// see docs/benchmarks/fixture-copy.md for the reproducing trail across both.

package gitkit

import "testing"

// BenchmarkCopyRepo measures the serial cost of CopyRepo.
func BenchmarkCopyRepo(b *testing.B) {
	for b.Loop() {
		CopyRepo(b)
	}
}
