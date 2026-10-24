//go:build tier0

// promote_test.go pins the two entry points of "lyx beam promote" over the cancel's fake substrates: the end of the run with its marker removed, the end-only stop, and the clear of the records and the claim.

package battencli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

func TestEndRunForPromote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(*cancelFake)
		// wantErr is a substring of the error, empty for none; wantEndOnly says it matches ErrPromoteEndOnly.
		wantErr     string
		wantEndOnly bool
		wantMarker  []bool
	}{
		{
			name:       "EndsTheRunAndRemovesTheMarker",
			wantMarker: []bool{true},
		},
		{
			name:        "UnreachableOriginEndsOnlyAndKeepsTheMarker",
			setup:       func(f *cancelFake) { f.pullErr = fmt.Errorf("pull: %w", boardengine.ErrOwnerOriginUnreachable) },
			wantErr:     `run "lyx beam promote some-slug" again once origin answers`,
			wantEndOnly: true,
			wantMarker:  []bool{false},
		},
		{
			name:    "RefusalNamesThePromoteRerun",
			setup:   func(f *cancelFake) { f.hubPaused = true },
			wantErr: `then "lyx beam promote some-slug" again`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := newCancelFake()
			if tt.setup != nil {
				tt.setup(fake)
			}

			err := endRunForPromote(context.Background(), cancelSlug, fake.deps())

			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("endRunForPromote() error = %v; want nil", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("endRunForPromote() error = %v; want it to contain %q", err, tt.wantErr)
			}
			if got := errors.Is(err, ErrPromoteEndOnly); got != tt.wantEndOnly {
				t.Errorf("errors.Is(err, ErrPromoteEndOnly) = %v; want %v", got, tt.wantEndOnly)
			}
			for _, name := range []string{"RemovePair", "DiscardTaskBranch", "RemoveRecords", "ClearClaim", "Notify"} {
				if fake.called(name) {
					t.Errorf("calls = %v; want no %s call", fake.calls, name)
				}
			}
			if !slices.Equal(fake.markerReleases, tt.wantMarker) {
				t.Errorf("marker releases = %v; want %v", fake.markerReleases, tt.wantMarker)
			}
			if fake.locksHeld != 0 {
				t.Errorf("batten locks held = %d; want 0", fake.locksHeld)
			}
		})
	}
}

func TestClearRunForPromote(t *testing.T) {
	t.Parallel()

	fake := newCancelFake()
	fake.parts = fabricengine.PairParts{TaskWorktree: true, RecordsWorktree: true, RecordsBranch: true, TaskBranch: true}

	if err := clearRunForPromote(cancelSlug, fake.deps()); err != nil {
		t.Fatalf("clearRunForPromote() = %v; want nil", err)
	}

	if !callsInOrder(fake.calls, []string{"RemoveRecords", "ClearClaim", "SpawnPush"}) {
		t.Errorf("calls = %v; want the records removed, the claim cleared and the push spawned", fake.calls)
	}
	for _, name := range []string{"EndForCancel", "TakeMarker", "RemovePair", "Notify"} {
		if fake.called(name) {
			t.Errorf("calls = %v; want no %s call", fake.calls, name)
		}
	}
}
