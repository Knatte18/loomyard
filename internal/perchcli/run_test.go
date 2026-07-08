// run_test.go covers the run verb's flag-shape validation and decodeProfile's
// strict YAML decode: a full valid profile (every field, including the gate
// mapping and both duration-string parses), a minimal valid profile, an
// unknown key, malformed YAML, and a malformed gate duration. It also checks
// that decodeProfile's output feeds perchengine's exported run-identity
// helpers (ProfileHash, DeriveRunID) without error, in the shape run.go's
// RunE itself relies on. Engine.Run itself is NOT exercised here — it needs
// a live mux/claude session; that coverage lives in the smoke test and the
// sandbox suite.

package perchcli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/perchengine"
)

// TestRunCLI_Run_MissingProfile verifies that "lyx perch run" without
// --profile fails with run's own manual flag-shape error (not cobra's
// MarkFlagRequired) before ever touching PersistentPreRunE's engine wiring.
// This case runs against an uninitialized (non-git) directory, so
// PersistentPreRunE's own abort error is also present in the captured
// output alongside the flag-specific error line — the same documented
// double-failure shape as burlercli's TestRunCLI_Run_MissingProfile.
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

// TestDecodeProfile covers decodeProfile's strict YAML decode: a full valid
// profile (every field, including the gate mapping and both Go-duration-
// string parses), a minimal valid profile, an unknown key (rejected per the
// yaml-strictness-split decision's KnownFields(true)), malformed YAML, and a
// malformed gate.timeout duration string.
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
fix-scope: overlay
tool-use: true
cluster-n: 0
gate:
  mode: command
  command: ["go", "test", "./..."]
  timeout: 10m
round-caps: [5, 8, 10]
judge-model: haiku
judge-effort: low
model: sonnet
effort: high
timeout: 30m
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
gate:
  mode: llm-verdict
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
gate:
  mode: llm-verdict
`,
			wantErr: true,
		},
		{
			name:    "MalformedYAML",
			yaml:    "target: [this is not: valid yaml: at all",
			wantErr: true,
		},
		{
			name: "BadGateDuration",
			yaml: `
target:
  instructions: "diff against main"
fasit:
  instructions: "the discussion"
rubric: "BLOCKING: x."
fix-scope: overlay
gate:
  mode: command
  command: ["go", "test"]
  timeout: not-a-duration
`,
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
			if profile.Gate.Mode == "" {
				t.Errorf("decodeProfile(%q) Profile.Gate.Mode is empty; want non-empty", tt.name)
			}
		})
	}
}

// TestDecodeProfile_FullValidFieldMapping asserts every field of a full
// valid profile YAML lands on the corresponding Profile field, including
// the gate.command argv, both Go-duration-string parses (gate.timeout and
// the top-level timeout), and round-caps — the zero-value edge case
// (tool-use: true, cluster-n: 0) a zero-value-blind mapping bug could
// silently drop.
func TestDecodeProfile_FullValidFieldMapping(t *testing.T) {
	data := []byte(`
target:
  paths: ["a.md", "b.md"]
  instructions: "review the pair"
fasit:
  paths: ["c.md"]
  instructions: "against c"
rubric: "BLOCKING: x. NIT: y."
fix-scope: overlay
tool-use: true
cluster-n: 0
gate:
  mode: command
  command: ["go", "test", "./..."]
  timeout: 10m
round-caps: [5, 8, 10]
judge-model: haiku
judge-effort: low
model: sonnet
effort: high
timeout: 30m
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
	if string(profile.FixScope) != "overlay" {
		t.Errorf("FixScope = %q; want %q", profile.FixScope, "overlay")
	}
	if !profile.ToolUse {
		t.Errorf("ToolUse = false; want true")
	}
	if profile.ClusterN != 0 {
		t.Errorf("ClusterN = %d; want 0", profile.ClusterN)
	}
	if string(profile.Gate.Mode) != "command" {
		t.Errorf("Gate.Mode = %q; want %q", profile.Gate.Mode, "command")
	}
	if got, want := profile.Gate.Command, []string{"go", "test", "./..."}; !equalStrings(got, want) {
		t.Errorf("Gate.Command = %v; want %v", got, want)
	}
	if profile.Gate.Timeout != 10*time.Minute {
		t.Errorf("Gate.Timeout = %s; want %s", profile.Gate.Timeout, 10*time.Minute)
	}
	if got, want := profile.RoundCaps, []int{5, 8, 10}; !equalInts(got, want) {
		t.Errorf("RoundCaps = %v; want %v", got, want)
	}
	if profile.JudgeModel != "haiku" {
		t.Errorf("JudgeModel = %q; want %q", profile.JudgeModel, "haiku")
	}
	if profile.JudgeEffort != "low" {
		t.Errorf("JudgeEffort = %q; want %q", profile.JudgeEffort, "low")
	}
	if profile.Model != "sonnet" {
		t.Errorf("Model = %q; want %q", profile.Model, "sonnet")
	}
	if profile.Effort != "high" {
		t.Errorf("Effort = %q; want %q", profile.Effort, "high")
	}
	if profile.Timeout != 30*time.Minute {
		t.Errorf("Timeout = %s; want %s", profile.Timeout, 30*time.Minute)
	}
}

// TestRunIdentity_DeriveRunIDShape asserts that decodeProfile's output feeds
// perchengine's exported run-identity helpers (ProfileHash, DeriveRunID)
// without error and produces the documented "<slug>-<hash8>" shape — the
// same call sequence run.go's RunE performs before constructing runDir.
func TestRunIdentity_DeriveRunIDShape(t *testing.T) {
	profile, err := decodeProfile([]byte(`
target:
  instructions: "diff against main"
fasit:
  instructions: "the discussion"
rubric: "BLOCKING: x."
fix-scope: overlay
gate:
  mode: llm-verdict
`))
	if err != nil {
		t.Fatalf("decodeProfile() unexpected error: %v", err)
	}

	hash, err := perchengine.ProfileHash(profile)
	if err != nil {
		t.Fatalf("ProfileHash() unexpected error: %v", err)
	}
	if len(hash) < 8 {
		t.Fatalf("ProfileHash() = %q; want at least 8 hex characters", hash)
	}

	id := perchengine.DeriveRunID("profiles/my-plan-review.yaml", hash)
	wantPrefix := "my-plan-review-"
	if !strings.HasPrefix(id, wantPrefix) {
		t.Errorf("DeriveRunID() = %q; want prefix %q", id, wantPrefix)
	}
	if !strings.HasSuffix(id, hash[:8]) {
		t.Errorf("DeriveRunID() = %q; want suffix %q (hash[:8])", id, hash[:8])
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

// equalInts reports whether got and want hold the same ints in the same
// order.
func equalInts(got, want []int) bool {
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
