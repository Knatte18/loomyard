// parity_test.go asserts the Gate Self-Check Parity Invariant for both mechanical gates shipped so
// far: the Discussion-Validate and Plan-Validate producers and their loomcli verbs must reach the
// same three-way verdict over the identical fixture, because both sides call the identical package
// function per the shared-implementation-is-the-whole-point Shared Decision.
//
// The comparison is three-way, not binary, per the discussion's parity-tests-per-gate decision: the
// producer's Done/Stuck/error trio and the verb's ok/findings-key trio are each mapped onto one
// shared parityVerdict before comparison, so a Stuck-vs-error mismatch is caught rather than
// collapsed onto a single "fail" side. Both halves take told paths and construct no repository and
// spawn no process, real hub, or fixture-tree copy, and no test here calls RunCLIIn -- so both stay
// tier 1.
package loomcli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
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

// producerVerdict maps a shedengine.ShedProducer's raw outcome/error onto a parityVerdict:
// shedengine.Done with a nil error is verdictDone; shedengine.Stuck with a nil error is
// verdictStuck; a non-nil error (regardless of outcome) is verdictError.
func producerVerdict(outcome shedengine.Outcome, err error) parityVerdict {
	if err != nil {
		return verdictError
	}
	if outcome == shedengine.Stuck {
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

// discussionParityCase is one fixture for TestGateParity_DiscussionValidate: build populates dir
// with whatever the fixture needs and returns the single *loomCLI both the producer and the verb
// read their paths from, so both halves run over the exact same on-disk fixture. want names the
// verdict both sides must reach.
type discussionParityCase struct {
	name  string
	build func(t *testing.T, dir string) *loomCLI
	want  parityVerdict
}

// TestGateParity_DiscussionValidate drives the Discussion-Validate producer and the
// validate-discussion verb over the same fixture set and asserts the two mapped verdicts agree,
// covering all three parityVerdict values: a clean fixture (done), a missing-support-log fixture
// and a missing-heading fixture (stuck), and a decision-record-is-a-directory fixture (error).
func TestGateParity_DiscussionValidate(t *testing.T) {
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

			producer := loomshed.NewDiscussionValidate("Discussion-Validate", c.env.DecisionRecordPath, c.env.SupportLogPath)
			outcome, _, err := producer.Call(context.Background())
			pv := producerVerdict(outcome, err)

			var out bytes.Buffer
			exitCode := clihelp.Execute(c.validateDiscussionCmd(), &out, nil)
			env := decodeSingleEnvelope(t, out.String())
			cv := cliVerdict(env)

			if pv != cv {
				t.Errorf(
					"parity mismatch for fixture %q: producer verdict = %q (outcome=%q, err=%v); CLI verdict = %q (exit=%d, raw=%q)",
					tc.name, pv, outcome, err, cv, exitCode, out.String(),
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

// planParityCase is one fixture for TestGateParity_PlanValidate: build populates anchorPath and
// worktreeRoot as needed and returns the single *loomCLI both the producer and the verb read their
// paths from, so both halves run over the exact same on-disk fixture. want carries the verdict both
// sides must reach in each of the two modes, keyed by whether requireApproved is set.
type planParityCase struct {
	name              string
	build             func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI
	wantFlagAbsent    parityVerdict
	wantRequireApprov parityVerdict
}

// TestGateParity_PlanValidate drives the Plan-Validate/Plan-Revalidate producer and the validate-plan
// verb over the same fixture set, crossed with both modes the two rows share, and asserts the two
// mapped verdicts agree in each cell. The four fixtures are: a clean approved plan, a clean
// unapproved plan, a format-invalid plan, and an absent plan directory.
//
// The Stuck_Unapproved-turned-Unapproved cell is the load-bearing one: in the flag-absent mode it
// must expect done, not stuck, because that is exactly what proves the F7 deadlock is gone --
// planparser.ValidateFormat never runs the plan-unapproved check, so an unapproved plan is a clean
// pre-review pass. The same fixture in --require-approved mode still expects stuck, because that
// mode is what Plan-Revalidate uses to confirm the flag landed after review.
func TestGateParity_PlanValidate(t *testing.T) {
	cases := []planParityCase{
		{
			name: "CleanApproved",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				return planFixture(t, anchorPath, worktreeRoot, true)
			},
			wantFlagAbsent:    verdictDone,
			wantRequireApprov: verdictDone,
		},
		{
			name: "Unapproved",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				return planFixture(t, anchorPath, worktreeRoot, false)
			},
			wantFlagAbsent:    verdictDone,
			wantRequireApprov: verdictStuck,
		},
		{
			name: "FormatInvalid",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				return planFixtureInvalidFormat(t, anchorPath, worktreeRoot)
			},
			wantFlagAbsent:    verdictStuck,
			wantRequireApprov: verdictStuck,
		},
		{
			name: "NoPlanDirectory",
			build: func(t *testing.T, anchorPath, worktreeRoot string) *loomCLI {
				return &loomCLI{env: shedrecipe.Env{AnchorPath: anchorPath, WorktreeRoot: worktreeRoot}}
			},
			wantFlagAbsent:    verdictError,
			wantRequireApprov: verdictError,
		},
	}

	modes := []struct {
		name            string
		requireApproved bool
		args            []string
		want            func(tc planParityCase) parityVerdict
	}{
		{
			name:            "FlagAbsent",
			requireApproved: false,
			args:            nil,
			want:            func(tc planParityCase) parityVerdict { return tc.wantFlagAbsent },
		},
		{
			name:            "RequireApproved",
			requireApproved: true,
			args:            []string{"--require-approved"},
			want:            func(tc planParityCase) parityVerdict { return tc.wantRequireApprov },
		},
	}

	for _, tc := range cases {
		for _, mode := range modes {
			t.Run(tc.name+"/"+mode.name, func(t *testing.T) {
				anchorPath := t.TempDir()
				worktreeRoot := t.TempDir()
				c := tc.build(t, anchorPath, worktreeRoot)

				producer := loomshed.NewPlanValidate("Plan-Validate", c.env.AnchorPath, c.env.WorktreeRoot, mode.requireApproved)
				outcome, _, err := producer.Call(context.Background())
				pv := producerVerdict(outcome, err)

				var out bytes.Buffer
				exitCode := clihelp.Execute(c.validatePlanCmd(), &out, mode.args)
				env := decodeSingleEnvelope(t, out.String())
				cv := cliVerdict(env)

				want := mode.want(tc)

				if pv != cv {
					t.Errorf(
						"parity mismatch for fixture %q mode %q: producer verdict = %q (outcome=%q, err=%v); CLI verdict = %q (exit=%d, raw=%q)",
						tc.name, mode.name, pv, outcome, err, cv, exitCode, out.String(),
					)
				}
				if pv != want {
					t.Errorf("fixture %q mode %q: producer verdict = %q; want %q", tc.name, mode.name, pv, want)
				}
			})
		}
	}
}
