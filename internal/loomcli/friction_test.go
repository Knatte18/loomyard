// friction_test.go covers the Tier 2 friction wiring this batch's driveCmd/runCmd call sites own: the
// once-per-task clear-and-create split ensureFrictionDirAfterSeed implements for runCmd, drive's own
// unconditional ensure, and the reflection-trigger decision shouldReflectFriction/reflectFriction
// implement for driveCmd. Every test here is untagged Tier 1: no exec.Command, no gitexec, no
// hubforge.NewHub, no real spawn, no time.Sleep at or above one second.

package loomcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/frictionengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestEnsureFrictionDirAfterSeed_NilErrorClearsThenCreates asserts a genuine first seed (nil seedErr)
// clears a pre-populated friction directory before recreating it empty.
func TestEnsureFrictionDirAfterSeed_NilErrorClearsThenCreates(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "friction")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", dir, err)
	}
	notePath := filepath.Join(dir, "some-note.md")
	if err := os.WriteFile(notePath, []byte("pre-existing note"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", notePath, err)
	}

	ensureFrictionDirAfterSeed(dir, nil)

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("Stat(%q) = %v; want the directory to exist after ensure", dir, err)
	}
	if _, err := os.Stat(notePath); !os.IsNotExist(err) {
		t.Errorf("Stat(%q) = %v; want the pre-existing note to have been cleared", notePath, err)
	}
}

// TestEnsureFrictionDirAfterSeed_ErrSeedExistsLeavesNotesUntouched asserts an ErrSeedExists re-entry
// -- the crash-resume guarantee -- leaves existing notes untouched but still ensures the directory
// exists.
func TestEnsureFrictionDirAfterSeed_ErrSeedExistsLeavesNotesUntouched(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "friction")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", dir, err)
	}
	notePath := filepath.Join(dir, "some-note.md")
	if err := os.WriteFile(notePath, []byte("pre-existing note"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", notePath, err)
	}

	ensureFrictionDirAfterSeed(dir, loomshed.ErrSeedExists)

	content, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v; want the pre-existing note to survive a resume", notePath, err)
	}
	if string(content) != "pre-existing note" {
		t.Errorf("note content = %q; want it untouched", content)
	}
}

// TestEnsureFrictionDirAfterSeed_EmptyDirSkipsBothOperations asserts an empty frictionDir is a no-op:
// Tier 2 is off and neither the clear nor the ensure runs.
func TestEnsureFrictionDirAfterSeed_EmptyDirSkipsBothOperations(t *testing.T) {
	t.Parallel()

	// No panic and no filesystem effect is the whole assertion here: an empty dir gives
	// os.RemoveAll/os.MkdirAll nothing to act on, and this call must not attempt either.
	ensureFrictionDirAfterSeed("", nil)
	ensureFrictionDirAfterSeed("", loomshed.ErrSeedExists)
}

// TestEnsureFrictionDirAfterSeed_MkdirAllFailureDoesNotError asserts a failed create (frictionDir's
// parent is a regular file, not a directory) leaves the call's own outcome unchanged: no panic, no
// returned error -- friction.EnsureDir's own contract is to warn and continue, never to fail the
// caller.
func TestEnsureFrictionDirAfterSeed_MkdirAllFailureDoesNotError(t *testing.T) {
	t.Parallel()

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", blocker, err)
	}
	dir := filepath.Join(blocker, "friction")

	// The assertion is that this returns at all, without panicking: ensureFrictionDirAfterSeed has no
	// error return, so a caller (runCmd) proceeds with the seed's own outcome regardless of whether
	// the directory could actually be created.
	ensureFrictionDirAfterSeed(dir, nil)
}

// TestDriveEnsuresAbsentFrictionDir asserts the same friction.EnsureDir call drive.go makes at
// startup creates an absent friction directory before the run proceeds -- mirrored here directly
// against the package driveCmd calls, since drive's own RunE is not independently invocable without a
// real Shed.
func TestDriveEnsuresAbsentFrictionDir(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "friction")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("Stat(%q) = %v; want the directory absent before the ensure", dir, err)
	}

	friction.EnsureDir(dir)

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("Stat(%q) = %v; want the directory to exist after EnsureDir", dir, err)
	}
}

// TestShouldReflectFriction covers every combination of the resolved friction directory and the run
// outcome shouldReflectFriction gates driveCmd's reflection call on: RunPaused never triggers it
// regardless of the directory, an empty directory never triggers it regardless of outcome, and
// RunDone/RunBlocked both trigger it when the directory is non-empty.
//
// The non-nil-err path is not exercised here because it is structurally unreachable: driveCmd's RunE
// already returns on a non-nil shed.Run error before this decision is ever consulted, so there is no
// outcome/err combination this function itself could be called with to represent it.
func TestShouldReflectFriction(t *testing.T) {
	tests := []struct {
		name        string
		frictionDir string
		outcome     shedengine.RunOutcome
		want        bool
	}{
		{"Done_DirSet", "/tmp/friction", shedengine.RunDone, true},
		{"Blocked_DirSet", "/tmp/friction", shedengine.RunBlocked, true},
		{"Paused_DirSet", "/tmp/friction", shedengine.RunPaused, false},
		{"Done_DirEmpty", "", shedengine.RunDone, false},
		{"Blocked_DirEmpty", "", shedengine.RunBlocked, false},
		{"Paused_DirEmpty", "", shedengine.RunPaused, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldReflectFriction(tt.frictionDir, tt.outcome); got != tt.want {
				t.Errorf("shouldReflectFriction(%q, %q) = %v; want %v", tt.frictionDir, tt.outcome, got, tt.want)
			}
		})
	}
}

// TestReflectFriction_DepsValidationFailureReportsFailed asserts a malformed Deps -- a relative
// frictionDir here, which frictionengine.Reflect's own validateDeps rejects -- is logged and reported
// as frictionengine.StatusFailed on c.reflectFriction's return, never surfaced as an error and never
// "filed": frictionengine has no such status, since nothing in Go parses the agent's own report file.
func TestReflectFriction_DepsValidationFailureReportsFailed(t *testing.T) {
	t.Parallel()

	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}

	c := &loomCLI{
		location:    loc,
		frictionDir: "relative-friction-dir", // not absolute: fails Deps validation deliberately
		runDeps:     websterengine.RunDeps{Geom: websterengine.Geometry{StencilsDir: "stencils"}},
	}

	got := c.reflectFriction()

	if got != frictionengine.StatusFailed {
		t.Errorf("c.reflectFriction() = %q; want %q", got, frictionengine.StatusFailed)
	}
	if got == "filed" {
		t.Error("c.reflectFriction() = \"filed\"; that status must never exist")
	}
}
