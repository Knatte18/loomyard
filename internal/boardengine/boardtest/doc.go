// Package boardtest holds the cross-cutting test suites for the boardengine module:
// benchmarks and concurrency stress tests.
// They live apart from internal/boardengine, where each *_test.go unit-tests the source file beside it.
//
// Everything here is black-box:
// it imports github.com/Knatte18/loomyard/internal/boardengine and exercises only the exported API.
// Run the standard suites with `go test ./...`;
// the git and integration suites are gated behind the `integration` build tag (see sync_test.go and bench_cli_test.go).
package boardtest
