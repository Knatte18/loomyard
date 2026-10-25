//go:build tier0

package loomcli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestFinalizeDirtGuard covers the Finalize arm composed with a later guard, then the receiver's wiring of both arms.
func TestFinalizeDirtGuard(t *testing.T) {
	t.Parallel()

	const settledRefusal = "the Plan-Review verdict is settled"
	dirtyCheckout, dirtyPaths := "/hub/parent", []string{"a.go", "b c.go"}
	wantDirt := []string{"/hub/parent", `"a.go", "b c.go"`, "lyx loom goto --to Finalize", "lyx loom resume"}
	errLater := errors.New("settle record unreadable")
	errRecipe := errors.New("recipe parse failure")

	tests := []struct {
		name string
		// isFinalizeErr makes the isFinalize fake fail.
		isFinalizeErr error
		paths         []string
		mergeRecord   bool
		dirtErr       error
		target        string
		// wantRefusal lists the substrings of the refusal; empty means the target is admitted.
		wantRefusal []string
		wantErr     error
		wantReads   int
	}{
		{name: "Finalize over dirt", paths: dirtyPaths, target: "Finalize", wantRefusal: wantDirt, wantReads: 1},
		{name: "Finalize over an empty list", target: "Finalize", wantReads: 1},
		{name: "Finalize over a merge record beside dirt", paths: dirtyPaths, mergeRecord: true, target: "Finalize", wantReads: 1},
		{name: "Finalize over a read error beside dirt", paths: dirtyPaths, dirtErr: errors.New("git failed"), target: "Finalize", wantReads: 1},
		{name: "settled review row", paths: dirtyPaths, target: "Plan-Burler-Review", wantRefusal: []string{settledRefusal}},
		{name: "later guard error", paths: dirtyPaths, target: "Webster-Burler-Review", wantErr: errLater},
		{name: "Parent-Align", paths: dirtyPaths, target: "Parent-Align"},
		{name: "isFinalize error", isFinalizeErr: errRecipe, paths: dirtyPaths, target: "Finalize", wantErr: errRecipe},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reads := 0
			parentDirt := func() (string, []string, bool, error) {
				reads++
				return dirtyCheckout, tt.paths, tt.mergeRecord, tt.dirtErr
			}
			isFinalize := loomrecipe.IsFinalizeRow
			if tt.isFinalizeErr != nil {
				isFinalize = func(string) (bool, error) { return false, tt.isFinalizeErr }
			}
			fakeSettled := func(target string) (string, error) {
				switch target {
				case "Plan-Burler-Review":
					return settledRefusal, nil
				case "Webster-Burler-Review":
					return "", errLater
				}
				return "", nil
			}
			guard := composeGotoGuards(finalizeDirtGuard(isFinalize, parentDirt), fakeSettled)

			refusal, err := guard(tt.target)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("guard(%q) error = %v; want %v", tt.target, err, tt.wantErr)
			}
			if len(tt.wantRefusal) == 0 && refusal != "" {
				t.Errorf("guard(%q) = %q; want the target admitted", tt.target, refusal)
			}
			for _, want := range tt.wantRefusal {
				if !strings.Contains(refusal, want) {
					t.Errorf("guard(%q) = %q; want it to contain %q", tt.target, refusal, want)
				}
			}
			if reads != tt.wantReads {
				t.Errorf("guard(%q) read the parent %d times; want %d", tt.target, reads, tt.wantReads)
			}
		})
	}

	t.Run("wiring", func(t *testing.T) {
		t.Parallel()
		loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
		subdir, isBouncer, err := loomrecipe.BouncerRunSubdir("Plan-Bouncer")
		if err != nil || !isBouncer {
			t.Fatalf("BouncerRunSubdir(Plan-Bouncer) = %q, %v, %v; want a Bouncer row", subdir, isBouncer, err)
		}
		runDir := filepath.Join(loomengine.LoomReviewsDir(loc), subdir)
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatal(err)
		}
		settled := `{"subject":{"kind":"document"},"generation":1,"round":1,"settled_at":"2026-10-20T10:00:00Z","settled":true}`
		if err := os.WriteFile(filepath.Join(runDir, "settle-record.json"), []byte(settled), 0o644); err != nil {
			t.Fatal(err)
		}
		c := &loomCLI{
			location: loc,
			routing: shedengine.Routing{Producers: []shedengine.ProducerDef{
				{Name: "Plan-Bouncer", Segment: "Plan-Review", OnStuck: "Plan-Burler-Review", OnDone: "Webster"},
				{Name: "Plan-Burler-Review", Segment: "Plan-Review", OnStuck: "Plan-Bouncer", OnDone: "Plan-Bouncer"},
			}},
			parentCodeDirt: func() (string, []string, bool, error) { return dirtyCheckout, dirtyPaths, false, nil },
		}

		rows := []struct {
			name        string
			guard       shedengine.GotoGuard
			target      string
			wantRefusal []string
		}{
			{"whole guard refuses Finalize", c.settledGuard(), "Finalize", wantDirt},
			{"whole guard refuses a settled review row", c.settledGuard(), "Plan-Burler-Review", []string{settledRefusal}},
			{"Finalize arm refuses Finalize", c.finalizeGuard(), "Finalize", wantDirt},
			{"Finalize arm admits a settled review row", c.finalizeGuard(), "Plan-Burler-Review", nil},
		}
		for _, row := range rows {
			refusal, err := row.guard(row.target)
			if err != nil {
				t.Errorf("%s: guard(%q) error = %v", row.name, row.target, err)
			}
			if len(row.wantRefusal) == 0 && refusal != "" {
				t.Errorf("%s: guard(%q) = %q; want the target admitted", row.name, row.target, refusal)
			}
			for _, want := range row.wantRefusal {
				if !strings.Contains(refusal, want) {
					t.Errorf("%s: guard(%q) = %q; want it to contain %q", row.name, row.target, refusal, want)
				}
			}
		}
	})

	t.Run("production read", func(t *testing.T) {
		t.Parallel()
		readErrors := []struct {
			name         string
			parentBranch func() (func(*lyxcwd.Location) (string, error), error)
		}{
			{"recorded parent branch unreadable", func() (func(*lyxcwd.Location) (string, error), error) {
				err := errors.New("the pair has no fabric origin record")
				return parentBranchSeam(err), err
			}},
			{"parent pair not openable", func() (func(*lyxcwd.Location) (string, error), error) {
				return parentBranchSeam(nil), nil
			}},
		}
		for _, tt := range readErrors {
			seam, wantErr := tt.parentBranch()
			c := &loomCLI{
				location:             &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."},
				recordedParentBranch: seam,
			}
			c.parentCodeDirt = c.parentCodeDirtFromFabric

			_, _, _, err := c.parentCodeDirtFromFabric()
			if err == nil || (wantErr != nil && !errors.Is(err, wantErr)) {
				t.Errorf("%s: parentCodeDirtFromFabric() error = %v; want %v", tt.name, err, wantErr)
			}
			if refusal, err := c.finalizeGuard()("Finalize"); refusal != "" || err != nil {
				t.Errorf("%s: finalizeGuard()(Finalize) = %q, %v; want admitted", tt.name, refusal, err)
			}
		}
	})
}
