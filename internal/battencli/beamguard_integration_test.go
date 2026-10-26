//go:build tier1

// beamguard_integration_test.go drives the prime-or-beam guard over a hubforge hub with a marked beam: the guard's verdicts, the batten verbs run through a beam's anchor, and the self-pair refusals.

package battencli

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// beamHub returns a hub with a marked beam "b", a member "m" forked from it, and a pair "p" forked from the prime, with the prime's worktree name.
func beamHub(t *testing.T) (h *hubforge.Hub, primeName string) {
	t.Helper()
	h = hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	hubforge.AddPairWith(t, h, "m", fabricengine.AddOptions{SkipPush: true, Parent: "b"})
	hubforge.AddPairWith(t, h, "p", fabricengine.AddOptions{SkipPush: true})
	primeName, err := fabricengine.PrimeName(h.Location)
	if err != nil {
		t.Fatalf("prime name: %v", err)
	}
	return h, primeName
}

// resolvePair resolves the code worktree of slug in h.
func resolvePair(t *testing.T, h *hubforge.Hub, slug string) *lyxcwd.Location {
	t.Helper()
	location, err := lyxcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("resolve the worktree of %q: %v", slug, err)
	}
	return location
}

func TestResolveBattenPrime(t *testing.T) {
	t.Parallel()

	boardkit.InstallReplay(t)
	h, primeName := beamHub(t)
	recordsSibling, err := lyxcwd.Resolve(h.PrimeRecords())
	if err != nil {
		t.Fatalf("resolve the prime's records sibling: %v", err)
	}

	tests := []struct {
		name     string
		location *lyxcwd.Location
		// wantErr is empty for an admitted worktree, else every substring the refusal names.
		wantErr []string
	}{
		{"PrimeReturnsItself", h.Location, nil},
		{"MarkedBeamReturnsThePrime", resolvePair(t, h, "b"), nil},
		{"PairForkedFromThePrimeNamesThePrime", resolvePair(t, h, "p"), []string{`"p"`, `"` + primeName + `"`}},
		{"PairForkedFromTheBeamNamesTheBeam", resolvePair(t, h, "m"), []string{`"m"`, `"b"`}},
		{"PrimesRecordsSiblingIsStillRefused", recordsSibling, []string{"records sibling", "prime worktree or a beam"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveBattenPrime(tt.location)
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("ResolveBattenPrime(%s) = %v; want it admitted", tt.location.WorktreeName, err)
				}
				if got.WorktreeName != primeName {
					t.Errorf("ResolveBattenPrime(%s) returned %q; want the prime %q", tt.location.WorktreeName, got.WorktreeName, primeName)
				}
				return
			}
			if err == nil {
				t.Fatalf("ResolveBattenPrime(%s) = nil; want a refusal", tt.location.WorktreeName)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("ResolveBattenPrime(%s) error = %q; want it to name %s", tt.location.WorktreeName, err.Error(), want)
				}
			}
		})
	}

	t.Run("BeamHeldByItsLandingNamesTheLandingAndItsParent", func(t *testing.T) {
		seedBoardTask(t, h, "b", "loom")
		board, err := boardengine.OpenHub(h.Path)
		if err != nil {
			t.Fatalf("open the hub board: %v", err)
		}
		claim := "running · Land"
		if err := board.SetStatus("b", &claim); err != nil {
			t.Fatalf("claim the beam's entry: %v", err)
		}

		_, err = ResolveBattenPrime(resolvePair(t, h, "b"))

		if err == nil {
			t.Fatal("ResolveBattenPrime over a beam held by its landing = nil; want a refusal")
		}
		for _, want := range []string{"being landed", claim, `"` + primeName + `"`} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("refusal = %q; want it to name %s", err.Error(), want)
			}
		}
	})

	t.Run("BeamAtBeamAwaitIsAdmittedAndFromParentAlignItsLandingHoldsIt", func(t *testing.T) {
		board, err := boardengine.OpenHub(h.Path)
		if err != nil {
			t.Fatalf("open the hub board: %v", err)
		}
		if err := board.SetStatus("b", nil); err != nil {
			t.Fatalf("release the beam's entry: %v", err)
		}
		if _, err := board.UpsertTask(map[string]any{"slug": "b", "kind": "project", "recipe": boardengine.ProjectRecipe}); err != nil {
			t.Fatalf("make the beam's entry a project: %v", err)
		}
		// Only the owner setter makes a project a beam.
		if _, err := boardengine.SetOwners(h.BoardDir(), boardengine.OwnerRequest{Machine: "box", Slugs: []string{"b"}, Beam: true}); err != nil {
			t.Fatalf("make the beam's project a beam: %v", err)
		}
		awaiting := boardengine.RunStatus("running", "Beam-Await")
		if err := board.SetStatus("b", &awaiting); err != nil {
			t.Fatalf("stand the beam's run at Beam-Await: %v", err)
		}

		got, err := ResolveBattenPrime(resolvePair(t, h, "b"))
		if err != nil || got.WorktreeName != primeName {
			t.Fatalf("ResolveBattenPrime at Beam-Await = %v, %v; want the prime %q admitted", got, err, primeName)
		}

		aligning := boardengine.RunStatus("running", "Parent-Align")
		if err := board.SetStatus("b", &aligning); err != nil {
			t.Fatalf("stand the beam's run at Parent-Align: %v", err)
		}
		_, err = ResolveBattenPrime(resolvePair(t, h, "b"))
		if err == nil || !strings.Contains(err.Error(), "being landed") || !strings.Contains(err.Error(), aligning) {
			t.Errorf("ResolveBattenPrime at Parent-Align = %v; want the landing refusal naming %q", err, aligning)
		}
	})
}

func TestVerbs_RunFromABeam(t *testing.T) {
	t.Parallel()

	boardkit.InstallReplay(t)
	h, primeName := beamHub(t)
	const runSlug = "prime-run"
	const doneSlug = "finished-task"
	seedCancelRun(t, h, runSlug, battenrecipe.NameWorktreeCreate, shedengine.StateBlocked, "running · Worktree-Create")
	writeCleanupFile(t, shedrun.StatusFile(h.Location, doneSlug), statusJSON(t, "done"))
	writeCleanupFile(t, shedrun.SeedFile(h.Location, doneSlug), []byte("{}"))
	rels := []string{shedrun.SeedRel(h.Location, doneSlug), shedrun.StatusRel(h.Location, doneSlug)}
	if _, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), h.Location, rels, "test: finished run", fabricengine.EnvSyncOptions()); err != nil || !committed {
		t.Fatalf("commit the finished run: committed=%v err=%v", committed, err)
	}
	beamCwd, memberCwd := h.PairCodeWorktree("b"), h.PairCodeWorktree("m")

	run := func(cwd string, args ...string) string {
		t.Helper()
		var out bytes.Buffer
		RunCLIIn(cwd, &out, args)
		return out.String()
	}

	t.Run("StatusReadsTheRunSeededAtThePrime", func(t *testing.T) {
		got := envelope.RequireOK(t, run(beamCwd, "status", runSlug))
		if got.Raw["state"] != string(shedengine.StateBlocked) || got.Raw["current_producer"] != battenrecipe.NameWorktreeCreate {
			t.Errorf("status from the beam = %v; want the prime's blocked run at Worktree-Create", got.Raw)
		}
	})

	t.Run("ArmingResolvesTheReceiverToThePrime", func(t *testing.T) {
		c := &battenCLI{}
		if _, err := c.armAt(resolvePair(t, h, "b"), runSlug, true, "status"); err != nil {
			t.Fatalf("armAt(status) from the beam = %v; want it admitted", err)
		}
		if c.location.WorktreeName != primeName {
			t.Errorf("receiver location = %q; want the prime %q", c.location.WorktreeName, primeName)
		}
		wantStatus := shedrun.StatusFile(h.Location, runSlug)
		if c.shedPaths.StatusPath != wantStatus {
			t.Errorf("status path = %q; want it under the prime's anchor at %q", c.shedPaths.StatusPath, wantStatus)
		}
		if wantLock := shedrun.StatusLock(h.Location, runSlug); c.shedPaths.StatusLockPath != wantLock {
			t.Errorf("status lock path = %q; want it under the prime's anchor at %q", c.shedPaths.StatusLockPath, wantLock)
		}
	})

	t.Run("CancelOfAnUnknownSlugReachesTheCancelsOwnRefusal", func(t *testing.T) {
		envelope.RequireErr(t, run(beamCwd, "cancel", "ghost"), "nothing of ghost is left to cancel")
	})

	t.Run("CleanupListsThePrimesRuns", func(t *testing.T) {
		got := envelope.RequireOK(t, run(beamCwd, "cleanup"))
		if removed := stringsOf(t, got.Raw["removed"]); !slices.Equal(removed, []string{doneSlug}) {
			t.Errorf("cleanup from the beam removed = %v; want the prime's done run [%s]", removed, doneSlug)
		}
	})

	t.Run("TheSameVerbsFromAPairForkedFromThePrimeNameThePrime", func(t *testing.T) {
		pairCwd := h.PairCodeWorktree("p")
		for _, args := range [][]string{{"status", runSlug}, {"cancel", "ghost"}, {"cleanup"}, {"lock", "list"}} {
			envelope.RequireErr(t, run(pairCwd, args...), `"`+primeName+`"`)
		}
	})

	t.Run("TheSameVerbsFromAPairForkedFromTheBeamNameTheBeam", func(t *testing.T) {
		envelope.RequireErr(t, run(memberCwd, "status", runSlug), `"b"`)
	})
}

func TestCancelCmd_RemoveRefusesFromInsideTheBeam(t *testing.T) {
	t.Parallel()

	h, primeName := beamHub(t)

	var out bytes.Buffer
	RunCLIIn(h.PairCodeWorktree("b"), &out, []string{"cancel", "b", "--remove"})

	envelope.RequireErr(t, out.String(), `"`+primeName+`"`)
}
