// crosscompile_test.go is the in-repo cross-compile gate for Linux support.
// It runs the real `go` toolchain with GOOS=linux and fails if any package in the module does not build.
// That includes every `_linux.go` file, which a native `go test` on Windows never compiles.
// It is the mechanical proof that the whole module cross-compiles for Linux.
// The test runs in the Tier 2 (`-tags integration`) run.
// A whole-module `GOOS=linux go build ./...` does not belong in the offline loop (Test Tier Purity Invariant).
// The per-batch `GOOS=linux go build` development gates mirror it.
//
// The build requires cgo: `github.com/Knatte18/quarry` links tree-sitter's C grammars.
// The module cannot relax that requirement, since quarry is a separate module.

package main

import (
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCrossCompileLinux cross-compiles the entire module for GOOS=linux.
// It fails on any non-zero exit and surfaces the compiler's combined output.
// It builds every package together, so a Linux-tagged file in any package fails it.
func TestCrossCompileLinux(t *testing.T) {
	tiergate.Require(t, 1)
	// Skip when the go toolchain is not on PATH, so a minimal CI image without it does not fail the gate.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	// The build needs a C compiler that targets linux/amd64.
	// On a linux/amd64 host that is the host's own "cc", a native build with no cross-toolchain involved.
	// Anywhere else it needs a linux/amd64 C cross-toolchain.
	// This repo neither provides nor assumes that any developer has configured one.
	// The test skips there rather than fail on infrastructure it was never meant to require;
	// a set CC is its signal that such a toolchain has been deliberately wired up.
	if (runtime.GOOS != "linux" || runtime.GOARCH != "amd64") && os.Getenv("CC") == "" {
		t.Skip("host is not linux/amd64 and CC is unset: no linux/amd64 C cross-toolchain is configured, and quarry hard-requires CGO_ENABLED=1")
	}

	// quarry's internal/cgoguard refuses CGO_ENABLED=0, so the build sets CGO_ENABLED=1.
	cmd := exec.Command("go", "build", "-o", os.DevNull, "./...")
	cmd.Dir = scankit.Root(t)
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=1")
	buildOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("GOOS=linux go build ./... failed:\n%s", buildOut)
	}
}
