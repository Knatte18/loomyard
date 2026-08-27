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
// <anchorPath>/_lyx/plan/, approved or not per approved. The sole card carries a Create group so
// path-missing never fires regardless of worktreeRoot's contents — a Create group's targets stay
// exempt from on-disk existence checking.
func seedPlanValidateFixture(t *testing.T, anchorPath string, approved bool) {
	t.Helper()

	planDir := filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}

	cardBody := "# Card 1 — first-card\n\n**Create:**\n- `internal/firstcard/new.go`\n\n" +
		"**Intent:** placeholder card.\n"
	if err := os.WriteFile(filepath.Join(planDir, "01-first-card.md"), []byte(cardBody), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}

	overview := fmt.Sprintf(
		"---\nformat: 4\napproved: %t\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — first-card — placeholder card 1\n",
		approved,
	)
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview file: %v", err)
	}
}

// seedFormatInvalidPlanValidateFixture writes a plan whose overview declares an unrecognized
// format, tripping format-unrecognized regardless of requireApproved — the mode dimension must not
// change a format-invalid plan's Stuck disposition either way.
func seedFormatInvalidPlanValidateFixture(t *testing.T, anchorPath string) {
	t.Helper()

	planDir := filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}

	cardBody := "# Card 1 — first-card\n\n**Create:**\n- `internal/firstcard/new.go`\n\n" +
		"**Intent:** placeholder card.\n"
	if err := os.WriteFile(filepath.Join(planDir, "01-first-card.md"), []byte(cardBody), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}

	overview := "---\nformat: 99\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — first-card — placeholder card 1\n"
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview file: %v", err)
	}
}

func TestPlanValidate_Call(t *testing.T) {
	tests := []struct {
		name            string
		seed            func(t *testing.T, anchorPath string)
		requireApproved bool
		wantOutcome     shedengine.Outcome
		wantErr         bool
	}{
		{
			name:            "UnapprovedFormatClean_RequireApprovedTrueIsStuck",
			seed:            func(t *testing.T, anchorPath string) { seedPlanValidateFixture(t, anchorPath, false) },
			requireApproved: true,
			wantOutcome:     shedengine.Stuck,
		},
		{
			name:            "UnapprovedFormatClean_RequireApprovedFalseIsDone",
			seed:            func(t *testing.T, anchorPath string) { seedPlanValidateFixture(t, anchorPath, false) },
			requireApproved: false,
			wantOutcome:     shedengine.Done,
		},
		{
			name:            "FormatInvalid_RequireApprovedTrueIsStuck",
			seed:            seedFormatInvalidPlanValidateFixture,
			requireApproved: true,
			wantOutcome:     shedengine.Stuck,
		},
		{
			name:            "FormatInvalid_RequireApprovedFalseIsStuck",
			seed:            seedFormatInvalidPlanValidateFixture,
			requireApproved: false,
			wantOutcome:     shedengine.Stuck,
		},
		{
			name:            "CleanApproved_RequireApprovedTrueIsDone",
			seed:            func(t *testing.T, anchorPath string) { seedPlanValidateFixture(t, anchorPath, true) },
			requireApproved: true,
			wantOutcome:     shedengine.Done,
		},
		{
			name:            "CleanApproved_RequireApprovedFalseIsDone",
			seed:            func(t *testing.T, anchorPath string) { seedPlanValidateFixture(t, anchorPath, true) },
			requireApproved: false,
			wantOutcome:     shedengine.Done,
		},
		{
			name:            "UnparseableOverview_RequireApprovedTrueIsError",
			seed:            func(t *testing.T, anchorPath string) {}, // no _lyx/plan/00-overview.md at all
			requireApproved: true,
			wantErr:         true,
		},
		{
			name:            "UnparseableOverview_RequireApprovedFalseIsError",
			seed:            func(t *testing.T, anchorPath string) {}, // no _lyx/plan/00-overview.md at all
			requireApproved: false,
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			anchorPath := t.TempDir()
			worktreeRoot := t.TempDir()
			tt.seed(t, anchorPath)

			p := NewPlanValidate("Plan-Validate", anchorPath, worktreeRoot, tt.requireApproved)
			outcome, pointer, err := p.Call(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Call() error = nil; want non-nil error for an unparseable plan directory")
				}
				if !strings.Contains(err.Error(), "planparser:") {
					t.Errorf("Call() error = %q; want a planparser-prefixed error", err.Error())
				}
				if outcome == shedengine.Stuck {
					t.Errorf("Call() outcome = %q; want no Stuck verdict for an unparseable plan", outcome)
				}
				return
			}

			if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			if outcome != tt.wantOutcome {
				t.Errorf("Call() outcome = %q; want %q", outcome, tt.wantOutcome)
			}
			if tt.wantOutcome == shedengine.Done {
				wantPath := filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
				if pointer.Path != wantPath {
					t.Errorf("Call() pointer.Path = %q; want %q", pointer.Path, wantPath)
				}
			}
		})
	}

	t.Run("CancelledContextReturnsErrorNotVerdict", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		seedPlanValidateFixture(t, anchorPath, true)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		p := NewPlanValidate("Plan-Validate", anchorPath, worktreeRoot, true)
		outcome, _, err := p.Call(ctx)
		if err == nil {
			t.Fatalf("Call(cancelled) error = nil; want non-nil error")
		}
		if outcome == shedengine.Done || outcome == shedengine.Stuck {
			t.Errorf("Call(cancelled) outcome = %q; want no verdict alongside a cancellation error", outcome)
		}
	})
}
