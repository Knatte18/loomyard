//go:build integration

// notify_integration_test.go proves PrimeOrchStrand over a real hub: from a task worktree it names the prime's orch strand, and answers not recorded when the prime records none.

package orchcli

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

func TestNotifyPrime_QueuesInThePrimeOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		strand     string
		reedStrand string
		wantName   string
		wantRec    bool
	}{
		{"a GUID the reed state holds is named by the reed state", "g1", "g1", "tst:orch-held", true},
		{"no GUID recorded is not recorded", "", "", "", false},
		{"a GUID the reed state does not hold takes the fixed orch name", "g2", "g1", "tst:orch", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPair(t, h, "notify-task")
			task, err := lyxcwd.ResolveWorktree(h.PairCodeWorktree("notify-task"))
			if err != nil {
				t.Fatalf("ResolveWorktree task: %v", err)
			}
			if tt.strand != "" {
				if err := orchengine.SaveState(PrimePaths(h.Location), orchengine.State{Phase: orchengine.PhaseIdle, Strand: tt.strand}); err != nil {
					t.Fatalf("SaveState: %v", err)
				}
			}
			if tt.reedStrand != "" {
				reedState := &reedengine.ReedState{Strands: []reedengine.Strand{{GUID: tt.reedStrand, Name: "tst:orch-held"}}}
				if err := reedengine.SaveState(filepath.Join(h.Location.AnchorPath(), lyxdirs.DotLyxDirName), reedState); err != nil {
					t.Fatalf("reedengine.SaveState: %v", err)
				}
			}

			primeOfTask, err := resolvePrime(task)
			if err != nil {
				t.Fatalf("resolvePrime(task): %v", err)
			}
			gotName, gotRecorded, err := PrimeOrchStrand(primeOfTask)
			if err != nil {
				t.Fatalf("PrimeOrchStrand() error = %v; want nil", err)
			}
			if gotName != tt.wantName || gotRecorded != tt.wantRec {
				t.Errorf("PrimeOrchStrand() = %q, %v; want %q, %v", gotName, gotRecorded, tt.wantName, tt.wantRec)
			}
		})
	}
}
