// originrecord_test.go tests the rules of the origin record, the beam mark and the remote-only read.

package fabriccore_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestOriginRecordRules(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("MarkBeam", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name                                        string
			recordFound, alreadyMarked, worktreePresent bool
			wantMark, wantNoWorktree, wantNoRecord      bool
		}{
			{"NoRecordsWorktree", false, false, false, false, true, false},
			{"NoRecord", false, false, true, false, false, true},
			{"AlreadyMarked", true, true, true, false, false, false},
			{"Marks", true, false, true, true, false, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				mark, noWorktree, noRecord := fabriccore.MarkBeamVerdict(tt.recordFound, tt.alreadyMarked, tt.worktreePresent)
				if mark != tt.wantMark || noWorktree != tt.wantNoWorktree || noRecord != tt.wantNoRecord {
					t.Errorf("MarkBeamVerdict(%v, %v, %v) = %v, %v, %v; want %v, %v, %v",
						tt.recordFound, tt.alreadyMarked, tt.worktreePresent,
						mark, noWorktree, noRecord, tt.wantMark, tt.wantNoWorktree, tt.wantNoRecord)
				}
			})
		}
	})

	t.Run("ReadOriginFor", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name                          string
			recordPresent, lockDirPresent bool
			wantRead, wantNoLockDir       bool
		}{
			{"NoRecord", false, false, false, false},
			{"RecordWithoutLockDir", true, false, false, true},
			{"Reads", true, true, true, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				read, noLockDir := fabriccore.ReadOriginForVerdict(tt.recordPresent, tt.lockDirPresent)
				if read != tt.wantRead || noLockDir != tt.wantNoLockDir {
					t.Errorf("ReadOriginForVerdict(%v, %v) = %v, %v; want %v, %v",
						tt.recordPresent, tt.lockDirPresent, read, noLockDir, tt.wantRead, tt.wantNoLockDir)
				}
			})
		}
	})

	t.Run("RemoteOnly", func(t *testing.T) {
		t.Parallel()
		tips := map[string]string{"task": "abc"}
		tests := []struct {
			name      string
			hasOrigin bool
			tips      map[string]string
			branch    string
			wantTip   string
			wantRead  bool
		}{
			{"NoOriginRemote", false, nil, "", "", false},
			{"TipsNotReadYet", true, nil, "", "", true},
			{"NoSuchBranchOnOrigin", true, tips, "other", "", false},
			{"OriginWithNoBranches", true, map[string]string{}, "task", "", false},
			{"BranchOnOrigin", true, tips, "task", "abc", true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				tip, read := fabriccore.RemoteOnly(tt.hasOrigin, tt.tips, tt.branch)
				if tip != tt.wantTip || read != tt.wantRead {
					t.Errorf("RemoteOnly(%v, %v, %q) = %q, %v; want %q, %v",
						tt.hasOrigin, tt.tips, tt.branch, tip, read, tt.wantTip, tt.wantRead)
				}
			})
		}
	})
}
