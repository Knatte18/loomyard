//go:build tier0

// wiring_darn_test.go covers the wiring a darn run adds to wire: the darn seams, the verify source and attempts, the perch's settings taken from loom.yaml's Code-Review keys, the board entry snapshot its discussion commit writes, the early refusals of a malformed or absent darn.yaml, the routing per recipe, the rename of retired darn rows when a verb arms, and the Darn-Write row's status and failure-record seams.
// Every test is Tier 1: wire resolves no cwd and spawns no process over a hand-built hub.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
)

// wantDarnLandingCall is the landing call gateConfigWithTwoTiers renders for the `finalize` key.
const wantDarnLandingCall = "lym gate test --wait --gate finalize"

//lym:seam internal/battenshed#InnerRunDeps
//lym:seam internal/battenshed#LandDeps
//lym:seam internal/battenshed#PrimeLock
//lym:seam internal/battenshed#RowNoticeDeps
//lym:seam internal/battenshed#SeedChildDeps
//lym:seam internal/battenshed#TeardownDeps
//lym:seam internal/configengine#Dirs
//lym:seam internal/driftengine#Deps
//lym:seam internal/driftengine#Paths
//lym:seam internal/landingshed#BeamDescribeDeps
//lym:seam internal/landingshed#Config
//lym:seam internal/landingshed#Deps
//lym:seam internal/landingshed#ParentLockDeps
//lym:seam internal/loomshed#DarnDeps
//lym:seam internal/loomshed#PRReworkDeps
//lym:seam internal/pawlengine#Deps
//lym:seam internal/pawlengine#Paths
//lym:seam internal/shedadapters#CommandDeps
//lym:seam internal/shedrecipe#Env
//lym:seam internal/shuttleengine#Config
//lym:seam internal/websterengine#Config
//lym:seam internal/websterengine#Geometry
//lym:seam internal/websterengine#RunDeps
func TestWire_DarnRecipe(t *testing.T) {
	t.Parallel()

	darnSeams := []string{"DarnTable", "Darn.Commit", "Darn.LatestOutcome"}

	t.Run("a darn seed fills the darn seams, the verify source and the attempts", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		seedDarnHubConfig(t, loc.HubPath, "5")
		writeHubGateConfig(t, loc.HubPath, gateConfigWithTwoTiers())
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
		// envkit.NilSeams skips ReviewInput through nilLegal, so a direct check pins that a darn seed keeps the shared wiring's seam, which answers not found since darn has no Parent-Align report.
		if c.env.ReviewInput == nil {
			t.Fatal("ReviewInput is nil after wire() of a darn seed; want it told")
		}
		if path, _, found, err := c.env.ReviewInput(); err != nil || found {
			t.Errorf("ReviewInput() = %q, %v, %v; want not found, nil", path, found, err)
		}
		if c.env.VerifyMergeBase == nil {
			t.Error("c.env.VerifyMergeBase is nil after wire() of a darn seed; want the merge base reader")
		}
		if c.env.DiscussionHistory == nil {
			t.Error("c.env.DiscussionHistory is nil after wire() of a darn seed; want the history seam")
		}
		if c.env.VerifyTreeScope == nil {
			t.Error("c.env.VerifyTreeScope is nil after wire() of a darn seed; want the tree scope reader")
		}
		if c.env.DarnVerifyAttempts != 5 {
			t.Errorf("c.env.DarnVerifyAttempts = %d; want 5", c.env.DarnVerifyAttempts)
		}
		if got, want := c.env.GateFindingsFile("Darn-Write"), shedrun.GateFindingsFile(loc, c.runID, "Darn-Write"); got != want {
			t.Errorf("c.env.GateFindingsFile(\"Darn-Write\") = %q; want %q", got, want)
		}
		if segment := c.env.Segments["Code-Review"]; segment.MaxBounces != 1 || segment.ConfigFile != "loom.yaml" {
			t.Errorf("Segments[Code-Review] = %+v; want loom.yaml's template budget 1", segment)
		}
		if _, ok := c.env.Segments["Darn-Review"]; ok {
			t.Error("a darn seed filled a Darn-Review segment; want the perch to be loom's Code-Review")
		}
		if command, err := c.env.VerifyCommand("finalize"); err != nil || command != wantDarnLandingCall {
			t.Errorf("c.env.VerifyCommand(finalize) = %q, %v; want the landing call %q, nil", command, err, wantDarnLandingCall)
		}
	})

	t.Run("the discussion commit seam snapshots the board entry before it commits", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		seedDarnHubConfig(t, loc.HubPath, "3")
		writeHubGateConfig(t, loc.HubPath, gateConfigWithTwoTiers())
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		// The hub holds no board entry, so only the snapshot-writing seam refuses, and it names the snapshot.
		if err := c.env.CommitDiscussion(); err == nil || !strings.Contains(err.Error(), "snapshot") {
			t.Errorf("c.env.CommitDiscussion() = %v; want the board entry snapshot's refusal", err)
		}
	})

	t.Run("with a tree listed the verify gate's told command stays the row key's whole call", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		seedDarnHubConfig(t, loc.HubPath, "3", "tree_scope: {}", `tree_scope: {"docs": ""}`)
		writeHubGateConfig(t, loc.HubPath, gateConfigWithTwoTiers())
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if command, err := c.env.VerifyCommand("finalize"); err != nil || command != wantDarnLandingCall {
			t.Errorf("c.env.VerifyCommand(finalize) = %q, %v; want the landing call %q, nil", command, err, wantDarnLandingCall)
		}
	})

	t.Run("a local loom override sets a darn run's perch budget and hard cap", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		seedDarnHubConfig(t, loc.HubPath, "3")
		localPath := configengine.LocalFile(loc.AnchorPath(), "loom")
		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(localPath), err)
		}
		if err := os.WriteFile(localPath, []byte("code_max_bounces: 2\ncode_hard_max_bounces: 3\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", localPath, err)
		}
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		segment := c.env.Segments["Code-Review"]
		if segment.MaxBounces != 2 || segment.MaxBouncesKey != "code_max_bounces" || segment.HardMaxBounces != 3 || segment.HardMaxBouncesKey != "code_hard_max_bounces" || segment.ConfigFile != "loom.yaml" {
			t.Errorf("Segments[Code-Review] = %+v; want the local budget 2 and hard cap 3 under their code keys in loom.yaml", segment)
		}
	})

	t.Run("a loom seed leaves the darn seams nil", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeLoom}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.env.DiscussionHistory != nil {
			t.Error("a loom seed filled c.env.DiscussionHistory; want nil")
		}
		if c.env.DarnTable != nil || c.env.Darn.Commit != nil || c.env.VerifyMergeBase != nil || c.env.VerifyTreeScope != nil || c.env.DarnVerifyAttempts != 0 {
			t.Errorf("a loom seed filled a darn seam: DarnTable nil = %v, Darn.Commit nil = %v, VerifyMergeBase nil = %v, VerifyTreeScope nil = %v, DarnVerifyAttempts = %d", c.env.DarnTable == nil, c.env.Darn.Commit == nil, c.env.VerifyMergeBase == nil, c.env.VerifyTreeScope == nil, c.env.DarnVerifyAttempts)
		}
	})

	t.Run("a loom seed over a recorded parent branch gets the merge base reader in the round form", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		recordPath := fabricengine.OriginRecordPath(loc)
		if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(recordPath), err)
		}
		if err := os.WriteFile(recordPath, []byte("{}"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", recordPath, err)
		}
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeLoom}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.env.VerifyMergeBase == nil || c.env.VerifyTreeScope != nil {
			t.Errorf("a loom seed over an origin record: VerifyMergeBase nil = %v, VerifyTreeScope nil = %v; want the reader set and the round form", c.env.VerifyMergeBase == nil, c.env.VerifyTreeScope == nil)
		}
	})

	t.Run("a malformed tree key names the editor way forward in place of the hub config one", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		seedDarnHubConfig(t, loc.HubPath, "3", "tree_scope: {}", `tree_scope: {"./tools": ""}`)
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
		err := c.wire(loc, loc.AnchorPath())
		if err == nil {
			t.Fatal("wire() of a darn seed with a malformed tree key = nil; want a refusal")
		}
		for _, want := range []string{"./tools", "lym config darn's editor", "then re-run the lym command that refused"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("wire() error = %q; want it to contain %q", err.Error(), want)
			}
		}
		if strings.Contains(err.Error(), hubConfigWayForward) {
			t.Errorf("wire() error = %q; want no hub config way forward, which cannot clear the key", err.Error())
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

// TestArmAt_RenamesRetiredDarnRows pins that arming any verb over a darn run carries a status file still naming the writer row's former name onto Darn-Write, and leaves a loom run's file alone.
// A live driver holding the run lock keeps the old name, and the verb still arms.
func TestArmAt_RenamesRetiredDarnRows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		recipe      string
		holdRunLock bool
		want        string
	}{
		{name: "darn", recipe: shedrun.RecipeDarn, want: loomshed.NameDarnWrite},
		{name: "loom", recipe: shedrun.RecipeLoom, want: "Darn"},
		{name: "darn with a live driver", recipe: shedrun.RecipeDarn, holdRunLock: true, want: "Darn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := hubLocation(t, "pair", ".")
			if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, shedrun.Seed{Recipe: tt.recipe, Driver: shedrun.DriverGo}); err != nil {
				t.Fatalf("shedrun.WriteSeed() = %v; want nil", err)
			}
			statusPath, statusLock := shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID)
			if err := os.MkdirAll(filepath.Dir(statusLock), 0o755); err != nil {
				t.Fatal(err)
			}
			seed := shedengine.Status{CurrentProducer: "Darn", State: shedengine.StateBlocked, History: []shedengine.HistoryEntry{{Producer: "Darn", Outcome: shedengine.Stuck}}}
			if err := state.WriteJSON(statusPath, statusLock, seed); err != nil {
				t.Fatalf("state.WriteJSON() = %v; want nil", err)
			}
			if tt.holdRunLock {
				runLock := shedrun.RunLock(loc, shedrun.SelfRunID)
				held, locked, err := lock.TryAcquireWriteLock(runLock)
				if err != nil || !locked {
					t.Fatalf("TryAcquireWriteLock(%q) = _, %v, %v; want held", runLock, locked, err)
				}
				t.Cleanup(func() { _ = held.Release() })
			}

			c := &loomCLI{}
			if _, err := c.armAt(loc, "status", nil); err != nil {
				t.Fatalf("armAt(status) = %v; want nil", err)
			}
			got, _, err := state.ReadJSON[shedengine.Status](statusPath, statusLock)
			if err != nil {
				t.Fatalf("read status = %v; want nil", err)
			}
			if got.CurrentProducer != tt.want || got.History[0].Producer != tt.want {
				t.Errorf("CurrentProducer, History[0].Producer = %q, %q; want both %q", got.CurrentProducer, got.History[0].Producer, tt.want)
			}
		})
	}
}

// TestRenameRetiredRows pins that renameRetiredRows carries every field of a status file naming a retired row onto its successor, for the loom table and for the darn one, and leaves other rows alone.
func TestRenameRetiredRows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		renames map[string]string
		retired string
		want    string
	}{
		{name: "a loom run parked on Merge-In", renames: loomrecipe.LoomRetiredRows(), retired: "Merge-In", want: loomshed.NameParentAlign},
		{name: "a darn run parked on Darn", renames: loomrecipe.DarnRetiredRows(), retired: "Darn", want: loomshed.NameDarnWrite},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := hubLocation(t, "pair", ".")
			statusPath, statusLock := shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID)
			if err := os.MkdirAll(filepath.Dir(statusLock), 0o755); err != nil {
				t.Fatal(err)
			}
			seed := shedengine.Status{
				CurrentProducer: tt.retired, EnteredRow: tt.retired, PauseBefore: tt.retired, PauseAfter: tt.retired,
				State:   shedengine.StateBlocked,
				History: []shedengine.HistoryEntry{{Producer: loomshed.NameWebster, Outcome: shedengine.Done}, {Producer: tt.retired, Outcome: shedengine.Stuck}},
			}
			if err := state.WriteJSON(statusPath, statusLock, seed); err != nil {
				t.Fatalf("state.WriteJSON() = %v; want nil", err)
			}

			c := &loomCLI{runID: shedrun.SelfRunID, shedPaths: shedbuild.ShedPaths{
				StatusPath: statusPath, StatusLockPath: statusLock, LockPath: shedrun.RunLock(loc, shedrun.SelfRunID),
			}}
			if err := c.renameRetiredRows(tt.renames); err != nil {
				t.Fatalf("renameRetiredRows() = %v; want nil", err)
			}

			got, _, err := state.ReadJSON[shedengine.Status](statusPath, statusLock)
			if err != nil {
				t.Fatalf("read status = %v; want nil", err)
			}
			fields := []string{got.CurrentProducer, got.EnteredRow, got.PauseBefore, got.PauseAfter, got.History[1].Producer}
			for i, field := range fields {
				if field != tt.want {
					t.Errorf("renamed field %d = %q; want %q", i, field, tt.want)
				}
			}
			if got.History[0].Producer != loomshed.NameWebster {
				t.Errorf("History[0].Producer = %q; want %q, untouched", got.History[0].Producer, loomshed.NameWebster)
			}
		})
	}
}

func TestRecipeRouting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		recipe  string
		wantRow string
		absent  string
	}{
		{shedrun.RecipeLoom, loomshed.NameParentAlign, loomshed.NameDarnWrite},
		{"", loomshed.NameParentAlign, loomshed.NameDarnWrite},
		{shedrun.RecipeDarn, loomshed.NameDarnWrite, loomshed.NameParentAlign},
		{shedrun.RecipePawl, "Beam-Describe", loomshed.NameDarnWrite},
	}
	for _, tt := range tests {
		t.Run("recipe "+tt.recipe, func(t *testing.T) {
			t.Parallel()
			c := &loomCLI{recipe: tt.recipe}
			c.cfg.ReviewMaxBounces, c.cfg.ReviewHardMaxBounces = 3, 4
			routing, err := c.recipeRouting()
			if err != nil {
				t.Fatalf("recipeRouting() = %v; want nil", err)
			}
			rows := map[string]bool{}
			for _, p := range routing.Producers {
				rows[p.Name] = true
				if tt.recipe == shedrun.RecipePawl && p.Name == loomshed.NameCodeBouncer && p.MaxBounces != 3 {
					t.Errorf("a pawl run's Code-Bouncer MaxBounces = %d; want the Code-Review budget 3", p.MaxBounces)
				}
			}
			if !rows[tt.wantRow] || rows[tt.absent] {
				t.Errorf("recipeRouting() rows = %v; want %q and not %q", rows, tt.wantRow, tt.absent)
			}
		})
	}
}

// TestWriteBoardEntrySnapshot pins that the darn discussion commit's snapshot holds the task's board entry, and that an entry the board lacks is refused with nothing written.
// It sets the board skip variables, which are process-global, so it does not call t.Parallel.
func TestWriteBoardEntrySnapshot(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	t.Setenv("BOARD_SKIP_PUSH", "1")

	t.Run("the task's entry is written", func(t *testing.T) {
		location := &lymcwd.Location{HubPath: boardkit.HubWithBoardConfig(t), WorktreeName: "one-perch", AnchorRel: "."}
		board, err := boardengine.OpenHub(location.HubPath)
		if err != nil {
			t.Fatalf("OpenHub() error = %v; want nil", err)
		}
		if _, err := board.UpsertTask(map[string]any{"slug": "one-perch", "title": "One perch", "kind": "task", "labels": []string{"bug"}, "brief": "Merge the perches.", "body": "## Idea\n\nOne perch."}); err != nil {
			t.Fatalf("UpsertTask() error = %v; want nil", err)
		}

		if err := writeBoardEntrySnapshot(location); err != nil {
			t.Fatalf("writeBoardEntrySnapshot() = %v; want nil", err)
		}
		got, err := os.ReadFile(loomengine.DiscussionBoardEntry(location))
		if err != nil {
			t.Fatalf("read the snapshot: %v", err)
		}
		if want := darnengine.BoardEntrySnapshot("one-perch", "One perch", "Merge the perches.", "## Idea\n\nOne perch."); string(got) != string(want) {
			t.Errorf("snapshot = %q; want %q", got, want)
		}
	})

	t.Run("a pawl run's beam project entry is written", func(t *testing.T) {
		location := &lymcwd.Location{HubPath: boardkit.HubWithBoardConfig(t), WorktreeName: "a-beam", AnchorRel: "."}
		// A board write cannot make a beam, so the entry is written to the board file directly.
		board := `{"version":1,"entries":[{"id":0,"slug":"a-beam","title":"A beam","kind":"project","labels":["bug"],"beam":true,"brief":"Land the members.","body":"## Idea\n\nOne beam."}]}`
		if err := os.WriteFile(filepath.Join(fabricengine.BoardDir(location.HubPath), "board.json"), []byte(board), 0o644); err != nil {
			t.Fatalf("write board.json: %v", err)
		}

		if err := writeBoardEntrySnapshot(location); err != nil {
			t.Fatalf("writeBoardEntrySnapshot() = %v; want nil", err)
		}
		got, err := os.ReadFile(loomengine.DiscussionBoardEntry(location))
		if err != nil {
			t.Fatalf("read the snapshot: %v", err)
		}
		if want := darnengine.BoardEntrySnapshot("a-beam", "A beam", "Land the members.", "## Idea\n\nOne beam."); string(got) != string(want) {
			t.Errorf("snapshot = %q; want %q", got, want)
		}
	})

	t.Run("an entry the board lacks is refused with the way forward", func(t *testing.T) {
		location := &lymcwd.Location{HubPath: boardkit.HubWithBoardConfig(t), WorktreeName: "no-entry", AnchorRel: "."}
		err := writeBoardEntrySnapshot(location)
		if err == nil || !strings.Contains(err.Error(), `"no-entry"`) || !strings.Contains(err.Error(), "lym board upsert") {
			t.Errorf("writeBoardEntrySnapshot() = %v; want a refusal naming the slug and lym board upsert", err)
		}
		if _, statErr := os.Stat(loomengine.DiscussionBoardEntry(location)); !os.IsNotExist(statErr) {
			t.Errorf("snapshot stat = %v; want no file written", statErr)
		}
	})
}

func TestLatestDarnOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status *shedengine.Status
		// findingsKept writes the row's gate findings file before the read.
		findingsKept bool
		wantFound    bool
		want         loomshed.DarnOutcome
		// wantFindings expects the outcome to name the kept findings file.
		wantFindings bool
		// invalidPath gives the findings path a NUL byte, so stat fails with an invalid-argument error on every platform.
		invalidPath bool
		wantErr     bool
	}{
		{name: "absent status file"},
		{name: "no Darn-Write entry", status: &shedengine.Status{History: []shedengine.HistoryEntry{{Producer: loomshed.NamePreflight, Outcome: shedengine.Done}}}},
		{
			name:      "latest Darn-Write entry of a run that moved on, with no findings kept",
			status:    &shedengine.Status{CurrentProducer: loomshed.NameFinalize, State: shedengine.StateRunning, Error: "stale", History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Stuck}, {Producer: loomshed.NameDarnWrite, Outcome: shedengine.Done}}},
			wantFound: true,
			want:      loomshed.DarnOutcome{Outcome: shedengine.Done},
		},
		{
			name:         "a resumed run, its error cleared, names the kept findings",
			status:       &shedengine.Status{CurrentProducer: loomshed.NameDarnWrite, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Stuck}}},
			findingsKept: true,
			wantFound:    true,
			want:         loomshed.DarnOutcome{Outcome: shedengine.Stuck},
			wantFindings: true,
		},
		{
			name:         "a Darn-Write reroute entry is skipped for the last real entry",
			status:       &shedengine.Status{CurrentProducer: loomshed.NameDarnWrite, State: shedengine.StateBlocked, Error: "verify gate failed", History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Stuck}, {Producer: loomshed.NameDarnWrite, Outcome: shedengine.OutcomeReroute}}},
			findingsKept: true,
			wantFound:    true,
			want:         loomshed.DarnOutcome{Outcome: shedengine.Stuck},
			wantFindings: true,
		},
		{
			name:   "a lone Darn-Write reroute entry reports not found",
			status: &shedengine.Status{CurrentProducer: loomshed.NameParentAlign, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.OutcomeReroute}}},
		},
		{
			name:        "a findings path that cannot be statted is returned as the error",
			status:      &shedengine.Status{CurrentProducer: loomshed.NameDarnWrite, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Stuck}}},
			invalidPath: true,
			wantErr:     true,
		},
		{
			name:   "a lone Darn-Write goto entry reports not found",
			status: &shedengine.Status{CurrentProducer: loomshed.NameDarnWrite, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.OutcomeGoto}}},
		},
		{
			name:   "a lone Darn-Write skipped entry reports not found",
			status: &shedengine.Status{CurrentProducer: loomshed.NameFinalize, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.OutcomeSkipped}}},
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
			findingsPath := filepath.Join(dir, "Darn-Write.findings.md")
			if tt.findingsKept {
				if err := os.WriteFile(findingsPath, []byte("verify failed"), 0o644); err != nil {
					t.Fatalf("write the findings file: %v", err)
				}
			}
			if tt.invalidPath {
				findingsPath = filepath.Join(dir, "Darn-Write\x00.findings.md")
			}
			want := tt.want
			if tt.wantFindings {
				want.FindingsPath = findingsPath
			}
			got, found, err := latestDarnOutcome(statusPath, lockPath, findingsPath)
			if tt.wantErr {
				if err == nil || found || got != (loomshed.DarnOutcome{}) {
					t.Errorf("latestDarnOutcome() = %+v, %v, %v; want the stat error and not found", got, found, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("latestDarnOutcome() error = %v; want nil", err)
			}
			if found != tt.wantFound || got != want {
				t.Errorf("latestDarnOutcome() = %+v, %v; want %+v, %v", got, found, want, tt.wantFound)
			}
		})
	}
}

// TestDarnDeps_LatestOutcomeNamesTheRowsFindingsFile pins that the LatestOutcome seam reads the run's status file and names the findings file Env.GateFindingsFile gives the Darn-Write row.
func TestDarnDeps_LatestOutcomeNamesTheRowsFindingsFile(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "pair", ".")
	statusPath, statusLock := shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(statusLock), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := shedengine.Status{CurrentProducer: loomshed.NameDarnWrite, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Stuck}}}
	if err := state.WriteJSON(statusPath, statusLock, seed); err != nil {
		t.Fatalf("state.WriteJSON() = %v; want nil", err)
	}
	findingsPath := shedrun.GateFindingsFile(loc, shedrun.SelfRunID, loomshed.NameDarnWrite)
	if err := os.MkdirAll(filepath.Dir(findingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(findingsPath, []byte("verify failed"), 0o644); err != nil {
		t.Fatalf("write the findings file: %v", err)
	}

	got, found, err := darnDeps(loc, shedrun.SelfRunID).LatestOutcome()
	want := loomshed.DarnOutcome{Outcome: shedengine.Stuck, FindingsPath: findingsPath}
	if err != nil || !found || got != want {
		t.Errorf("LatestOutcome() = %+v, %v, %v; want %+v, true, nil", got, found, err, want)
	}
}

// firstNonEmptyState returns st, or the running state when st is empty, since a status file with no state does not decode.
func firstNonEmptyState(st shedengine.State) shedengine.State {
	if st == "" {
		return shedengine.StateRunning
	}
	return st
}
