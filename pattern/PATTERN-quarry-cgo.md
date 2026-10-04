# PATTERN-quarry-cgo

`lyx` is a cgo binary: `github.com/Knatte18/quarry`'s engine links tree-sitter's C grammars, and its own `internal/cgoguard` deliberately fails the build outright under `CGO_ENABLED=0`.
This is a hard requirement this module cannot relax from this side, since quarry lives outside this repository.

- Every build of this module needs `CGO_ENABLED=1` and a C compiler on `PATH` (gcc or clang on POSIX, mingw-w64 on Windows).
  `CGO_ENABLED` already defaults to `1` for a native build when a compiler is on `PATH`.
- `tools/deploy/main.go`'s build command sets `CGO_ENABLED=1` explicitly, so a production or dev deploy from a cgo-disabled environment fails at the compiler rather than shipping a broken binary.
- `cmd/lyx/crosscompile_test.go`'s `TestCrossCompileLinux` builds for `GOOS=linux` and `GOARCH=amd64` under `CGO_ENABLED=1`, never `=0`; this module is no longer a static, cgo-free cross-compile target.
  On a host that is not natively linux/amd64, the test needs a genuine linux/amd64 C cross-toolchain (signalled by `CC` being set) and skips rather than fails when one is not configured.
