// validate_test.go covers validate-discussion and validate-plan's clean, findings, and I/O-fault
// cases: exit code, exactly-one-JSON-line output, and the decoded envelope's ok value and findings
// key presence. Both verbs are driven directly against the leaf command built by
// validateDiscussionCmd/validatePlanCmd on a hand-populated *loomCLI, per the
// cli-tests-never-go-through-runcliin Shared Decision -- never through RunCLIIn or wire. Every case
// is tier 1: it spawns no subprocess, opens no real hub fixture, and sleeps on no real clock.

package loomcli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// requiredDiscussionSectionsForTest mirrors internal/discussionparser's own unexported
// requiredDiscussionSections list. It is duplicated here (rather than imported, since the source is
// unexported) so this suite can build a decision record carrying every required heading.
var requiredDiscussionSectionsForTest = []string{
	"## Goal",
	"## Scope",
	"## Decisions",
	"## Constraints",
	"## Auto-mode assumptions",
	"## Open risks",
	"## Acceptance criteria",
}

// decisionRecordFixture writes decisionContent and supportContent under dir, and returns a *loomCLI
// wired only with the two discussion path fields validateDiscussionCmd's RunE reads.
func decisionRecordFixture(t *testing.T, dir, decisionContent, supportContent string, writeSupport bool) *loomCLI {
	t.Helper()

	decisionPath := filepath.Join(dir, "decision-record.md")
	if err := os.WriteFile(decisionPath, []byte(decisionContent), 0o644); err != nil {
		t.Fatalf("write decision record: %v", err)
	}
	supportPath := filepath.Join(dir, "support-log.md")
	if writeSupport {
		if err := os.WriteFile(supportPath, []byte(supportContent), 0o644); err != nil {
			t.Fatalf("write support log: %v", err)
		}
	}

	return &loomCLI{env: shedrecipe.Env{
		DecisionRecordPath: decisionPath,
		SupportLogPath:     supportPath,
	}}
}

// allDiscussionSectionsContent returns a decision record body carrying every heading in
// requiredDiscussionSectionsForTest, in order, each with a one-line body.
func allDiscussionSectionsContent() string {
	var b strings.Builder
	for _, h := range requiredDiscussionSectionsForTest {
		b.WriteString(h)
		b.WriteString("\n\nbody text.\n\n")
	}
	return b.String()
}

func TestValidateDiscussionCmd(t *testing.T) {
	tests := []struct {
		name         string
		build        func(dir string) *loomCLI
		wantExit     int
		wantOK       bool
		wantFindings bool
		wantContains string
	}{
		{
			name: "Clean",
			build: func(dir string) *loomCLI {
				return decisionRecordFixture(t, dir, allDiscussionSectionsContent(), "support log body", true)
			},
			wantExit:     0,
			wantOK:       true,
			wantFindings: false,
		},
		{
			name: "Findings_SupportLogAbsent",
			build: func(dir string) *loomCLI {
				return decisionRecordFixture(t, dir, allDiscussionSectionsContent(), "", false)
			},
			wantExit:     1,
			wantOK:       false,
			wantFindings: true,
			wantContains: "support log does not exist",
		},
		{
			name: "Findings_HeadingMissing",
			build: func(dir string) *loomCLI {
				var b strings.Builder
				for _, h := range requiredDiscussionSectionsForTest {
					if h == "## Constraints" {
						continue
					}
					b.WriteString(h)
					b.WriteString("\n\nbody text.\n\n")
				}
				return decisionRecordFixture(t, dir, b.String(), "support log body", true)
			},
			wantExit:     1,
			wantOK:       false,
			wantFindings: true,
			wantContains: "## Constraints",
		},
		{
			name: "IOFault_DecisionRecordIsDirectory",
			build: func(dir string) *loomCLI {
				c := decisionRecordFixture(t, dir, allDiscussionSectionsContent(), "support log body", true)
				// Overwrite the decision record path with a directory so os.ReadFile fails with a
				// non-not-exist error, per the short-circuit-order-is-load-bearing Shared Decision.
				if err := os.Remove(c.env.DecisionRecordPath); err != nil {
					t.Fatalf("remove decision record: %v", err)
				}
				if err := os.Mkdir(c.env.DecisionRecordPath, 0o755); err != nil {
					t.Fatalf("mkdir decision record: %v", err)
				}
				return c
			},
			wantExit: 1,
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			c := tt.build(dir)

			var out bytes.Buffer
			exitCode := clihelp.Execute(c.validateDiscussionCmd(), &out, nil)

			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d; want %d (output: %q)", exitCode, tt.wantExit, out.String())
			}

			env := decodeSingleEnvelope(t, out.String())

			if ok, _ := env["ok"].(bool); ok != tt.wantOK {
				t.Errorf("envelope ok = %v; want %v", env["ok"], tt.wantOK)
			}

			findings, hasFindings := env["findings"]
			if hasFindings != tt.wantFindings {
				t.Errorf("envelope findings key present = %v; want %v (envelope: %v)", hasFindings, tt.wantFindings, env)
			}
			if tt.wantContains != "" {
				if !envelopeContains(findings, tt.wantContains) && !strings.Contains(out.String(), tt.wantContains) {
					t.Errorf("envelope does not mention %q; got: %v", tt.wantContains, env)
				}
			}
		})
	}
}

// planFixture writes a minimal, genuinely valid format-4 plan under <anchorPath>/_lyx/plan/,
// materializing every path the card names under worktreeRoot, then returns a *loomCLI wired only
// with the two plan path fields validatePlanCmd's RunE reads. approved controls the overview
// frontmatter's approved: value, so callers can trip plan-unapproved without touching anything
// else. The sole card is a format-4 Create card carrying a pinned Commit: value.
func planFixture(t *testing.T, anchorPath, worktreeRoot string, approved bool) *loomCLI {
	t.Helper()

	planDir := filepath.Join(anchorPath, "_lyx", "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}

	overview := "---\n" +
		"format: 4\n" +
		"approved: " + strconvBool(approved) + "\n" +
		"root: \n" +
		"---\n\n" +
		"# Plan: minimal validate-plan fixture\n\n" +
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

// strconvBool renders b as the bare YAML boolean literal "true" or "false".
func strconvBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// TestValidatePlanCmd_RequireApprovedFlagRegistered asserts the flag itself is registered on the
// command under its exact name with its documented default, since the repo's help-tree tests assert
// subcommand names rather than flags and would not catch a flag that failed to register.
func TestValidatePlanCmd_RequireApprovedFlagRegistered(t *testing.T) {
	c := &loomCLI{}
	flag := c.validatePlanCmd().Flags().Lookup("require-approved")
	if flag == nil {
		t.Fatal(`flag "require-approved" is not registered on validate-plan`)
	}
	if flag.DefValue != "false" {
		t.Errorf(`flag "require-approved" default = %q; want "false"`, flag.DefValue)
	}
}

func TestValidatePlanCmd(t *testing.T) {
	tests := []struct {
		name             string
		build            func(anchorPath, worktreeRoot string) *loomCLI
		wantExitAbsent   int
		wantOKAbsent     bool
		wantFindAbsent   bool
		wantExitRequire  int
		wantOKRequire    bool
		wantFindRequire  bool
		wantFindingsName string
	}{
		{
			name: "Clean",
			build: func(anchorPath, worktreeRoot string) *loomCLI {
				return planFixture(t, anchorPath, worktreeRoot, true)
			},
			wantExitAbsent:  0,
			wantOKAbsent:    true,
			wantFindAbsent:  false,
			wantExitRequire: 0,
			wantOKRequire:   true,
			wantFindRequire: false,
		},
		{
			// Unapproved is the load-bearing case: the default (flag-absent) mode now succeeds --
			// planparser.ValidateFormat never runs the plan-unapproved check -- while
			// --require-approved still reports the plan-unapproved finding, matching Plan-Revalidate.
			name: "Unapproved",
			build: func(anchorPath, worktreeRoot string) *loomCLI {
				return planFixture(t, anchorPath, worktreeRoot, false)
			},
			wantExitAbsent:   0,
			wantOKAbsent:     true,
			wantFindAbsent:   false,
			wantExitRequire:  1,
			wantOKRequire:    false,
			wantFindRequire:  true,
			wantFindingsName: "plan-unapproved",
		},
		{
			name: "ParseFault_NoPlanDirectory",
			build: func(anchorPath, worktreeRoot string) *loomCLI {
				return &loomCLI{env: shedrecipe.Env{
					AnchorPath:   anchorPath,
					WorktreeRoot: worktreeRoot,
				}}
			},
			wantExitAbsent:  1,
			wantOKAbsent:    false,
			wantFindAbsent:  false,
			wantExitRequire: 1,
			wantOKRequire:   false,
			wantFindRequire: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/FlagAbsent", func(t *testing.T) {
			anchorPath := t.TempDir()
			worktreeRoot := t.TempDir()
			c := tt.build(anchorPath, worktreeRoot)

			var out bytes.Buffer
			exitCode := clihelp.Execute(c.validatePlanCmd(), &out, nil)

			if exitCode != tt.wantExitAbsent {
				t.Errorf("exit code = %d; want %d (output: %q)", exitCode, tt.wantExitAbsent, out.String())
			}

			env := decodeSingleEnvelope(t, out.String())

			if ok, _ := env["ok"].(bool); ok != tt.wantOKAbsent {
				t.Errorf("envelope ok = %v; want %v", env["ok"], tt.wantOKAbsent)
			}

			_, hasFindings := env["findings"]
			if hasFindings != tt.wantFindAbsent {
				t.Errorf("envelope findings key present = %v; want %v (envelope: %v)", hasFindings, tt.wantFindAbsent, env)
			}
		})

		t.Run(tt.name+"/RequireApproved", func(t *testing.T) {
			anchorPath := t.TempDir()
			worktreeRoot := t.TempDir()
			c := tt.build(anchorPath, worktreeRoot)

			var out bytes.Buffer
			exitCode := clihelp.Execute(c.validatePlanCmd(), &out, []string{"--require-approved"})

			if exitCode != tt.wantExitRequire {
				t.Errorf("exit code = %d; want %d (output: %q)", exitCode, tt.wantExitRequire, out.String())
			}

			env := decodeSingleEnvelope(t, out.String())

			if ok, _ := env["ok"].(bool); ok != tt.wantOKRequire {
				t.Errorf("envelope ok = %v; want %v", env["ok"], tt.wantOKRequire)
			}

			findings, hasFindings := env["findings"]
			if hasFindings != tt.wantFindRequire {
				t.Errorf("envelope findings key present = %v; want %v (envelope: %v)", hasFindings, tt.wantFindRequire, env)
			}
			if tt.wantFindingsName != "" && !envelopeContains(findings, tt.wantFindingsName) {
				t.Errorf("envelope findings does not mention %q; got: %v", tt.wantFindingsName, env)
			}
		})
	}
}

// decodeSingleEnvelope asserts that captured is exactly one JSON line -- split on newline,
// discarding a single trailing empty element -- and decodes it into a map[string]any so the
// presence or absence of a key can be asserted directly rather than by substring.
func decodeSingleEnvelope(t *testing.T, captured string) map[string]any {
	t.Helper()

	lines := strings.Split(captured, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) != 1 {
		t.Fatalf("captured output is not exactly one line: %q", captured)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &env); err != nil {
		t.Fatalf("decode envelope line %q: %v", lines[0], err)
	}
	return env
}

// envelopeContains reports whether v -- expected to be a []any of strings, as JSON decodes the
// "findings" key's value -- contains a string carrying substr.
func envelopeContains(v any, substr string) bool {
	items, ok := v.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		s, ok := item.(string)
		if ok && strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
