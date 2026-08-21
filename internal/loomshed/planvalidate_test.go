package loomshed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// seedPlanValidateFixture writes a syntactically complete, one-card plan-format plan under
// <anchorPath>/_lyx/plan/, approved or not per approved. The sole card carries a Creates: entry so
// path-missing never fires regardless of worktreeRoot's contents.
func seedPlanValidateFixture(t *testing.T, anchorPath string, approved bool) {
	t.Helper()

	planDir := filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}

	cardBody := "# Card 1 — first-card\n\n**What:** placeholder card.\n**Context:** none\n**Edits:** none\n" +
		"**Creates:**\n- `internal/firstcard/new.go`\n**Deletes:** none\n**Moves:** none\n**Depends-on:** none\n"
	if err := os.WriteFile(filepath.Join(planDir, "01-first-card.md"), []byte(cardBody), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}

	overview := fmt.Sprintf(
		"---\nformat: 3\napproved: %t\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — first-card — placeholder card 1\n",
		approved,
	)
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview file: %v", err)
	}
}

func TestPlanValidate_Call(t *testing.T) {
	t.Run("ZeroFindingsMapsToDone", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		seedPlanValidateFixture(t, anchorPath, true)

		p := NewPlanValidate("Plan-Validate", anchorPath, worktreeRoot)
		outcome, pointer, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
		}
		wantPath := filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
		if pointer.Path != wantPath {
			t.Errorf("Call() pointer.Path = %q; want %q", pointer.Path, wantPath)
		}
	})

	t.Run("AtLeastOneFindingMapsToStuck", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		seedPlanValidateFixture(t, anchorPath, false) // unapproved -> plan-unapproved finding

		p := NewPlanValidate("Plan-Validate", anchorPath, worktreeRoot)
		outcome, _, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
	})

	t.Run("UnparseablePlanMapsToError", func(t *testing.T) {
		anchorPath := t.TempDir() // no _lyx/plan/00-overview.md at all
		worktreeRoot := t.TempDir()

		p := NewPlanValidate("Plan-Validate", anchorPath, worktreeRoot)
		outcome, _, err := p.Call(context.Background())
		if err == nil {
			t.Fatalf("Call() error = nil; want non-nil error for an unparseable plan directory")
		}
		if !strings.Contains(err.Error(), "planparser:") {
			t.Errorf("Call() error = %q; want a planparser-prefixed error", err.Error())
		}
		if outcome == shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want no Stuck verdict for an unparseable plan", outcome)
		}
	})

	t.Run("CancelledContextReturnsErrorNotVerdict", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		seedPlanValidateFixture(t, anchorPath, true)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		p := NewPlanValidate("Plan-Validate", anchorPath, worktreeRoot)
		outcome, _, err := p.Call(ctx)
		if err == nil {
			t.Fatalf("Call(cancelled) error = nil; want non-nil error")
		}
		if outcome == shedengine.Done || outcome == shedengine.Stuck {
			t.Errorf("Call(cancelled) outcome = %q; want no verdict alongside a cancellation error", outcome)
		}
	})
}
