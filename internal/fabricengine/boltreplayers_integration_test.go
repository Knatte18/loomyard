// boltreplayers_integration_test.go proves the registry of Bolt replayers.
// It covers what a replay, the ended-set read and the owner read do with no replayer or run ledger registered.
// It also covers how a replayer other than the board's is set aside.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeReplayer is a Bolt replayer named fake.
// It claims `notes/fake.json`, writes the commit's bytes of it, and reports one loss.
type fakeReplayer struct{}

func (fakeReplayer) Name() string { return "fake" }

func (fakeReplayer) Claims(path string) bool { return path == "notes/fake.json" }

func (fakeReplayer) Apply(req fabricengine.ReplayRequest) (fabricengine.ReplayResult, error) {
	for _, file := range req.Files {
		if err := os.MkdirAll(filepath.Join(req.Dir, filepath.Dir(file.Path)), 0o755); err != nil {
			return fabricengine.ReplayResult{}, err
		}
		if err := os.WriteFile(filepath.Join(req.Dir, file.Path), file.After, 0o644); err != nil {
			return fabricengine.ReplayResult{}, err
		}
	}
	return fabricengine.ReplayResult{Lost: []fabricengine.ReplayFinding{{Entry: "x", Reason: "origin changed it too"}}}, nil
}

// emptyEntryLedger is the board's replayer and run ledger, whose Apply reports one loss that names no entry.
type emptyEntryLedger struct {
	boardengine.FabricReplay
}

func (emptyEntryLedger) Apply(fabricengine.ReplayRequest) (fabricengine.ReplayResult, error) {
	return fabricengine.ReplayResult{Empty: true, Lost: []fabricengine.ReplayFinding{{Reason: "origin changed it too"}}}, nil
}

func TestBoltReplayers(t *testing.T) {
	tiergate.Require(t, 1)
	// Not parallel: it empties and fills the process-wide registry of Bolt replayers.
	boardkit.InstallReplay(t)

	// divergedBolt commits rel on the Bolt checkout and moves origin past it.
	// It returns the fixture, the checkout and the local commit.
	divergedBolt := func(t *testing.T, rel, content string) (f *recordsFixture, board, local string) {
		t.Helper()
		f = newRecordsFixture(t)
		board = f.hub.BoardDir()
		branch := gitkit.CurrentBranch(t, board)
		local = gitkit.CommitFile(t, board, rel, content, "local "+rel)
		f.originCommit(branch, "notes/up.md", "up\n", "upstream work")
		gitkit.Git(t, board, "fetch")
		return f, board, local
	}

	t.Run("With no replayer registered a commit touching no claimed file replays by cherry-pick", func(t *testing.T) {
		f, board, local := divergedBolt(t, "stencils/x.md", "local\n")
		defer fabricengine.WithoutBoltReplayersForTest()()

		outcome, err := fabricengine.ReplayBoltForTest(f.hub.Location, board, nil)
		if err != nil {
			t.Fatalf("ReplayBoltForTest: %v", err)
		}
		if !slices.Equal(outcome.Replayed, []string{local}) || len(outcome.SetAside) != 0 {
			t.Errorf("outcome = %+v, want only %s replayed", outcome, local)
		}
		requireClean(t, board)
	})

	t.Run("With only a replayer that is no run ledger registered a claimed commit fails naming the registration and leaves the tip", func(t *testing.T) {
		f, board, local := divergedBolt(t, "notes/fake.json", "{}\n")
		defer fabricengine.WithoutBoltReplayersForTest()()
		defer fabricengine.RegisterBoltReplayer(fakeReplayer{})()

		_, err := fabricengine.ReplayBoltForTest(f.hub.Location, board, nil)
		if !errors.Is(err, fabricengine.ErrReplayerMissing) || !strings.Contains(err.Error(), "RegisterBoltReplayer") {
			t.Fatalf("ReplayBoltForTest error = %v, want ErrReplayerMissing naming RegisterBoltReplayer", err)
		}
		if got := gitkit.RevParse(t, board, "HEAD"); got != local {
			t.Errorf("HEAD = %s, want the starting tip %s", got, local)
		}
	})

	t.Run("With no replayer registered the ended-set read and the owner read fail with ErrReplayerMissing", func(t *testing.T) {
		f := newRecordsFixture(t)
		board := f.hub.BoardDir()
		branch := gitkit.CurrentBranch(t, board)
		pair := newMovesPair(t)
		defer fabricengine.WithoutBoltReplayersForTest()()

		if _, err := fabricengine.ReadEndedSetForTest(board, "refs/remotes/origin/"+branch, []string{"a"}, nil); !errors.Is(err, fabricengine.ErrReplayerMissing) {
			t.Errorf("ReadEndedSetForTest error = %v, want ErrReplayerMissing", err)
		}
		seams := fabricengine.OriginSeams{OwnerOf: func(string, []byte, string) (string, string, bool, error) { return "", "", false, nil }}
		if _, err := fabricengine.ReconcileOrigin(pair.Location, fabricengine.ReconcileScope{}, seams, fabricengine.ReconcileOptions{}); !errors.Is(err, fabricengine.ErrReplayerMissing) {
			t.Errorf("ReconcileOrigin error = %v, want it to wrap ErrReplayerMissing", err)
		}
	})

	t.Run("A replayer beside the board's that reports a loss has its kept change committed and the original tagged under its name", func(t *testing.T) {
		f, board, local := divergedBolt(t, "notes/fake.json", "{\"kept\":true}\n")
		defer fabricengine.RegisterBoltReplayer(fakeReplayer{})()

		outcome, err := fabricengine.ReplayBoltForTest(f.hub.Location, board, nil)
		if err != nil {
			t.Fatalf("ReplayBoltForTest: %v", err)
		}
		wantTag := "set-aside/replayer/fake/" + local[:12]
		if !slices.Equal(outcome.Replayed, []string{local}) || len(outcome.SetAside) != 1 || outcome.SetAside[0].Tag != wantTag || len(outcome.Lost) != 1 {
			t.Errorf("outcome = %+v, want %s replayed, one loss and the original tagged %s", outcome, local, wantTag)
		}
		if got := gitkit.RevParse(t, board, wantTag+"^{commit}"); got != local {
			t.Errorf("tag %s names %s, want %s", wantTag, got, local)
		}
		if data, err := os.ReadFile(filepath.Join(board, "notes", "fake.json")); err != nil || string(data) != "{\"kept\":true}\n" {
			t.Errorf("notes/fake.json = %q, %v; want the replayer's kept bytes", data, err)
		}
		requireClean(t, board)
	})

	t.Run("A ledger loss that names no entry tags the original under the ledger's replayer name", func(t *testing.T) {
		f, board, local := divergedBolt(t, "board.json", boardFileOf(t, boardEntry("a")))
		defer fabricengine.RegisterBoltReplayer(emptyEntryLedger{})()

		outcome, err := fabricengine.ReplayBoltForTest(f.hub.Location, board, nil)
		if err != nil {
			t.Fatalf("ReplayBoltForTest: %v", err)
		}
		wantTag := "set-aside/replayer/board/" + local[:12]
		if len(outcome.SetAside) != 1 || outcome.SetAside[0].Commit != local || outcome.SetAside[0].Tag != wantTag {
			t.Errorf("set aside = %+v, want %s under %s", outcome.SetAside, local, wantTag)
		}
	})
}
