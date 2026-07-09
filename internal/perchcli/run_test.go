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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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

// TestRunCLI_Run_InvalidRunID verifies that an explicit --run-id carrying a
// path separator (the class of value that would escape the perch runs
// directory via filepath.Join, e.g. "../elsewhere") is rejected loud before
// run ever reads --profile's decoded content or touches PersistentPreRunE's
// engine wiring — this case, like MissingProfile above, runs against an
// uninitialized directory and needs only a --profile flag value that reads
// as a file path (its content is never reached).
func TestRunCLI_Run_InvalidRunID(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	profilePath := filepath.Join(dir, "profile.yaml")
	if err := os.WriteFile(profilePath, []byte("target:\n  instructions: x\n"), 0o644); err != nil {
		t.Fatalf("write profile fixture: %v", err)
	}

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"run", "--profile", profilePath, "--run-id", "../../escaped"})

	if exitCode != 1 {
		t.Errorf(`RunCLI([run --run-id ../../escaped]) = %d; want 1`, exitCode)
	}
	if !strings.Contains(out.String(), "lowercase alphanumerics and dashes only") {
		t.Errorf(`RunCLI([run --run-id ../../escaped]) output missing the run-id shape error; got: %q`, out.String())
	}
}

// TestRunCLI_Run_WeftSyncRunsOnEngineError verifies that Engine.Run
// returning a hard error still gets the SAME weft commit+push treatment a
// successful terminal outcome does, per the Weft Git Invariant: perchcli is
// the loop owner regardless of how the block ended. A profile whose
// round-caps ladder fails Profile.validate (non-increasing entries) makes
// Engine.Run return an error deterministically, with no live mux/claude
// substrate needed — validate runs before the first round would ever spawn,
// so this test pre-seeds the run dir with a placeholder artifact (standing
// in for what a real partially-completed block, e.g. a completed round
// before a later could-not-start gate error, would have left behind) to
// prove the sync call actually runs and actually commits it on this path.
func TestRunCLI_Run_WeftSyncRunsOnEngineError(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")
	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"shuttle": shuttleengine.ConfigTemplate(),
		"mux":     muxengine.ConfigTemplate(),
		"perch":   perchengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)

	profilePath := filepath.Join(fixture.Hub, "profile.yaml")
	profileContent := "target:\n  instructions: x\nfasit:\n  instructions: y\nrubric: r\nfix-scope: overlay\ngate:\n  mode: llm-verdict\nround-caps: [5, 3]\n"
	if err := os.WriteFile(profilePath, []byte(profileContent), 0o644); err != nil {
		t.Fatalf("write profile fixture: %v", err)
	}

	// Stand in for a real partially-completed block's leftover artifact:
	// Profile.validate fails before Engine.Run ever creates the run dir
	// itself, so a placeholder file is planted directly inside the
	// weft-prime worktree at the path the host's "_lyx" junction would
	// otherwise transparently resolve to (this fixture predates "lyx init",
	// so no junction exists yet — writing straight into WeftPrime is the
	// established pattern other cli test suites use, e.g. weftcli's
	// TestRunCLI_EnvMapToOption).
	placeholderDir := filepath.Join(fixture.WeftPrime, hubgeometry.LyxDirName, "perch", "weft-on-error")
	if err := os.MkdirAll(placeholderDir, 0o755); err != nil {
		t.Fatalf("mkdir placeholder run dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(placeholderDir, "round-1-review.md"), []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("write placeholder artifact: %v", err)
	}

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"run", "--profile", profilePath, "--run-id", "weft-on-error"})

	if exitCode != 1 {
		t.Fatalf(`RunCLI([run]) = %d; want 1 (a bad round-caps ladder must fail Profile.validate)`, exitCode)
	}
	if !strings.Contains(out.String(), "strictly increasing") {
		t.Fatalf(`RunCLI([run]) output missing the round-caps validation error; got: %q`, out.String())
	}

	weftLog := gitLogOneline(t, fixture.WeftPrime)
	if !strings.Contains(weftLog, "weft-on-error ERROR") {
		t.Errorf("weft log = %q; want a %q commit even though Engine.Run returned an error", weftLog, "perch: weft-on-error ERROR")
	}
}

// TestRunCLI_Run_WeftCommitExcludesLockFiles verifies the block-exit weft
// commit stages a run dir's real block state (state.json, round artifacts)
// but never its machine-local advisory-lock files (run.lock,
// state.json.lock): committing those would leak runtime noise into durable
// weft history and materialize stale lock files on every other machine's
// weft pull. Uses the same deterministic engine-error skeleton as
// TestRunCLI_Run_WeftSyncRunsOnEngineError — the sync path is identical for
// every outcome, so the cheapest deterministic exit exercises the pathspec.
func TestRunCLI_Run_WeftCommitExcludesLockFiles(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")
	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"shuttle": shuttleengine.ConfigTemplate(),
		"mux":     muxengine.ConfigTemplate(),
		"perch":   perchengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)

	profilePath := filepath.Join(fixture.Hub, "profile.yaml")
	profileContent := "target:\n  instructions: x\nfasit:\n  instructions: y\nrubric: r\nfix-scope: overlay\ngate:\n  mode: llm-verdict\nround-caps: [5, 3]\n"
	if err := os.WriteFile(profilePath, []byte(profileContent), 0o644); err != nil {
		t.Fatalf("write profile fixture: %v", err)
	}

	// Stand in for a real block's run dir: state alongside the two lock
	// files a real Engine.Run leaves behind (see the WeftSyncRunsOnEngineError
	// test above for why this is planted straight into WeftPrime).
	runDir := filepath.Join(fixture.WeftPrime, hubgeometry.LyxDirName, "perch", "lock-exclusion")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir placeholder run dir: %v", err)
	}
	for name, content := range map[string]string{
		"state.json":        "{}",
		"round-1-review.md": "placeholder",
		"run.lock":          "",
		"state.json.lock":   "",
	} {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write placeholder %s: %v", name, err)
		}
	}

	var out bytes.Buffer
	if exitCode := RunCLI(&out, []string{"run", "--profile", profilePath, "--run-id", "lock-exclusion"}); exitCode != 1 {
		t.Fatalf(`RunCLI([run]) = %d; want 1 (a bad round-caps ladder must fail Profile.validate), output: %s`, exitCode, out.String())
	}

	// The commit must carry the block state and nothing lock-shaped.
	tracked := gitLsFiles(t, fixture.WeftPrime)
	if !strings.Contains(tracked, "lock-exclusion/state.json\n") || !strings.Contains(tracked, "lock-exclusion/round-1-review.md") {
		t.Errorf("weft tracked files = %q; want state.json and round-1-review.md committed", tracked)
	}
	if strings.Contains(tracked, ".lock") {
		t.Errorf("weft tracked files = %q; want no *.lock file ever committed", tracked)
	}
}

// TestRunCLI_Run_BusyBlockSkipsWeftSync verifies that a run refused because
// another invocation holds the block's run.lock does NOT run the block-exit
// weft sync: the loser changed nothing on disk, and syncing would commit
// (and push) the WINNER's in-flight partial state under a misleading
// "perch: <id> ERROR" message. A dirty file is planted in the weft-side
// _lyx (standing in for the winner's mid-round state) to prove the sync
// would have had something to commit and still did not run.
func TestRunCLI_Run_BusyBlockSkipsWeftSync(t *testing.T) {
	t.Setenv("WEFT_SKIP_PUSH", "1")
	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"shuttle": shuttleengine.ConfigTemplate(),
		"mux":     muxengine.ConfigTemplate(),
		"perch":   perchengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)

	profilePath := filepath.Join(fixture.Hub, "profile.yaml")
	profileContent := "target:\n  instructions: x\nfasit:\n  instructions: y\nrubric: r\nfix-scope: overlay\ngate:\n  mode: llm-verdict\nround-caps: [3]\n"
	if err := os.WriteFile(profilePath, []byte(profileContent), 0o644); err != nil {
		t.Fatalf("write profile fixture: %v", err)
	}

	// The winner's in-flight state, planted straight into WeftPrime (this
	// fixture predates "lyx init", so no junction exists — same pattern as
	// the weft tests above). runDirBase resolves against the HOST cwd, so
	// hold the run.lock there; the dirty weft file proves the skipped sync
	// had real material.
	hostRunDir := filepath.Join(hubgeometry.PerchRunsDir(fixture.Hub), "busyblock")
	if err := os.MkdirAll(hostRunDir, 0o755); err != nil {
		t.Fatalf("mkdir host run dir: %v", err)
	}
	weftDirty := filepath.Join(fixture.WeftPrime, hubgeometry.LyxDirName, "perch", "busyblock")
	if err := os.MkdirAll(weftDirty, 0o755); err != nil {
		t.Fatalf("mkdir weft dirty dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(weftDirty, "round-1-review.md"), []byte("winner's in-flight partial"), 0o644); err != nil {
		t.Fatalf("write weft dirty file: %v", err)
	}

	// Stand in for the winning invocation: hold the run.lock for the whole
	// losing call, exactly as a mid-round Engine.Run does.
	runLock, locked, err := lock.TryAcquireWriteLock(filepath.Join(hostRunDir, "run.lock"))
	if err != nil || !locked {
		t.Fatalf("TryAcquireWriteLock() = (%v, %v); want a held lock", locked, err)
	}
	defer runLock.Release()

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"run", "--profile", profilePath, "--run-id", "busyblock"})
	if exitCode != 1 {
		t.Fatalf(`RunCLI([run --run-id busyblock]) = %d; want 1, output: %s`, exitCode, out.String())
	}
	if !strings.Contains(out.String(), "already running") {
		t.Errorf(`RunCLI([run --run-id busyblock]) output missing "already running"; got: %q`, out.String())
	}

	weftLog := gitLogOneline(t, fixture.WeftPrime)
	if strings.Contains(weftLog, "busyblock") {
		t.Errorf("weft log = %q; want NO commit from the losing invocation (the winner syncs at its own exit)", weftLog)
	}
}

// gitLsFiles runs `git ls-files` inside dir and returns its output, failing
// the test loudly if git cannot be invoked.
func gitLsFiles(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "ls-files")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git ls-files in %q: %v (output: %s)", dir, err, output)
	}
	return string(output)
}

// gitLogOneline runs `git log --oneline` inside dir and returns its output,
// failing the test loudly if git cannot be invoked.
func gitLogOneline(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "log", "--oneline")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log --oneline in %q: %v (output: %s)", dir, err, output)
	}
	return string(output)
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

// TestDeriveBlockRunID_StableAcrossTuningOverlay proves the run identity is
// derived from the profile as decoded from the FILE: overlaying the tuning
// flags afterwards (as runCmd does) cannot change the id, so a re-run with
// different --model/--effort/--timeout resolves to the same run dir and hits
// the engine's loud identity check instead of silently forking a new block.
func TestDeriveBlockRunID_StableAcrossTuningOverlay(t *testing.T) {
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

	idBefore, err := deriveBlockRunID("profiles/p.yaml", profile, "")
	if err != nil {
		t.Fatalf("deriveBlockRunID() unexpected error: %v", err)
	}

	// The overlay runCmd applies AFTER derivation must not affect the id:
	// derive again from the same file-decoded profile while a copy carries
	// the overlaid tuning, exactly mirroring runCmd's ordering.
	overlaid := profile
	overlaid.Model = "sonnet"
	overlaid.Effort = "high"
	overlaid.Timeout = 5 * time.Minute

	idAfter, err := deriveBlockRunID("profiles/p.yaml", profile, "")
	if err != nil {
		t.Fatalf("deriveBlockRunID() unexpected error: %v", err)
	}
	if idBefore != idAfter {
		t.Errorf("deriveBlockRunID() = %q then %q; want identical for the same profile file", idBefore, idAfter)
	}

	// An OVERLAID profile hashes differently — which is exactly what makes
	// the engine's identity check refuse a re-run with different flags.
	overlaidHash, err := perchengine.ProfileHash(overlaid)
	if err != nil {
		t.Fatalf("ProfileHash(overlaid) unexpected error: %v", err)
	}
	fileHash, err := perchengine.ProfileHash(profile)
	if err != nil {
		t.Fatalf("ProfileHash(file) unexpected error: %v", err)
	}
	if overlaidHash == fileHash {
		t.Error("ProfileHash(overlaid) == ProfileHash(file); want the tuning overlay to change the engine identity hash")
	}

	// An explicit --run-id always wins, untouched.
	explicit, err := deriveBlockRunID("profiles/p.yaml", profile, "my-explicit-id")
	if err != nil {
		t.Fatalf("deriveBlockRunID(explicit) unexpected error: %v", err)
	}
	if explicit != "my-explicit-id" {
		t.Errorf("deriveBlockRunID(explicit) = %q; want %q", explicit, "my-explicit-id")
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

// TestDecodeProfile_EmptyRoundCapsStaysNonNil proves an explicit
// `round-caps: []` decodes to a non-nil empty slice — the value
// perchengine.Profile.validate rejects loud — while an absent key stays nil
// (the "unset, use the default chain" spelling). The decode layer must
// preserve that distinction or the engine cannot tell the two apart.
func TestDecodeProfile_EmptyRoundCapsStaysNonNil(t *testing.T) {
	explicit, err := decodeProfile([]byte("target:\n  instructions: x\nfasit:\n  instructions: y\nrubric: r\nfix-scope: overlay\ngate:\n  mode: llm-verdict\nround-caps: []\n"))
	if err != nil {
		t.Fatalf("decodeProfile(round-caps: []) unexpected error: %v", err)
	}
	if explicit.RoundCaps == nil || len(explicit.RoundCaps) != 0 {
		t.Errorf("decodeProfile(round-caps: []).RoundCaps = %v; want a non-nil empty slice", explicit.RoundCaps)
	}

	absent, err := decodeProfile([]byte("target:\n  instructions: x\nfasit:\n  instructions: y\nrubric: r\nfix-scope: overlay\ngate:\n  mode: llm-verdict\n"))
	if err != nil {
		t.Fatalf("decodeProfile(no round-caps) unexpected error: %v", err)
	}
	if absent.RoundCaps != nil {
		t.Errorf("decodeProfile(no round-caps).RoundCaps = %v; want nil", absent.RoundCaps)
	}
}

// TestResolveRunTarget_DerivesIDBeforeOverlay pins runCmd's load-bearing
// ordering at the call site RunE actually uses (resolveRunTarget), not just
// the isolated deriveBlockRunID helper: the run id — and thus the run dir —
// derives from the FILE-decoded profile BEFORE the tuning flags overlay, so
// two invocations of the same profile file with DIFFERENT
// --model/--effort/--timeout resolve to the SAME id and run dir. That
// stability is what makes the engine's identity check refuse a re-run with
// changed flags instead of silently forking a fresh block. A reorder that
// overlaid the flags before deriving the id would make the id depend on
// --model and diverge here — the exact regression an isolated helper test
// (TestDeriveBlockRunID_StableAcrossTuningOverlay) cannot catch, because it
// never exercises resolveRunTarget's ordering.
func TestResolveRunTarget_DerivesIDBeforeOverlay(t *testing.T) {
	fileProfile, err := decodeProfile([]byte(`
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

	c := &perchCLI{runDirBase: t.TempDir()}

	// First invocation: no tuning flags.
	idPlain, dirPlain, profPlain, err := c.resolveRunTarget("profiles/p.yaml", "", fileProfile, "", "", 0)
	if err != nil {
		t.Fatalf("resolveRunTarget(plain) unexpected error: %v", err)
	}

	// Second invocation: the SAME profile file, different tuning flags. Because
	// the id derives from the pre-overlay file content, both id and run dir must
	// be identical — a reorder deriving from the overlaid profile would make
	// idTuned depend on --model and diverge from idPlain.
	idTuned, dirTuned, profTuned, err := c.resolveRunTarget("profiles/p.yaml", "", fileProfile, "sonnet", "high", 5*time.Minute)
	if err != nil {
		t.Fatalf("resolveRunTarget(tuned) unexpected error: %v", err)
	}
	if idTuned != idPlain {
		t.Errorf("resolveRunTarget id = %q with tuning, %q without; want identical (id must ignore the tuning overlay)", idTuned, idPlain)
	}
	if dirTuned != dirPlain {
		t.Errorf("resolveRunTarget runDir = %q with tuning, %q without; want identical", dirTuned, dirPlain)
	}
	if !strings.HasPrefix(dirTuned, c.runDirBase) {
		t.Errorf("resolveRunTarget runDir = %q; want it under runDirBase %q", dirTuned, c.runDirBase)
	}

	// The overlay DID land on the returned profile (so the block runs with the
	// flags), and it changed the engine identity hash — which is what makes a
	// resume with different flags fail loud rather than fork.
	if profTuned.Model != "sonnet" || profTuned.Effort != "high" || profTuned.Timeout != 5*time.Minute {
		t.Errorf("resolveRunTarget profile = {Model:%q Effort:%q Timeout:%s}; want the tuning overlay applied", profTuned.Model, profTuned.Effort, profTuned.Timeout)
	}
	if profPlain.Model != "" || profPlain.Effort != "" || profPlain.Timeout != 0 {
		t.Errorf("resolveRunTarget(plain) profile = {Model:%q Effort:%q Timeout:%s}; want no overlay", profPlain.Model, profPlain.Effort, profPlain.Timeout)
	}

	plainHash, err := perchengine.ProfileHash(profPlain)
	if err != nil {
		t.Fatalf("ProfileHash(plain) unexpected error: %v", err)
	}
	tunedHash, err := perchengine.ProfileHash(profTuned)
	if err != nil {
		t.Fatalf("ProfileHash(tuned) unexpected error: %v", err)
	}
	if plainHash == tunedHash {
		t.Error("ProfileHash(plain) == ProfileHash(tuned); want the tuning overlay to change the engine identity hash")
	}

	// An explicit --run-id still wins, untouched by the overlay.
	idExplicit, _, _, err := c.resolveRunTarget("profiles/p.yaml", "my-explicit-id", fileProfile, "sonnet", "", 0)
	if err != nil {
		t.Fatalf("resolveRunTarget(explicit) unexpected error: %v", err)
	}
	if idExplicit != "my-explicit-id" {
		t.Errorf("resolveRunTarget(explicit) id = %q; want %q", idExplicit, "my-explicit-id")
	}
}
