// Package lyxtest holds the shared git-fixture support machinery for Loomyard's
// test suites across internal/worktree, internal/weft, and internal/paths.
// It owns the fixture builders and per-test isolation helpers, following the
// template-built-once + per-test filesystem copy pattern to minimize setup overhead
// and maximize parallelism. See MustRun, CopyHostHub, CopyPaired, and CopyWeft.
package lyxtest
