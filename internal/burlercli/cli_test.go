// cli_test.go covers the burlercli cobra seam through RunCLI: bare-group listing, the
// unknown-subcommand JSON envelope, the PersistentPreRunE group-command guard, run's required
// --profile flag, the help-tree Short completeness check, decodeProfile's strict YAML decode, and
// resultEnvelope's success-envelope shape (including its forkCount nil guard).
// Engine.Run itself is NOT exercised here — it needs a live reed/claude session;
// that coverage lives in the smoke test and the sandbox suite.

package burlercli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/spf13/cobra"
)

// TestRunCLI_NoArgs verifies that "lyx burler" with no subcommand lists the run verb and exits 0 —
// no git repo is needed, since the PersistentPreRunE guard skips layout/config/engine resolution
// for the group command itself.
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

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand exits 1 and emits a JSON error
// envelope with ok=false, without needing a git repo (the PersistentPreRunE guard for cmd.Name() ==
// "burler" fires before layout resolution).
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

// TestRunCLI_GroupGuard_OutsideGitRepo asserts the PersistentPreRunE guard: bare "lyx burler" works
// outside a git repository, mirroring shuttlecli's guard rationale (neither the bare listing nor
// the unknown-subcommand path should require layout/config resolution).
func TestRunCLI_GroupGuard_OutsideGitRepo(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) outside a git repo = %d; want 0", exitCode)
	}
}

// TestRunCLI_Run_MissingProfile verifies that "lyx burler run" without --profile fails with run's
// own manual flag-shape error (not cobra's MarkFlagRequired) before ever touching
// PersistentPreRunE's engine wiring.
// This case runs against an uninitialized (non-git) directory, which resolves to standalone mode:
// the pre-run therefore succeeds and only the verb's own flag error is emitted. XDG_STATE_HOME and
// LOCALAPPDATA are redirected to the test's own temp tree before RunCLI so the standalone wiring's
// Derive call and stencil seed land there instead of the operator's real state directory. This test
// is already not t.Parallel(), which t.Setenv requires; it stays that way.
func TestRunCLI_Run_MissingProfile(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"run"})

	if exitCode != 1 {
		t.Errorf(`RunCLI([run]) = %d; want 1`, exitCode)
	}
	if !strings.Contains(out.String(), "--profile is required") {
		t.Errorf(`RunCLI([run]) output missing "--profile is required"; got: %q`, out.String())
	}
}

// TestCommand_EveryCommandHasShort walks the full burler command tree and asserts that every
// command — the parent group and every subcommand — carries a non-empty Short, per the CLI/Cobra
// Invariant.
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

// TestDecodeProfile covers decodeProfile's strict YAML decode: a full valid profile (every field
// lands, including the boolean/zero-value edge cases tool-use: true and cluster-fan: ""), a minimal
// valid profile, an unknown key (rejected per the yaml-strictness-split decision's
// KnownFields(true)), the now-removed cluster-n key specifically (rejected the same way), and
// malformed YAML.
func TestDecodeProfile(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantErr   bool
		errSubstr string
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
cluster-fan: "standard"
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
			// cluster-n was replaced with cluster-fan; the strict decode
			// must reject the old key by name, not silently ignore it.
			name: "UnknownKeyClusterN",
			yaml: `
target:
  instructions: "diff against main"
fasit:
  instructions: "the discussion"
rubric: "BLOCKING: x."
fix-scope: overlay
cluster-n: 0
review-path: review.md
fixer-report-path: fixer-report.md
`,
			wantErr:   true,
			errSubstr: "cluster-n",
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
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("decodeProfile(%q) error = %q; want substring %q", tt.name, err.Error(), tt.errSubstr)
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

// TestDecodeProfile_FullValidFieldMapping asserts every field of a full valid profile YAML lands on
// the corresponding Profile field, including the boolean/zero-value edge cases (tool-use: true,
// cluster-fan: "standard") that a zero-value-blind mapping bug could silently drop.
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
cluster-fan: "standard"
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
	if profile.ClusterFan != "standard" {
		t.Errorf("ClusterFan = %q; want %q", profile.ClusterFan, "standard")
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

// TestResultEnvelope_ForkCountNilGuard asserts resultEnvelope's forkCount guards a nil ForkAudit
// (the non-cluster or non-done case) to 0 rather than panicking,
// reports the real fork count plus the raw ClusterWarnings slice when ForkAudit is set,
// and lands the told mode/stateDir/stencilsDir parameters under their own keys.
func TestResultEnvelope_ForkCountNilGuard(t *testing.T) {
	t.Run("nil ForkAudit", func(t *testing.T) {
		env := resultEnvelope(burlerengine.Result{Outcome: shuttleengine.OutcomeDone}, "hub", "", "/hub/stencils")
		if got := env["forkCount"]; got != 0 {
			t.Errorf(`resultEnvelope() forkCount = %v; want 0`, got)
		}
		// env["clusterWarnings"] holds a nil []string boxed in an `any` —
		// comparing the interface itself to nil would always be false (a
		// typed-nil-in-interface is never == nil), so assert on length.
		if got := env["clusterWarnings"].([]string); len(got) != 0 {
			t.Errorf(`resultEnvelope() clusterWarnings = %v; want empty`, got)
		}
		if got := env["mode"]; got != "hub" {
			t.Errorf(`resultEnvelope() mode = %v; want "hub"`, got)
		}
		if got := env["stateDir"]; got != "" {
			t.Errorf(`resultEnvelope() stateDir = %v; want ""`, got)
		}
		if got := env["stencilsDir"]; got != "/hub/stencils" {
			t.Errorf(`resultEnvelope() stencilsDir = %v; want "/hub/stencils"`, got)
		}
	})

	t.Run("populated ForkAudit and warnings", func(t *testing.T) {
		result := burlerengine.Result{
			Outcome: shuttleengine.OutcomeDone,
			ForkAudit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{{TranscriptPath: "a"}, {TranscriptPath: "b"}},
			},
			ClusterWarnings: []string{`fork "b" never returned a final report`},
		}
		env := resultEnvelope(result, "standalone", "/state/dir", "/state/dir/_lyx/stencils")
		if got := env["forkCount"]; got != 2 {
			t.Errorf(`resultEnvelope() forkCount = %v; want 2`, got)
		}
		gotWarnings, ok := env["clusterWarnings"].([]string)
		if !ok || len(gotWarnings) != 1 {
			t.Errorf(`resultEnvelope() clusterWarnings = %v; want one warning`, env["clusterWarnings"])
		}
		if got := env["mode"]; got != "standalone" {
			t.Errorf(`resultEnvelope() mode = %v; want "standalone"`, got)
		}
		if got := env["stateDir"]; got != "/state/dir" {
			t.Errorf(`resultEnvelope() stateDir = %v; want "/state/dir"`, got)
		}
		if got := env["stencilsDir"]; got != "/state/dir/_lyx/stencils" {
			t.Errorf(`resultEnvelope() stencilsDir = %v; want "/state/dir/_lyx/stencils"`, got)
		}
	})
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

// TestRunVerb_AbortedPreRunEmitsOneEnvelopeNotTwo is R6-16's regression test. The RunE checked
// --profile before clihelp.ShouldAbort, against CONSTRAINTS.md's CLI/Cobra Invariant, so a wiring
// refusal plus a missing --profile emitted two error envelopes — and the second one named a flag
// while the real failure was the refusal above it.
func TestRunVerb_AbortedPreRunEmitsOneEnvelopeNotTwo(t *testing.T) {
	var out bytes.Buffer
	// --target-dir naming a path that does not exist makes wireStandalone refuse in the pre-run;
	// --profile is deliberately omitted so the flag check would fire too if it ran.
	code := RunCLIIn(t.TempDir(), &out, []string{"run", "--target-dir", filepath.Join(t.TempDir(), "absent")})
	if code == 0 {
		t.Fatalf("RunCLIIn() = 0; want a non-zero exit for a refused pre-run. output: %s", out.String())
	}
	if got := strings.Count(out.String(), `"ok":false`); got != 1 {
		t.Errorf("RunCLIIn() emitted %d error envelopes; want exactly 1. output: %s", got, out.String())
	}
	if strings.Contains(out.String(), "--profile is required") {
		t.Errorf("RunCLIIn() reported the missing --profile over the pre-run refusal; the refusal is what the operator needs. output: %s", out.String())
	}
}

// TestRunVerb_RelativeProfileResolvesAgainstSeamCwd is R6-17's direct regression test. Nothing else
// in this file pins it: cli_test.go otherwise covers only the missing-`--profile` refusal and
// decodeProfile, and neither internal/cliwire's own tests nor either batch-3 enforcement test can
// catch a swapped or dropped first argument to os.ReadFile, so a silent regression to the process
// working directory is reachable.
//
// It drives the `run` verb the way TestRunVerb_AbortedPreRunEmitsOneEnvelopeNotTwo already does, with
// c.cwd pointed at a t.TempDir() holding a profile file, passing that file's own name as a RELATIVE
// --profile value.
//
// The fixture deliberately fails decodeProfile's strict decode -- this is the case's central
// constraint, not an incidental detail. `run`'s RunE must never reach c.reedUp on this fixture: doing
// so would call reedEngine.Up() and boot a live tmux/reed session from an untagged tier-1 test, which
// the Test Tier Purity Invariant forbids. Do not "fix" this fixture by making the profile valid --
// that would let the flow reach c.reedUp and breach the invariant this test's own tier depends on.
//
// The assertion is POSITIVE, not merely "does not contain": reaching decodeProfile's own "profile
// YAML" error prefix proves os.ReadFile succeeded, which proves the relative --profile resolved
// against c.cwd. A "does not contain" assertion alone would pass vacuously if the invocation ended
// earlier -- and it can: RunE checks clihelp.ShouldAbort before ever reaching the read, and an
// aborted pre-run returns nil there, satisfying a purely negative assertion without exercising the
// resolution this case exists to pin. wiring runs in PersistentPreRunE, ahead of RunE, so the pre-run
// must genuinely succeed for this case to mean anything.
//
// Both XDG_STATE_HOME and LOCALAPPDATA are redirected to fresh t.TempDir() values before the call,
// and this case is not t.Parallel(): a successful pre-run reaches the real standalonestate.Derive and
// the real stencilstore.Reconcile, so without both redirects this untagged case would seed a stencils
// tree and a trace sink into the developer's own state home -- exactly why
// internal/burlercli/cli_integration_test.go sets both and carries //go:build integration.
func TestRunVerb_RelativeProfileResolvesAgainstSeamCwd(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Cleanup(func() { logger.SetDurableSinkDir("") })

	cwd := t.TempDir()
	// A plain repository root is not required for standalone mode to wire successfully -- a genuine
	// non-repository directory folds into the same ModeStandalone verdict -- so no ".git" marker is
	// seeded here.
	const profileName = "profile.yaml"
	if err := os.WriteFile(filepath.Join(cwd, profileName), []byte("not: valid: yaml: at: all"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var out bytes.Buffer
	code := RunCLIIn(cwd, &out, []string{"run", "--profile", profileName})

	if code == 0 {
		t.Fatalf("RunCLIIn() = 0; want a non-zero exit for a profile that fails decodeProfile. output: %s", out.String())
	}
	got := out.String()
	if !strings.Contains(got, "profile YAML") {
		t.Errorf("RunCLIIn() output missing decodeProfile's own \"profile YAML\" error prefix -- want proof that os.ReadFile succeeded against the relative --profile resolved through c.cwd. output: %q", got)
	}
	if strings.Contains(got, "read --profile") {
		t.Errorf("RunCLIIn() output contains \"read --profile\"; want decodeProfile's own error, not an os.ReadFile failure -- the relative --profile must have resolved successfully. output: %q", got)
	}
}
