package loomshed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/planglyph"
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
		"---\nformat: 5\napproved: %t\nlanguage: none\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — first-card — placeholder card 1\n",
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

	overview := "---\nformat: 99\napproved: true\nlanguage: none\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — first-card — placeholder card 1\n"
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

	t.Run("QuarryUnavailableReturnsErrorNotStuck", func(t *testing.T) {
		anchorPath := t.TempDir()
		seedGlyphPlanFixture(t, anchorPath, true, "sub#Foo", "")
		// A language: go plan pointed at a worktreeRoot that does not exist: openRepo cannot open it,
		// so the producer must return an error rather than mapping the outage to Stuck.
		worktreeRoot := filepath.Join(t.TempDir(), "does-not-exist")

		p := NewPlanValidate("Plan-Validate", anchorPath, worktreeRoot, true)
		outcome, _, err := p.Call(context.Background())
		if err == nil {
			t.Fatalf("Call() error = nil; want non-nil error for a quarry-unavailable worktreeRoot")
		}
		if outcome == shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want no Stuck verdict for a quarry outage", outcome)
		}
	})

	t.Run("InformationalOnlyFindingsAreDone", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := writeGlyphRepoFixture(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
		// "newpkg#Bar" resolves not_found with unit: not_found, which createFindings reports as the
		// informational create-new-unit finding -- no blocking finding in this plan.
		seedGlyphPlanFixture(t, anchorPath, true, "newpkg#Bar", "")

		buf := captureGateWarnings(t)
		p := NewPlanValidate("Plan-Validate", anchorPath, worktreeRoot, true)
		outcome, pointer, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Fatalf("Call() outcome = %q; want %q for an informational-only findings set", outcome, shedengine.Done)
		}
		wantPath := filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
		if pointer.Path != wantPath {
			t.Errorf("Call() pointer.Path = %q; want %q", pointer.Path, wantPath)
		}
		logged := buf.String()
		if !strings.Contains(logged, "create-new-unit") {
			t.Errorf("log = %q; want it to surface the informational finding for visibility on the pass path", logged)
		}
	})

	t.Run("MixedBlockingAndInformationalIsStuck", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := writeGlyphRepoFixture(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
		// "newpkg#Bar" is informational (create-new-unit); "sub#Missing" resolves not_found with
		// unit: found, which statusFindings reports as the blocking glyph-not-found finding. The
		// mixed set must map to Stuck: one blocking finding is enough to fail the gate.
		seedGlyphPlanFixture(t, anchorPath, true, "newpkg#Bar", "sub#Missing")

		p := NewPlanValidate("Plan-Validate", anchorPath, worktreeRoot, true)
		outcome, _, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Fatalf("Call() outcome = %q; want %q for a set carrying one blocking finding", outcome, shedengine.Stuck)
		}
	})
}

// writeGlyphRepoFixture writes files (keyed by repository-relative path) under a fresh t.TempDir()
// and returns that directory's absolute path, ready to hand to NewPlanValidate as worktreeRoot --
// duplicated from internal/planglyph/repo_test.go's writeFixtureRepo per the
// duplicate-test-helpers-rather-than-share-them Shared Decision.
func writeGlyphRepoFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) failed: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", full, err)
		}
	}
	return root
}

// seedGlyphPlanFixture writes a syntactically complete, one-card language: go plan under
// <anchorPath>/_lyx/plan/. createTarget names the card's sole Create group entry; when useTarget is
// non-empty it also carries a Uses: entry naming useTarget, so a test can add a second glyph target
// resolved outside the Create inversion.
func seedGlyphPlanFixture(t *testing.T, anchorPath string, approved bool, createTarget, useTarget string) {
	t.Helper()

	planDir := filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}

	usesBlock := ""
	if useTarget != "" {
		usesBlock = fmt.Sprintf("\n**Uses:**\n- `%s`\n", useTarget)
	}
	cardBody := fmt.Sprintf(
		"# Card 1 — first-card\n\n**Create:**\n- `%s`\n%s\n**Intent:** placeholder card.\n",
		createTarget, usesBlock,
	)
	if err := os.WriteFile(filepath.Join(planDir, "01-first-card.md"), []byte(cardBody), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}

	overview := fmt.Sprintf(
		"---\nformat: 5\napproved: %t\nlanguage: go\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — first-card — placeholder card 1\n",
		approved,
	)
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview file: %v", err)
	}
}

// TestHasBlockingFinding_UnrecognizedSeverityFailsClosed is R6-27's regression test.
// planglyph.Severity is an open string type, so an unrecognized value — or the zero value a
// hand-built Finding carries — took the informational branch and returned Done with the plan
// directory as its pointer: the run advanced past a finding meant to block it.
func TestHasBlockingFinding_UnrecognizedSeverityFailsClosed(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		severity planglyph.Severity
		want     bool
	}{
		{"blocking blocks", planglyph.SeverityBlocking, true},
		{"informational passes", planglyph.SeverityInformational, false},
		{"the zero value blocks", planglyph.Severity(""), true},
		{"an unrecognized severity blocks", planglyph.Severity("advisory"), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hasBlockingFinding([]planglyph.Finding{{Check: "some-check", Severity: tt.severity}})
			if got != tt.want {
				t.Errorf("hasBlockingFinding(severity %q) = %v; want %v", tt.severity, got, tt.want)
			}
		})
	}
}
