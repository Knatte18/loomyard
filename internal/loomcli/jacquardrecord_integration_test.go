// jacquardrecord_integration_test.go drives loom's verbs over real hub pairs.
// Each pair's run builds from a stored decision record.
// It pins the first start's refusal of an entry that holds none, the decision verbs' Record-Seed path,
// and the Record-Seed Stuck recoveries up to the start that commits a re-seed.
// No behavior here reaches a tmux server: every start that passes its bootstrap refuses at reed Up.

package loomcli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// jacquardVerbTimeout bounds each lym verb these tests run as a subprocess.
const jacquardVerbTimeout = 60 * time.Second

// seedGoRun writes loc's self-run seed for recipe and the go driver, with the parent branch the origin record holds.
// It returns that parent branch.
func seedGoRun(t *testing.T, loc *lymcwd.Location, recipe string) string {
	t.Helper()
	recorded, found, err := fabricengine.ReadOrigin(loc)
	if err != nil || !found {
		t.Fatalf("ReadOrigin before seeding the %s run: found=%v err=%v", recipe, found, err)
	}
	seed := shedrun.Seed{Recipe: recipe, Driver: shedrun.DriverGo, Params: map[string]string{loomshed.SeedParentParam: recorded.ParentBranch}}
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, seed); err != nil {
		t.Fatalf("WriteSeed(%s): %v", recipe, err)
	}
	return recorded.ParentBranch
}

// upsertBoardEntry writes a released task entry for slug on loc's hub board.
// A non-nil record is stored as the entry's decision record, which flags it discussed.
func upsertBoardEntry(t *testing.T, loc *lymcwd.Location, slug string, record []byte) {
	t.Helper()
	boardDir := fabriclayout.BoardDir(loc.HubPath)
	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: boardDir}, "board")
	if err != nil {
		t.Fatalf("load the board config: %v", err)
	}
	cfg.Path = boardDir
	board := boardengine.New(cfg)
	if _, err := board.UpsertTask(map[string]any{"slug": slug, "title": slug, "kind": "task", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("UpsertTask(%s): %v", slug, err)
	}
	if err := board.SetStatus(slug, nil); err != nil {
		t.Fatalf("SetStatus(%s, nil): %v", slug, err)
	}
	if record == nil {
		return
	}
	if err := board.StoreDecisionRecord(slug, record); err != nil {
		t.Fatalf("StoreDecisionRecord(%s): %v", slug, err)
	}
}

// writeRunStatus writes status as loc's self-run status file.
func writeRunStatus(t *testing.T, loc *lymcwd.Location, status shedengine.Status) {
	t.Helper()
	lockPath := shedrun.StatusLock(loc, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("create the status lock directory: %v", err)
	}
	if err := state.WriteJSON(shedrun.StatusFile(loc, shedrun.SelfRunID), lockPath, status); err != nil {
		t.Fatalf("write the run status: %v", err)
	}
}

// plantStuckAtRecordSeed seeds loc's status file for slug and leaves it blocked at Record-Seed.
// The row leaves a run so when its record is missing and its entry holds no stored record.
func plantStuckAtRecordSeed(t *testing.T, loc *lymcwd.Location, slug, parent string) {
	t.Helper()
	statusPath, lockPath := shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID)
	if err := loomshed.Seed(statusPath, lockPath, slug, parent); err != nil {
		t.Fatalf("loomshed.Seed: %v", err)
	}
	err := state.UpdateJSON(statusPath, lockPath, func(st shedengine.Status, _ bool) (shedengine.Status, error) {
		st.State = shedengine.StateBlocked
		st.CurrentProducer = loomshed.NameRecordSeed
		st.Error = "the decision record is missing and the board entry holds no stored decision record, so there is no record to seed"
		return st, nil
	})
	if err != nil {
		t.Fatalf("leave the run blocked at Record-Seed: %v", err)
	}
}

// readRunStatus reads loc's self-run status file, failing the test when it is absent.
func readRunStatus(t *testing.T, loc *lymcwd.Location) shedengine.Status {
	t.Helper()
	status, found, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID))
	if err != nil || !found {
		t.Fatalf("read the run status: found=%v err=%v", found, err)
	}
	return status
}

// runLymVerb runs the built lym at exe in worktree with args, failing the test only when the process cannot be run.
func runLymVerb(t *testing.T, exe, worktree string, args ...string) (string, int) {
	t.Helper()
	out, code, err := runLoomCLIWithEnvNoFatal(exe, worktree, skipWalkPush, jacquardVerbTimeout, args...)
	if err != nil {
		t.Fatalf("lym %s: %v; output: %s", strings.Join(args, " "), err, out)
	}
	return out, code
}

// TestLoomStart_JudgesAStoredRecordOnAFirstStartOnly pins the first start's check of a jacquard run's stored record.
// A jacquard entry with none is refused before any status file is written, naming both ways forward.
// A board that does not read is refused there too, with its own way forward.
// An entry with a stored record, an undiscussed tapestry entry and a start over an existing status file pass the check,
// so each goes on to refuse at reed Up instead.
func TestLoomStart_JudgesAStoredRecordOnAFirstStartOnly(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	exe := lymbin.Build(t)
	slug := badReedUpSlug
	refusal := loomrecipe.NoStoredRecordRefusal(slug)

	t.Run("NoStoredRecordRefusesTheFirstStart", func(t *testing.T) {
		t.Parallel()
		loc, worktree := newBadReedUpFixture(t, func(*testing.T, *lymcwd.Location) {})
		parent := seedGoRun(t, loc, shedrun.RecipeJacquard)

		out, code := runLymVerb(t, exe, worktree, "loom", "start")

		if code != 1 {
			t.Fatalf("lym loom start exit = %d; want 1; output: %s", code, out)
		}
		// The commands are quoted in the message, so they are matched after the envelope's JSON escaping is decoded.
		message := envelope.RequireErr(t, out, refusal).Error
		for _, want := range []string{
			`"lym batten cancel ` + slug + ` --remove"`,
			`"rm '` + shedrun.SeedFile(loc, shedrun.SelfRunID) + `'"`,
			`"lym shed seed self --recipe tapestry --driver go --param parent=` + parent + `"`,
		} {
			if !strings.Contains(message, want) {
				t.Errorf("lym loom start error lacks %q; error: %s", want, message)
			}
		}
		if _, err := os.Stat(shedrun.StatusFile(loc, shedrun.SelfRunID)); !os.IsNotExist(err) {
			t.Errorf("status file after the refused start: stat err %v; want none written", err)
		}
	})

	t.Run("UnreadableBoardRefusesNamingTheWayForward", func(t *testing.T) {
		t.Parallel()
		loc, worktree := newBadReedUpFixture(t, func(t *testing.T, loc *lymcwd.Location) {
			seedGoRun(t, loc, shedrun.RecipeJacquard)
			path := filepath.Join(fabriclayout.BoardDir(loc.HubPath), "board.json")
			if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
				t.Fatalf("write the unparsable board: %v", err)
			}
		})

		out, code := runLymVerb(t, exe, worktree, "loom", "start")

		if code != 1 {
			t.Fatalf("lym loom start exit = %d; want 1; output: %s", code, out)
		}
		// The hub reconcile logs a warning about the same board ahead of the envelope, which is the last line.
		lines := strings.Split(strings.TrimSpace(out), "\n")
		message := envelope.RequireErr(t, lines[len(lines)-1], "loom: read the entry's stored decision record: ").Error
		if want := "; way forward: fix the cause it names, then run the verb again"; !strings.HasSuffix(message, want) {
			t.Errorf("lym loom start error does not end in %q; error: %s", want, message)
		}
		if _, err := os.Stat(shedrun.StatusFile(loc, shedrun.SelfRunID)); !os.IsNotExist(err) {
			t.Errorf("status file after the refused start: stat err %v; want none written", err)
		}
	})

	passes := []struct {
		name  string
		setup func(t *testing.T, loc *lymcwd.Location)
	}{
		{"StoredRecordPassesTheCheck", func(t *testing.T, loc *lymcwd.Location) {
			seedGoRun(t, loc, shedrun.RecipeJacquard)
			upsertBoardEntry(t, loc, slug, loomshed.BoardRecord("The approved goal."))
		}},
		{"UndiscussedTapestryEntryPassesTheCheck", func(t *testing.T, loc *lymcwd.Location) {
			seedGoRun(t, loc, shedrun.RecipeTapestry)
			upsertBoardEntry(t, loc, slug, nil)
		}},
		{"StartOverAnExistingStatusFilePassesTheCheck", func(t *testing.T, loc *lymcwd.Location) {
			seedGoRun(t, loc, shedrun.RecipeJacquard)
			seedAndCommitStatus(t, loc, slug)
		}},
	}
	for _, tt := range passes {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc, worktree := newBadReedUpFixture(t, tt.setup)

			out, code := runLymVerb(t, exe, worktree, "loom", "start")

			if code != 1 {
				t.Fatalf("lym loom start exit = %d; want 1 at reed Up; output: %s", code, out)
			}
			if strings.Contains(out, refusal) {
				t.Errorf("lym loom start refused for a missing stored record; want the check passed; output: %s", out)
			}
			if _, err := os.Stat(shedrun.StatusFile(loc, shedrun.SelfRunID)); err != nil {
				t.Errorf("status file after a start past the check: %v; want it seeded", err)
			}
		})
	}
}

// TestRecordSeedStuck_RecoveriesMoveTheRunOntoDiscussion follows the ways forward of a run Stuck at Record-Seed.
// A tapestry run moves onto Discussion-Write with goto, where its resume runs Discussion.
// A jacquard run in a pair batten did not make is re-seeded as tapestry and moved the same way,
// and its next start commits the new seed while the run stays at Discussion-Write.
// The resume that drives Discussion needs a tmux server, so this stops at the row the resume runs.
func TestRecordSeedStuck_RecoveriesMoveTheRunOntoDiscussion(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	exe := lymbin.Build(t)
	slug := badReedUpSlug

	requireAtDiscussionWrite := func(t *testing.T, loc *lymcwd.Location) {
		t.Helper()
		if got := readRunStatus(t, loc).CurrentProducer; got != loomshed.NameDiscussionWrite {
			t.Errorf("current row = %q; want %q", got, loomshed.NameDiscussionWrite)
		}
	}

	t.Run("Tapestry", func(t *testing.T) {
		t.Parallel()
		loc, worktree := newBadReedUpFixture(t, func(*testing.T, *lymcwd.Location) {})
		parent := seedGoRun(t, loc, shedrun.RecipeTapestry)
		plantStuckAtRecordSeed(t, loc, slug, parent)

		out, code := runLymVerb(t, exe, worktree, "loom", "goto", "--to", loomshed.NameDiscussionWrite)

		if code != 0 {
			t.Fatalf("lym loom goto --to %s exit = %d; want 0; output: %s", loomshed.NameDiscussionWrite, code, out)
		}
		requireAtDiscussionWrite(t, loc)
	})

	t.Run("ReseededJacquard", func(t *testing.T) {
		t.Parallel()
		loc, worktree := newBadReedUpFixture(t, func(*testing.T, *lymcwd.Location) {})
		parent := seedGoRun(t, loc, shedrun.RecipeJacquard)
		plantStuckAtRecordSeed(t, loc, slug, parent)

		if err := os.Remove(shedrun.SeedFile(loc, shedrun.SelfRunID)); err != nil {
			t.Fatalf("remove the jacquard seed: %v", err)
		}
		seedGoRun(t, loc, shedrun.RecipeTapestry)
		out, code := runLymVerb(t, exe, worktree, "loom", "goto", "--to", loomshed.NameDiscussionWrite)
		if code != 0 {
			t.Fatalf("lym loom goto --to %s exit = %d; want 0; output: %s", loomshed.NameDiscussionWrite, code, out)
		}
		out, code = runLymVerb(t, exe, worktree, "loom", "start")

		if code != 1 {
			t.Fatalf("lym loom start exit = %d; want 1 at reed Up; output: %s", code, out)
		}
		committed, found, err := fabricengine.CommittedAnchoredFile(loc, shedrun.SeedRel(loc, shedrun.SelfRunID))
		if err != nil || !found {
			t.Fatalf("read the committed seed: found=%v err=%v", found, err)
		}
		var seed shedrun.Seed
		if err := json.Unmarshal(committed, &seed); err != nil {
			t.Fatalf("decode the committed seed %q: %v", committed, err)
		}
		if seed.Recipe != shedrun.RecipeTapestry {
			t.Errorf("committed seed recipe = %q; want %q", seed.Recipe, shedrun.RecipeTapestry)
		}
		requireAtDiscussionWrite(t, loc)
	})
}

// TestDecisionAdd_JacquardRunTakesTheRecordSeedPath drives `decision add` on a jacquard run before and past Record-Seed.
// Before it, a run with no record is sent to resume; past it, an uncommitted record is sent to the Record-Seed goto.
// It runs without t.Parallel: the strand environment variable is process-global.
func TestDecisionAdd_JacquardRunTakesTheRecordSeedPath(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv(agentname.StrandNameEnv, "")

	tests := []struct {
		name string
		row  string
		// writeRecord leaves a decision record on disk, uncommitted.
		writeRecord bool
		want        []string
	}{
		{name: "BeforeRecordSeed", row: loomshed.NamePreflight, want: []string{"no decision record", "once Record-Seed has run", `run "lym loom resume"`}},
		{name: "PastRecordSeedWithAnUncommittedRecord", row: loomshed.NamePlanWrite, writeRecord: true, want: []string{"not committed at HEAD", `"lym loom goto --to Record-Seed"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := hubforge.CopyHub(t, pairHubShape)
			pair, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(hubforge.TemplatePairSlug))
			if err != nil {
				t.Fatalf("resolve the pair: %v", err)
			}
			seedGoRun(t, pair, shedrun.RecipeJacquard)
			writeRunStatus(t, pair, shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: tt.row})
			if tt.writeRecord {
				record := runRecordOf(pair, shedrun.RecipeJacquard)
				if err := os.MkdirAll(record.dir, 0o755); err != nil {
					t.Fatalf("create the discussion directory: %v", err)
				}
				if err := os.WriteFile(record.path, []byte(decisionPriorRecord), 0o644); err != nil {
					t.Fatalf("write the decision record: %v", err)
				}
			}

			var out bytes.Buffer
			code := clihelp.ExecuteIn(Command(), pair.WorktreePath(), &out, []string{"decision", "add", "--by", "operator", "--title", "T", "--decision", "D.", "--rationale", "R."})

			if code != 1 {
				t.Fatalf("decision add exit = %d; want 1; output: %s", code, out.String())
			}
			for _, want := range tt.want {
				envelope.RequireErr(t, out.String(), want)
			}
			if strings.Contains(out.String(), loomshed.NameRecordWrite) {
				t.Errorf("decision add output names %s; want the jacquard run's own Record-Seed; output: %s", loomshed.NameRecordWrite, out.String())
			}
		})
	}
}
