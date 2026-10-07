// cli_test.go covers the webstercli cobra seam through RunCLI: the PersistentPreRunE group-command guard and the help-tree stale-language check.
// It also covers the spawn-free verbs (validate, status, pause, await-batch, rebaseline, restore-plan, and the no-run refusals of accept-audit and restore-plan) and fabricSync's SkipGit-before-Open guard ordering directly, since none of those need a live tmux/claude substrate or even a git repository beyond a plain t.TempDir().
// Pathspec-shape coverage now lives in sync_integration_test.go, which proves the exclude-file transients stay uncommitted through a real git repo rather than asserting a pathspec string shape against a since-deleted helper.
// Every fixture here builds a *websterCLI literal directly, bypassing Command()'s PersistentPreRunE, webster's own package-local injection point for these tests.
// Every other verb's own behavior (begin-batch, record-batch, recover-batch, run) is covered by verbs_test.go.
package webstercli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

func TestRunCLI_GroupGuard_OutsideGitRepo(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	// An empty, non-nil slice keeps cobra from reading the test binary's own flags as arguments.
	exitCode := RunCLI(&out, []string{})

	if exitCode != 0 {
		t.Errorf("RunCLI(no arguments) outside a git repo = %d; want 0", exitCode)
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

// TestFabricSync_GuardOrdering verifies the WEFT_SKIP_GIT bypass short-circuits before path validation, and that without it fabricSync validates the pair's paths before any git work.
// It sets WEFT_SKIP_GIT and WEFT_SKIP_PUSH, so it is not parallel.
func TestFabricSync_GuardOrdering(t *testing.T) {
	cases := []struct {
		name     string
		skipGit  string
		wantMiss bool
	}{
		{"skip-git bypass needs no fabric worktree", "1", false},
		{"non-bypass validates the pair paths", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("WEFT_SKIP_GIT", tc.skipGit)
			t.Setenv("WEFT_SKIP_PUSH", "")

			hub := t.TempDir()
			layout := &lyxcwd.Location{HubPath: hub, WorktreeName: filepath.Base(filepath.Join(hub, "pair")), AnchorRel: "."}
			open := func() (*fabricengine.Fabric, error) { return fabricengine.Open(layout) }

			committed, err := fabricSync(open, layout.AnchorRel, "guard probe")
			if committed {
				t.Error("fabricSync() committed = true; want false, nothing can be committed")
			}
			if !tc.wantMiss {
				if err != nil {
					t.Fatalf("fabricSync() error = %v; want nil, the bypass must never touch the filesystem or git", err)
				}
				return
			}
			var missing *fabricengine.ErrMissingPath
			if !errors.As(err, &missing) {
				t.Fatalf("fabricSync() error = %v; want a *fabricengine.ErrMissingPath from Open's stat validation", err)
			}
		})
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

// onlyCreatePlan returns a one-card plan whose card Creates target.
func onlyCreatePlan(language, target string) plankit.Plan {
	return plankit.Plan{
		Approved: true,
		Language: language,
		Framing:  "Framing.",
		Cards: []plankit.Card{{
			Number:  1,
			Slug:    "only",
			Summary: "placeholder card",
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{target}}},
			Intent:  "placeholder card.",
		}},
	}
}

// twoCardUsesPlan returns a two-card plan in which the "consumer" card Uses the file the "producer" card Creates;
// consumerNumber and producerNumber place the two cards, so the plan is in executable order only when the producer's number is lower.
func twoCardUsesPlan(consumerNumber, producerNumber int) plankit.Plan {
	cards := []plankit.Card{
		{
			Number:  consumerNumber,
			Slug:    "consumer",
			Summary: "reads the producer's file",
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{"internal/consumer/new.go"}}},
			Uses:    []string{"internal/producer/new.go"},
			Intent:  "placeholder card.",
		},
		{
			Number:  producerNumber,
			Slug:    "producer",
			Summary: "creates the file",
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{"internal/producer/new.go"}}},
			Intent:  "placeholder card.",
		},
	}
	if consumerNumber > producerNumber {
		cards[0], cards[1] = cards[1], cards[0]
	}
	return plankit.Plan{Approved: true, Framing: "Framing.", Cards: cards}
}

// seedValidPlanDir writes a valid plan with one card into dir.
func seedValidPlanDir(t *testing.T, dir string) {
	t.Helper()
	plankit.Write(t, dir, onlyCreatePlan("", "internal/only/new.go"))
}

// TestValidateCmd_Envelopes drives validate over one plan state per row and checks its exit code and envelope:
// a clean plan, a missing plan, a finding keyed by card (never by batch), an informational-only finding that still reports success, a blocking finding that carries its own severity, and a worktree root quarry cannot open, which names quarry rather than the plan.
func TestValidateCmd_Envelopes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		seed     func(t *testing.T, c *websterCLI)
		wantExit int
		wantIn   []string
		wantOut  []string
	}{
		{
			name:     "clean plan",
			seed:     func(t *testing.T, c *websterCLI) { seedValidPlanDir(t, c.geom.PlanDir) },
			wantExit: 0,
			wantIn:   []string{`"valid":true`, `"cards":1`},
		},
		{
			name:     "missing plan",
			seed:     func(*testing.T, *websterCLI) {},
			wantExit: 1,
			wantIn:   []string{`"ok":false`},
		},
		{
			name:     "findings use the card key",
			seed:     func(t *testing.T, c *websterCLI) { seedMissingIntentPlanDir(t, c.geom.PlanDir) },
			wantExit: 1,
			wantIn:   []string{`"ok":false`, `"check":"card-missing-field"`, `"card":"1-only"`},
			// findingsEnvelope must key each finding by f.Card, not f.Batch, and emit only check/card/detail.
			wantOut: []string{`"batch":`},
		},
		{
			name: "informational findings surface on success",
			seed: func(t *testing.T, c *websterCLI) {
				seedGlyphPlanDir(t, c.geom.PlanDir, c.geom.WorktreeRoot, "newpkg#Bar")
			},
			wantExit: 0,
			wantIn:   []string{`"valid":true`, "create-new-unit"},
		},
		{
			name: "blocking finding carries its severity",
			seed: func(t *testing.T, c *websterCLI) {
				// "sub#Missing" resolves not_found with unit: found -- statusFindings' blocking glyph-not-found.
				seedGlyphPlanDir(t, c.geom.PlanDir, c.geom.WorktreeRoot, "sub#Foo")
				card := "# Card 1 — only\n\n**Create:**\n- `sub#Foo`\n\n**Uses:**\n- `sub#Missing`\n\n**Intent:** placeholder card.\n"
				if err := os.WriteFile(filepath.Join(c.geom.PlanDir, "01-only.md"), []byte(card), 0o644); err != nil {
					t.Fatalf("write card file: %v", err)
				}
			},
			wantExit: 1,
			wantIn:   []string{`"ok":false`, `"check":"glyph-not-found"`, `"severity":"blocking"`},
		},
		{
			name:     "a Uses naming a later card's target is refused under the consumer card",
			seed:     func(t *testing.T, c *websterCLI) { plankit.Write(t, c.geom.PlanDir, twoCardUsesPlan(1, 2)) },
			wantExit: 1,
			wantIn:   []string{`"ok":false`, `"check":"uses-later-target"`, `"card":"1-consumer"`, "card 2 targets"},
		},
		{
			name:     "the same Uses after the card that produces its target passes",
			seed:     func(t *testing.T, c *websterCLI) { plankit.Write(t, c.geom.PlanDir, twoCardUsesPlan(2, 1)) },
			wantExit: 0,
			wantIn:   []string{`"valid":true`, `"cards":2`},
		},
		{
			name: "unopenable worktree root names quarry",
			seed: func(t *testing.T, c *websterCLI) {
				plankit.Write(t, c.geom.PlanDir, onlyCreatePlan("go", "sub#Foo"))
				// c.geom is a plain struct value, so this write reaches only this row's *websterCLI;
				// a path that does not exist, decoupled from the plan's own on-disk location, makes quarry.Open fail without disturbing the plan read.
				c.geom.WorktreeRoot = filepath.Join(t.TempDir(), "does-not-exist")
			},
			wantExit: 1,
			wantIn:   []string{"quarry"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, _ := newTestCLI(t)
			tc.seed(t, c)

			var out bytes.Buffer
			exitCode := clihelp.Execute(c.validateCmd(), &out, []string{})

			got := out.String()
			if exitCode != tc.wantExit {
				t.Fatalf("validate = %d; want %d, output: %s", exitCode, tc.wantExit, got)
			}
			for _, want := range tc.wantIn {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q; got %q", want, got)
				}
			}
			for _, unwanted := range tc.wantOut {
				if strings.Contains(got, unwanted) {
					t.Errorf("output carries %q; got %q", unwanted, got)
				}
			}
		})
	}
}

// seedMissingIntentPlanDir writes a plan with a card missing the **Intent:** label.
func seedMissingIntentPlanDir(t *testing.T, dir string) {
	t.Helper()
	files := plankit.Render(onlyCreatePlan("", "internal/only/new.go"))
	card := files["01-only.md"]
	cut := bytes.Index(card, []byte("\n**Intent:**"))
	if cut < 0 {
		t.Fatalf("rendered card has no Intent label to remove: %q", card)
	}
	files["01-only.md"] = card[:cut]
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

// seedGlyphPlanDir writes a syntactically complete, one-card language: go plan into dir, whose sole
// card's Create group targets createTarget. It also writes worktreeRoot/sub/a.go so a glyph target
// naming the "sub" unit resolves, and returns nothing -- callers read back through c.geom.PlanDir /
// c.geom.WorktreeRoot exactly as seedValidPlanDir's other callers do.
func seedGlyphPlanDir(t *testing.T, planDir, worktreeRoot, createTarget string) {
	t.Helper()

	plankit.Write(t, planDir, onlyCreatePlan("go", createTarget))

	subDir := filepath.Join(worktreeRoot, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "a.go"), []byte("package sub\n\nfunc Foo() {}\n"), 0o644); err != nil {
		t.Fatalf("write sub/a.go: %v", err)
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

	plankit.Write(t, planDir, plankit.Plan{
		Approved: true,
		Language: "go",
		Framing:  "Framing.",
		Cards: []plankit.Card{
			{
				Number:  1,
				Slug:    "first",
				Summary: "the card whose work has already landed",
				Groups:  []plankit.Group{{Label: "Create", Targets: []string{"sub#Foo"}}},
			},
			{
				Number:  2,
				Slug:    "second",
				Summary: "the card still pending",
				Groups:  []plankit.Group{{Label: "Create", Targets: []string{"newpkg#Bar"}}},
			},
		},
	})

	subDir := filepath.Join(worktreeRoot, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "a.go"), []byte("package sub\n\nfunc Foo() {}\n"), 0o644); err != nil {
		t.Fatalf("write sub/a.go: %v", err)
	}
}

// TestValidateCmd_ScopeFollowsRunProgress is R4-07's direct regression test.
// The verb advertises itself as the gate "lyx webster run" applies, and Run scopes that gate by the run's begun cards;
// validate ran the whole-plan check set unconditionally, so the instant one Create card landed the verb exited 1 over a plan Run resumes without complaint.
//
// One plan drives every row.
// With no run recorded, card 1's already-existing Create target is a blocking create-already-exists and the verb must still refuse -- that is the pre-flight answer the verb exists for.
// With state.json recording batch 1 begun, terminal or not, that same finding is the plan working as designed and must vanish, leaving only card 2's informational finding and exit 0.
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
		exitCode := clihelp.Execute(c.validateCmd(), &out, []string{})

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

	for _, tc := range []struct {
		name  string
		batch *websterengine.BatchState
	}{
		{"TerminalBatchScopesToPendingCards", &websterengine.BatchState{Terminal: true, Status: "done"}},
		{"BegunBatchScopesToPendingCards", &websterengine.BatchState{Slug: "first"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestCLI(t)
			c.batcher = identity
			seedTwoCardGlyphPlanDir(t, c.geom.PlanDir, c.geom.WorktreeRoot)

			fingerprint, err := websterengine.Fingerprint(c.geom.PlanDir)
			if err != nil {
				t.Fatalf("websterengine.Fingerprint(planDir) = %v; want nil", err)
			}
			state := &websterengine.State{
				RunGUID:         "run-guid",
				PlanFingerprint: fingerprint,
				Batches:         map[int]*websterengine.BatchState{1: tc.batch},
			}
			if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, state); err != nil {
				t.Fatalf("SaveState: %v", err)
			}

			var out bytes.Buffer
			exitCode := clihelp.Execute(c.validateCmd(), &out, []string{})

			if exitCode != 0 {
				t.Fatalf("validate with batch 1 begun = %d; want 0 -- a begun Create card's target existing is the plan working as designed, output: %s", exitCode, out.String())
			}
			got := out.String()
			if !strings.Contains(got, `"scope":"pending"`) {
				t.Errorf("output missing scope:pending; got %q", got)
			}
			if strings.Contains(got, "create-already-exists") {
				t.Errorf("output still carries card 1's create-already-exists finding after batch 1 was begun; got %q", got)
			}
		})
	}
}

// TestValidateCmd_RebaselinesStalePlanFingerprint is WS-1's own regression test (crucible round sonnet-xhigh-r8): validate's own resolve pass can rewrite the plan on disk (handle canonicalization) exactly as begin-batch's own ValidateDispatch call can, but before this fix it never restamped state.json's PlanFingerprint afterward the way every bracket verb already does — so a run's crash/resume guard silently desynced from a plan validate itself had just rewritten, and the next begin-batch/record-batch/run refused the (validate's own sanctioned) edit as a foreign one, forcing --fresh and discarding the run's progress.
//
// This drives the observable end state directly rather than depending on quarry's own handle grammar to construct a genuine mid-call rewrite: state.json is seeded with a fingerprint that does NOT match the real on-disk plan (standing in for "the plan changed since state.json was last written, by validate's own rewrite or otherwise"), and the assertion is that validate corrects it to the plan's actual current fingerprint — the same unconditional re-baseline begin-batch performs after every ValidateDispatch call, regardless of whether that specific call happened to rewrite anything.
//
//testtiming:keep pins that validate restamps a stale plan fingerprint and leaves the run's other state fields untouched, which its covering tests do not assert
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

	// The recorded fingerprint matches the unedited plan: validate refuses a plan edited since the run
	// recorded it (TestValidateCmd_RefusesOverviewEditWithoutRestamp), so a restamp is only ever earned over an unedited one.
	state := &websterengine.State{
		RunGUID:         "run-guid",
		PlanFingerprint: realFingerprint,
	}
	if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.validateCmd(), &out, []string{})
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

// TestBringUpDisposition_RecoverAndRun proves the two verbs that spawn an agent boot the standalone reed session first, each with its own watch disposition.
// recover-batch is R4-11's regression: it spawns a COLD recovery strand through reed.AddStrand, which needs a live reed session, but it never called the in-process bring-up seam wireStandalone arms, so in standalone mode (where the session is gone once a run ends) it failed inside AddStrand advising `lyx reed up`, a hub-only verb that cannot reach standalone geometry.
// Its batch 99 exists in no plan, so findBatch refuses it immediately: the bring-up message can only win over the batch-not-found message if the guard runs before the spawn machinery, which makes this an ordering proof.
// recover-batch is short-lived and its context would be gone before a bound watcher observed anything, so it passes watch: false; run binds the watcher to the run's own context and passes watch: true.
// Each row's fake returns an error so the call ends right after the reedUp check, never reaching the spawn.
func TestBringUpDisposition_RecoverAndRun(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		verb      func(*websterCLI) *cobra.Command
		args      []string
		withRun   bool
		wantWatch bool
		// notWant is a refusal that would mean the bring-up ran too late to matter.
		notWant string
	}{
		{"recover-batch boots with watch false", (*websterCLI).recoverBatchCmd, []string{"99", "--wait", "1ns"}, true, false, "not found in the plan's execution batches"},
		{"run boots with watch true", (*websterCLI).runCmd, []string{}, false, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, _ := newTestCLI(t)
			seedValidPlanDir(t, c.geom.PlanDir)
			if tc.withRun {
				identity, err := batcher.Select("identity")
				if err != nil {
					t.Fatalf("batcher.Select(identity) = %v; want nil", err)
				}
				c.batcher = identity
				if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, &websterengine.State{
					RunGUID: "run-guid",
					Batches: map[int]*websterengine.BatchState{},
				}); err != nil {
					t.Fatalf("SaveState: %v", err)
				}
			}

			bringUps := 0
			var gotWatch bool
			c.reedUp = func(ctx context.Context, watch bool) error {
				bringUps++
				gotWatch = watch
				return errors.New("no tmux server available in this test")
			}

			var out bytes.Buffer
			exitCode := clihelp.Execute(tc.verb(c), &out, tc.args)

			if bringUps != 1 {
				t.Fatalf("c.reedUp calls = %d; want exactly 1 -- the verb spawns an agent and must boot standalone's own reed session first", bringUps)
			}
			if gotWatch != tc.wantWatch {
				t.Errorf("c.reedUp watch = %v; want %v", gotWatch, tc.wantWatch)
			}
			if exitCode != 1 {
				t.Fatalf("verb with a failing reed bring-up = %d; want 1, output: %s", exitCode, out.String())
			}
			got := out.String()
			if !strings.Contains(got, "bring up the standalone reed session") {
				t.Errorf("output does not name the reed bring-up failure; got %q", got)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Errorf("output carries %q, so the bring-up ran too late to matter; got %q", tc.notWant, got)
			}
		})
	}
}

// singleFlagEnvelope matches the one-word machine-readable signal shape webster's verbs use to tell
// Master WHY a call refused: a whole envelope whose only field is a boolean flag
// (map[string]any{"plan_drifted": true}). Master keys a failure-ladder rung off each such flag, so
// the flag is a contract term, not an implementation detail.
var singleFlagEnvelope = regexp.MustCompile(`map\[string\]any\{"([a-z_]+)": true\}`)

// TestMasterStencilCoversEverySingleFlagRefusal is R4-34's direct regression test. record-batch emitted {"card_not_done": true} on an ErrCardNotDone refusal, but the Master stencil's failure ladder carried no rung for it, so Master fell through to generic error handling on a refusal with a specific meaning and a specific disposition.
// record-batch's own flags today are batch_failed and report_archived, which ride in multi-field envelopes (with batch, and warnings on the first) and so are outside this regexp's shape;
// the master stencil's ladder names both all the same.
//
// The flag set is read out of this package's own source rather than pinned as a list, because a hand-maintained list is forgotten by exactly the change that adds a new flag -- which is how this gap arose.
// What the regexp deliberately does NOT cover is a flag riding along inside a multi-field envelope (status's own "paused" report field, for instance): those are state a caller reads, not refusals a ladder must answer.
//
//testtiming:keep a guard that fires when a single-flag refusal envelope is missing from the master stencil's failure ladder, which its covering test does not assert
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

func TestStatusCmd(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// state is the recorded run; nil leaves no state.json.
		state  *websterengine.State
		wantIn []string
	}{
		{"not initialized without a state.json", nil, []string{`"initialized":false`}},
		{
			name: "lists every batch with its kind, terminality and digest",
			state: &websterengine.State{
				RunGUID:         "guid-1",
				PlanFingerprint: "fp-1",
				Batches: map[int]*websterengine.BatchState{
					1: {Slug: "first", Kind: "fork", Status: "done", Terminal: true, Digest: &fakeDigest},
					2: {Slug: "second", Kind: "recovery", Status: "", Terminal: false},
				},
			},
			wantIn: []string{
				`"run_guid":"guid-1"`, `"plan_fingerprint":"fp-1"`,
				`"kind":"fork"`, `"kind":"recovery"`,
				`"has_digest":true`, `"has_digest":false`,
				`"terminal":true`, `"terminal":false`,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, _ := newTestCLI(t)
			if tc.state != nil {
				if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, tc.state); err != nil {
					t.Fatalf("SaveState() error = %v", err)
				}
			}

			var out bytes.Buffer
			exitCode := clihelp.Execute(c.statusCmd(), &out, []string{})

			if exitCode != 0 {
				t.Fatalf("status = %d; want 0, output: %s", exitCode, out.String())
			}
			for _, want := range tc.wantIn {
				if !strings.Contains(out.String(), want) {
					t.Errorf("status output missing %q; got %q", want, out.String())
				}
			}
		})
	}
}

//testtiming:keep pins that a second pause call is idempotent and reports paused again, which its covering test does not assert
func TestPauseCmd_RequestsPauseIdempotent(t *testing.T) {
	c, _ := newTestCLI(t)

	for i := 0; i < 2; i++ {
		var out bytes.Buffer
		exitCode := clihelp.Execute(c.pauseCmd(), &out, []string{})
		if exitCode != 0 {
			t.Fatalf("pause call %d = %d; want 0, output: %s", i+1, exitCode, out.String())
		}
		if !strings.Contains(out.String(), `"paused":true`) {
			t.Errorf("pause call %d output missing paused:true; got %q", i+1, out.String())
		}
	}
}

var fakeDigest = websterengine.Digest{Batch: "01-first", Status: websterengine.DigestStatusDone}

// testPlanFingerprint recomputes the plan-identity hash websterengine's own unexported fingerprint computes -- duplicated here since this test lives in webstercli, an external package, and that algorithm is deliberately not exported.
// It must stay in lock-step with websterengine's own implementation: a SHA-256 digest over every "*.md" file's sorted name and contents in planDir.
func testPlanFingerprint(t *testing.T, planDir string) string {
	t.Helper()
	entries, err := os.ReadDir(planDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", planDir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", name, err)
		}
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// newRunTestCLI returns newTestCLI's CLI with the identity batcher selected -- what PersistentPreRunE would have resolved by default -- and the one-card plan seeded.
func newRunTestCLI(t *testing.T) *websterCLI {
	t.Helper()
	identity, err := batcher.Select("")
	if err != nil {
		t.Fatalf("batcher.Select(\"\") error = %v", err)
	}
	c, _ := newTestCLI(t)
	c.batcher = identity
	seedValidPlanDir(t, c.geom.PlanDir)
	return c
}

// seedRunState writes a minimal state.json, fingerprint-matched to c's on-disk plan, standing in for the state the run verb would have already created before Master ever calls a bracket verb.
func seedRunState(t *testing.T, c *websterCLI, assertedModel string) *websterengine.State {
	t.Helper()
	st := &websterengine.State{
		RunGUID:         "guid-1",
		PlanFingerprint: testPlanFingerprint(t, c.geom.PlanDir),
		MasterStrand:    "master-strand-1",
		MasterSessionID: "master-session-1",
		AssertedModel:   assertedModel,
		Batches:         map[int]*websterengine.BatchState{},
	}
	if err := websterengine.RestampPlanBaseline(st, c.geom.PlanDir, c.geom.WebsterDir); err != nil {
		t.Fatalf("RestampPlanBaseline() error = %v", err)
	}
	if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	return st
}

// writeBatchReport seeds reportsDir with a batch-report YAML file for batch 1 ("01-only") at its plan-format-pinned filename, status OK, using websterengine's own WriteReport so the on-disk shape always matches ParseReport's contract.
// headSHA must equal the worktree's actual current HEAD when the caller drives record-batch's terminal path;
// any non-empty placeholder is fine for await-batch (presence-only) and recover-batch (no such cross-check).
func writeBatchReport(t *testing.T, reportsDir, headSHA string) {
	t.Helper()
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatalf("mkdir reports dir: %v", err)
	}
	path := filepath.Join(reportsDir, websterengine.ReportFileName(1, "only"))
	report := &websterengine.Report{Status: websterengine.ReportStatusOK, HeadSHA: headSHA}
	if err := websterengine.WriteReport(path, report); err != nil {
		t.Fatalf("write batch report: %v", err)
	}
}

// seedTwoCardPlan rewrites the plan in planDir as two cards: card 1 "only" and card 2 "second" with the given intent text, so a later edit to card 2 changes the plan fingerprint without touching card 1.
func seedTwoCardPlan(t *testing.T, planDir, secondIntent string) {
	t.Helper()
	plankit.Write(t, planDir, plankit.Plan{
		Approved: true,
		Framing:  "Framing.",
		Cards: []plankit.Card{
			{
				Number:  1,
				Slug:    "only",
				Summary: "placeholder card",
				Groups:  []plankit.Group{{Label: "Create", Targets: []string{"internal/only/new.go"}}},
				Intent:  "placeholder card.",
			},
			{
				Number:  2,
				Slug:    "second",
				Summary: "second card",
				Groups:  []plankit.Group{{Label: "Create", Targets: []string{"internal/only/two.go"}}},
				Intent:  secondIntent,
			},
		},
	})
}

// failingFabricOpen is an openFabric that cannot reach the fabric repo, so fabricSync errors exactly where a failed fabric commit would.
func failingFabricOpen() (*fabricengine.Fabric, error) {
	return nil, fmt.Errorf("fabric commit failed (injected)")
}

// wantWayForward fails unless got carries the trailing way-forward clause and names substr in it.
func wantWayForward(t *testing.T, got, substr string) {
	t.Helper()
	i := strings.Index(got, "way forward:")
	if i < 0 {
		t.Fatalf("output has no way forward clause; got %q", got)
	}
	if !strings.Contains(got[i:], substr) {
		t.Errorf("way forward clause missing %q; got %q", substr, got[i:])
	}
}

// TestAwaitBatchCmd_ReportPresenceEnvelope proves await-batch's two envelopes: {"report": true} the moment the batch's report file exists, and {"report": false} once the bounded wait elapses with no report -- the first step passes --wait 1ns explicitly to keep the window near-instant, versus the production default (websterengine.DefaultAwaitWaitS) used whenever --wait is omitted -- with no state.json ever read or written, since the verb is deliberately stateless.
// The scenario calls t.Parallel as a whole and no step does, since the steps share the one CLI.
func TestAwaitBatchCmd_ReportPresenceEnvelope(t *testing.T) {
	t.Parallel()

	c := newRunTestCLI(t)

	t.Run("NoReport_WindowElapses", func(t *testing.T) {
		var out strings.Builder
		exitCode := clihelp.Execute(c.awaitBatchCmd(), &out, []string{"1", "--wait", "1ns"})
		if exitCode != 0 {
			t.Fatalf("await-batch 1 = %d; want 0, output: %s", exitCode, out.String())
		}
		if !strings.Contains(out.String(), `"report":false`) {
			t.Errorf("output missing report:false; got %q", out.String())
		}
	})

	t.Run("ReportPresent", func(t *testing.T) {
		writeBatchReport(t, c.geom.ReportsDir, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
		var out strings.Builder
		exitCode := clihelp.Execute(c.awaitBatchCmd(), &out, []string{"1"})
		if exitCode != 0 {
			t.Fatalf("await-batch 1 = %d; want 0, output: %s", exitCode, out.String())
		}
		got := out.String()
		for _, want := range []string{`"batch":"01-only"`, `"report":true`} {
			if !strings.Contains(got, want) {
				t.Errorf("output missing %q; got %q", want, got)
			}
		}
	})

	// Statelessness: neither call may have created a state.json.
	loaded, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if loaded != nil {
		t.Error("await-batch created or mutated state.json; the verb must be stateless")
	}
}

// TestPersistPlanFingerprintRebaseline covers the round-4 review's R4-01 CLI half: both bracket
// verbs re-baseline State.PlanFingerprint the instant a sanctioned plan rewrite lands on disk, and
// either verb can then fail on a later step of the same call. Discarding that re-baseline with the
// failed call left the plan on disk carrying webster's own edit while state.json recorded the
// pre-rewrite fingerprint, which every later bracket verb then refused as a foreign edit.
//
// The unchanged-fingerprint case must write nothing at all: that is what keeps a genuine foreign
// edit failing ErrFingerprintMismatch exactly as it did.
func TestPersistPlanFingerprintRebaseline(t *testing.T) {
	t.Parallel()

	newGeom := func(t *testing.T) websterengine.Geometry {
		t.Helper()
		websterDir := t.TempDir()
		return websterengine.Geometry{WebsterDir: websterDir, ScratchDir: t.TempDir()}
	}

	t.Run("unchanged fingerprint writes nothing", func(t *testing.T) {
		t.Parallel()
		geom := newGeom(t)
		st := &websterengine.State{RunGUID: "g1", PlanFingerprint: "same"}

		if err := persistPlanFingerprintRebaseline(geom, st, "same"); err != nil {
			t.Fatalf("persistPlanFingerprintRebaseline() error = %v; want nil", err)
		}
		loaded, err := websterengine.LoadState(geom.WebsterDir, geom.ScratchDir)
		if err != nil {
			t.Fatalf("LoadState() error = %v", err)
		}
		if loaded != nil {
			t.Errorf("LoadState() = %+v; want nil — an unchanged fingerprint must write no state at all", loaded)
		}
	})

	t.Run("changed fingerprint is persisted", func(t *testing.T) {
		t.Parallel()
		geom := newGeom(t)
		if err := websterengine.SaveState(geom.WebsterDir, geom.ScratchDir, &websterengine.State{RunGUID: "g2", PlanFingerprint: "before the rewrite"}); err != nil {
			t.Fatalf("seed SaveState() error = %v", err)
		}
		st := &websterengine.State{RunGUID: "g2", PlanFingerprint: "after the rewrite"}

		if err := persistPlanFingerprintRebaseline(geom, st, "before the rewrite"); err != nil {
			t.Fatalf("persistPlanFingerprintRebaseline() error = %v; want nil", err)
		}
		loaded, err := websterengine.LoadState(geom.WebsterDir, geom.ScratchDir)
		if err != nil {
			t.Fatalf("LoadState() error = %v", err)
		}
		if loaded == nil {
			t.Fatal("LoadState() = nil; want the re-baselined state persisted so the next bracket verb reads it")
		}
		if loaded.PlanFingerprint != "after the rewrite" {
			t.Errorf("LoadState().PlanFingerprint = %q; want %q", loaded.PlanFingerprint, "after the rewrite")
		}
	})

	// R6-4: the re-baseline persists the FINGERPRINT and nothing else. RecordBatch advances
	// State.SeenForkTranscripts before the step that can fail, so saving the caller's whole in-memory
	// State persisted the fork's transcript as consumed on a failed call — and the resumed
	// record-batch then found no new transcripts and wedged.
	t.Run("only the fingerprint is persisted, never the caller's other mutations", func(t *testing.T) {
		t.Parallel()
		geom := newGeom(t)
		onDisk := &websterengine.State{RunGUID: "g3", PlanFingerprint: "before the rewrite"}
		if err := websterengine.SaveState(geom.WebsterDir, geom.ScratchDir, onDisk); err != nil {
			t.Fatalf("seed SaveState() error = %v", err)
		}
		st := &websterengine.State{
			RunGUID:             "g3",
			PlanFingerprint:     "after the rewrite",
			SeenForkTranscripts: []string{"/transcripts/fork-a.jsonl"},
		}

		if err := persistPlanFingerprintRebaseline(geom, st, "before the rewrite"); err != nil {
			t.Fatalf("persistPlanFingerprintRebaseline() error = %v; want nil", err)
		}
		loaded, err := websterengine.LoadState(geom.WebsterDir, geom.ScratchDir)
		if err != nil {
			t.Fatalf("LoadState() error = %v", err)
		}
		if loaded == nil {
			t.Fatal("LoadState() = nil; want the re-baselined state persisted")
		}
		if loaded.PlanFingerprint != "after the rewrite" {
			t.Errorf("LoadState().PlanFingerprint = %q; want %q", loaded.PlanFingerprint, "after the rewrite")
		}
		if len(loaded.SeenForkTranscripts) != 0 {
			t.Errorf("LoadState().SeenForkTranscripts = %v; want empty — persisting it marks the fork's transcript consumed on a call that FAILED, and the resumed record-batch then finds nothing to attribute", loaded.SeenForkTranscripts)
		}
	})

	t.Run("nil state writes nothing", func(t *testing.T) {
		t.Parallel()
		geom := newGeom(t)
		if err := persistPlanFingerprintRebaseline(geom, nil, "anything"); err != nil {
			t.Fatalf("persistPlanFingerprintRebaseline(nil) error = %v; want nil", err)
		}
	})
}

// TestValidateCmd_RefusesOverviewEditWithoutRestamp proves validate refuses a plan whose 00-overview.md changed since the run recorded it:
// it exits non-zero naming rebaseline and leaves PlanFileHashes and PlanFingerprint untouched in state.json, and the next run entry's fingerprint check still refuses the edit.
// It sets WEFT_SKIP_GIT, so it is not parallel.
func TestValidateCmd_RefusesOverviewEditWithoutRestamp(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	c := newRunTestCLI(t)
	before := seedRunState(t, c, "master-model")

	overviewPath := filepath.Join(c.geom.PlanDir, "00-overview.md")
	data, err := os.ReadFile(overviewPath)
	if err != nil {
		t.Fatalf("read overview: %v", err)
	}
	if err := os.WriteFile(overviewPath, append(data, []byte("\n## verify:\n\ntrue\n")...), 0o644); err != nil {
		t.Fatalf("edit overview: %v", err)
	}

	var out strings.Builder
	exitCode := clihelp.Execute(c.validateCmd(), &out, []string{})
	if exitCode == 0 {
		t.Fatalf("validate on an edited plan = 0; want non-zero, output: %s", out.String())
	}
	if !strings.Contains(out.String(), "rebaseline") {
		t.Errorf("output = %s; want it to name rebaseline", out.String())
	}

	after, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
	if err != nil || after == nil {
		t.Fatalf("LoadState() after validate = %v, %v; want a state", after, err)
	}
	if after.PlanFingerprint != before.PlanFingerprint {
		t.Errorf("PlanFingerprint = %q; want %q unchanged", after.PlanFingerprint, before.PlanFingerprint)
	}
	if fmt.Sprint(after.PlanFileHashes) != fmt.Sprint(before.PlanFileHashes) {
		t.Errorf("PlanFileHashes = %v; want %v unchanged", after.PlanFileHashes, before.PlanFileHashes)
	}
	// Run's entry check is this same fingerprint comparison, so the edit is still refused there.
	if err := websterengine.PlanEditError(after, c.geom.PlanDir); !errors.Is(err, websterengine.ErrFingerprintMismatch) {
		t.Errorf("PlanEditError() after validate = %v; want ErrFingerprintMismatch", err)
	}
}

// TestValidateCmd_Regression329_ForthcomingCreateTargetPassesPending pins #329 at validate: batch 1 is begun but not terminal and nothing has landed,
// and card 2 Uses card 1's Create target, which does not exist yet.
// validate must scope to `pending` and exit 0 instead of reporting the Use as glyph-not-found.
func TestValidateCmd_Regression329_ForthcomingCreateTargetPassesPending(t *testing.T) {
	t.Parallel()

	identity, err := batcher.Select("identity")
	if err != nil {
		t.Fatalf("batcher.Select(identity) = %v; want nil", err)
	}
	c, _ := newTestCLI(t)
	c.batcher = identity

	if err := os.MkdirAll(c.geom.PlanDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	subDir := filepath.Join(c.geom.WorktreeRoot, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "a.go"), []byte("package sub\n\nfunc Foo() {}\n"), 0o644); err != nil {
		t.Fatalf("write sub/a.go: %v", err)
	}
	plankit.Write(t, c.geom.PlanDir, plankit.Plan{
		Approved: true,
		Language: "go",
		Framing:  "Framing.",
		Cards: []plankit.Card{
			{
				Number:  1,
				Slug:    "first",
				Summary: "creates a symbol",
				Groups:  []plankit.Group{{Label: "Create", Targets: []string{"newpkg#Bar"}}},
				Intent:  "creates a symbol.",
			},
			{
				Number:        2,
				Slug:          "second",
				Summary:       "uses it",
				Groups:        []plankit.Group{{Label: "Edit", Targets: []string{"sub#Foo"}}},
				Uses:          []string{"newpkg#Bar"},
				Intent:        "uses it.",
				ImpactSummary: "none.",
			},
		},
	})

	state := &websterengine.State{
		RunGUID:         "run-guid",
		PlanFingerprint: testPlanFingerprint(t, c.geom.PlanDir),
		Batches:         map[int]*websterengine.BatchState{1: {Slug: "first", Kind: "fork"}},
	}
	if err := websterengine.RestampPlanBaseline(state, c.geom.PlanDir, c.geom.WebsterDir); err != nil {
		t.Fatalf("RestampPlanBaseline() error = %v", err)
	}
	if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	var out strings.Builder
	exitCode := clihelp.Execute(c.validateCmd(), &out, []string{})

	if exitCode != 0 {
		t.Fatalf("validate on a begun, unrecorded batch = %d; want 0, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"scope":"pending"`) {
		t.Errorf("output missing scope:pending; got %q", out.String())
	}
}

// TestRebaselineCmd_AcceptsForeignEditAndKeepsRecords proves a plan edit to a later card is accepted:
// the verb exits 0 with batches_kept 1, restamps the fingerprint and leaves batch 1's record intact.
// It sets WEFT_SKIP_GIT, so it is not parallel.
func TestRebaselineCmd_AcceptsForeignEditAndKeepsRecords(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	c := newRunTestCLI(t)
	seedTwoCardPlan(t, c.geom.PlanDir, "second card.")
	st := seedRunState(t, c, "master-model")
	st.Batches[1] = &websterengine.BatchState{Slug: "only", Cards: []string{"01-only"}, StartSHA: "abc123", Kind: "fork", Digest: &websterengine.Digest{Batch: "01-only", Status: websterengine.DigestStatusDone, HeadSHA: "def456"}}
	if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	before := st.PlanFingerprint

	seedTwoCardPlan(t, c.geom.PlanDir, "second card, edited mid-run.")

	var out strings.Builder
	exitCode := clihelp.Execute(c.rebaselineCmd(), &out, []string{"--card", "2"})
	if exitCode != 0 {
		t.Fatalf("rebaseline = %d; want 0, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"cards_accepted":["02-second.md"]`) {
		t.Errorf("output missing cards_accepted; got %q", out.String())
	}
	if !strings.Contains(out.String(), `"batches_kept":1`) {
		t.Errorf("output missing batches_kept:1; got %q", out.String())
	}

	loaded, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() = %v, %v; want a state, nil", loaded, err)
	}
	want := testPlanFingerprint(t, c.geom.PlanDir)
	if loaded.PlanFingerprint != want || loaded.PlanFingerprint == before {
		t.Errorf("PlanFingerprint = %q; want the recomputed %q (was %q)", loaded.PlanFingerprint, want, before)
	}
	bs := loaded.Batches[1]
	if bs == nil || bs.StartSHA != "abc123" || bs.Digest == nil || bs.Digest.HeadSHA != "def456" {
		t.Errorf("batch 1 record = %+v; want it intact", bs)
	}
}

// TestRebaselineCmd_Refusals proves each refused rebaseline exits non-zero with its way forward and leaves state.json byte-identical:
// an edited card the operator did not name (naming --card), a --card value that is not a positive integer (a usage error naming the value), and a removed card whose batch was begun (naming --fresh).
// It sets WEFT_SKIP_GIT, so it is not parallel.
func TestRebaselineCmd_Refusals(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")

	cases := []struct {
		name string
		// arrange edits the plan and state after the run's state.json is seeded and returns the verb's arguments.
		arrange func(t *testing.T, c *websterCLI) []string
		wantIn  []string
	}{
		{
			name: "edited card not named",
			arrange: func(t *testing.T, c *websterCLI) []string {
				seedTwoCardPlan(t, c.geom.PlanDir, "second card, edited mid-run.")
				return []string{}
			},
			wantIn: []string{"--card"},
		},
		{
			name: "non-numeric card",
			arrange: func(*testing.T, *websterCLI) []string {
				return []string{"--card", "x"}
			},
			wantIn: []string{`\"x\"`, "way forward"},
		},
		{
			name: "removed card of a begun batch",
			arrange: func(t *testing.T, c *websterCLI) []string {
				st, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
				if err != nil || st == nil {
					t.Fatalf("LoadState() = %v, %v; want a state", st, err)
				}
				st.Batches[1] = &websterengine.BatchState{Slug: "only", Cards: []string{"01-only"}, StartSHA: "abc123", Kind: "fork"}
				if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, st); err != nil {
					t.Fatalf("SaveState() error = %v", err)
				}
				// Replace card 1 with a differently-slugged card so batch 1 no longer exists.
				if err := os.Remove(filepath.Join(c.geom.PlanDir, "01-only.md")); err != nil {
					t.Fatalf("remove card: %v", err)
				}
				plankit.Write(t, c.geom.PlanDir, plankit.Plan{
					Approved: true,
					Framing:  "Framing.",
					Cards: []plankit.Card{{
						Number:  1,
						Slug:    "other",
						Summary: "replacement card",
						Groups:  []plankit.Group{{Label: "Create", Targets: []string{"internal/only/other.go"}}},
						Intent:  "replacement card.",
					}},
				})
				return []string{}
			},
			wantIn: []string{"--fresh"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newRunTestCLI(t)
			// The state is recorded against the two-card plan so the "edited card" row has an unnamed edit to refuse.
			seedTwoCardPlan(t, c.geom.PlanDir, "second card.")
			seedRunState(t, c, "master-model")
			statePath := filepath.Join(c.geom.WebsterDir, "state.json")
			args := tc.arrange(t, c)
			before, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatalf("read state.json: %v", err)
			}

			var out strings.Builder
			if code := clihelp.Execute(c.rebaselineCmd(), &out, args); code == 0 {
				t.Fatalf("rebaseline = 0; want non-zero, output: %s", out.String())
			}
			for _, want := range tc.wantIn {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output missing %q; got %q", want, out.String())
				}
			}
			after, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatalf("read state.json: %v", err)
			}
			if string(before) != string(after) {
				t.Error("state.json changed on a refused rebaseline; want byte-identical")
			}
		})
	}
}

// TestRebaselineCmd_FabricSyncFailureWayForward reaches rebaseline's fabric-sync refusal and checks it names the same way forward as the bracket verbs' sync refusals.
// The restamped state is saved locally before the sync, which is what that way forward commits.
// It sets WEFT_SKIP_GIT, so it is not parallel.
func TestRebaselineCmd_FabricSyncFailureWayForward(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	c := newRunTestCLI(t)
	seedTwoCardPlan(t, c.geom.PlanDir, "second card.")
	seedRunState(t, c, "master-model")
	seedTwoCardPlan(t, c.geom.PlanDir, "second card, edited mid-run.")
	c.openFabric = failingFabricOpen

	var out strings.Builder
	if code := clihelp.Execute(c.rebaselineCmd(), &out, []string{"--card", "02"}); code == 0 {
		t.Fatalf("rebaseline with a failing sync = 0; want non-zero, output: %s", out.String())
	}
	wantWayForward(t, out.String(), "lyx fabric commit")
	loaded, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() = %v, %v; want the saved state", loaded, err)
	}
	if want := testPlanFingerprint(t, c.geom.PlanDir); loaded.PlanFingerprint != want {
		t.Errorf("PlanFingerprint = %q; want the restamped %q saved despite the sync failure", loaded.PlanFingerprint, want)
	}
}

// TestRestorePlanCmd_RestoresEditedCard proves an edited card is reported and written back, with state.json byte-identical.
// It sets WEFT_SKIP_GIT, so it is not parallel.
func TestRestorePlanCmd_RestoresEditedCard(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	c := newRunTestCLI(t)
	seedRunState(t, c, "master-model")
	cardPath := filepath.Join(c.geom.PlanDir, "01-only.md")
	recorded, err := os.ReadFile(cardPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cardPath, []byte("edited after the run recorded the plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(c.geom.WebsterDir, "state.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if code := clihelp.Execute(c.restorePlanCmd(), &out, []string{}); code != 0 {
		t.Fatalf("restore-plan = %d; want 0, output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "01-only.md") {
		t.Errorf("output %q does not name the restored card", out.String())
	}
	got, err := os.ReadFile(cardPath)
	if err != nil || string(got) != string(recorded) {
		t.Errorf("card = %q, %v; want the recorded bytes %q", got, err, recorded)
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("state.json changed by restore-plan")
	}
}

// TestRunStateVerbs_NoRunRefusal proves each verb that reads the run's state.json refuses when there is none, naming the run verb.
func TestRunStateVerbs_NoRunRefusal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		verb   func(*websterCLI) *cobra.Command
		wantIn string
	}{
		{"accept-audit", (*websterCLI).acceptAuditCmd, "webster run"},
		{"restore-plan", (*websterCLI).restorePlanCmd, "lyx webster run"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := newRunTestCLI(t)
			var out strings.Builder
			if code := clihelp.Execute(tc.verb(c), &out, []string{}); code == 0 {
				t.Fatalf("%s with no run = 0; want non-zero, output: %s", tc.name, out.String())
			}
			if !strings.Contains(out.String(), tc.wantIn) {
				t.Errorf("output = %q; want it to name %q", out.String(), tc.wantIn)
			}
		})
	}
}
