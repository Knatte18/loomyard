// cli_test.go covers the webstercli cobra seam through RunCLI: bare-group listing, the
// unknown-subcommand JSON envelope, the PersistentPreRunE group-command guard, and the help-tree
// Short completeness check.
// It also covers the three spawn-free verbs (validate/status/pause) and fabricSync's
// SkipGit-before-Open guard ordering directly, since none of those need a live tmux/claude substrate
// or even a git repository beyond a plain t.TempDir().
// Pathspec-shape coverage now lives in sync_integration_test.go, which proves the exclude-file
// transients stay uncommitted through a real git repo rather than asserting a pathspec string shape
// against a since-deleted helper.
// Every fixture here builds a *websterCLI literal directly, bypassing Command()'s
// PersistentPreRunE, webster's own package-local injection point for these tests.
// Every other verb's own behavior (begin-batch, record-batch, recover-batch, run) is covered by
// verbs_test.go.
package webstercli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

func TestRunCLI_NoArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) = %d; want 0", exitCode)
	}
}

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

func TestRunCLI_GroupGuard_OutsideGitRepo(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) outside a git repo = %d; want 0", exitCode)
	}
}

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

func TestCommand_AllEightSubcommandsRegistered(t *testing.T) {
	want := []string{"validate", "run", "status", "pause", "begin-batch", "await-batch", "record-batch", "recover-batch"}
	got := map[string]bool{}
	for _, sub := range Command().Commands() {
		got[sub.Name()] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("Command() is missing subcommand %q", name)
		}
	}
}

func TestCommand_LongStringsHaveNoStaleBatchLanguage(t *testing.T) {
	forbidden := []string{"--restart-chain", "restart-chain", "chain", "oversized"}

	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		lower := strings.ToLower(cmd.Long)
		for _, bad := range forbidden {
			if strings.Contains(lower, bad) {
				t.Errorf("command %q Long string contains stale batch-era language %q:\n%s", cmd.CommandPath(), bad, cmd.Long)
			}
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(Command())
}

// containsString reports whether haystack contains needle.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// TestFabricSync_SkipGitBypassNeedsNoFabricWorktree verifies the WEFT_SKIP_GIT bypass short-circuits
// before path validation.
func TestFabricSync_SkipGitBypassNeedsNoFabricWorktree(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	t.Setenv("WEFT_SKIP_PUSH", "")

	hub := t.TempDir()
	layout := &lyxcwd.Location{HubPath: hub, WorktreeName: filepath.Base(filepath.Join(hub, "warp")), AnchorRel: "."}
	open := func() (*fabricengine.Fabric, error) { return fabricengine.Open(layout) }

	committed, err := fabricSync(open, layout.AnchorRel, "bypass probe")
	if err != nil {
		t.Fatalf("fabricSync() error = %v; want nil, the bypass must never touch the filesystem or git", err)
	}
	if committed {
		t.Error("fabricSync() committed = true; want false in bypass mode")
	}
}

// TestFabricSync_NonBypassValidatesPairPaths verifies fabricSync validates paths without
// WEFT_SKIP_GIT.
func TestFabricSync_NonBypassValidatesPairPaths(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	t.Setenv("WEFT_SKIP_PUSH", "")

	hub := t.TempDir()
	layout := &lyxcwd.Location{HubPath: hub, WorktreeName: filepath.Base(filepath.Join(hub, "warp")), AnchorRel: "."}
	open := func() (*fabricengine.Fabric, error) { return fabricengine.Open(layout) }

	committed, err := fabricSync(open, layout.AnchorRel, "missing-pair probe")
	if committed {
		t.Error("fabricSync() committed = true; want false, no repo exists to commit to")
	}
	var missing *fabricengine.ErrMissingPath
	if !errors.As(err, &missing) {
		t.Fatalf("fabricSync() error = %v; want a *fabricengine.ErrMissingPath from Open's stat validation", err)
	}
}

// TestRunDeps_OpenBisectorNilWhenOpenFabricNil proves run.go's own websterengine.RunDeps
// construction, not just c.openFabric in isolation: with c.openFabric nil (standalone mode),
// runDeps must leave RunDeps.OpenBisector literally nil rather than wrapping the nil opener in a
// non-nil closure, since a non-nil closure over a nil c.openFabric panics the first time
// runIntegrationStage invokes it instead of taking its own nil-OpenBisector bypass.
func TestRunDeps_OpenBisectorNilWhenOpenFabricNil(t *testing.T) {
	c := &websterCLI{cfg: websterengine.Config{}}

	deps := c.runDeps()

	if deps.OpenBisector != nil {
		t.Error("runDeps().OpenBisector != nil; want nil when c.openFabric is nil")
	}
}

// TestRunDeps_OpenBisectorWrapsOpenFabric proves the hub-mode half of the same construction: a
// non-nil c.openFabric is wrapped into a non-nil OpenBisector that proxies through to it.
func TestRunDeps_OpenBisectorWrapsOpenFabric(t *testing.T) {
	wantErr := errors.New("probe: openFabric reached")
	c := &websterCLI{
		cfg:        websterengine.Config{},
		openFabric: func() (*fabricengine.Fabric, error) { return nil, wantErr },
	}

	deps := c.runDeps()

	if deps.OpenBisector == nil {
		t.Fatal("runDeps().OpenBisector = nil; want a wrapped closure when c.openFabric is non-nil")
	}
	if _, err := deps.OpenBisector(); !errors.Is(err, wantErr) {
		t.Errorf("runDeps().OpenBisector() error = %v; want the wrapped c.openFabric() error %v", err, wantErr)
	}
}

// newTestCLI builds a minimal *websterCLI for validate/status/pause testing without a live git repo.
//
// The layout anchors at the non-"." subpath "backend" so AnchorPath() and WorktreePath() are
// distinguishable strings, proving the plan paths behave correctly at a nested anchor. It does NOT
// prove anchoring itself: this helper both computes planDir and is the same value the tests seed
// into via seedValidPlanDir, so a WorktreePath() slip at the computation below would stay
// self-consistent and pass at any AnchorRel. The subpath-anchored PersistentPreRunE case in
// internal/webstercli/verbs_test.go is the place anchoring is actually proven for this module.
func newTestCLI(t *testing.T) (*websterCLI, string) {
	t.Helper()
	hub := t.TempDir()
	layout := &lyxcwd.Location{HubPath: filepath.Dir(hub), WorktreeName: filepath.Base(hub), AnchorRel: "backend"}
	c := &websterCLI{
		cfg:        websterengine.Config{},
		geom:       hubgeom.WebsterGeometry(layout),
		anchorRel:  layout.AnchorRel,
		refMatcher: fabricengine.NewRefScanner(layout),
		openFabric: func() (*fabricengine.Fabric, error) { return fabricengine.Open(layout) },
	}
	return c, hub
}

// seedValidPlanDir writes a valid plan-format plan with one card into dir.
func seedValidPlanDir(t *testing.T, dir string) {
	t.Helper()
	overview := "---\nformat: 3\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n" +
		"1 — only — placeholder card\n"
	card := "# Card 1 — only\n\n**What:** placeholder card.\n**Context:** none\n**Edits:** none\n" +
		"**Creates:**\n- `internal/only/new.go`\n**Deletes:** none\n**Moves:** none\n**Depends-on:** none\n"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "01-only.md"), []byte(card), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}
}

func TestValidateCmd_ValidPlan(t *testing.T) {
	c, _ := newTestCLI(t)
	seedValidPlanDir(t, c.geom.PlanDir)

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.validateCmd(), &out, nil)

	if exitCode != 0 {
		t.Fatalf("validate on a clean plan = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"valid":true`) {
		t.Errorf("output missing valid:true; got %q", got)
	}
	if !strings.Contains(got, `"cards":1`) {
		t.Errorf("output missing cards:1; got %q", got)
	}
}

func TestValidateCmd_MissingPlan(t *testing.T) {
	c, _ := newTestCLI(t)

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.validateCmd(), &out, nil)

	if exitCode != 1 {
		t.Fatalf("validate on a missing plan = %d; want 1, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Errorf("output missing ok:false; got %q", out.String())
	}
}

// seedMissingFieldPlanDir writes a plan with a card missing the **Deletes:** label.
func seedMissingFieldPlanDir(t *testing.T, dir string) {
	t.Helper()
	overview := "---\nformat: 3\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n" +
		"1 — only — placeholder card\n"
	card := "# Card 1 — only\n\n**What:** placeholder card.\n**Context:** none\n**Edits:** none\n" +
		"**Creates:**\n- `internal/only/new.go`\n**Moves:** none\n**Depends-on:** none\n"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "01-only.md"), []byte(card), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}
}

func TestValidateCmd_FindingsUseCardKey(t *testing.T) {
	c, _ := newTestCLI(t)
	seedMissingFieldPlanDir(t, c.geom.PlanDir)

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.validateCmd(), &out, nil)

	if exitCode != 1 {
		t.Fatalf("validate on a plan missing a required field = %d; want 1, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"ok":false`) {
		t.Errorf("output missing ok:false; got %q", got)
	}
	if !strings.Contains(got, `"check":"card-missing-field"`) {
		t.Errorf("output missing check:card-missing-field; got %q", got)
	}
	if !strings.Contains(got, `"card":"1-only"`) {
		t.Errorf("output missing card:1-only (findingsEnvelope must key each finding by f.Card, not f.Batch); got %q", got)
	}
	if strings.Contains(got, `"batch":`) {
		t.Errorf("output carries a stale batch key; findingsEnvelope must emit only check/card/detail; got %q", got)
	}
}

func TestStatusCmd_NotInitialized(t *testing.T) {
	c, _ := newTestCLI(t)

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.statusCmd(), &out, nil)

	if exitCode != 0 {
		t.Fatalf("status with no state.json = %d; want 0, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"initialized":false`) {
		t.Errorf("output missing initialized:false; got %q", out.String())
	}
}

func TestStatusCmd_WithBatches(t *testing.T) {
	c, _ := newTestCLI(t)

	st := &websterengine.State{
		RunGUID:         "guid-1",
		PlanFingerprint: "fp-1",
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "first", Kind: "fork", Status: "done", Terminal: true, Digest: &fakeDigest},
			2: {Slug: "second", Kind: "recovery", Status: "", Terminal: false},
		},
	}
	if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.statusCmd(), &out, nil)

	if exitCode != 0 {
		t.Fatalf("status = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	for _, want := range []string{
		`"run_guid":"guid-1"`, `"plan_fingerprint":"fp-1"`,
		`"kind":"fork"`, `"kind":"recovery"`,
		`"has_digest":true`, `"has_digest":false`,
		`"terminal":true`, `"terminal":false`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("status output missing %q; got %q", want, got)
		}
	}
}

func TestPauseCmd_RequestsPauseIdempotent(t *testing.T) {
	c, _ := newTestCLI(t)

	for i := 0; i < 2; i++ {
		var out bytes.Buffer
		exitCode := clihelp.Execute(c.pauseCmd(), &out, nil)
		if exitCode != 0 {
			t.Fatalf("pause call %d = %d; want 0, output: %s", i+1, exitCode, out.String())
		}
		if !strings.Contains(out.String(), `"paused":true`) {
			t.Errorf("pause call %d output missing paused:true; got %q", i+1, out.String())
		}
	}
}

var fakeDigest = websterengine.Digest{Batch: "01-first", Status: websterengine.DigestStatusDone}
