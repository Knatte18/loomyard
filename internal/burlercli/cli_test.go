// cli_test.go covers the burlercli cobra seam through RunCLI: bare-group
// listing, the unknown-subcommand JSON envelope, the PersistentPreRunE
// group-command guard, run's required --profile flag, the help-tree Short
// completeness check, and decodeProfile's strict YAML decode. Engine.Run
// itself is NOT exercised here — it needs a live mux/claude session; that
// coverage lives in the smoke test and the sandbox suite.

package burlercli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestRunCLI_NoArgs verifies that "lyx burler" with no subcommand lists the
// run verb and exits 0 — no git repo is needed, since the PersistentPreRunE
// guard skips layout/config/engine resolution for the group command itself.
func TestRunCLI_NoArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) = %d; want 0", exitCode)
	}

	got := out.String()
	if !strings.Contains(got, "run") {
		t.Errorf("RunCLI(nil) no-arg listing missing subcommand %q; got:\n%s", "run", got)
	}
}

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand exits 1
// and emits a JSON error envelope with ok=false, without needing a git repo
// (the PersistentPreRunE guard for cmd.Name() == "burler" fires before
// layout resolution).
func TestRunCLI_UnknownSubcommand(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"bogus"})

	if exitCode != 1 {
		t.Errorf("RunCLI(bogus) = %d; want 1", exitCode)
	}

	got := out.String()
	if !strings.Contains(got, `"ok":false`) {
		t.Errorf("RunCLI(bogus) output missing ok:false envelope; got: %q", got)
	}
	if !strings.Contains(got, "unknown") {
		t.Errorf("RunCLI(bogus) output missing \"unknown\"; got: %q", got)
	}
}

// TestRunCLI_GroupGuard_OutsideGitRepo asserts the PersistentPreRunE guard:
// bare "lyx burler" works outside a git repository, mirroring shuttlecli's
// guard rationale (neither the bare listing nor the unknown-subcommand path
// should require layout/config resolution).
func TestRunCLI_GroupGuard_OutsideGitRepo(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) outside a git repo = %d; want 0", exitCode)
	}
}

// TestRunCLI_Run_MissingProfile verifies that "lyx burler run" without
// --profile fails with run's own manual flag-shape error (not cobra's
// MarkFlagRequired) before ever touching PersistentPreRunE's engine wiring.
// This case runs against an uninitialized (non-git) directory, so
// PersistentPreRunE's own abort error is also present in the captured
// output alongside the flag-specific error line — the same documented
// double-failure shape as shuttlecli's TestRunCLI_Run_FlagValidation.
func TestRunCLI_Run_MissingProfile(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"run"})

	if exitCode != 1 {
		t.Errorf(`RunCLI([run]) = %d; want 1`, exitCode)
	}
	if !strings.Contains(out.String(), "--profile is required") {
		t.Errorf(`RunCLI([run]) output missing "--profile is required"; got: %q`, out.String())
	}
}

// TestCommand_EveryCommandHasShort walks the full burler command tree and
// asserts that every command — the parent group and every subcommand —
// carries a non-empty Short, per the CLI/Cobra Invariant.
func TestCommand_EveryCommandHasShort(t *testing.T) {
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Short == "" {
			t.Errorf("command %q has empty Short", cmd.CommandPath())
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(Command())
}

// TestDecodeProfile covers decodeProfile's strict YAML decode: a full valid
// profile (every field lands, including the boolean/zero-value edge cases
// tool-use: true and cluster-n: 0), a minimal valid profile, an unknown key
// (rejected per the yaml-strictness-split decision's KnownFields(true)), and
// malformed YAML.
func TestDecodeProfile(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "FullValid",
			yaml: `
target:
  paths: ["a.md", "b.md"]
  instructions: "review the pair"
fasit:
  paths: ["c.md"]
  instructions: "against c"
rubric: "BLOCKING: x. NIT: y."
fix-scope: source
tool-use: true
cluster-n: 0
review-path: review.md
fixer-report-path: fixer-report.md
prior-reviews: ["prior-review.md"]
prior-fixer-reports: ["prior-fixer.md"]
`,
		},
		{
			name: "MinimalValid",
			yaml: `
target:
  instructions: "diff against main"
fasit:
  instructions: "the discussion"
rubric: "BLOCKING: x."
fix-scope: overlay
review-path: review.md
fixer-report-path: fixer-report.md
`,
		},
		{
			name: "UnknownKey",
			yaml: `
target:
  instructions: "diff against main"
fasit:
  instructions: "the discussion"
rubric: "BLOCKING: x."
fixscope: overlay
review-path: review.md
fixer-report-path: fixer-report.md
`,
			wantErr: true,
		},
		{
			name:    "MalformedYAML",
			yaml:    "target: [this is not: valid yaml: at all",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := decodeProfile([]byte(tt.yaml))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("decodeProfile(%q) error = nil; want error", tt.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeProfile(%q) unexpected error: %v", tt.name, err)
			}
			if profile.Rubric == "" {
				t.Errorf("decodeProfile(%q) Profile.Rubric is empty; want non-empty", tt.name)
			}
			if profile.ReviewPath == "" || profile.FixerReportPath == "" {
				t.Errorf("decodeProfile(%q) ReviewPath/FixerReportPath empty; want both set", tt.name)
			}
		})
	}
}

// TestDecodeProfile_FullValidFieldMapping asserts every field of a full
// valid profile YAML lands on the corresponding Profile field, including the
// boolean/zero-value edge cases (tool-use: true, cluster-n: 0) that a
// zero-value-blind mapping bug could silently drop.
func TestDecodeProfile_FullValidFieldMapping(t *testing.T) {
	data := []byte(`
target:
  paths: ["a.md", "b.md"]
  instructions: "review the pair"
fasit:
  paths: ["c.md"]
  instructions: "against c"
rubric: "BLOCKING: x. NIT: y."
fix-scope: source
tool-use: true
cluster-n: 0
review-path: review.md
fixer-report-path: fixer-report.md
prior-reviews: ["prior-review.md"]
prior-fixer-reports: ["prior-fixer.md"]
`)

	profile, err := decodeProfile(data)
	if err != nil {
		t.Fatalf("decodeProfile() unexpected error: %v", err)
	}

	if got, want := profile.Target.Paths, []string{"a.md", "b.md"}; !equalStrings(got, want) {
		t.Errorf("Target.Paths = %v; want %v", got, want)
	}
	if profile.Target.Instructions != "review the pair" {
		t.Errorf("Target.Instructions = %q; want %q", profile.Target.Instructions, "review the pair")
	}
	if got, want := profile.Fasit.Paths, []string{"c.md"}; !equalStrings(got, want) {
		t.Errorf("Fasit.Paths = %v; want %v", got, want)
	}
	if profile.Fasit.Instructions != "against c" {
		t.Errorf("Fasit.Instructions = %q; want %q", profile.Fasit.Instructions, "against c")
	}
	if string(profile.FixScope) != "source" {
		t.Errorf("FixScope = %q; want %q", profile.FixScope, "source")
	}
	if !profile.ToolUse {
		t.Errorf("ToolUse = false; want true")
	}
	if profile.ClusterN != 0 {
		t.Errorf("ClusterN = %d; want 0", profile.ClusterN)
	}
	if profile.ReviewPath != "review.md" {
		t.Errorf("ReviewPath = %q; want %q", profile.ReviewPath, "review.md")
	}
	if profile.FixerReportPath != "fixer-report.md" {
		t.Errorf("FixerReportPath = %q; want %q", profile.FixerReportPath, "fixer-report.md")
	}
	if got, want := profile.PriorReviews, []string{"prior-review.md"}; !equalStrings(got, want) {
		t.Errorf("PriorReviews = %v; want %v", got, want)
	}
	if got, want := profile.PriorFixerReports, []string{"prior-fixer.md"}; !equalStrings(got, want) {
		t.Errorf("PriorFixerReports = %v; want %v", got, want)
	}
}

// equalStrings reports whether got and want hold the same strings in the
// same order.
func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
