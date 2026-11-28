// anchorqueue_integration_test.go drives the landing-queue backstop a fabric verb runs when it takes a parent lock.

package fabriccli_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingqueue"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/testkit/permkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The test is not parallel: its pushes-skipped row sets FABRIC_SKIP_PUSH in the process environment.
func TestAcquireBranchLock_WritesTheLandingsAStandingMarkOwes(t *testing.T) {
	tiergate.Require(t, 1)
	tests := []struct {
		name string
		// offBranch leaves the marked commits on a side branch the parent does not hold.
		offBranch  bool
		noUpstream bool
		// prunedRef deletes the parent's tracking ref and keeps its upstream setting.
		prunedRef bool
		// missing marks commits the repository does not hold.
		missing      bool
		skipPush     bool
		ledgerLocked bool
		wantLandings []string
	}{
		{name: "OneLandingPerOwedCommit", wantLandings: []string{"one", "two"}},
		{name: "CommitsTheParentDoesNotHold", offBranch: true},
		{name: "ParentWithNoUpstream", noUpstream: true, wantLandings: []string{"one", "two"}},
		{name: "ParentWithItsTrackingRefPruned", prunedRef: true},
		{name: "CommitsTheRepositoryLacks", missing: true},
		{name: "PushesSkipped", skipPush: true},
		{name: "UnwritableLedgerKeepsTheMark", ledgerLocked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipPush {
				t.Setenv("FABRIC_SKIP_PUSH", "1")
			} else {
				t.Parallel()
			}
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			prime := h.PrimeWorktree()
			branch := strings.TrimSpace(gitkit.Git(t, prime, "rev-parse", "--abbrev-ref", "HEAD"))
			base := strings.TrimSpace(gitkit.Git(t, prime, "rev-parse", "HEAD"))
			records := strings.TrimSpace(gitkit.Git(t, fabricengine.RecordsWorktree(h.Location), "rev-parse", "HEAD"))
			if tt.offBranch {
				gitkit.MustRun(t, prime, "git", "checkout", "-b", "side")
			}
			mark := landingqueue.MoveMark{Base: base}
			for _, slug := range []string{"one", "two"} {
				gitkit.CommitFile(t, prime, slug+".txt", slug, "land "+slug)
				commit := strings.TrimSpace(gitkit.Git(t, prime, "rev-parse", "HEAD"))
				mark.Members = append(mark.Members, landingqueue.MarkMember{Slug: slug, Commit: commit, Records: records})
			}
			if tt.offBranch {
				gitkit.MustRun(t, prime, "git", "checkout", branch)
			}
			if tt.noUpstream {
				gitkit.MustRun(t, prime, "git", "branch", "--unset-upstream")
			}
			if tt.prunedRef {
				gitkit.MustRun(t, prime, "git", "update-ref", "-d", "refs/remotes/origin/"+branch)
			}
			if tt.missing {
				for i := range mark.Members {
					mark.Members[i].Commit = strings.Repeat("1", 40)
				}
			}
			boltDir := fabriclayout.BoltDir(h.Path)
			queue := &landingqueue.Store{Dir: landingqueue.Dir(boltDir), Branch: branch}
			if err := queue.WriteMark(mark); err != nil {
				t.Fatalf("WriteMark: %v", err)
			}
			ledger := filepath.Dir(pushledger.EntryPath(h.Path, pushledger.LandingKey))
			if tt.ledgerLocked {
				if err := os.MkdirAll(ledger, 0o755); err != nil {
					t.Fatalf("create the push ledger: %v", err)
				}
				permkit.Chmod(t, ledger, 0o500)
			}

			release, err := fabriccli.AcquireBranchLockForTest(context.Background(), h.Location, boltDir, "anchor")

			_, marked, markErr := queue.ReadMark()
			if markErr != nil {
				t.Fatalf("ReadMark: %v", markErr)
			}
			locks := &parentlock.Store{Dir: parentlock.Dir(boltDir)}
			if tt.ledgerLocked {
				if err == nil || !strings.Contains(err.Error(), "move mark of branch "+branch) || !strings.Contains(err.Error(), "way forward:") {
					t.Errorf("AcquireBranchLock() error = %v; want one naming the move mark and the way forward", err)
				}
				if !marked {
					t.Error("the move mark is gone; want it kept")
				}
				if record, held, err := locks.Read(branch); err != nil || held {
					t.Errorf("parent lock = %+v, held %v, %v; want it free", record, held, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("AcquireBranchLock() error = %v; want nil", err)
			}
			defer release()
			if marked {
				t.Error("the move mark stands; want it removed")
			}
			entry, _, err := pushledger.Read(h.Path, pushledger.LandingKey)
			if err != nil {
				t.Fatalf("read the push ledger: %v", err)
			}
			var got []string
			for _, landing := range entry.Landings {
				got = append(got, landing.Run)
				at := slices.IndexFunc(mark.Members, func(m landingqueue.MarkMember) bool { return m.Slug == landing.Run })
				want := pushledger.Landing{Run: landing.Run, Tip: mark.Members[at].Commit, RecordsCommit: records, ParentBranch: branch}
				landing.At = time.Time{}
				if landing != want {
					t.Errorf("landing = %+v; want %+v", landing, want)
				}
			}
			if !slices.Equal(got, tt.wantLandings) {
				t.Errorf("landed runs = %v; want %v", got, tt.wantLandings)
			}
		})
	}
}
