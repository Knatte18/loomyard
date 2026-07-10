// crosscompile_test.go is the durable in-repo cross-compile gate for Linux support.
// It shells out to the real `go` toolchain with GOOS=linux and fails the build if any
// package in the module — including every `_linux.go` file, which the host's native
// `go test` on Windows never compiles — fails to build. This is the mechanical proof
// that the whole module cross-compiles for Linux; it adds no CI, because the repo has
// none and enforces every invariant via `go test`.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCrossCompileLinux cross-compiles the entire module for GOOS=linux and fails on
// any non-zero exit, surfacing the compiler's combined output. It is the whole-module
// analogue of the per-batch `GOOS=linux go build ./<pkg>/...` gates run during
// development: those check one package as it lands, this one is the durable guard that
// every seamed package (proc, fslink, vscode, configengine, tools/deploy) plus every
// Linux-tagged file added across the task still compiles together, indefinitely.
func TestCrossCompileLinux(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH, so this
	// gate never blocks environments (e.g. a minimal CI image) that lack it.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	// Resolve the module root via `go env GOMOD` rather than assuming the test's
	// working directory, so this gate works regardless of which package `go test`
	// was invoked from. GOMOD prints os.DevNull (or empty) when no module applies,
	// in which case there is nothing to cross-compile.
	out, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD failed: %v\n%s", err, out)
	}
	goMod := strings.TrimSpace(string(out))
	if goMod == "" || goMod == os.DevNull {
		t.Skip("no enclosing Go module (go env GOMOD is empty)")
	}
	moduleRoot := filepath.Dir(goMod)

	// Build every package under the module root with GOOS=linux, GOARCH=amd64, and
	// CGO_ENABLED=0 (a static, dependency-free cross-compile), discarding the binary
	// output since only the compile result matters here. This is the step that
	// forces the compiler to see every `_linux.go` file, proving the whole module
	// compiles for Linux even though it was authored and tested on Windows.
	cmd := exec.Command("go", "build", "-o", os.DevNull, "./...")
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	buildOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("GOOS=linux go build ./... failed:\n%s", buildOut)
	}
}
