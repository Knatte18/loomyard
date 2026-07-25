// main_test.go covers resolveDest, the destination-directory resolution logic
// for the -dev / -dest flags. Tests are Go-native and Tier-1 pure: no
// go build / go env spawns, so the goBinDir() fallback (which shells out to
// `go env`) is intentionally left uncovered here.

package main

import (
	"testing"

	"github.com/Knatte18/loomyard/tools/internal/devbin"
)

// TestResolveDest_DevAndDestMutuallyExclusive verifies that passing both -dev
// and a non-empty -dest is rejected rather than silently preferring one.
func TestResolveDest_DevAndDestMutuallyExclusive(t *testing.T) {
	_, err := resolveDest(true, "/x")
	if err == nil {
		t.Error("resolveDest(true, \"/x\") = nil error; want mutual-exclusion error")
	}
}

// TestResolveDest_DevUsesDerivedDevBinDir verifies that -dev alone resolves
// to devbin.Dir(), never a hardcoded path.
func TestResolveDest_DevUsesDerivedDevBinDir(t *testing.T) {
	want, err := devbin.Dir()
	if err != nil {
		t.Fatalf("devbin.Dir() error: %v", err)
	}

	got, err := resolveDest(true, "")
	if err != nil {
		t.Fatalf("resolveDest(true, \"\") error: %v", err)
	}
	if got != want {
		t.Errorf("resolveDest(true, \"\") = %q; want %q", got, want)
	}
}

// TestResolveDest_DestPassedThrough verifies that a non-empty -dest is
// returned verbatim when -dev is not set.
func TestResolveDest_DestPassedThrough(t *testing.T) {
	want := "/some/dir"

	got, err := resolveDest(false, want)
	if err != nil {
		t.Fatalf("resolveDest(false, %q) error: %v", want, err)
	}
	if got != want {
		t.Errorf("resolveDest(false, %q) = %q; want %q", want, got, want)
	}
}
