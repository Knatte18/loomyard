// pairpushdeferred_integration_test.go covers PairPushDeferred and PairLastCommit over the pair of a hubforge hub.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newHubPair adds the pair slug to a fresh hub and returns the hub with the pair's location.
func newHubPair(t *testing.T, slug string) (*hubforge.Hub, *lymcwd.Location) {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, slug)
	pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}
	return h, pair
}

func TestPairPushDeferred(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// flagSides are the sides whose pending flag the row writes.
		flagSides []string
		want      bool
	}{
		{name: "NoPendingFlag", want: false},
		{name: "ACodeSideFlag", flagSides: []string{fabricengine.PushSideCodeForTest}, want: true},
		{name: "ARecordsSideFlag", flagSides: []string{fabricengine.PushSideRecordsForTest}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, pair := newHubPair(t, "deferred-run")
			lockDir := fabriclayout.RecordsLockDirPath(fabricengine.RecordsWorktree(pair))
			if err := os.MkdirAll(lockDir, 0o755); err != nil {
				t.Fatalf("create the lock directory: %v", err)
			}
			for _, side := range tt.flagSides {
				if err := os.WriteFile(fabricengine.PendingFlagPathForTest(lockDir, side), []byte("{}"), 0o644); err != nil {
					t.Fatalf("write the %s pending flag: %v", side, err)
				}
			}

			if got := fabricengine.PairPushDeferred(pair); got != tt.want {
				t.Errorf("PairPushDeferred() = %v; want %v", got, tt.want)
			}
		})
	}
}

// commitEmptyAt commits an empty commit in dir whose committer time is date, an RFC 3339 time.
// The environment is set on the one git process, so parallel tests are not affected.
func commitEmptyAt(t *testing.T, dir, date string) {
	t.Helper()

	cmd := exec.Command("git", "commit", "-q", "--allow-empty", "-m", "dated commit")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+date, "GIT_AUTHOR_DATE="+date)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit in %s: %v; output: %s", dir, err, output)
	}
}

func TestPairLastCommit(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const (
		codeOlder    = "2020-01-01T00:00:00Z"
		recordsNewer = "2021-01-01T00:00:00Z"
	)
	tests := []struct {
		name                  string
		codeDate, recordsDate string
		removeRecordsWorktree bool
		want                  string
	}{
		{name: "TheRecordsTipIsNewer", codeDate: codeOlder, recordsDate: recordsNewer, want: recordsNewer},
		{name: "TheCodeTipIsNewer", codeDate: recordsNewer, recordsDate: codeOlder, want: recordsNewer},
		{name: "WithoutARecordsWorktreeTheCodeTipCounts", codeDate: codeOlder, recordsDate: recordsNewer, removeRecordsWorktree: true, want: codeOlder},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, pair := newHubPair(t, "last-commit-run")
			commitEmptyAt(t, h.PairCodeWorktree("last-commit-run"), tt.codeDate)
			commitEmptyAt(t, h.PairRecordsSibling("last-commit-run"), tt.recordsDate)
			if tt.removeRecordsWorktree {
				if err := os.RemoveAll(filepath.Clean(fabricengine.RecordsWorktree(pair))); err != nil {
					t.Fatalf("remove the records worktree: %v", err)
				}
			}
			want, err := time.Parse(time.RFC3339, tt.want)
			if err != nil {
				t.Fatalf("parse %q: %v", tt.want, err)
			}

			got, err := fabricengine.PairLastCommit(pair)

			if err != nil {
				t.Fatalf("PairLastCommit() error = %v; want nil", err)
			}
			if !got.Equal(want) {
				t.Errorf("PairLastCommit() = %v; want %v", got, want)
			}
		})
	}
}
