// parity_test.go asserts the Gate Self-Check Parity Invariant for the four gate sites the two
// gate closures cover: NewDiscussionGate against validate-discussion, and NewPlanGate against
// validate-plan in each of its two flag modes. Both the gate closure and its loomcli verb must
// reach the same three-way verdict over the identical fixture, because both sides call the
// identical package function per the shared-implementation-is-the-whole-point Shared Decision.
//
// The comparison is three-way, not binary, per the discussion's parity-tests-per-gate decision: the
// gate closure's Passed/error trio and the verb's ok/findings-key trio are each mapped onto one
// shared parityVerdict before comparison, so a Stuck-vs-error mismatch is caught rather than
// collapsed onto a single "fail" side. Both halves take told paths and construct no repository and
// spawn no process, real hub, or fixture-tree copy, and no test here calls RunCLIIn -- so both stay
// tier 1.
package loomcli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// parityVerdict is the shared three-valued outcome both the producer side and the CLI side map
// onto before comparison. Declaring it as a string-typed constant set (rather than comparing raw
// producer outcomes against raw exit codes) is what makes a mismatch failure message name the two
// sides' verdicts readably.
type parityVerdict string

const (
	verdictDone  parityVerdict = "done"
	verdictStuck parityVerdict = "stuck"
	verdictError parityVerdict = "error"
)

// producerVerdict maps a shuttleengine.Gate closure's raw (GateResult, error) onto a parityVerdict:
// a non-nil error is verdictError; GateResult.Passed == false is verdictStuck; GateResult.Passed ==
// true is verdictDone.
func producerVerdict(result shuttleengine.GateResult, err error) parityVerdict {
	if err != nil {
		return verdictError
	}
	if !result.Passed {
		return verdictStuck
	}
	return verdictDone
}

// cliVerdict maps a decoded verb envelope onto a parityVerdict: ok == true is verdictDone; ok ==
// false with a "findings" key present is verdictStuck; ok == false without a "findings" key is
// verdictError. The findings-key presence check is structural, per the envelope-and-exit-contract
// Shared Decision, never a matter of message wording.
func cliVerdict(env map[string]any) parityVerdict {
	ok, _ := env["ok"].(bool)
	if ok {
		return verdictDone
	}
	if _, hasFindings := env["findings"]; hasFindings {
		return verdictStuck
	}
	return verdictError
}

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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.build(t, t.TempDir())

			gate := loomshed.NewDiscussionGate(c.env.DecisionRecordPath, c.env.SupportLogPath)
			result, err := gate()
			pv := producerVerdict(result, err)

			var out bytes.Buffer
			exitCode := clihelp.Execute(c.validateDiscussionCmd(), &out, nil)
			env := decodeSingleEnvelope(t, out.String())
			cv := cliVerdict(env)

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

// planFixtureInvalidFormat writes a plan under <anchorPath>/_lyx/plan/ whose frontmatter carries an
// unrecognized format value, tripping format-unrecognized regardless of mode, and returns a
// *loomCLI wired only with the two plan path fields validatePlanCmd's RunE reads. It is a local
// helper distinct from validate_test.go's planFixture, which always writes the recognized format.
func planFixtureInvalidFormat(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
	t.Helper()

	planDir := filepath.Join(anchorPath, "_lyx", "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}

	overview := "---\n" +
		"format: 1\n" +
		"approved: true\n" +
		"root: \n" +
		"language: none\n" +
		"---\n\n" +
		"# Plan: format-invalid fixture\n\n" +
		"## Card Index\n\n" +
		"1 — validate-fixture — a minimal fixture card\n"
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview: %v", err)
	}

	card := "# Card 1 — validate-fixture\n\n" +
		"**Create:**\n" +
		"- `fixture-output.txt`\n\n" +
		"**Intent:** minimal fixture card for validate-plan tests.\n\n" +
		"**Commit:** `1: validate-fixture`\n"
	if err := os.WriteFile(filepath.Join(planDir, "01-validate-fixture.md"), []byte(card), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}

	return &loomCLI{env: shedrecipe.Env{
		AnchorPath:   anchorPath,
		WorktreeRoot: worktreeRoot,
	}}
}

// glyphRepoPlanFixture writes a syntactically complete, one-card language: go plan under
// <anchorPath>/_lyx/plan/ whose sole card's Create group targets createTarget and, when useTarget
// is non-empty, whose Uses: field also names useTarget, and returns a *loomCLI wired with
// anchorPath and worktreeRoot -- duplicated from internal/loomcli/validate_test.go's own
// glyphPlanFixture per the duplicate-test-helpers-rather-than-share-them Shared Decision, extended
// with the optional Uses: field this parity table's fifth and sixth fixtures both need to reach the
// resolve-backed half without the Uses target also tripping the Create-side inversion.
func glyphRepoPlanFixture(t *testing.T, anchorPath, worktreeRoot, createTarget, useTarget string) *loomCLI {
	t.Helper()

	planDir := filepath.Join(anchorPath, "_lyx", "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}

	overview := "---\n" +
		"format: 5\n" +
		"approved: true\n" +
		"language: go\n" +
		"---\n\n" +
		"# Plan: minimal glyph parity fixture\n\n" +
		"## Card Index\n\n" +
		"1 — validate-fixture — a minimal fixture card\n"
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview: %v", err)
	}

	usesBlock := ""
	if useTarget != "" {
		usesBlock = fmt.Sprintf("\n**Uses:**\n- `%s`\n", useTarget)
	}
	card := fmt.Sprintf(
		"# Card 1 — validate-fixture\n\n**Create:**\n- `%s`\n%s\n**Intent:** minimal fixture card for validate-plan tests.\n\n**Commit:** `1: validate-fixture`\n",
		createTarget, usesBlock,
	)
	if err := os.WriteFile(filepath.Join(planDir, "01-validate-fixture.md"), []byte(card), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}

	return &loomCLI{env: shedrecipe.Env{
		AnchorPath:   anchorPath,
		WorktreeRoot: worktreeRoot,
	}}
}

// writeGlyphRepoForParityTest writes files (keyed by repository-relative path) under dir --
// duplicated from internal/planglyph/repo_test.go's writeFixtureRepo per the
// duplicate-test-helpers-rather-than-share-them Shared Decision.
func writeGlyphRepoForParityTest(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) failed: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", full, err)
		}
	}
}

// planParityCase is one fixture for TestGateParity_PlanGate: build populates anchorPath and
// worktreeRoot as needed and returns the single *loomCLI both the gate closure and the verb read
// their paths from, so both halves run over the exact same on-disk fixture. wantGate and wantCLI
// carry the verdict each side must reach; they agree for every fixture but NoPlanDirectory, the one
// deliberate, documented divergence (see that case's own comment).
type planParityCase struct {
	name     string
	build    func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI
	wantGate parityVerdict
	wantCLI  parityVerdict
}

// TestGateParity_PlanGate drives NewPlanGate and the validate-plan verb, flag-absent mode only, over
// the same fixture set and asserts the two mapped verdicts agree. The mode cross-product this test
// used to drive is gone: both plan gate sites (Plan-Write's own gate and Plan-Burler's own gate) run
// planglyph.ValidateFormat and neither has a --require-approved counterpart -- that flag's own
// guarantee now rests entirely on the approve seam failing loudly if it is ever wired nil (see
// approveseam_test.go), not on any row re-checking the flag. validate_test.go covers
// --require-approved's own CLI behaviour in isolation; this file's subject is parity with a gate,
// and no gate answers to that flag.
//
// The fixtures are: a clean approved plan, a clean unapproved plan, a format-invalid plan, an absent
// plan directory, a glyph-not-resolving plan, an informational-only plan, and a quarry-unavailable
// plan.
//
// The Unapproved case is the load-bearing one: it must expect done, not stuck, because that is
// exactly what proves the F7 deadlock is gone -- planglyph.ValidateFormat never runs the
// plan-unapproved check, so an unapproved plan is a clean pre-review pass on both sides.
func TestGateParity_PlanGate(t *testing.T) {
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
			// NoPlanDirectory is the one expected divergence the Gate Self-Check Parity Invariant's
			// own rewrite carves out: it binds the two sides to the same package FUNCTION, which both
			// still call here -- planglyph.ValidateFormat -- and the divergence lives strictly in the
			// ParsePlan pre-step ahead of it, whose disposition the gate deliberately reverses.
			// NewPlanGate's ParsePlan carve-out (gates.go) routes a missing or malformed overview to
			// findings (verdictStuck) because its bounce target is the live session that just wrote
			// the file, holding full context; the verb keeps returning the identical error as a
			// returned error (verdictError) because a standalone verb has no session to re-prompt.
			// Both dispositions are reasoned, not accidental, so this case is asserted explicitly
			// rather than folded into the pv == cv comparison every other fixture uses.
			name: "NoPlanDirectory",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				return &loomCLI{env: shedrecipe.Env{AnchorPath: anchorPath, WorktreeRoot: worktreeRoot}}
			},
			wantGate: verdictStuck,
			wantCLI:  verdictError,
		},
		{
			// GlyphNotResolving covers the resolve-backed half the move onto planglyph introduces: a
			// Uses: entry naming a unit that exists but a member that does not resolves the blocking
			// glyph-not-found finding over a real quarry-openable worktreeRoot. The Create target is
			// its own brand-new, informational-only unit, so the blocking verdict this case asserts is
			// attributable to the Uses: side alone.
			name: "GlyphNotResolving",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				writeGlyphRepoForParityTest(t, worktreeRoot, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
				return glyphRepoPlanFixture(t, anchorPath, worktreeRoot, "newpkg2#Baz", "sub#Missing")
			},
			wantGate: verdictStuck,
			wantCLI:  verdictStuck,
		},
		{
			// InformationalOnly covers the cell that would have caught the bounce loop had it
			// existed before the move: a Create target introducing a brand-new package produces only
			// the informational create-new-unit finding, and both sides must read that severity the
			// same way, reaching done rather than bouncing on a condition Plan-Write cannot fix.
			name: "InformationalOnly",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				writeGlyphRepoForParityTest(t, worktreeRoot, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
				return glyphRepoPlanFixture(t, anchorPath, worktreeRoot, "newpkg3#Qux", "")
			},
			wantGate: verdictDone,
			wantCLI:  verdictDone,
		},
		{
			// QuarryUnavailable points both sides at a worktreeRoot that is not a repository, under
			// language: go rather than language: none -- the declaration is what makes this fixture
			// distinguishable from the four language: none fixtures above, which share the same kind
			// of non-repository root and must stay clean. Both sides must report the same
			// infrastructure-error disposition, proving it is symmetric across the pair rather than
			// merely implemented twice.
			name: "QuarryUnavailable",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				badRoot := filepath.Join(t.TempDir(), "does-not-exist")
				return glyphRepoPlanFixture(t, anchorPath, badRoot, "sub#Foo", "")
			},
			wantGate: verdictError,
			wantCLI:  verdictError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			anchorPath := t.TempDir()
			worktreeRoot := t.TempDir()
			c := tc.build(t, anchorPath, worktreeRoot)

			gate := loomshed.NewPlanGate(c.env.AnchorPath, c.env.WorktreeRoot)
			result, err := gate()
			pv := producerVerdict(result, err)

			var out bytes.Buffer
			exitCode := clihelp.Execute(c.validatePlanCmd(), &out, nil)
			env := decodeSingleEnvelope(t, out.String())
			cv := cliVerdict(env)

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
		})
	}
}

// TestGenericVerbs_AcceptOptionalRunIDPositional asserts each of loom's four generic verbs --
// run, step, status, pause -- independently carries cobra.MaximumNArgs(1): zero and one
// positional are both accepted, and two are refused. Command() builds the whole cobra tree with no
// I/O of its own, and (*cobra.Command).Find is a pure tree traversal, so this test spawns no
// process and stays Tier 1.
func TestGenericVerbs_AcceptOptionalRunIDPositional(t *testing.T) {
	root := Command()

	for _, name := range []string{"run", "step", "status", "pause"} {
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
