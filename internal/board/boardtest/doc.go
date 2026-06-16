// Package boardtest holds Loomyard's cross-cutting ("on-the-side") test suites for
// the board module: benchmarks, concurrency stress tests, and git-backed
// integration tests. These are deliberately kept out of internal/board, where
// each *_test.go sits 1:1 next to the source file it unit-tests.
//
// Everything here is black-box: it imports github.com/Knatte18/loomyard/internal/board
// and exercises only the exported API. Run the standard suites with
// `go test ./...`; the git/integration suites are gated behind the `integration`
// build tag (see integration_test.go and bench_git_test.go).
package boardtest
