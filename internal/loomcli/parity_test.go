// parity_test.go asserts the Gate Self-Check Parity Invariant for the gate sites the loomshed gate closures cover.
// It covers NewDiscussionGate against validate-discussion, and NewPlanGate against validate-plan's flag-absent mode.
// Both the gate closure and its loomcli verb must reach the same three-way verdict over the identical fixture.
// Both sides call the identical package function.
//
// The comparison is three-way, not binary.
// The gate closure's Passed/error trio and the verb's ok/findings-key trio each map onto one shared parityVerdict.
// That lets a Stuck-vs-error mismatch be caught rather than collapsed onto a single "fail" side.
// Both halves take told paths and construct no repository, spawn no process, use no real hub, or copy a fixture tree.
// No test here calls RunCLIIn, so both stay tier 1.
package loomcli

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// discussionParityCase is one fixture for TestGateParity_DiscussionGate: build populates dir
// with whatever the fixture needs and returns the single *loomCLI both the gate closure and the
// verb read their paths from, so both halves run over the exact same on-disk fixture. want names
// the verdict both sides must reach.
type discussionParityCase struct {
	name  string
	build func(t *testing.T, dir string) *loomCLI
	want  parityVerdict
}

// TestGateParity_DiscussionGate drives NewDiscussionGate and the validate-discussion verb over the
// same fixture set and asserts the two mapped verdicts agree, covering all three parityVerdict
// values: a clean fixture (done), a missing-support-log fixture and a missing-heading fixture
// (stuck), and a decision-record-is-a-directory fixture (error).
func TestGateParity_DiscussionGate(t *testing.T) {
	tiergate.Require(t, 0)
	cases := []discussionParityCase{
		{
			name: "Clean",
			build: func(t *testing.T, dir string) *loomCLI {
				return decisionRecordFixture(t, dir, allDiscussionSectionsContent(), "support log body", true)
			},
			want: verdictDone,
		},
		{
			name: "Stuck_SupportLogMissing",
			build: func(t *testing.T, dir string) *loomCLI {
				return decisionRecordFixture(t, dir, allDiscussionSectionsContent(), "", false)
			},
			want: verdictStuck,
		},
		{
			name: "Stuck_HeadingMissing",
			build: func(t *testing.T, dir string) *loomCLI {
				var b []byte
				for _, h := range requiredDiscussionSectionsForTest {
					if h == "## Constraints" {
						continue
					}
					b = append(b, []byte(h+"\n\nbody text.\n\n")...)
				}
				return decisionRecordFixture(t, dir, string(b), "support log body", true)
			},
			want: verdictStuck,
		},
		{
			name: "Error_DecisionRecordIsDirectory",
			build: func(t *testing.T, dir string) *loomCLI {
				c := decisionRecordFixture(t, dir, allDiscussionSectionsContent(), "support log body", true)
				if err := os.Remove(c.env.DecisionRecordPath); err != nil {
					t.Fatalf("remove decision record: %v", err)
				}
				if err := os.Mkdir(c.env.DecisionRecordPath, 0o755); err != nil {
					t.Fatalf("mkdir decision record: %v", err)
				}
				return c
			},
			want: verdictError,
		},
		{
			name: "Stuck_RelevantMalformed",
			build: func(t *testing.T, dir string) *loomCLI {
				c := decisionRecordFixture(t, dir, allDiscussionSectionsContent(), "support log body", true)
				writeStrayRelevant(t, c.env.RelevantPath)
				return c
			},
			want: verdictStuck,
		},
		{
			name: "Error_RelevantIsDirectory",
			build: func(t *testing.T, dir string) *loomCLI {
				c := decisionRecordFixture(t, dir, allDiscussionSectionsContent(), "support log body", true)
				if err := os.Mkdir(c.env.RelevantPath, 0o755); err != nil {
					t.Fatalf("mkdir relevant list: %v", err)
				}
				return c
			},
			want: verdictError,
		},
		{
			name: "Darn_Clean",
			build: func(t *testing.T, dir string) *loomCLI {
				record := allDiscussionSectionsContent() + addedDecisionEntryForTest
				return darnRecordFixture(t, dir, record, [][]byte{[]byte(record)}, new(int))
			},
			want: verdictDone,
		},
		{
			name: "Darn_Stuck_DroppedEntry",
			build: func(t *testing.T, dir string) *loomCLI {
				held := allDiscussionSectionsContent() + addedDecisionEntryForTest
				return darnRecordFixture(t, dir, allDiscussionSectionsContent(), [][]byte{[]byte(held)}, new(int))
			},
			want: verdictStuck,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.build(t, t.TempDir())

			gate := loomshed.NewDiscussionGate(loomshed.DiscussionCheck{
				RecordPath:     c.env.DecisionRecordPath,
				SupportLogPath: c.env.SupportLogPath,
				RelevantPath:   c.env.RelevantPath,
				WorktreeRoot:   c.env.WorktreeRoot,
				Index:          c.env.PlanIndex,
				History:        c.env.DiscussionHistory,
				Citations:      c.env.RecordEngine == "DiscussionWrite",
			})
			result, err := gate()
			pv := producerVerdict(result, err)

			var out bytes.Buffer
			exitCode := clihelp.Execute(c.validateDiscussionCmd(), &out, nil)
			cv := cliVerdict(envelope.Decode(t, out.String()))

			if pv != cv {
				t.Errorf(
					"parity mismatch for fixture %q: gate verdict = %q (result=%+v, err=%v); CLI verdict = %q (exit=%d, raw=%q)",
					tc.name, pv, result, err, cv, exitCode, out.String(),
				)
			}
			if pv != tc.want {
				t.Errorf("fixture %q: producer verdict = %q; want %q", tc.name, pv, tc.want)
			}
		})
	}
}

// planFixtureInvalidFormat writes a plan under <anchorPath>/_lym/plan/ whose frontmatter carries an
// unrecognized format value, tripping format-unrecognized regardless of mode.
// It returns a *loomCLI with only the two plan path fields set, the ones the validate-plan verb reads.
func planFixtureInvalidFormat(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
	t.Helper()

	files := plankit.Render(plankit.Plan{
		Approved: true,
		Language: "none",
		Cards: []plankit.Card{{
			Number:  1,
			Slug:    "validate-fixture",
			Summary: "a minimal fixture card",
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{"fixture-output.txt"}}},
			Intent:  "minimal fixture card for validate-plan tests.",
			Commit:  "1: validate-fixture",
		}},
	})
	recognized := fmt.Sprintf("format: %d\n", planparser.RecognizedFormat)
	overview := bytes.Replace(files["00-overview.md"], []byte(recognized), []byte("format: 1\n"), 1)
	if bytes.Equal(overview, files["00-overview.md"]) {
		t.Fatalf("overview does not carry %q", recognized)
	}
	files["00-overview.md"] = overview

	planDir := filepath.Join(anchorPath, "_lym", "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(planDir, name), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	return &loomCLI{location: hubLocationAt(t, anchorPath, ""), env: shedrecipe.Env{
		PlanIndex:    planglyph.NewIndex(fabricengine.NewReferenceRule()),
		AnchorPath:   anchorPath,
		WorktreeRoot: worktreeRoot,
	}}
}

// fabricReferenceCommand is a command that reaches a sibling worktree of the fabric repo.
// It is the shape the card-fabric-reference check refuses in a plan's verify: section.
var fabricReferenceCommand = "cat " + fabricengine.RecordsWorktree(&lymcwd.Location{HubPath: "/hub", WorktreeName: "task"}) + "/_lym/plan.md"

// verifySectionPlanFixture writes a language: none plan under <anchorPath>/_lym/plan/.
// Its verify: section runs verifyBody.
// It returns a *loomCLI wired with anchorPath and worktreeRoot.
func verifySectionPlanFixture(t *testing.T, anchorPath, worktreeRoot, verifyBody string) *loomCLI {
	t.Helper()
	return verifyPlanFixture(t, anchorPath, worktreeRoot, "none", verifyBody, "")
}

// verifyPlanFixture writes a plan under <anchorPath>/_lym/plan/ in the given language.
// Its verify: section runs verifyBody, and its sole card's `**Verify:**` line is cardVerify.
// It returns a *loomCLI wired with anchorPath and worktreeRoot.
func verifyPlanFixture(t *testing.T, anchorPath, worktreeRoot, language, verifyBody, cardVerify string) *loomCLI {
	t.Helper()

	plankit.Write(t, filepath.Join(anchorPath, "_lym", "plan"), plankit.Plan{
		Approved: true,
		Language: language,
		Sections: []plankit.Section{{Heading: "verify:", Body: verifyBody}},
		Cards: []plankit.Card{{
			Number:  1,
			Slug:    "validate-fixture",
			Summary: "a minimal fixture card",
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{"fixture-output.txt"}}},
			Intent:  "minimal fixture card for validate-plan tests.",
			Commit:  "1: validate-fixture",
			Verify:  cardVerify,
		}},
	})

	return &loomCLI{location: hubLocationAt(t, anchorPath, ""), env: shedrecipe.Env{
		PlanIndex:    planglyph.NewIndex(fabricengine.NewReferenceRule()),
		AnchorPath:   anchorPath,
		WorktreeRoot: worktreeRoot,
	}}
}

// glyphRepoPlanFixture writes a syntactically complete, one-card language: go plan under <anchorPath>/_lym/plan/.
// The sole card's Create group targets createTarget, and a non-empty useTarget also goes in its Uses: field.
// That lets the resolve-backed half reach useTarget without the Uses target tripping the Create-side inversion.
// It returns a *loomCLI wired with anchorPath and worktreeRoot.
func glyphRepoPlanFixture(t *testing.T, anchorPath, worktreeRoot, createTarget, useTarget string) *loomCLI {
	t.Helper()

	var uses []string
	if useTarget != "" {
		uses = []string{useTarget}
	}
	plankit.Write(t, filepath.Join(anchorPath, "_lym", "plan"), plankit.Plan{
		Approved: true,
		Language: "go",
		Cards: []plankit.Card{{
			Number:  1,
			Slug:    "validate-fixture",
			Summary: "a minimal fixture card",
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{createTarget}}},
			Uses:    uses,
			Intent:  "minimal fixture card for validate-plan tests.",
			Commit:  "1: validate-fixture",
		}},
	})

	return &loomCLI{location: hubLocationAt(t, anchorPath, ""), env: shedrecipe.Env{
		PlanIndex:    planglyph.NewIndex(fabricengine.NewReferenceRule()),
		AnchorPath:   anchorPath,
		WorktreeRoot: worktreeRoot,
	}}
}

// symbolRenamePlanFixture writes a one-card plan whose card renames a symbol.
// The plan needs the hub's rename tool.
// It returns a *loomCLI wired with anchorPath and worktreeRoot.
func symbolRenamePlanFixture(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
	t.Helper()

	plankit.Write(t, filepath.Join(anchorPath, "_lym", "plan"), plankit.Plan{
		Approved: true,
		Language: "none",
		Sections: []plankit.Section{{Heading: "Rename mechanic", Body: "1. Run the rename tool."}},
		Cards: []plankit.Card{{
			Number:  1,
			Slug:    "validate-fixture",
			Summary: "a minimal fixture card",
			Groups:  []plankit.Group{{Label: "Rename", Targets: []string{"`sub#Foo` -> `plan:sub#Bar`"}}},
			Intent:  "minimal fixture card for validate-plan tests.",
			Commit:  "1: validate-fixture",
		}},
	})

	return &loomCLI{env: shedrecipe.Env{
		PlanIndex:    planglyph.NewIndex(fabricengine.NewReferenceRule()),
		AnchorPath:   anchorPath,
		WorktreeRoot: worktreeRoot,
	}}
}

// writeLensReview writes content as the plan directory's lens review file under anchorPath.
func writeLensReview(t *testing.T, anchorPath, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(planparser.PlanDir(anchorPath), planparser.LensReviewFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write lens review: %v", err)
	}
}

// planParityCase is one fixture for TestGateParity_PlanGate.
// build populates anchorPath and worktreeRoot as needed.
// It returns the single *loomCLI that the gate closure and the verb both read their paths from.
// Both halves therefore run over the same on-disk fixture.
// wantGate and wantCLI are the verdicts each side must reach.
// They agree for every fixture except NoPlanDirectory, the one deliberate divergence.
type planParityCase struct {
	name     string
	build    func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI
	wantGate parityVerdict
	wantCLI  parityVerdict
	// wantFinding, when set, is a substring both the gate's findings and the verb's envelope must carry.
	wantFinding string
	// lensReview builds the gate with NewLensReviewPlanGate and runs the verb with --lens-review.
	lensReview bool
	// languageContent is the hub's language.yaml the verb loads; renameTools is what the gate is told, empty for none.
	languageContent string
	renameTools     []string
	// gateContent is the hub's gate.yaml the verb loads, empty for none; tierTags are the tier keys the gate is told.
	gateContent string
	tierTags    []string
	// seamTests tells the gate the seam reader over the fixture worktree, as the verb tells itself.
	seamTests bool
	// absentFinding, when set, is a substring neither the gate's findings nor the verb's envelope may carry.
	absentFinding string
}

// testTierPlanFixture writes a one-card go plan whose card creates sub/new_test.go and states no tier for it.
func testTierPlanFixture(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
	t.Helper()

	plankit.WriteTree(t, worktreeRoot, map[string]string{"sub/a.go": "package sub\n"})
	plankit.Write(t, planparser.PlanDir(anchorPath), plankit.Plan{
		Approved: true,
		Language: "go",
		Cards: []plankit.Card{{
			Number:  1,
			Slug:    "validate-fixture",
			Summary: "a minimal fixture card",
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{"sub/new_test.go"}}},
			Intent:  "minimal fixture card for validate-plan tests.",
			Commit:  "1: validate-fixture",
		}},
	})

	return &loomCLI{env: shedrecipe.Env{
		PlanIndex:    planglyph.NewIndex(fabricengine.NewReferenceRule()),
		AnchorPath:   anchorPath,
		WorktreeRoot: worktreeRoot,
	}}
}

// TestGateParity_PlanGate drives NewPlanGate and the validate-plan verb, flag-absent mode.
// It also drives NewLensReviewPlanGate and the verb's --lens-review mode.
// Each pair runs over the same fixture set, and the two mapped verdicts must agree.
//
// Both plan gate sites run planglyph.ValidateFormat, and neither has a --require-approved counterpart.
// That flag's guarantee rests on the approve seam failing loudly if it is ever wired nil;
// internal/loomrecipe/approveseam_test.go covers that.
// validate_test.go covers --require-approved's CLI behaviour in isolation.
// This file's subject is parity with a gate, and no gate answers to that flag.
//
// The fixtures cover a clean approved plan, a clean unapproved plan, and a format-invalid plan.
// They also cover an absent plan directory, a glyph-not-resolving plan, and an informational-only plan.
// A plan listing a test file under Edit and a quarry-unavailable plan complete the set.
//
// The Unapproved case is the load-bearing one: it must expect done, not stuck.
// planglyph.ValidateFormat never runs the plan-unapproved check,
// so an unapproved plan is a clean pre-review pass on both sides.
func TestGateParity_PlanGate(t *testing.T) {
	tiergate.Require(t, 0)
	cases := []planParityCase{
		{
			name: "CleanApproved",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				return planFixture(t, anchorPath, worktreeRoot, true)
			},
			wantGate: verdictDone,
			wantCLI:  verdictDone,
		},
		{
			name: "Unapproved",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				return planFixture(t, anchorPath, worktreeRoot, false)
			},
			wantGate: verdictDone,
			wantCLI:  verdictDone,
		},
		{
			name: "FormatInvalid",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				return planFixtureInvalidFormat(t, anchorPath, worktreeRoot)
			},
			wantGate: verdictStuck,
			wantCLI:  verdictStuck,
		},
		{
			// FabricReference puts a command that reaches the fabric repo in the verify: section.
			// The gate and the verb both report card-fabric-reference.
			name: "FabricReference",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				return verifySectionPlanFixture(t, anchorPath, worktreeRoot, fabricReferenceCommand)
			},
			wantGate:    verdictStuck,
			wantCLI:     verdictStuck,
			wantFinding: "card-fabric-reference",
		},
		{
			// VerifyNestedModule gives the card a Verify: that tests a nested module's packages from the root.
			// The gate and the verb both report verify-nested-module.
			name: "VerifyNestedModule",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				plankit.WriteTree(t, worktreeRoot, map[string]string{"nested/go.mod": "module nested\n"})
				return verifyPlanFixture(t, anchorPath, worktreeRoot, "go", "", "go test ./nested/...")
			},
			wantGate:    verdictStuck,
			wantCLI:     verdictStuck,
			wantFinding: "verify-nested-module",
		},
		{
			// VerifyModuleWide gives the card a module-wide Verify: the gate and the verb both report verify-module-wide.
			name: "VerifyModuleWide",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				return verifyPlanFixture(t, anchorPath, worktreeRoot, "none", "go build ./internal/x", "go test ./...")
			},
			wantGate:    verdictStuck,
			wantCLI:     verdictStuck,
			wantFinding: "verify-module-wide",
		},
		{
			// PatternEntryLineCap fences a PATTERN.md entry line over the cap in the card.
			// The gate and the verb both report pattern-entry-line-cap.
			name: "PatternEntryLineCap",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				c := planFixture(t, anchorPath, worktreeRoot, true)
				cardFile := filepath.Join(anchorPath, "_lym", "plan", "01-validate-fixture.md")
				data, err := os.ReadFile(cardFile)
				if err != nil {
					t.Fatalf("read card file: %v", err)
				}
				overCap := "- `PATTERN-x` — " + strings.Repeat("x", pattern.MaxEntryTextChars+1)
				if err := os.WriteFile(cardFile, append(data, []byte("\n```\n"+overCap+"\n```\n")...), 0o644); err != nil {
					t.Fatalf("write card file: %v", err)
				}
				return c
			},
			wantGate:    verdictStuck,
			wantCLI:     verdictStuck,
			wantFinding: "pattern-entry-line-cap",
		},
		{
			// DoneCardEdited records a done batch whose card hash differs from the card file.
			// The gate and the verb both report done-card-edited.
			name: "DoneCardEdited",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				c := planFixture(t, anchorPath, worktreeRoot, true)
				st := &websterengine.State{Batches: map[int]*websterengine.BatchState{
					1: {
						Slug:       "validate-fixture",
						Cards:      []string{"01-validate-fixture"},
						Terminal:   true,
						Status:     websterengine.DigestStatusDone,
						CardHashes: map[string]string{"01-validate-fixture": "hash-of-an-earlier-card-file"},
					},
				}}
				if err := websterengine.SaveState(websterengine.Dir(anchorPath), websterengine.ScratchDir(anchorPath), st); err != nil {
					t.Fatalf("SaveState: %v", err)
				}
				return c
			},
			wantGate:    verdictStuck,
			wantCLI:     verdictStuck,
			wantFinding: "done-card-edited",
		},
		{
			// NoPlanDirectory is the one deliberate divergence: both sides call planglyph.ValidateFormat.
			// The split lies in the ParsePlan pre-step ahead of it, which the gate and the verb answer differently.
			// NewPlanGate reports a missing or malformed overview as findings (verdictStuck);
			// its bounce target is the live session that wrote the file, holding full context.
			// The verb returns the same error as a returned error (verdictError);
			// a standalone verb has no session to re-prompt.
			// Both dispositions are intended.
			// This case asserts them explicitly, rather than through the pv == cv comparison every other fixture uses.
			name: "NoPlanDirectory",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				return &loomCLI{location: hubLocationAt(t, anchorPath, ""), env: shedrecipe.Env{AnchorPath: anchorPath, WorktreeRoot: worktreeRoot, PlanIndex: planglyph.NewIndex(fabricengine.NewReferenceRule())}}
			},
			wantGate: verdictStuck,
			wantCLI:  verdictError,
		},
		{
			// GlyphNotResolving covers the resolve-backed side of the glyph check.
			// A Uses: entry naming an existing unit with an unresolved member yields the blocking glyph-not-found finding.
			// The case runs it over a real quarry-openable worktreeRoot.
			// The Create target is a brand-new, informational-only unit,
			// so the blocking verdict this case asserts comes from the Uses: side alone.
			name: "GlyphNotResolving",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				plankit.WriteTree(t, worktreeRoot, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
				return glyphRepoPlanFixture(t, anchorPath, worktreeRoot, "newpkg2#Baz", "sub#Missing")
			},
			wantGate: verdictStuck,
			wantCLI:  verdictStuck,
		},
		{
			// InformationalOnly covers a Create target that introduces a brand-new package.
			// It produces only the informational create-new-unit finding.
			// Both sides must read that severity the same way, so the case reaches done.
			// It does not bounce on a condition Plan-Write cannot fix.
			name: "InformationalOnly",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				plankit.WriteTree(t, worktreeRoot, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
				return glyphRepoPlanFixture(t, anchorPath, worktreeRoot, "newpkg3#Qux", "")
			},
			wantGate: verdictDone,
			wantCLI:  verdictDone,
		},
		{
			// EditTestFile pins that a card's plain test-file path under Edit yields edit-test-file on both sides.
			// ParsePlan canonicalizes that path into a file self glyph.
			name: "EditTestFile",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				plankit.WriteTree(t, worktreeRoot, map[string]string{"sub/a_test.go": "package sub\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n"})
				plankit.Write(t, planparser.PlanDir(anchorPath), plankit.Plan{
					Approved: true,
					Language: "go",
					Cards: []plankit.Card{{
						Number:        1,
						Slug:          "validate-fixture",
						Summary:       "a minimal fixture card",
						Groups:        []plankit.Group{{Label: "Edit", Targets: []string{"sub/a_test.go"}}},
						Intent:        "minimal fixture card for validate-plan tests.",
						ImpactSummary: "Touches the test file.",
						Commit:        "1: validate-fixture",
					}},
				})
				return &loomCLI{location: hubLocationAt(t, anchorPath, ""), env: shedrecipe.Env{
					PlanIndex:    planglyph.NewIndex(fabricengine.NewReferenceRule()),
					AnchorPath:   anchorPath,
					WorktreeRoot: worktreeRoot,
				}}
			},
			wantGate:    verdictStuck,
			wantCLI:     verdictStuck,
			wantFinding: "edit-test-file",
		},
		{
			// QuarryUnavailable points both sides at a worktreeRoot that is not a repository, under language: go.
			// The declaration separates this fixture from the language: none fixtures above.
			// Those share the same kind of non-repository root and must stay clean.
			// Both sides must report the same infrastructure-error disposition.
			// The case proves that disposition is symmetric across the pair, not merely implemented twice.
			name: "QuarryUnavailable",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				badRoot := filepath.Join(t.TempDir(), "does-not-exist")
				return glyphRepoPlanFixture(t, anchorPath, badRoot, "sub#Foo", "")
			},
			wantGate: verdictError,
			wantCLI:  verdictError,
		},
		{
			name:       "LensReviewMissing",
			lensReview: true,
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				return planFixture(t, anchorPath, worktreeRoot, true)
			},
			wantGate:    verdictStuck,
			wantCLI:     verdictStuck,
			wantFinding: "lens-review-missing",
		},
		{
			name:       "LensReviewRecorded",
			lensReview: true,
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				c := planFixture(t, anchorPath, worktreeRoot, true)
				writeLensReview(t, anchorPath, "The lens review.\n")
				return c
			},
			wantGate: verdictDone,
			wantCLI:  verdictDone,
		},
		{
			// LensReviewEmptyIgnoredWithoutTheFlag pins that validate-plan without the flag and NewPlanGate ignore the file.
			name: "LensReviewEmptyIgnoredWithoutTheFlag",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				c := planFixture(t, anchorPath, worktreeRoot, true)
				writeLensReview(t, anchorPath, "")
				return c
			},
			wantGate: verdictDone,
			wantCLI:  verdictDone,
		},
		{
			name: "SymbolRenameWithNoRenameTool",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				return symbolRenamePlanFixture(t, anchorPath, worktreeRoot)
			},
			wantGate:    verdictStuck,
			wantCLI:     verdictStuck,
			wantFinding: "rename-tool-missing",
		},
		{
			// The verb reads the hub's language.yaml itself, so a listed tool clears the finding on both sides.
			name: "SymbolRenameWithARenameTool",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				return symbolRenamePlanFixture(t, anchorPath, worktreeRoot)
			},
			languageContent: "rename_tools:\n  - gopls rename\n",
			renameTools:     []string{"gopls rename"},
			wantGate:        verdictDone,
			wantCLI:         verdictDone,
			absentFinding:   "rename-tool-missing",
		},
		{
			name:        "CreatedTestFileWithNoTier",
			build:       testTierPlanFixture,
			gateContent: gateConfigWithTwoTiers(),
			tierTags:    []string{"tier0", "tier1"},
			wantGate:    verdictStuck,
			wantCLI:     verdictStuck,
			wantFinding: "test-tier-unstated",
		},
		{
			name:       "CreatedTestFileWithNoTierUnderLensReview",
			lensReview: true,
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				c := testTierPlanFixture(t, anchorPath, worktreeRoot)
				writeLensReview(t, anchorPath, "The lens review.\n")
				return c
			},
			gateContent: gateConfigWithTwoTiers(),
			tierTags:    []string{"tier0", "tier1"},
			wantGate:    verdictStuck,
			wantCLI:     verdictStuck,
			wantFinding: "test-tier-unstated",
		},
		{
			name:        "MarkedStructEditedWithoutItsSeamTest",
			build:       seamTestPlanFixture,
			seamTests:   true,
			wantGate:    verdictStuck,
			wantCLI:     verdictStuck,
			wantFinding: "seam-test-unlisted",
		},
		{
			name:       "MarkedStructEditedWithoutItsSeamTestUnderLensReview",
			lensReview: true,
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				c := seamTestPlanFixture(t, anchorPath, worktreeRoot)
				writeLensReview(t, anchorPath, "The lens review.\n")
				return c
			},
			seamTests:   true,
			wantGate:    verdictStuck,
			wantCLI:     verdictStuck,
			wantFinding: "seam-test-unlisted",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			anchorPath := t.TempDir()
			worktreeRoot := t.TempDir()
			c := tc.build(t, anchorPath, worktreeRoot)
			c.location = hubLocationAt(t, anchorPath, tc.languageContent)
			if tc.gateContent != "" {
				writeHubGateConfig(t, c.location.HubPath, tc.gateContent)
			}
			newGate := loomshed.NewPlanGate
			var args []string
			if tc.lensReview {
				newGate = loomshed.NewLensReviewPlanGate
				args = []string{"--lens-review"}
			}
			planContext := loomshed.PlanContext{RenameTools: tc.renameTools, TierTags: tc.tierTags}
			if tc.seamTests {
				planContext.SeamTests = loomshed.SeamTestReader(c.env.WorktreeRoot)
			}
			gate := newGate(c.env.AnchorPath, c.env.WorktreeRoot, c.env.RunScratchDir, planglyph.NewIndex(fabricengine.NewReferenceRule()), planContext)
			result, err := gate()
			pv := producerVerdict(result, err)

			var out bytes.Buffer
			exitCode := clihelp.Execute(c.validatePlanCmd(), &out, args)
			cv := cliVerdict(envelope.Decode(t, out.String()))

			if tc.wantGate == tc.wantCLI {
				if pv != cv {
					t.Errorf(
						"parity mismatch for fixture %q: gate verdict = %q (result=%+v, err=%v); CLI verdict = %q (exit=%d, raw=%q)",
						tc.name, pv, result, err, cv, exitCode, out.String(),
					)
				}
			}
			if pv != tc.wantGate {
				t.Errorf("fixture %q: gate verdict = %q; want %q", tc.name, pv, tc.wantGate)
			}
			if cv != tc.wantCLI {
				t.Errorf("fixture %q: CLI verdict = %q; want %q", tc.name, cv, tc.wantCLI)
			}
			if tc.wantFinding != "" && (!strings.Contains(result.Findings, tc.wantFinding) || !strings.Contains(out.String(), tc.wantFinding)) {
				t.Errorf("fixture %q: want %q in both the gate findings %q and the verb output %q", tc.name, tc.wantFinding, result.Findings, out.String())
			}
			if tc.absentFinding != "" && (strings.Contains(result.Findings, tc.absentFinding) || strings.Contains(out.String(), tc.absentFinding)) {
				t.Errorf("fixture %q: want %q in neither the gate findings %q nor the verb output %q", tc.name, tc.absentFinding, result.Findings, out.String())
			}
		})
	}
}

// TestGenericVerbs_AcceptOptionalRunIDPositional asserts that each generic verb carries cobra.MaximumNArgs(1).
// Zero and one positional are accepted, and two are refused.
// Command() builds the whole cobra tree with no I/O of its own, and (*cobra.Command).Find is a pure tree traversal,
// so this test spawns no process and stays Tier 1.
//
//testtiming:keep pins run, step, status, pause and goto each accepting zero or one positional run-id and refusing two; its covering test builds the tree without asserting argument counts
func TestGenericVerbs_AcceptOptionalRunIDPositional(t *testing.T) {
	tiergate.Require(t, 0)
	root := Command()

	for _, name := range []string{"run", "step", "status", "pause", "goto"} {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := root.Find([]string{name})
			if err != nil {
				t.Fatalf("root.Find(%q) = %v; want nil", name, err)
			}
			if cmd.Args == nil {
				t.Fatalf("%s.Args = nil; want cobra.MaximumNArgs(1)", name)
			}

			if err := cmd.Args(cmd, nil); err != nil {
				t.Errorf("%s.Args(cmd, nil) = %v; want nil (zero positionals accepted)", name, err)
			}
			if err := cmd.Args(cmd, []string{"a-run-id"}); err != nil {
				t.Errorf("%s.Args(cmd, [%q]) = %v; want nil (one positional accepted)", name, "a-run-id", err)
			}
			if err := cmd.Args(cmd, []string{"a-run-id", "extra"}); err == nil {
				t.Errorf("%s.Args(cmd, [%q, %q]) = nil; want a refusal (two positionals)", name, "a-run-id", "extra")
			}
		})
	}
}

// TestGateParity_DescriptionGate drives landingshed.NewDescriptionGate and the validate-description
// verb over the same fixture set and asserts that the two mapped verdicts agree.
// A well-formed file maps to done, and a file with a Co-Authored-By line maps to stuck.
// A directory at the description path, a read failure other than not-exist, maps to error.
func TestGateParity_DescriptionGate(t *testing.T) {
	tiergate.Require(t, 0)
	cases := []discussionParityCase{
		{
			name: "Clean",
			build: func(t *testing.T, dir string) *loomCLI {
				return descriptionFixture(t, dir, "# Title\n\nbody\n")
			},
			want: verdictDone,
		},
		{
			name: "Stuck_CoAuthoredBy",
			build: func(t *testing.T, dir string) *loomCLI {
				return descriptionFixture(t, dir, "# Title\n\nbody\n\nCo-Authored-By: X <x@y>\n")
			},
			want: verdictStuck,
		},
		{
			name: "Error_DescriptionIsDirectory",
			build: func(t *testing.T, dir string) *loomCLI {
				c := descriptionFixture(t, dir, "# Title\n\nbody\n")
				if err := os.Remove(c.env.DescriptionPath); err != nil {
					t.Fatalf("remove description: %v", err)
				}
				if err := os.Mkdir(c.env.DescriptionPath, 0o755); err != nil {
					t.Fatalf("mkdir description: %v", err)
				}
				return c
			},
			want: verdictError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.build(t, t.TempDir())

			result, err := landingshed.NewDescriptionGate(c.env.DescriptionPath)()
			pv := producerVerdict(result, err)

			var out bytes.Buffer
			exitCode := clihelp.Execute(c.validateDescriptionCmd(), &out, nil)
			cv := cliVerdict(envelope.Decode(t, out.String()))

			if pv != cv {
				t.Errorf(
					"parity mismatch for fixture %q: gate verdict = %q (result=%+v, err=%v); CLI verdict = %q (exit=%d, raw=%q)",
					tc.name, pv, result, err, cv, exitCode, out.String(),
				)
			}
			if pv != tc.want {
				t.Errorf("fixture %q: producer verdict = %q; want %q", tc.name, pv, tc.want)
			}
		})
	}
}

// reworkParityFixture writes a one-card language: go plan under anchorPath.
// Its card is numbered cardNumber, sets first_card to cardNumber when that is above 1, and creates newpkg#Bar.
// It adds uses, an Edit group for edit, and a verify: section when each is non-empty.
// The returned *loomCLI's committed-file seam serves a one-card plan (card 1 creating sub#Foo) as the HEAD plan.
// It serves nothing when committed is false.
// The told number is therefore 2.
func reworkParityFixture(t *testing.T, anchorPath, worktreeRoot string, cardNumber int, uses, edit, verify string, committed bool) *loomCLI {
	t.Helper()
	groups := []plankit.Group{{Label: "Create", Targets: []string{"newpkg#Bar"}}}
	impactSummary := ""
	if edit != "" {
		groups = append(groups, plankit.Group{Label: "Edit", Targets: []string{edit}})
		impactSummary = "Touches the edited file."
	}
	var usesTargets []string
	if uses != "" {
		usesTargets = []string{uses}
	}
	firstCard := 0
	if cardNumber > 1 {
		firstCard = cardNumber
	}
	var sections []plankit.Section
	if verify != "" {
		sections = []plankit.Section{{Heading: "verify:", Body: verify}}
	}
	plankit.Write(t, planparser.PlanDir(anchorPath), plankit.Plan{
		Approved:  true,
		Language:  "go",
		FirstCard: firstCard,
		Framing:   "Framing.",
		Sections:  sections,
		Cards: []plankit.Card{{
			Number:        cardNumber,
			Slug:          "new-card",
			Summary:       fmt.Sprintf("placeholder card %d", cardNumber),
			Groups:        groups,
			Uses:          usesTargets,
			Intent:        "new generation card.",
			ImpactSummary: impactSummary,
		}},
	})

	head := map[string][]byte{}
	if committed {
		oldPlan := plankit.Render(plankit.Plan{
			Approved: true,
			Language: "go",
			Framing:  "Framing.",
			Cards: []plankit.Card{{
				Number:  1,
				Slug:    "first-card",
				Summary: "placeholder card 1",
				Groups:  []plankit.Group{{Label: "Create", Targets: []string{"sub#Foo"}}},
				Intent:  "placeholder card.",
			}},
		})
		for name, data := range oldPlan {
			head[path.Join(planparser.PlanDirRel(), name)] = data
		}
	}
	return &loomCLI{location: hubLocationAt(t, anchorPath, ""), env: shedrecipe.Env{
		PlanIndex:    planglyph.NewIndex(fabricengine.NewReferenceRule()),
		AnchorPath:   anchorPath,
		WorktreeRoot: worktreeRoot,
		Rework: loomshed.PRReworkDeps{ReadCommitted: func(rel string) ([]byte, bool, error) {
			data, ok := head[rel]
			return data, ok, nil
		}},
	}}
}

// seamTestPlanFixture writes a one-card language: go plan whose card edits the struct internal/s#Deps.
// A test in internal/c marks that struct with `//lym:seam`, and the plan does not list that test.
func seamTestPlanFixture(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
	t.Helper()

	plankit.WriteTree(t, worktreeRoot, map[string]string{
		"internal/s/s.go":      "package s\n\ntype Deps struct{}\n",
		"internal/c/c_test.go": "package c\n\nimport \"testing\"\n\n//lym:seam internal/s#Deps\nfunc TestDeps(t *testing.T) {}\n",
	})
	plankit.Write(t, planparser.PlanDir(anchorPath), plankit.Plan{
		Approved: true,
		Language: "go",
		Cards: []plankit.Card{{
			Number:        1,
			Slug:          "validate-fixture",
			Summary:       "a minimal fixture card",
			Groups:        []plankit.Group{{Label: "Edit", Targets: []string{"internal/s#Deps"}}},
			Intent:        "minimal fixture card for validate-plan tests.",
			ImpactSummary: "Touches the struct.",
			Commit:        "1: validate-fixture",
		}},
	})

	return &loomCLI{env: shedrecipe.Env{
		PlanIndex:    planglyph.NewIndex(fabricengine.NewReferenceRule()),
		AnchorPath:   anchorPath,
		WorktreeRoot: worktreeRoot,
	}}
}
