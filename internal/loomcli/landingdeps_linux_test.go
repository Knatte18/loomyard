//go:build tier0

// landingdeps_linux_test.go holds the loop-lock owner test, which needs a process start time only Linux reports.

package loomcli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// TestLoopLockOwner asserts the lock's owner process is the run's live loop, and the calling step process for a dead loop, the teardown's mark and no record at all.
func TestLoopLockOwner(t *testing.T) {
	t.Parallel()

	parentPID := os.Getppid()
	parentStart, _ := proc.StartTime(parentPID)
	if parentStart == "" {
		t.Fatalf("proc.StartTime(%d) reported no start time; want the parent's", parentPID)
	}
	liveLoop := proc.TreeRecord{PID: parentPID, StartTime: parentStart}
	self := proc.SelfRecord()

	tests := []struct {
		name    string
		record  *shedverbs.LoopPIDRecord
		wantPID int
	}{
		{name: "a live loop is the owner process", record: &shedverbs.LoopPIDRecord{LoopID: "loop-1", Loop: liveLoop}, wantPID: parentPID},
		{name: "a dead loop falls back to the step", record: &shedverbs.LoopPIDRecord{LoopID: "loop-1", Loop: proc.TreeRecord{PID: parentPID, StartTime: "not-its-start-time"}}, wantPID: self.PID},
		{name: "the teardown mark counts as no loop", record: &shedverbs.LoopPIDRecord{LoopID: shedverbs.TeardownLoopID, Loop: liveLoop}, wantPID: self.PID},
		{name: "no record is the step", wantPID: self.PID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			loc := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
			if tt.record != nil {
				if err := shedverbs.WriteLoopPIDRecord(shedrun.LoopPIDFile(loc, shedrun.SelfRunID), *tt.record); err != nil {
					t.Fatalf("WriteLoopPIDRecord() error = %v", err)
				}
			}

			owner, err := loopLockOwner(loc, shedrun.SelfRunID)()
			if err != nil {
				t.Fatalf("loopLockOwner()() error = %v", err)
			}
			if owner.PID != tt.wantPID {
				t.Errorf("owner.PID = %d; want %d", owner.PID, tt.wantPID)
			}
			if owner.Slug != "pair" || owner.Pair != loc.WorktreePath() || owner.RunID != "pair" {
				t.Errorf("owner = %+v; want the run named for the worktree %q at %q", owner, "pair", loc.WorktreePath())
			}
		})
	}
}
