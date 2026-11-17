//go:build !windows

// coveragescope_unix_integration_test.go covers the miss a coverage scope takes when the Go toolchain cannot be read.
// It runs against a real paired hub and a stand-in go command written as a shell script.

package gatecli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestOpenSelector_ToolchainEnvironmentUnreadable(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	// The stand-in prints a GOROOT and then fails the version and platform read.
	goBinary := filepath.Join(t.TempDir(), "go")
	script := "#!/bin/sh\nif [ \"$2\" = GOROOT ]; then echo /goroot; exit 0; fi\necho 'go: toolchain broken' >&2\nexit 1\n"
	if err := os.WriteFile(goBinary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	request := testRequest{coverageMaps: t.TempDir(), goBinary: goBinary}

	selector, miss := openSelector(context.Background(), gateslot.Config{}, h.Location, h.PrimeWorktree(), request)
	if selector != nil {
		t.Error("openSelector() selector != nil; want none for an unreadable toolchain")
	}
	if miss == nil || miss.Reason != testcover.MissMapUnreadable || !strings.Contains(miss.Detail, "go env: ") {
		t.Errorf("openSelector() miss = %v; want a miss for %q naming the go env failure", miss, testcover.MissMapUnreadable)
	}
}
