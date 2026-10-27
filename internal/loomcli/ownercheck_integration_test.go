//go:build tier1

// ownercheck_integration_test.go drives the run owner verdict over a hub forged by hubforge whose Bolt origin records the owners.
// This package's own testmain_test.go arms the hermetic git test environment for the whole binary,
// so this file adds no TestMain.

package loomcli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
)

// TestRunOwnerVerdict covers the run owner verdict with a fixed machine name over a hub whose Bolt origin records the owners, with a batten status file at the prime for the run's pair so the reconcile judges it:
// origin's Bolt recording another machine as the prime's owner reads the run owned elsewhere with that owner, and with both origins unreachable the local board's owner decides.
func TestRunOwnerVerdict(t *testing.T) {
	t.Parallel()
	boardkit.InstallReplay(t)

	const slug = "owned-task"
	const here = "here"
	tests := []struct {
		name string
		// recorder is the machine the board records as the prime's owner, whom the run's root task reads as its owner.
		recorder string
		// unreachable points both origin URLs at an address nothing answers before the verdict.
		unreachable bool
		want        loomcli.OwnerVerdict
	}{
		{name: "OriginRecordingAnotherOwnerReadsOwnedElsewhere", recorder: "there", want: loomcli.OwnerVerdict{Owner: "there", AdoptSubject: "root", Elsewhere: true}},
		{name: "UnreachableOriginReadsTheLocalBoardOwnedElsewhere", recorder: "there", unreachable: true, want: loomcli.OwnerVerdict{Owner: "there", AdoptSubject: "root", Elsewhere: true}},
		{name: "UnreachableOriginReadsTheLocalBoardOwnedHere", recorder: here, unreachable: true, want: loomcli.OwnerVerdict{Owner: here, AdoptSubject: "root"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPair(t, h, slug)
			running := shedengine.Status{CurrentProducer: "Run-Shed", State: shedengine.StateRunning}
			statusPath, statusLockPath := shedrun.StatusFile(h.Location, slug), shedrun.StatusLock(h.Location, slug)
			for _, dir := range []string{filepath.Dir(statusPath), filepath.Dir(statusLockPath)} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("make the prime's batten status directory %s: %v", dir, err)
				}
			}
			if err := state.WriteJSON(statusPath, statusLockPath, running); err != nil {
				t.Fatalf("write the prime's batten status of %q: %v", slug, err)
			}

			boardConfig, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
			if err != nil {
				t.Fatalf("load board config: %v", err)
			}
			boardConfig.Path = h.BoardDir()
			board := boardengine.New(boardConfig)
			if _, err := board.UpsertTask(map[string]any{"slug": slug, "title": slug, "kind": "task", "labels": []string{"bug"}}); err != nil {
				t.Fatalf("seed board task %q: %v", slug, err)
			}
			if err := board.Sync(); err != nil {
				t.Fatalf("push the seeded board: %v", err)
			}
			request := boardengine.OwnerRequest{Machine: tt.recorder, Prime: true, Takeover: true}
			if _, err := boardengine.SetOwners(h.BoardDir(), request); err != nil {
				t.Fatalf("record the owners: %v", err)
			}
			if tt.unreachable {
				for _, repo := range []string{h.PrimeWorktree(), h.PrimeRecords()} {
					gitkit.Git(t, repo, "remote", "set-url", "origin", "http://127.0.0.1:1/x.git")
				}
			}
			taskLocation, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
			if err != nil {
				t.Fatalf("resolve the pair's worktree: %v", err)
			}

			got, err := loomcli.RunOwnerVerdict(taskLocation, slug, func() (string, error) { return here, nil })

			if err != nil {
				t.Fatalf("RunOwnerVerdict() error = %v; want nil", err)
			}
			if got != tt.want {
				t.Errorf("RunOwnerVerdict() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
