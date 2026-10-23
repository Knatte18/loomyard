//go:build integration

// startguard_integration_test.go drives armAt and "lyx batten run" over a hubforge hub to pin that the pre-run guard refuses before any write: no seed, no status file, no board pull or claim and no window.

package battencli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// TestArmAt_StartGuardRefusesBeforeAnyWrite asserts the guard refuses a start before armAt pulls the board, claims the prime or writes a seed:
// a dirty source refuses naming the path, a cancel marker refuses a step, and "run --window" on the dirty source refuses on the envelope with no seed, so no window was opened.
func TestArmAt_StartGuardRefusesBeforeAnyWrite(t *testing.T) {
	t.Parallel()

	const slug = "guarded-task"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	readme := filepath.Join(h.PrimeWorktree(), "README")
	clean, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	owners := boardengine.Owners{Prime: "here", Entries: map[string]boardengine.OwnerEntry{}}
	assertNothingWritten := func(t *testing.T, fake *fakeOwners) {
		t.Helper()
		if fake.pulls != 0 || len(fake.sets) != 0 {
			t.Errorf("owner seams saw %d pulls and writes %v; want none", fake.pulls, fake.sets)
		}
		assertNoRunWritten(t, h.Location, slug)
	}

	t.Run("DirtyTrackedFileRefusesTheRun", func(t *testing.T) {
		if err := os.WriteFile(readme, append(clean, "changed\n"...), 0o644); err != nil {
			t.Fatal(err)
		}
		fake := &fakeOwners{machine: "here", owners: owners}
		c := &battenCLI{owner: fake.seams()}

		_, err := c.armAt(h.Location, slug, true, "run")

		if err == nil {
			t.Fatal("armAt(run) over a dirty source = nil; want a refusal")
		}
		for _, want := range []string{"README", "commit or revert"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("armAt(run) error = %q; want it to carry %q", err.Error(), want)
			}
		}
		assertNothingWritten(t, fake)
	})

	t.Run("RunWindowOnTheDirtySourceRefusesOnTheEnvelopeWithNoSeed", func(t *testing.T) {
		var out bytes.Buffer
		RunCLIIn(h.PrimeWorktree(), &out, []string{"run", slug, "--window"})

		envelope.RequireErr(t, out.String(), "README")
		assertNoRunWritten(t, h.Location, slug)
	})

	t.Run("CancelMarkerRefusesTheStep", func(t *testing.T) {
		if err := os.WriteFile(readme, clean, 0o644); err != nil {
			t.Fatal(err)
		}
		held, err := takeCancelMarker(h.Location, slug, time.Second)
		if err != nil {
			t.Fatalf("takeCancelMarker: %v", err)
		}
		t.Cleanup(func() { _ = held.release(true) })
		fake := &fakeOwners{machine: "here", owners: owners}
		c := &battenCLI{owner: fake.seams()}

		_, err = c.armAt(h.Location, slug, true, "step")

		if err == nil || !strings.Contains(err.Error(), "a cancel of "+slug+" is under way") {
			t.Fatalf("armAt(step) with a cancel marker = %v; want the cancel refusal", err)
		}
		assertNothingWritten(t, fake)
	})
}

// assertNoRunWritten fails the test when slug has a seed or a status file under location.
func assertNoRunWritten(t *testing.T, location *lyxcwd.Location, slug string) {
	t.Helper()
	if _, found, err := shedrun.ReadSeed(location, slug); err != nil || found {
		t.Errorf("ReadSeed(%q) = (found %v, err %v); want no seed", slug, found, err)
	}
	if pathExists(shedrun.StatusFile(location, slug)) {
		t.Errorf("a status file exists for %q; want none", slug)
	}
}
