// startguard_integration_test.go drives armAt and "lym batten run" over a hubforge hub.
// It pins that the pre-run guard refuses before any write.
// A refused start leaves no seed, no status file, no board pull or claim, and no window.

package battencli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// A batten verb reconciles a hub whose build stamp is stale as it arms, whether or not the arm then succeeds.
func TestArm_StaleStampReconcilesTheHubBase(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	gitkit.CommitFile(t, h.BoardDir(), configengine.ConfigFileRel("loom"), "code_max_bounces: 2\n", "fixture: old loom key")
	c := &battenCLI{}

	// The status verb names no run here, which arm may refuse; the reconcile comes first.
	_, _ = c.arm(h.PrimeWorktree(), "status", nil)

	base, err := os.ReadFile(configengine.ConfigFile(h.BoardDir(), "loom"))
	if err != nil {
		t.Fatalf("read the hub base loom.yaml: %v", err)
	}
	if got := string(base); !strings.Contains(got, "max_bounces: 2") || strings.Contains(got, "code_max_bounces") {
		t.Errorf("hub base loom.yaml = %q; want the old key nested", got)
	}
}

// TestArmAt_StartGuardRefusesBeforeAnyWrite asserts the guard refuses a start before any write:
// no board pull, no prime claim, and no seed write.
// A dirty source refuses naming the path.
// A cancel marker refuses a step.
// "run" on a dirty source refuses on the envelope.
func TestArmAt_StartGuardRefusesBeforeAnyWrite(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "guarded-task"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	// The pair is added first: a pair forks only from a clean source.
	hubforge.AddPairWith(t, h, "own-beam", fabricengine.AddOptions{SkipPush: true, Beam: true})
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

	t.Run("RunOfTheBeamFromInsideItRefusesBeforeAnyWrite", func(t *testing.T) {
		beam, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("own-beam"))
		if err != nil {
			t.Fatal(err)
		}
		primeName, err := fabricengine.PrimeName(h.Location)
		if err != nil {
			t.Fatal(err)
		}
		fake := &fakeOwners{machine: "here", owners: owners}
		c := &battenCLI{owner: fake.seams()}

		_, err = c.armAt(beam, "own-beam", true, "run")

		if err == nil || !strings.Contains(err.Error(), `"`+primeName+`"`) {
			t.Fatalf("armAt(run) of the beam from inside it = %v; want a refusal naming the beam's parent %q", err, primeName)
		}
		if fake.pulls != 0 || len(fake.sets) != 0 {
			t.Errorf("owner seams saw %d pulls and writes %v; want none", fake.pulls, fake.sets)
		}
		assertNoRunWritten(t, h.Location, "own-beam")
	})

	t.Run("RunOnTheDirtySourceRefusesOnTheEnvelopeWithNoSeed", func(t *testing.T) {
		var out bytes.Buffer
		clihelp.ExecuteIn(Command(), h.PrimeWorktree(), &out, []string{"run", slug})

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
		t.Cleanup(func() { _ = held.release() })
		fake := &fakeOwners{machine: "here", owners: owners}
		c := &battenCLI{owner: fake.seams()}

		_, err = c.armAt(h.Location, slug, true, "step")

		if err == nil || !strings.Contains(err.Error(), "a cancel of "+slug+" is under way") {
			t.Fatalf("armAt(step) with a cancel marker = %v; want the cancel refusal", err)
		}
		assertNothingWritten(t, fake)
	})
}

// TestArmAt_JacquardEntryWithNoStoredRecordRefusesBeforeAnyWrite runs a jacquard entry that holds no stored record.
// The guard refuses before any write: no board pull or claim, no seed, no status file and no task worktree.
// The refusal names the upsert that runs the entry as tapestry.
func TestArmAt_JacquardEntryWithNoStoredRecordRefusesBeforeAnyWrite(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "jacquard-task"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
	if err != nil {
		t.Fatalf("load board config: %v", err)
	}
	cfg.Path = h.BoardDir()
	board := boardengine.New(cfg)
	if _, err := board.UpsertTask(map[string]any{"slug": slug, "title": slug, "kind": "task", "labels": []string{"bug"}, "recipe": shedrun.RecipeJacquard}); err != nil {
		t.Fatalf("seed board entry %s: %v", slug, err)
	}
	// A new task is a draft, which a fresh run refuses before this check.
	if err := board.SetStatus(slug, nil); err != nil {
		t.Fatalf("release board task %s: %v", slug, err)
	}
	fake := &fakeOwners{machine: "here", owners: boardengine.Owners{Prime: "here", Entries: map[string]boardengine.OwnerEntry{}}}
	c := &battenCLI{owner: fake.seams()}

	_, err = c.armAt(h.Location, slug, true, "run")

	if err == nil {
		t.Fatal("armAt(run) of a jacquard entry with no stored record = nil; want a refusal")
	}
	for _, want := range []string{loomrecipe.NoStoredRecordRefusal(slug), `lym board upsert '{"slug":"` + slug + `","recipe":"tapestry"}'`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("armAt(run) error = %q; want it to carry %q", err.Error(), want)
		}
	}
	if fake.pulls != 0 || len(fake.sets) != 0 {
		t.Errorf("owner seams saw %d pulls and writes %v; want none", fake.pulls, fake.sets)
	}
	assertNoRunWritten(t, h.Location, slug)
	if _, err := os.Stat(h.PairCodeWorktree(slug)); !os.IsNotExist(err) {
		t.Errorf("task worktree after the refused run: stat err %v; want none made", err)
	}
}

// assertNoRunWritten fails the test when slug has a seed or a status file under location.
func assertNoRunWritten(t *testing.T, location *lymcwd.Location, slug string) {
	t.Helper()
	if _, found, err := shedrun.ReadSeed(location, slug); err != nil || found {
		t.Errorf("ReadSeed(%q) = (found %v, err %v); want no seed", slug, found, err)
	}
	if pathExists(shedrun.StatusFile(location, slug)) {
		t.Errorf("a status file exists for %q; want none", slug)
	}
}
