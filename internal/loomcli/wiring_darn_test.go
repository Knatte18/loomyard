// wiring_darn_test.go covers the wiring a darn run adds to wire: the darn seams, the verify source and attempts, the early refusal of an empty verify command, the routing per recipe, and the Darn row's status and failure-record seams.
// Every test is Tier 1: wire resolves no cwd and spawns no process over a hand-built hub.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// seedDarnHubConfig writes the hub-wide darn.yaml into hub's board directory with verify and verifyAttempts substituted into the template.
func seedDarnHubConfig(t *testing.T, hub, verify, verifyAttempts string) {
	t.Helper()
	configDir := filepath.Join(fabricengine.BoardDir(hub), "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	contents := strings.Replace(darnengine.ConfigTemplate(), `verify: ""`, "verify: "+verify, 1)
	contents = strings.Replace(contents, "verify_attempts: 3", "verify_attempts: "+verifyAttempts, 1)
	if err := os.WriteFile(filepath.Join(configDir, "darn.yaml"), []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile darn.yaml = %v; want nil", err)
	}
}

func TestWire_DarnRecipe(t *testing.T) {
	t.Parallel()

	darnSeams := []string{"DarnSpec", "Darn.ReadRejection", "Darn.ClearRejection", "Darn.Commit", "Darn.LatestOutcome", "Darn.PublishFailure"}

	t.Run("a darn seed fills the darn seams, the verify source and the attempts", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		seedDarnHubConfig(t, loc.HubPath, `"go test ./..."`, "5")
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}

		nils := envkit.NilSeams(c.env)
		for _, seam := range darnSeams {
			for _, nilSeam := range nils {
				if nilSeam == seam {
					t.Errorf("c.env.%s is nil after wire() of a darn seed", seam)
				}
			}
		}
		if c.env.VerifyMergeBase == nil {
			t.Error("c.env.VerifyMergeBase is nil after wire() of a darn seed; want the merge base reader")
		}
		if c.env.DarnVerifyAttempts != 5 {
			t.Errorf("c.env.DarnVerifyAttempts = %d; want 5", c.env.DarnVerifyAttempts)
		}
		if command, err := c.env.VerifyCommand(); err != nil || command != "go test ./..." {
			t.Errorf("c.env.VerifyCommand() = %q, %v; want %q, nil", command, err, "go test ./...")
		}
		if got := c.verifySource().failedWayForward; !strings.Contains(got, `"lyx loom goto --to Darn"`) {
			t.Errorf("verifySource().failedWayForward = %q; want it to route back to the Darn row", got)
		}
	})

	t.Run("a loom seed leaves the darn seams nil", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeLoom}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.env.DarnSpec != nil || c.env.Darn.Commit != nil || c.env.VerifyMergeBase != nil || c.env.DarnVerifyAttempts != 0 {
			t.Errorf("a loom seed filled a darn seam: DarnSpec nil = %v, Darn.Commit nil = %v, VerifyMergeBase nil = %v, DarnVerifyAttempts = %d", c.env.DarnSpec == nil, c.env.Darn.Commit == nil, c.env.VerifyMergeBase == nil, c.env.DarnVerifyAttempts)
		}
		if got := c.verifySource().failedWayForward; !strings.Contains(got, `"lyx loom goto --to Webster-Burler"`) {
			t.Errorf("verifySource().failedWayForward = %q; want it to route back to the Webster-Burler row", got)
		}
	})

	t.Run("an empty verify command is refused early with its way forward", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		seedDarnHubConfig(t, loc.HubPath, `""`, "3")
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
		err := c.wire(loc, loc.AnchorPath())
		if err == nil {
			t.Fatal("wire() of a darn seed with an empty verify = nil; want a refusal")
		}
		for _, want := range []string{`"verify" is empty`, `lyx config darn --set verify=<command>`, `lyx loom resume`} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("wire() error = %q; want it to contain %q", err.Error(), want)
			}
		}
	})

	t.Run("an absent darn.yaml names the hub config way forward", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
		err := c.wire(loc, loc.AnchorPath())
		if err == nil || !strings.HasSuffix(err.Error(), hubConfigWayForward) {
			t.Errorf("wire() error = %v; want it to end with %q", err, hubConfigWayForward)
		}
	})
}

func TestRecipeRouting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		recipe  string
		wantRow string
		absent  string
	}{
		{shedrun.RecipeLoom, loomshed.NamePRRework, loomshed.NameDarn},
		{"", loomshed.NamePRRework, loomshed.NameDarn},
		{shedrun.RecipeDarn, loomshed.NameDarn, loomshed.NamePRRework},
	}
	for _, tt := range tests {
		t.Run("recipe "+tt.recipe, func(t *testing.T) {
			t.Parallel()
			c := &loomCLI{recipe: tt.recipe}
			c.cfg.ReviewMaxBounces = 3
			routing, err := c.recipeRouting()
			if err != nil {
				t.Fatalf("recipeRouting() = %v; want nil", err)
			}
			rows := map[string]bool{}
			for _, p := range routing.Producers {
				rows[p.Name] = true
			}
			if !rows[tt.wantRow] || rows[tt.absent] {
				t.Errorf("recipeRouting() rows = %v; want %q and not %q", rows, tt.wantRow, tt.absent)
			}
		})
	}
}

func TestLatestDarnOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		status    *shedengine.Status
		wantFound bool
		want      loomshed.DarnOutcome
	}{
		{name: "absent status file"},
		{name: "no Darn entry", status: &shedengine.Status{History: []shedengine.HistoryEntry{{Producer: loomshed.NamePreflight, Outcome: shedengine.Done}}}},
		{
			name:      "latest Darn entry of a run that moved on carries no reason",
			status:    &shedengine.Status{CurrentProducer: loomshed.NamePublish, State: shedengine.StateRunning, Error: "stale", History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarn, Outcome: shedengine.Stuck}, {Producer: loomshed.NameDarn, Outcome: shedengine.Done}}},
			wantFound: true,
			want:      loomshed.DarnOutcome{Outcome: shedengine.Done},
		},
		{
			name:      "halted at Darn carries the status error",
			status:    &shedengine.Status{CurrentProducer: loomshed.NameDarn, State: shedengine.StateBlocked, Error: "verify gate failed", History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarn, Outcome: shedengine.Stuck}}},
			wantFound: true,
			want:      loomshed.DarnOutcome{Outcome: shedengine.Stuck, Reason: "verify gate failed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			statusPath, lockPath := filepath.Join(dir, "status.json"), filepath.Join(dir, "status.json.lock")
			if tt.status != nil {
				tt.status.State = firstNonEmptyState(tt.status.State)
				if err := state.WriteJSON(statusPath, lockPath, *tt.status); err != nil {
					t.Fatalf("WriteJSON() = %v; want nil", err)
				}
			}
			got, found, err := latestDarnOutcome(statusPath, lockPath)
			if err != nil {
				t.Fatalf("latestDarnOutcome() error = %v; want nil", err)
			}
			if found != tt.wantFound || got != tt.want {
				t.Errorf("latestDarnOutcome() = %+v, %v; want %+v, %v", got, found, tt.want, tt.wantFound)
			}
		})
	}
}

// firstNonEmptyState returns st, or the running state when st is empty, since a status file with no state does not decode.
func firstNonEmptyState(st shedengine.State) shedengine.State {
	if st == "" {
		return shedengine.StateRunning
	}
	return st
}

func TestDarnDeps_PublishFailureNamesThePresentRecord(t *testing.T) {
	t.Parallel()

	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
	verifyDir := t.TempDir()
	deps := darnDeps(loc, shedrun.SelfRunID, verifyDir)

	if path, found, err := deps.PublishFailure(); err != nil || found || path != "" {
		t.Fatalf("PublishFailure() with no record = %q, %v, %v; want empty, false, nil", path, found, err)
	}

	paths := verifytree.NewPaths(loc.WorktreePath(), verifyDir)
	if err := verifytree.WritePublishFailure(paths, verifytree.PublishFailure{Kind: verifytree.FailureKindPlanVerify}); err != nil {
		t.Fatalf("WritePublishFailure() = %v; want nil", err)
	}
	if path, found, err := deps.PublishFailure(); err != nil || !found || path != paths.PublishFailure {
		t.Errorf("PublishFailure() with a record = %q, %v, %v; want %q, true, nil", path, found, err, paths.PublishFailure)
	}
}

func TestDarnDeps_RejectionSeamsReadAndClearTheRecord(t *testing.T) {
	t.Parallel()

	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
	deps := darnDeps(loc, shedrun.SelfRunID, t.TempDir())

	if _, found, err := deps.ReadRejection(); err != nil || found {
		t.Fatalf("ReadRejection() with no record = found %v, %v; want false, nil", found, err)
	}
	if err := deps.ClearRejection(); err != nil {
		t.Errorf("ClearRejection() with no record = %v; want nil", err)
	}
}
