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
	"regexp"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/batcher"
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

// seedValidPlanDir writes a valid format-4 plan with one card into dir.
func seedValidPlanDir(t *testing.T, dir string) {
	t.Helper()
	overview := "---\nformat: 5\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n" +
		"1 — only — placeholder card\n"
	card := "# Card 1 — only\n\n**Create:**\n- `internal/only/new.go`\n\n**Intent:** placeholder card.\n"
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

// seedMissingIntentPlanDir writes a format-4 plan with a card missing the **Intent:** label.
func seedMissingIntentPlanDir(t *testing.T, dir string) {
	t.Helper()
	overview := "---\nformat: 5\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n" +
		"1 — only — placeholder card\n"
	card := "# Card 1 — only\n\n**Create:**\n- `internal/only/new.go`\n"
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
	seedMissingIntentPlanDir(t, c.geom.PlanDir)

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

// seedGlyphPlanDir writes a syntactically complete, one-card language: go plan into dir, whose sole
// card's Create group targets createTarget. It also writes worktreeRoot/sub/a.go so a glyph target
// naming the "sub" unit resolves, and returns nothing -- callers read back through c.geom.PlanDir /
// c.geom.WorktreeRoot exactly as seedValidPlanDir's other callers do.
func seedGlyphPlanDir(t *testing.T, planDir, worktreeRoot, createTarget string) {
	t.Helper()

	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	overview := "---\nformat: 5\napproved: true\nlanguage: go\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n" +
		"1 — only — placeholder card\n"
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview: %v", err)
	}
	card := "# Card 1 — only\n\n**Create:**\n- `" + createTarget + "`\n\n**Intent:** placeholder card.\n"
	if err := os.WriteFile(filepath.Join(planDir, "01-only.md"), []byte(card), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}

	subDir := filepath.Join(worktreeRoot, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "a.go"), []byte("package sub\n\nfunc Foo() {}\n"), 0o644); err != nil {
		t.Fatalf("write sub/a.go: %v", err)
	}
}

// TestValidateCmd_InformationalFindingsSurfaceOnSuccess asserts that an informational-only findings
// set (create-new-unit, on a Create target introducing a brand-new package) still reports a
// success envelope, carrying the findings under their own key rather than dropping them.
func TestValidateCmd_InformationalFindingsSurfaceOnSuccess(t *testing.T) {
	c, _ := newTestCLI(t)
	seedGlyphPlanDir(t, c.geom.PlanDir, c.geom.WorktreeRoot, "newpkg#Bar")

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.validateCmd(), &out, nil)

	if exitCode != 0 {
		t.Fatalf("validate on an informational-only plan = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"valid":true`) {
		t.Errorf("output missing valid:true; got %q", got)
	}
	if !strings.Contains(got, "create-new-unit") {
		t.Errorf("output missing the surfaced informational finding create-new-unit; got %q", got)
	}
}

// TestValidateCmd_BlockingFindingCarriesSeverity asserts a blocking finding's error envelope
// carries the finding's own severity alongside check/card/detail.
func TestValidateCmd_BlockingFindingCarriesSeverity(t *testing.T) {
	c, _ := newTestCLI(t)
	// "sub#Missing" resolves not_found with unit: found -- statusFindings' blocking glyph-not-found.
	seedGlyphPlanDir(t, c.geom.PlanDir, c.geom.WorktreeRoot, "sub#Foo")
	card := "# Card 1 — only\n\n**Create:**\n- `sub#Foo`\n\n**Uses:**\n- `sub#Missing`\n\n**Intent:** placeholder card.\n"
	if err := os.WriteFile(filepath.Join(c.geom.PlanDir, "01-only.md"), []byte(card), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.validateCmd(), &out, nil)

	if exitCode != 1 {
		t.Fatalf("validate on a blocking-findings plan = %d; want 1, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"ok":false`) {
		t.Errorf("output missing ok:false; got %q", got)
	}
	if !strings.Contains(got, `"check":"glyph-not-found"`) {
		t.Errorf("output missing check:glyph-not-found; got %q", got)
	}
	if !strings.Contains(got, `"severity":"blocking"`) {
		t.Errorf("output missing severity:blocking on the blocking finding; got %q", got)
	}
}

// TestValidateCmd_QuarryUnavailableNamesQuarry asserts that a quarry-unavailable error maps to
// output.Err with a message naming quarry rather than the plan.
func TestValidateCmd_QuarryUnavailableNamesQuarry(t *testing.T) {
	c, _ := newTestCLI(t)
	planDir := c.geom.PlanDir
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	overview := "---\nformat: 5\napproved: true\nlanguage: go\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n" +
		"1 — only — placeholder card\n"
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview: %v", err)
	}
	card := "# Card 1 — only\n\n**Create:**\n- `sub#Foo`\n\n**Intent:** placeholder card.\n"
	if err := os.WriteFile(filepath.Join(planDir, "01-only.md"), []byte(card), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}
	// c.geom is a plain struct value, so this field write reaches only this test's *websterCLI;
	// pointing WorktreeRoot at a path that does not exist, decoupled from planDir's own real
	// on-disk location, is what makes quarry.Open fail without disturbing ParsePlan's own read.
	c.geom.WorktreeRoot = filepath.Join(t.TempDir(), "does-not-exist")

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.validateCmd(), &out, nil)

	if exitCode != 1 {
		t.Fatalf("validate with an unopenable worktreeRoot = %d; want 1, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), "quarry") {
		t.Errorf("output does not name quarry; want it to name quarry rather than the plan; got %q", out.String())
	}
}

// seedTwoCardGlyphPlanDir writes a two-card, language: go plan into planDir whose FIRST card
// Creates a symbol that already exists on disk (worktreeRoot/sub/a.go's Foo) and whose SECOND card
// Creates a brand-new unit. The first card is therefore a blocking create-already-exists finding
// under the whole-plan check set and nothing at all once it counts as completed; the second card is
// informational in both scopes. That asymmetry is what lets one plan tell validate's two scopes
// apart.
func seedTwoCardGlyphPlanDir(t *testing.T, planDir, worktreeRoot string) {
	t.Helper()

	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	overview := "---\nformat: 5\napproved: true\nlanguage: go\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n" +
		"1 — first — the card whose work has already landed\n" +
		"2 — second — the card still pending\n"
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview: %v", err)
	}
	first := "# Card 1 — first\n\n**Create:**\n- `sub#Foo`\n\n**Intent:** the card whose work has already landed.\n"
	if err := os.WriteFile(filepath.Join(planDir, "01-first.md"), []byte(first), 0o644); err != nil {
		t.Fatalf("write first card file: %v", err)
	}
	second := "# Card 2 — second\n\n**Create:**\n- `newpkg#Bar`\n\n**Intent:** the card still pending.\n"
	if err := os.WriteFile(filepath.Join(planDir, "02-second.md"), []byte(second), 0o644); err != nil {
		t.Fatalf("write second card file: %v", err)
	}

	subDir := filepath.Join(worktreeRoot, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "a.go"), []byte("package sub\n\nfunc Foo() {}\n"), 0o644); err != nil {
		t.Fatalf("write sub/a.go: %v", err)
	}
}

// TestValidateCmd_ScopeFollowsRunProgress is R4-07's direct regression test. The verb advertises
// itself as the gate "lyx webster run" applies, and Run scopes that gate by the run's own completed
// cards; validate ran the whole-plan check set unconditionally, so the instant one Create card
// landed the verb exited 1 over a plan Run resumes without complaint.
//
// One plan drives both rows. With no run recorded, card 1's already-existing Create target is a
// blocking create-already-exists and the verb must still refuse -- that is the pre-flight answer the
// verb exists for. With state.json recording batch 1 terminal, that same finding is the plan working
// as designed and must vanish, leaving only card 2's informational finding and exit 0.
func TestValidateCmd_ScopeFollowsRunProgress(t *testing.T) {
	identity, err := batcher.Select("identity")
	if err != nil {
		t.Fatalf("batcher.Select(identity) = %v; want nil", err)
	}

	t.Run("NoRunRecordedGetsWholePlanAnswer", func(t *testing.T) {
		c, _ := newTestCLI(t)
		c.batcher = identity
		seedTwoCardGlyphPlanDir(t, c.geom.PlanDir, c.geom.WorktreeRoot)

		var out bytes.Buffer
		exitCode := clihelp.Execute(c.validateCmd(), &out, nil)

		if exitCode != 1 {
			t.Fatalf("validate with no run recorded = %d; want 1 (the whole-plan answer still refuses card 1), output: %s", exitCode, out.String())
		}
		got := out.String()
		if !strings.Contains(got, `"scope":"whole-plan"`) {
			t.Errorf("output missing scope:whole-plan; got %q", got)
		}
		if !strings.Contains(got, "create-already-exists") {
			t.Errorf("output missing the blocking create-already-exists finding for card 1; got %q", got)
		}
	})

	t.Run("TerminalBatchScopesToPendingCards", func(t *testing.T) {
		c, _ := newTestCLI(t)
		c.batcher = identity
		seedTwoCardGlyphPlanDir(t, c.geom.PlanDir, c.geom.WorktreeRoot)

		state := &websterengine.State{
			RunGUID: "run-guid",
			Batches: map[int]*websterengine.BatchState{
				1: {Terminal: true, Status: "done"},
			},
		}
		if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, state); err != nil {
			t.Fatalf("SaveState: %v", err)
		}

		var out bytes.Buffer
		exitCode := clihelp.Execute(c.validateCmd(), &out, nil)

		if exitCode != 0 {
			t.Fatalf("validate with batch 1 terminal = %d; want 0 -- a completed Create card's target existing is the plan working as designed, output: %s", exitCode, out.String())
		}
		got := out.String()
		if !strings.Contains(got, `"scope":"pending"`) {
			t.Errorf("output missing scope:pending; got %q", got)
		}
		if strings.Contains(got, "create-already-exists") {
			t.Errorf("output still carries card 1's create-already-exists finding after batch 1 went terminal; got %q", got)
		}
	})
}

// TestValidateCmd_RebaselinesStalePlanFingerprint is WS-1's own regression test (crucible round
// sonnet-xhigh-r8): validate's own resolve pass can rewrite the plan on disk (handle
// canonicalization) exactly as begin-batch's own ValidateDispatch call can, but before this fix it
// never restamped state.json's PlanFingerprint afterward the way every bracket verb already does —
// so a run's crash/resume guard silently desynced from a plan validate itself had just rewritten,
// and the next begin-batch/record-batch/run refused the (validate's own sanctioned) edit as a
// foreign one, forcing --fresh and discarding the run's progress.
//
// This drives the observable end state directly rather than depending on quarry's own handle
// grammar to construct a genuine mid-call rewrite: state.json is seeded with a fingerprint that
// does NOT match the real on-disk plan (standing in for "the plan changed since state.json was last
// written, by validate's own rewrite or otherwise"), and the assertion is that validate corrects it
// to the plan's actual current fingerprint — the same unconditional re-baseline begin-batch performs
// after every ValidateDispatch call, regardless of whether that specific call happened to rewrite
// anything.
func TestValidateCmd_RebaselinesStalePlanFingerprint(t *testing.T) {
	identity, err := batcher.Select("identity")
	if err != nil {
		t.Fatalf("batcher.Select(identity) = %v; want nil", err)
	}

	c, _ := newTestCLI(t)
	c.batcher = identity
	seedValidPlanDir(t, c.geom.PlanDir)

	realFingerprint, err := websterengine.Fingerprint(c.geom.PlanDir)
	if err != nil {
		t.Fatalf("websterengine.Fingerprint(planDir) = %v; want nil", err)
	}

	state := &websterengine.State{
		RunGUID:         "run-guid",
		PlanFingerprint: "stale-fingerprint-from-before-the-plan-changed",
	}
	if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.validateCmd(), &out, nil)
	if exitCode != 0 {
		t.Fatalf("validate on a clean plan = %d; want 0, output: %s", exitCode, out.String())
	}

	reloaded, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState after validate: %v", err)
	}
	if reloaded == nil {
		t.Fatalf("LoadState after validate returned nil; want the seeded state still present")
	}
	if reloaded.PlanFingerprint != realFingerprint {
		t.Errorf("state.json's PlanFingerprint after validate = %q; want %q (the plan's real current fingerprint) — a stale fingerprint left in place is exactly the desync that forces every later begin-batch/record-batch/run to refuse a genuinely current plan as foreign", reloaded.PlanFingerprint, realFingerprint)
	}
	// The run's other fields survive the re-baseline untouched — persistPlanFingerprintRebaseline
	// reloads state fresh and writes only the fingerprint field, never the caller's whole in-memory
	// copy, for exactly the reason its own doc comment states (R6-4).
	if reloaded.RunGUID != "run-guid" {
		t.Errorf("state.json's RunGUID after validate = %q; want %q (the fingerprint restamp must not clobber other fields)", reloaded.RunGUID, "run-guid")
	}
}

// TestRecoverBatchCmd_BootsStandaloneReedSessionFirst is R4-11's direct regression test.
// recover-batch spawns a COLD recovery strand through reed.AddStrand, which needs a live reed
// session, but it never called the in-process bring-up seam wireStandalone arms. In standalone mode
// the session is gone once a run ends, so the verb failed inside AddStrand advising `lyx reed up` —
// a hub-only verb that cannot reach standalone geometry at all.
//
// Batch 99 exists in no plan, so RecoverSpawnOrAttach's own findBatch refuses it immediately. That
// is what makes this test an ordering proof rather than a mere presence one: the bring-up message
// can only win over the batch-not-found message if the guard runs before the spawn machinery.
func TestRecoverBatchCmd_BootsStandaloneReedSessionFirst(t *testing.T) {
	identity, err := batcher.Select("identity")
	if err != nil {
		t.Fatalf("batcher.Select(identity) = %v; want nil", err)
	}

	c, _ := newTestCLI(t)
	c.batcher = identity
	seedValidPlanDir(t, c.geom.PlanDir)
	if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, &websterengine.State{
		RunGUID: "run-guid",
		Batches: map[int]*websterengine.BatchState{},
	}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	bringUps := 0
	c.reedUp = func() error {
		bringUps++
		return errors.New("no tmux server available in this test")
	}

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.recoverBatchCmd(), &out, []string{"99", "--wait", "1ns"})

	if bringUps != 1 {
		t.Fatalf("c.reedUp calls = %d; want exactly 1 -- recover-batch spawns an agent and must boot standalone's own reed session first", bringUps)
	}
	if exitCode != 1 {
		t.Fatalf("recover-batch with a failing reed bring-up = %d; want 1, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "bring up the standalone reed session") {
		t.Errorf("output does not name the reed bring-up failure; got %q", got)
	}
	if strings.Contains(got, "not found in the plan's execution batches") {
		t.Errorf("output reports the batch-not-found refusal, so the bring-up ran too late to matter; got %q", got)
	}
}

// singleFlagEnvelope matches the one-word machine-readable signal shape webster's verbs use to tell
// Master WHY a call refused: a whole envelope whose only field is a boolean flag
// (map[string]any{"plan_drifted": true}). Master keys a failure-ladder rung off each such flag, so
// the flag is a contract term, not an implementation detail.
var singleFlagEnvelope = regexp.MustCompile(`map\[string\]any\{"([a-z_]+)": true\}`)

// TestMasterStencilCoversEverySingleFlagRefusal is R4-34's direct regression test. record-batch
// emits {"card_not_done": true} on an ErrCardNotDone refusal, but the Master stencil's failure
// ladder carried no rung for it, so Master fell through to generic error handling on a refusal with
// a specific meaning and a specific disposition.
//
// The flag set is read out of this package's own source rather than pinned as a list, because a
// hand-maintained list is forgotten by exactly the change that adds a new flag -- which is how this
// gap arose. What the regexp deliberately does NOT cover is a flag riding along inside a
// multi-field envelope (status's own "paused" report field, for instance): those are state a caller
// reads, not refusals a ladder must answer.
func TestMasterStencilCoversEverySingleFlagRefusal(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the webstercli package directory: %v", err)
	}

	found := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, match := range singleFlagEnvelope.FindAllStringSubmatch(string(source), -1) {
			found[match[1]] = name
		}
	}

	if len(found) == 0 {
		t.Fatal("no single-flag refusal envelopes found in this package; the regexp has drifted from the code shape it is meant to track, so this test is asserting nothing")
	}

	for flag, file := range found {
		if !strings.Contains(string(stencils.WebsterTemplateMaster), flag) {
			t.Errorf("%s emits the single-flag envelope {%q: true}, but contracts/stencils/webster/webster-template-master.md's failure ladder never mentions %q -- Master would fall through to generic error handling on a refusal that has its own meaning and its own disposition", file, flag, flag)
		}
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
