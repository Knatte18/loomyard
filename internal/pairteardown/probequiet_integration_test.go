//go:build integration

// probequiet_integration_test.go pins probeQuiet's reading of the Go driver's lock on a real git worktree, which the untagged tests cannot create.

package pairteardown

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

func TestProbeQuietSeesTheGoDriverLock(t *testing.T) {
	hub := t.TempDir()
	taskRoot := filepath.Join(hub, "slug")
	if err := os.MkdirAll(taskRoot, 0o755); err != nil {
		t.Fatalf("create the task root: %v", err)
	}
	gitkit.MustRun(t, taskRoot, "git", "init")
	task, err := lyxcwd.ResolveWorktree(taskRoot)
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}
	if err := os.MkdirAll(shedrun.ScratchDir(task, shedrun.SelfRunID), 0o755); err != nil {
		t.Fatalf("create the scratch directory: %v", err)
	}
	// A park marker lets the probe skip the reed strand lookup, so only the locks decide.
	if err := os.WriteFile(shedrun.ParkMarker(task, shedrun.SelfRunID), nil, 0o644); err != nil {
		t.Fatalf("write the park marker: %v", err)
	}
	td := &Teardown{prime: &lyxcwd.Location{HubPath: hub, AnchorRel: "."}}

	st, err := td.probeQuiet("slug")
	if err != nil || !st.quiet || st.goDriver {
		t.Fatalf("probeQuiet with every lock free = %+v, %v; want quiet", st, err)
	}

	held := mustHoldLock(t, shedrun.DriverLock(task, shedrun.SelfRunID))
	defer func() { _ = held.Release() }()
	st, err = td.probeQuiet("slug")
	if err != nil || st.quiet || !st.goDriver {
		t.Errorf("probeQuiet with the driver lock held = %+v, %v; want not quiet with goDriver", st, err)
	}
	if err := td.busyError("slug", st); !errors.Is(err, ErrDriverBusy) || !strings.Contains(err.Error(), "Go driver") {
		t.Errorf("busyError = %v; want ErrDriverBusy naming the Go driver", err)
	}
}
