// removelanding_integration_test.go covers `lym fabric remove` over owed, dropped and pushes-off landings.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// landedPair is a pair with a records commit origin lacks and a landing commit left unpushed on the prime.
type landedPair struct {
	slug string
	held string
	tip  string
}

// addLandedPair adds the pair slug and gives it the held records commit and the landing commit.
func addLandedPair(t *testing.T, h *hubforge.Hub, slug string) landedPair {
	t.Helper()
	hubforge.AddPair(t, h, slug)
	held := gitkit.CommitFile(t, h.PairRecordsSibling(slug), "held.txt", "held\n", "held records of "+slug)
	tip := gitkit.CommitFile(t, h.PrimeWorktree(), slug+".txt", "landed\n", "land "+slug+"\n\n"+fabriclayout.LandingTrailerKey+": "+slug)
	return landedPair{slug: slug, held: held, tip: tip}
}

func writeRemoveLedger(t *testing.T, h *hubforge.Hub, key pushledger.Key, entry pushledger.Entry) {
	t.Helper()
	err := pushledger.Update(h.Path, key, time.Unix(1, 0), func(pushledger.Entry, bool) (pushledger.Entry, bool, error) {
		return entry, true, nil
	})
	if err != nil {
		t.Fatalf("pushledger.Update(%s): %v", key, err)
	}
}

// TestCommand_RemoveRefusesWhileALandingStands drives `lym fabric remove` and the pair teardown over one hub.
// It is serial: its last steps set FABRIC_SKIP_PUSH for the process.
func TestCommand_RemoveRefusesWhileALandingStands(t *testing.T) {
	tiergate.Require(t, 1)
	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
	if err != nil {
		t.Fatalf("load board config: %v", err)
	}
	cfg.Path = h.BoardDir()
	board := boardengine.New(cfg)
	seedEntry := func(slug, status string) {
		t.Helper()
		if _, err := board.UpsertTask(map[string]any{"slug": slug, "title": slug, "kind": "task", "labels": []string{"bug"}}); err != nil {
			t.Fatalf("seed the board entry of %s: %v", slug, err)
		}
		if err := board.SetStatus(slug, &status); err != nil {
			t.Fatalf("set the status of %s: %v", slug, err)
		}
	}
	recordsRoot, err := fabricengine.RecordsRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("RecordsRepoRoot: %v", err)
	}
	originRefs := func() string {
		return gitkit.Git(t, h.RecordsBare, "for-each-ref") + gitkit.Git(t, h.CodeBare, "for-each-ref")
	}
	requireRefused := func(t *testing.T, slug string) {
		t.Helper()
		before := originRefs()
		for _, flags := range [][]string{nil, {"--force"}, {"--remote"}} {
			code, out := runFabric(t, h.PrimeWorktree(), append(append([]string{"remove"}, flags...), slug)...)
			if code == 0 {
				t.Fatalf("remove %v exit = 0; want a refusal: %s", flags, out)
			}
			envelope.RequireErr(t, out, "wait for the pusher to push the parent branch")
		}
		if _, err := os.Stat(h.PairCodeWorktree(slug)); err != nil {
			t.Errorf("stat the pair = %v; want it standing", err)
		}
		if got := originRefs(); got != before {
			t.Errorf("origin refs after the refusals = %q; want them unchanged from %q", got, before)
		}
		task, found, err := board.GetTask(slug)
		if err != nil || !found || (task.Status != nil && *task.Status == boardengine.StatusDone) {
			t.Errorf("board entry of %s = %+v, found %v, %v; want it not done", slug, task, found, err)
		}
	}

	standing := addLandedPair(t, h, "standing-landing")
	seedEntry(standing.slug, "running · Finalize")
	writeRemoveLedger(t, h, pushledger.LandingKey, pushledger.Entry{Landings: []pushledger.Landing{{
		Run: standing.slug, Tip: standing.tip, RecordsCommit: standing.held, ParentBranch: "main", At: time.Unix(1, 0),
	}}})
	requireRefused(t, standing.slug)

	// A lost ledger leaves the landing commit on the parent, which still holds the pair.
	if err := os.RemoveAll(filepath.Dir(pushledger.EntryPath(h.Path, pushledger.LandingKey))); err != nil {
		t.Fatalf("delete the ledger: %v", err)
	}
	requireRefused(t, standing.slug)

	// A dropped landing holds nothing but the records commit, which no ref on origin may reach.
	for _, flags := range [][]string{nil, {"--remote"}} {
		dropped := addLandedPair(t, h, "dropped-landing"+strings.Join(flags, ""))
		seedEntry(dropped.slug, "running · Finalize")
		pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(dropped.slug))
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}
		if err := fabricengine.KeepHeldRecords(pair, dropped.slug, dropped.held); err != nil {
			t.Fatalf("KeepHeldRecords: %v", err)
		}
		writeRemoveLedger(t, h, pushledger.LandingKey, pushledger.Entry{Dropped: map[string]string{dropped.slug: dropped.tip}})
		pairKey := pushledger.RecordsKey(h.PairRecordsSibling(dropped.slug))
		writeRemoveLedger(t, h, pairKey, pushledger.Entry{Worktree: pair.WorktreePath(), Slug: dropped.slug, HeldRecords: dropped.held})

		code, out := runFabric(t, h.PrimeWorktree(), append(append([]string{"remove"}, flags...), dropped.slug)...)

		if code != 0 {
			t.Fatalf("remove %v of a dropped landing exit = %d: %s", flags, code, out)
		}
		if reached := gitkit.Git(t, h.RecordsBare, "rev-list", "--all"); strings.Contains(reached, dropped.held) {
			t.Errorf("remove %v: a ref on origin reaches the held commit %s", flags, dropped.held)
		}
		if refs := gitkit.Git(t, recordsRoot, "for-each-ref", "refs/held-records/"+dropped.slug); strings.TrimSpace(refs) != "" {
			t.Errorf("remove %v: held-records ref = %q; want it gone", flags, refs)
		}
		if entry, found, err := pushledger.Read(h.Path, pairKey); err != nil || found {
			t.Errorf("remove %v: pair ledger entry = %+v, found %v, %v; want none", flags, entry, found, err)
		}
	}

	// A landing made with pushes off writes no entry and marks the board done itself, so it holds no pair.
	td, err := pairteardown.New(h.Location)
	if err != nil {
		t.Fatalf("pairteardown.New: %v", err)
	}
	for _, tt := range []struct{ slug, skipAtRemoval string }{{"pushes-off-still-off", "1"}, {"pushes-off-back-on", ""}} {
		t.Setenv("FABRIC_SKIP_PUSH", "")
		landed := addLandedPair(t, h, tt.slug)
		seedEntry(landed.slug, boardengine.StatusDone)
		t.Setenv("FABRIC_SKIP_PUSH", tt.skipAtRemoval)

		if _, err := td.RemovePair(pairteardown.Request{Slug: landed.slug, Force: true}); err != nil {
			t.Errorf("RemovePair(%s) = %v; want nil", landed.slug, err)
		}
		if _, err := os.Stat(h.PairCodeWorktree(landed.slug)); !os.IsNotExist(err) {
			t.Errorf("stat the pair of %s = %v; want it removed", landed.slug, err)
		}
	}
}
