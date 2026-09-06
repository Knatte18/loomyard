//go:build integration

// crosscompile_test.go is the durable in-repo cross-compile gate for Linux support.
// It shells out to the real `go` toolchain with GOOS=linux and fails the build if any
// package in the module — including every `_linux.go` file, which the host's native
// `go test` on Windows never compiles — fails to build. This is the mechanical proof
// that the whole module cross-compiles for Linux; it adds no CI, because the repo has
// none and enforces every invariant via `go test`. The gate now runs on every Tier 2
// (`-tags integration`) run rather than on every `go test`, because a whole-module
// `GOOS=linux go build ./...` does not belong in the offline loop (Test Tier Purity
// Invariant); the per-batch `GOOS=linux go build` development gates it mirrors are
// unchanged.
//
// The build no longer runs under CGO_ENABLED=0: `github.com/Knatte18/quarry` links
// tree-sitter's C grammars and its own internal/cgoguard fails the compile outright under
// CGO_ENABLED=0, deliberately and unconditionally — a hard requirement this module inherited
// the moment it took quarry on as a dependency, and cannot relax from this side since quarry
// lives outside this worktree. See CONSTRAINTS.md's Quarry CGO Requirement Invariant.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCrossCompileLinux cross-compiles the entire module for GOOS=linux and fails on any non-zero
// exit, surfacing the compiler's combined output.
// It is the whole-module analogue of the per-batch `GOOS=linux go build ./<pkg>/...` gates run
// during development: those check one package as it lands, this one is the durable guard that every
// seamed package (proc, fslink, vscode, configengine, tools/deploy) plus every Linux-tagged file
// added across the task still compiles together, indefinitely.
func TestCrossCompileLinux(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH, so this
	// gate never blocks environments (e.g. a minimal CI image) that lack it.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	// quarry's cgo requirement (see the file doc comment) means this build now needs a real C
	// compiler targeting linux/amd64. That is just the host's own "cc" when the host already IS
	// linux/amd64 — a native build, no cross-toolchain involved — but is a genuine linux/amd64
	// cross C toolchain anywhere else, which this repo does not provide or assume any developer
	// has configured. Skip there rather than fail on infrastructure this gate was never meant to
	// require; CC set is this test's signal that one has been deliberately wired up.
	if (runtime.GOOS != "linux" || runtime.GOARCH != "amd64") && os.Getenv("CC") == "" {
		t.Skip("host is not linux/amd64 and CC is unset: no linux/amd64 C cross-toolchain is configured, and quarry hard-requires CGO_ENABLED=1")
	}

	// Resolve the module root via `go env GOMOD` rather than assuming the test's working directory.
	out, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD failed: %v\n%s", err, out)
	}
	goMod := strings.TrimSpace(string(out))
	if goMod == "" || goMod == os.DevNull {
		t.Skip("no enclosing Go module (go env GOMOD is empty)")
	}
	moduleRoot := filepath.Dir(goMod)

	// Build every package for GOOS=linux, GOARCH=amd64, CGO_ENABLED=1: quarry's own cgoguard
	// refuses CGO_ENABLED=0 outright, so this can no longer be the static, cgo-free cross-compile
	// it once was — see the file doc comment.
	cmd := exec.Command("go", "build", "-o", os.DevNull, "./...")
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=1")
	buildOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("GOOS=linux go build ./... failed:\n%s", buildOut)
	}
}
