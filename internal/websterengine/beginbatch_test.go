// beginbatch_test.go exercises BeginBatch end to end (Tier 1 — see docs/benchmarks/running-tests.md): a temp directory backs WorktreeRoot with a fakeGit answering the HeadSHA capture, while the strand-stop seam is a recordingStopper.
// The plan itself is a minimal *planparser.Plan (Dir only — begin-batch never reads Plan.Cards, only deps.Batches, the already-derived execution batches), backed by a t.TempDir() seeded with a throwaway markdown file so the fingerprint gate has something real to hash.
// There is no chain/restart path and no oversized role under the flat card-list model:
// this file's own mustFingerprint helper duplicates fingerprint.go's pure hashing algorithm rather than importing anything, since this file deliberately stays in the external websterengine_test package (fingerprint itself is package-private).

package websterengine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// seedPlanDir creates a t.TempDir() seeded with one throwaway markdown file,
// so the fingerprint gate has something real to hash — BeginBatch's
// fingerprint gate reads planDir directly, never deps.Batches, so no actual
// plan-format parsing is needed for these tests (per the card's own literal-
// Batches-value requirement).
func seedPlanDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	plankit.Write(t, dir, plankit.Plan{
		Approved: true,
		Language: "go",
		Cards: []plankit.Card{
			{Number: 1, Slug: "json-flag", Summary: "add the json flag", Intent: "placeholder card.", Groups: []plankit.Group{{Label: "Prosa", Targets: []string{"base.txt"}}}},
			{Number: 2, Slug: "list-tests", Summary: "list the tests", Intent: "placeholder card.", Groups: []plankit.Group{{Label: "Prosa", Targets: []string{"base.txt"}}}},
		},
	})
	return dir
}

// mustFingerprint replicates fingerprint.go's own algorithm exactly
// (SHA-256 over every "*.md" file's name and contents, sorted, NUL-
// separated) so a test can seed a State.PlanFingerprint that matches what
// BeginBatch will independently recompute — fingerprint itself is
// package-private and this file's tests deliberately stay in the external
// websterengine_test package (see the file's own doc comment).
func mustFingerprint(t *testing.T, planDir string) string {
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
			t.Fatalf("read %s: %v", name, err)
		}
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// beginCard returns a minimal single-card batcher.Batch identifying number
// and slug — begin-batch's batchIdentity assumption (batch ≡ card under the
// identity batchifier).
func beginCard(number int, slug string) batcher.Batch {
	return batcher.Batch{Cards: []planparser.Card{{Number: number, Slug: slug, Title: slug, Intent: "placeholder card " + slug}}}
}

// beginFixture is a fully-wired set of BeginBatch dependencies: a temp directory holding base.txt as WorktreeRoot over a fakeGit at one commit, fresh webster/reports/prompts temp dirs, two literal single-card execution batches backed by a seeded plan dir for the fingerprint gate.
type beginFixture struct {
	Deps      websterengine.BeginDeps
	Stopper   *recordingStopper
	Git       *fakeGit
	Worktree  string
	PlanDir   string
	PromptDir string
}

func newBeginFixture(t *testing.T) *beginFixture {
	t.Helper()

	planDir := seedPlanDir(t)
	fp := mustFingerprint(t, planDir)

	plan, err := planparser.ParsePlan(planDir)
	if err != nil {
		t.Fatalf("ParsePlan(%q): %v", planDir, err)
	}
	batches := []batcher.Batch{
		beginCard(1, "json-flag"),
		beginCard(2, "list-tests"),
	}

	worktree := t.TempDir()
	writeWorktreeFile(t, worktree, "base.txt", "base")
	git := newFakeGit()
	index := newFakeIndex()

	promptsDir := t.TempDir()
	stopper := &recordingStopper{}

	// webster's prompts are read from disk at call time now, so the fixture's
	// hub must carry them before BeginBatch reaches RenderForkPrompt.
	hubPath := filepath.Dir(worktree)
	seedHubStencils(t, hubPath)

	deps := websterengine.BeginDeps{
		Plan:    plan,
		Batches: batches,
		State:   &websterengine.State{PlanFingerprint: fp, MasterStrand: "master-strand-1"},
		Config:  websterengine.Config{SelfFixCap: 2},
		Stopper: stopper,
		Geom: websterengine.Geometry{
			AnchorRoot:   worktree,
			WorktreeRoot: worktree,
			WebsterDir:   t.TempDir(),
			ScratchDir:   t.TempDir(),
			ReportsDir:   t.TempDir(),
			PromptsDir:   promptsDir,
			StencilsDir:  fabricengine.StencilsDir(hubPath),
			SpecsDir:     fabricengine.SpecsDir(hubPath),
			PlanDir:      planDir,
			Git:          git,
			Index:        index,
		},
	}

	return &beginFixture{Deps: deps, Stopper: stopper, Git: git, Worktree: worktree, PlanDir: planDir, PromptDir: promptsDir}
}

// TestBeginBatch_Refusals proves each entry refusal names its way forward: a pause, a plan edited after run init, and a report already on disk for each recorded state (terminal done, stuck, failed or dead, and begun non-terminal fork or recovery), each message naming the record it saw and the one remedy that state calls for, and a geometry with no code index, whose message names the missing wiring.
func TestBeginBatch_Refusals(t *testing.T) {
	t.Parallel()

	seedReport := func(t *testing.T, fx *beginFixture) string {
		t.Helper()
		reportPath := filepath.Join(fx.Deps.Geom.ReportsDir, websterengine.ReportFileName(1, "json-flag"))
		if err := os.WriteFile(reportPath, []byte("status: OK\nhead_sha: deadbeef\n"), 0o644); err != nil {
			t.Fatalf("seed report: %v", err)
		}
		return reportPath
	}

	tests := []struct {
		name    string
		prepare func(t *testing.T, fx *beginFixture)
		// wantIs is the sentinel the error wraps, nil for none.
		wantIs error
		// wantText lists what the error names; wantNotText what it must not.
		wantText    []string
		wantNotText []string
		check       func(t *testing.T, fx *beginFixture)
	}{
		{
			name: "a plan edited after run init points at the reset-to-start route",
			prepare: func(t *testing.T, fx *beginFixture) {
				fx.Deps.State.PlanFingerprint = "0000000000000000000000000000000000000000000000000000000000000000"
			},
			wantIs:   websterengine.ErrFingerprintMismatch,
			wantText: []string{"1) lyx webster reset --to start; 2) lyx webster run"},
		},
		{
			name: "an overview change confined to its Card Index names rebaseline with the added card",
			prepare: func(t *testing.T, fx *beginFixture) {
				if err := websterengine.RestampPlanBaseline(fx.Deps.State, fx.PlanDir, fx.Deps.Geom.WebsterDir); err != nil {
					t.Fatalf("RestampPlanBaseline() error = %v", err)
				}
				editOverview(t, fx, func(text string) string { return text + "3 — third — the follow-up card\n" })
				writePlanFile(t, fx, "03-third.md", "# Card 3 — third\n\n**Intent:** new.\n")
			},
			wantIs:      websterengine.ErrFingerprintMismatch,
			wantText:    []string{"lyx webster rebaseline --card 03"},
			wantNotText: []string{"never rebaselined", "outside its Card Index"},
		},
		{
			name: "an overview change outside its Card Index names the follow-up card landing",
			prepare: func(t *testing.T, fx *beginFixture) {
				if err := websterengine.RestampPlanBaseline(fx.Deps.State, fx.PlanDir, fx.Deps.Geom.WebsterDir); err != nil {
					t.Fatalf("RestampPlanBaseline() error = %v", err)
				}
				editOverview(t, fx, func(text string) string {
					return strings.Replace(text, "## Card Index", "Reframed.\n\n## Card Index", 1)
				})
			},
			wantIs:      websterengine.ErrFingerprintMismatch,
			wantText:    []string{"00-overview.md changed outside its Card Index", followUpWayForward},
			wantNotText: []string{"never rebaselined"},
		},
		{
			name: "a report over a terminal done record says to begin the next batch and leaves the record untouched",
			prepare: func(t *testing.T, fx *beginFixture) {
				seedReport(t, fx)
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{
					1: {Slug: "json-flag", Kind: "fork", Terminal: true, Status: "done"},
				}
			},
			wantText:    []string{"terminal with status done", "way forward: the batch is finished, so begin the next batch"},
			wantNotText: []string{"record-batch", "recover-batch", "--restart-chain"},
			check: func(t *testing.T, fx *beginFixture) {
				if bs := fx.Deps.State.Batches[1]; !bs.Terminal || bs.Status != "done" {
					t.Errorf("Batches[1] = %+v; want the terminal done record untouched by the refusal", bs)
				}
			},
		},
		{
			name: "a report over a begun non-terminal fork record names record-batch and stays for it",
			prepare: func(t *testing.T, fx *beginFixture) {
				seedReport(t, fx)
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{
					1: {Slug: "json-flag", Kind: "fork"},
				}
			},
			wantText:    []string{"begun and not terminal", "way forward: `lyx webster record-batch 1`, after fixing whatever its last refusal named"},
			wantNotText: []string{"recover-batch"},
			check: func(t *testing.T, fx *beginFixture) {
				reportPath := filepath.Join(fx.Deps.Geom.ReportsDir, websterengine.ReportFileName(1, "json-flag"))
				if _, statErr := os.Stat(reportPath); statErr != nil {
					t.Errorf("stat(report) = %v; want it left for record-batch", statErr)
				}
			},
		},
		{
			name: "a report over a begun non-terminal recovery record names recover-batch",
			prepare: func(t *testing.T, fx *beginFixture) {
				seedReport(t, fx)
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{
					1: {Slug: "json-flag", Kind: "recovery"},
				}
			},
			wantText:    []string{"begun and not terminal", "way forward: `lyx webster recover-batch 1`"},
			wantNotText: []string{"record-batch"},
		},
		{
			name: "a report over a terminal stuck record names recover-batch",
			prepare: func(t *testing.T, fx *beginFixture) {
				seedReport(t, fx)
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{
					1: {Slug: "json-flag", Kind: "fork", Terminal: true, Status: "stuck"},
				}
			},
			wantText:    []string{"terminal with status stuck", "way forward: `lyx webster recover-batch 1`"},
			wantNotText: []string{"record-batch"},
		},
		{
			name: "a report over a terminal failed record names recover-batch",
			prepare: func(t *testing.T, fx *beginFixture) {
				seedReport(t, fx)
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{
					1: {Slug: "json-flag", Kind: "fork", Terminal: true, Status: "failed"},
				}
			},
			wantText:    []string{"terminal with status failed", "way forward: `lyx webster recover-batch 1`"},
			wantNotText: []string{"record-batch"},
		},
		{
			name: "a report over a terminal dead record ends the run stuck",
			prepare: func(t *testing.T, fx *beginFixture) {
				seedReport(t, fx)
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{
					1: {Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead"},
				}
			},
			wantText:    []string{"terminal with status dead", "way forward: the recovery of batch 1 is dead and earns no retry, so end the run stuck naming the batch"},
			wantNotText: []string{"record-batch", "recover-batch", "exhausted"},
		},
		{
			name: "a report over a dead recovery at the cap with an unrendered amendment ends the run stuck without exhausted",
			prepare: func(t *testing.T, fx *beginFixture) {
				seedReport(t, fx)
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{
					1: {Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead", Recoveries: 2, AmendedCards: []websterengine.AmendedCard{{Card: "01-json-flag"}}},
				}
			},
			wantText:    []string{"terminal with status dead", "end the run stuck naming the batch"},
			wantNotText: []string{"exhausted", "record-batch", "recover-batch"},
		},
		{
			name: "a report over a stuck fork record at the cap names the exhausted routes",
			prepare: func(t *testing.T, fx *beginFixture) {
				seedReport(t, fx)
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{
					1: {Slug: "json-flag", Kind: "fork", Terminal: true, Status: "stuck", Recoveries: 2},
				}
			},
			wantIs:      websterengine.ErrRecoveryExhausted,
			wantText:    []string{"exhausted", "end the run stuck naming the batch", "lyx webster reset --to batch-start --batch 01"},
			wantNotText: []string{"record-batch"},
		},
		{
			name: "a report over a failed fork record at the cap with an unrendered amendment names recover-batch",
			prepare: func(t *testing.T, fx *beginFixture) {
				seedReport(t, fx)
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{
					1: {Slug: "json-flag", Kind: "fork", Terminal: true, Status: "failed", Recoveries: 2, AmendedCards: []websterengine.AmendedCard{{Card: "01-json-flag"}}},
				}
			},
			wantText:    []string{"terminal with status failed", "way forward: `lyx webster recover-batch 1`"},
			wantNotText: []string{"exhausted", "record-batch"},
		},
		{
			name: "a report over a dead recovery at the cap names the exhausted routes",
			prepare: func(t *testing.T, fx *beginFixture) {
				seedReport(t, fx)
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{
					1: {Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead", Recoveries: 2},
				}
			},
			wantIs:      websterengine.ErrRecoveryExhausted,
			wantText:    []string{"exhausted", "end the run stuck naming the batch", "lyx webster reset --to batch-start --batch 01"},
			wantNotText: []string{"record-batch"},
		},
		{
			name: "a report over a terminal dead recovery that committed names recover-batch once more",
			prepare: func(t *testing.T, fx *beginFixture) {
				seedReport(t, fx)
				start := fx.Git.head
				fx.Git.commit()
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{
					1: {Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead", Recoveries: 1, RecoveryStartSHA: start},
				}
			},
			wantText:    []string{"terminal with status dead", "way forward: `lyx webster recover-batch 1`, once more"},
			wantNotText: []string{"exhausted", "record-batch"},
		},
		{
			name: "a geometry with no code index names the missing wiring",
			prepare: func(t *testing.T, fx *beginFixture) {
				fx.Deps.Geom.Index = nil
			},
			wantText: []string{"Geometry.Index"},
		},
		{
			name: "an unreadable worktree directory names the path and the re-run, and writes no prompt",
			prepare: func(t *testing.T, fx *beginFixture) {
				writeWorktreeFile(t, fx.Worktree, "go.mod", "module m\n")
				writeWorktreeFile(t, fx.Worktree, "internal/locked/l.go", "package l\n")
				locked := filepath.Join(fx.Worktree, "internal", "locked")
				makeUnreadable(t, locked)
				t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
			},
			wantText: []string{"internal/locked", "way forward: transient, re-run `lyx webster begin-batch 1`"},
			check: func(t *testing.T, fx *beginFixture) {
				entries, err := os.ReadDir(fx.PromptDir)
				if err != nil || len(entries) != 0 {
					t.Errorf("prompts dir = %v, %v; want it empty", entries, err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fx := newBeginFixture(t)
			tt.prepare(t, fx)

			_, err := websterengine.BeginBatch(fx.Deps, 1)
			if err == nil {
				t.Fatal("BeginBatch() error = nil; want a refusal")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("BeginBatch() error = %v; want errors.Is(err, %v)", err, tt.wantIs)
			}
			for _, want := range tt.wantText {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("BeginBatch() error = %q; want it to name %q", err, want)
				}
			}
			for _, notWant := range tt.wantNotText {
				if strings.Contains(err.Error(), notWant) {
					t.Errorf("BeginBatch() error = %q; want no %q", err, notWant)
				}
			}
			if tt.check != nil {
				tt.check(t, fx)
			}
		})
	}
}

// TestBeginBatch_PromptFile proves the fork prompt is written under PromptsDir with {{.prev_digest}}
// populated from the batch that ran immediately before in execution order, whatever the numbers, or
// the first-batch sentinel for the batch sitting first, and with each card's Go-derived gate command.
//
//testtiming:keep pins the prompt's predecessor digest or first-batch sentinel in execution order, the prompt path and the card gate command; the covering tests render a prompt without reading these
func TestBeginBatch_PromptFile(t *testing.T) {
	t.Parallel()

	done := func(number int, slug, head string) *websterengine.BatchState {
		return &websterengine.BatchState{
			Slug:     slug,
			Terminal: true,
			Status:   "done",
			Digest: &websterengine.Digest{
				Batch:   fmt.Sprintf("%02d-%s", number, slug),
				Status:  websterengine.DigestStatusDone,
				HeadSHA: head,
			},
		}
	}
	reordered := func(t *testing.T, fx *beginFixture) {
		// Batch 2 (list-tests) runs before batch 1 (json-flag) in execution order.
		fx.Deps.Batches = []batcher.Batch{beginCard(2, "list-tests"), beginCard(1, "json-flag")}
	}

	tests := []struct {
		name    string
		prepare func(t *testing.T, fx *beginFixture)
		number  int
		want    []string
		// wantNot lists text the prompt must not contain.
		wantNot []string
		// check inspects the whole prompt, for an ordering no single phrase pins.
		check func(t *testing.T, prompt string)
	}{
		{name: "batch 1 renders the first-batch sentinel", number: 1, want: []string{"none (first batch)"}},
		{
			name: "batch N>1 renders the persisted predecessor digest",
			prepare: func(t *testing.T, fx *beginFixture) {
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{1: done(1, "json-flag", "deadbeef")}
			},
			number: 2,
			want:   []string{"01-json-flag", "done", "head_sha=deadbeef"},
		},
		{
			name: "the predecessor digest names the guard's deviations",
			prepare: func(t *testing.T, fx *beginFixture) {
				d := done(1, "json-flag", "deadbeef")
				d.Digest.Deviations = []string{"a.go", "docs/b.md"}
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{1: d}
			},
			number: 2,
			want:   []string{"scope-guard deviations=a.go,docs/b.md"},
		},
		{
			name: "reordered execution: the predecessor is the batch that ran before it",
			prepare: func(t *testing.T, fx *beginFixture) {
				reordered(t, fx)
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{2: done(2, "list-tests", "cafef00d")}
			},
			number: 1,
			want:   []string{"02-list-tests", "head_sha=cafef00d"},
		},
		{
			name:    "reordered execution: the batch sitting first renders the sentinel regardless of its number",
			prepare: reordered,
			number:  2,
			want:    []string{"none (first batch)"},
		},
		{
			name: "each card carries its gate steps",
			prepare: func(t *testing.T, fx *beginFixture) {
				if err := os.MkdirAll(filepath.Join(fx.Worktree, "internal", "alpha"), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				fx.Deps.Batches[0].Cards[0].TargetGroups = []planparser.TargetGroup{
					{Type: planparser.CardTypeEdit, Refs: []string{"internal/alpha/alpha.go#"}},
				}
			},
			number: 1,
			want:   []string{"  - `lyx gate test ./internal/alpha`\n  - `lyx loom lint-comments`\n  - `lyx loom lint-pattern`"},
		},
		{
			name: "a marked guard test ends each card's steps with the guard step before the lints",
			prepare: func(t *testing.T, fx *beginFixture) {
				guardFixture(t, fx, guardSourceMarked, "internal/alpha/alpha.go#")
			},
			number: 1,
			want:   []string{"  - `lyx gate test ./internal/alpha`\n  - `lyx gate test --tags integration ./internal/g -- -run '^(TestG)$'`\n  - `lyx loom lint-comments`"},
		},
		{
			name: "a misplaced marker renders no guard step and names the file, and the begin succeeds",
			prepare: func(t *testing.T, fx *beginFixture) {
				guardFixture(t, fx, guardSourceMisplaced, "internal/alpha/alpha.go#")
			},
			number:  1,
			want:    []string{"internal/g/g_test.go"},
			wantNot: []string{"-run '^("},
		},
		{
			name: "a misplaced marker in a package the card targets adds the fix line after the error text",
			prepare: func(t *testing.T, fx *beginFixture) {
				guardFixture(t, fx, guardSourceMisplaced, "internal/g/g.go#")
				fx.Deps.Batches[0].Cards[0].SourcePath = "_lyx/plan/01-json-flag.md"
			},
			number: 1,
			check: func(t *testing.T, prompt string) {
				errorAt := strings.Index(prompt, "internal/g/g_test.go")
				if errorAt < 0 || strings.LastIndex(prompt, "01-json-flag.md") < errorAt {
					t.Errorf("prompt has no card pointer after the scan error text; got:\n%s", prompt)
				}
			},
		},
		{
			name: "a guard step over the command limit names the limit and renders no guard step",
			prepare: func(t *testing.T, fx *beginFixture) {
				var source strings.Builder
				source.WriteString("package g\n")
				for i := 0; i < 700; i++ {
					fmt.Fprintf(&source, "\n//lyx:guard\nfunc TestGuardNumber%04d(t *testing.T) {}\n", i)
				}
				guardFixture(t, fx, source.String(), "internal/alpha/alpha.go#")
			},
			number:  1,
			want:    []string{fmt.Sprintf("the %d-character command limit", impactset.MaxCommandLength)},
			wantNot: []string{"-run '^("},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fx := newBeginFixture(t)
			if tt.prepare != nil {
				tt.prepare(t, fx)
			}
			result, err := websterengine.BeginBatch(fx.Deps, tt.number)
			if err != nil {
				t.Fatalf("BeginBatch() error = %v; want nil", err)
			}
			if filepath.Dir(result.PromptPath) != fx.PromptDir {
				t.Errorf("PromptPath dir = %q; want %q", filepath.Dir(result.PromptPath), fx.PromptDir)
			}
			data, err := os.ReadFile(result.PromptPath)
			if err != nil {
				t.Fatalf("read prompt file %s: %v", result.PromptPath, err)
			}
			for _, want := range tt.want {
				if !strings.Contains(string(data), want) {
					t.Errorf("prompt file does not contain %q; got:\n%s", want, data)
				}
			}
			for _, notWant := range tt.wantNot {
				if strings.Contains(string(data), notWant) {
					t.Errorf("prompt file contains %q; want none; got:\n%s", notWant, data)
				}
			}
			if tt.check != nil {
				tt.check(t, string(data))
			}
		})
	}
}

const (
	guardSourceMarked    = "package g\n\n//lyx:guard\nfunc TestG(t *testing.T) {}\n"
	guardSourceMisplaced = "package g\n\n//lyx:guard\nfunc helper() {}\n"
)

// guardFixture gives the fixture's worktree a root go.mod, an internal/g package holding test source in g_test.go, an internal/alpha directory, and card 1 the Edit targets named.
func guardFixture(t *testing.T, fx *beginFixture, testSource string, targets ...string) {
	t.Helper()
	writeWorktreeFile(t, fx.Worktree, "go.mod", "module m\n")
	writeWorktreeFile(t, fx.Worktree, "internal/g/g_test.go", testSource)
	writeWorktreeFile(t, fx.Worktree, "internal/alpha/alpha.go", "package alpha\n")
	fx.Deps.Batches[0].Cards[0].TargetGroups = []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: targets}}
}

// TestBeginBatch_Record proves the batch record BeginBatch leaves: a first begin sets CurrentBatch,
// a fresh fork record at the current head with its NN-<slug> card set, and creates a missing reports
// dir, which a shell-writing fork's report needs; a re-begin over a non-terminal record keeps its
// StartSHA (or records the head when it had none), its card set, its audit warnings and its fork
// transcripts, and re-begins past its own already-landed Create target (the 2026-09-30 wedge).
//
//testtiming:keep pins every field a first begin and each re-begin shape records, and the reports dir a first begin creates; the covering tests check only that a begin succeeds
func TestBeginBatch_Record(t *testing.T) {
	t.Parallel()

	const recordedStart = "0123456789abcdef0123456789abcdef01234567"
	warning := websterengine.AuditWarning{Identity: "s/parent:named-spawn:1", Class: "named-spawn", Detail: "spawned x"}
	tests := []struct {
		name string
		// prior is the batch-1 record before the call; nil means a first begin.
		prior   *websterengine.BatchState
		prepare func(t *testing.T, fx *beginFixture)
		// wantStart returns the StartSHA the record and result must carry.
		wantStart func(fx *beginFixture) string
		// wantArchived is whether the result must name an archived report under the reports dir.
		wantArchived bool
		check        func(t *testing.T, fx *beginFixture, bs *websterengine.BatchState)
	}{
		{
			name: "a first begin records a fresh fork batch and creates the reports dir",
			prepare: func(t *testing.T, fx *beginFixture) {
				fx.Deps.Geom.ReportsDir = filepath.Join(t.TempDir(), "reports")
			},
			wantStart: func(fx *beginFixture) string { return fx.Git.head },
			check: func(t *testing.T, fx *beginFixture, bs *websterengine.BatchState) {
				if fx.Deps.State.CurrentBatch != 1 {
					t.Errorf("State.CurrentBatch = %d; want 1", fx.Deps.State.CurrentBatch)
				}
				if bs.Slug != "json-flag" || bs.Kind != "fork" || bs.SpawnedAt == "" {
					t.Errorf("State.Batches[1] = %+v; want Slug=json-flag Kind=fork SpawnedAt=<non-empty>", bs)
				}
				if info, err := os.Stat(fx.Deps.Geom.ReportsDir); err != nil || !info.IsDir() {
					t.Errorf("stat(reports dir) = %v, %v; want the dir created by BeginBatch", info, err)
				}
			},
		},
		{
			name:  "a re-begin keeps the recorded StartSHA though the head moved",
			prior: &websterengine.BatchState{Slug: "json-flag", Kind: "fork", StartSHA: recordedStart},
			prepare: func(t *testing.T, fx *beginFixture) {
				fx.Git.commit()
			},
			wantStart: func(*beginFixture) string { return recordedStart },
		},
		{
			name:      "a re-begin over a record without a StartSHA records the head",
			prior:     &websterengine.BatchState{Slug: "json-flag", Kind: "fork"},
			wantStart: func(fx *beginFixture) string { return fx.Git.head },
		},
		{
			name:      "a re-begin keeps the audit warnings and fork transcripts and opens an empty bracket list",
			prior:     &websterengine.BatchState{Slug: "json-flag", Kind: "fork", AuditWarnings: []websterengine.AuditWarning{warning}, ForkTranscripts: []string{"subagents/f1.jsonl"}, BracketTranscripts: []string{"subagents/f1.jsonl"}},
			wantStart: func(fx *beginFixture) string { return fx.Git.head },
			check: func(t *testing.T, fx *beginFixture, bs *websterengine.BatchState) {
				if len(bs.AuditWarnings) != 1 || bs.AuditWarnings[0] != warning {
					t.Errorf("Batches[1].AuditWarnings = %v; want [%v]", bs.AuditWarnings, warning)
				}
				if !slices.Equal(bs.ForkTranscripts, []string{"subagents/f1.jsonl"}) {
					t.Errorf("Batches[1].ForkTranscripts = %v; want the prior record's transcripts", bs.ForkTranscripts)
				}
				if len(bs.BracketTranscripts) != 0 {
					t.Errorf("Batches[1].BracketTranscripts = %v; want none", bs.BracketTranscripts)
				}
			},
		},
		{
			name:      "a re-begin carries the recovery count, its sessions and its start commit",
			prior:     &websterengine.BatchState{Slug: "json-flag", Kind: "recovery", Recoveries: 2, RecoverySessions: []string{"r1", "r2"}, RecoveryStartSHA: recordedStart},
			wantStart: func(fx *beginFixture) string { return fx.Git.head },
			check: func(t *testing.T, fx *beginFixture, bs *websterengine.BatchState) {
				if bs.Kind != "fork" || bs.Recoveries != 2 || bs.RecoveryStartSHA != recordedStart {
					t.Errorf("Batches[1] = %+v; want a fork record carrying Recoveries 2 and RecoveryStartSHA %s", bs, recordedStart)
				}
				if want := []string{"r1", "r2"}; !slices.Equal(bs.RecoverySessions, want) {
					t.Errorf("Batches[1].RecoverySessions = %v; want %v", bs.RecoverySessions, want)
				}
			},
		},
		{
			name:  "a re-begin past its own landed Create target is not refused",
			prior: &websterengine.BatchState{Slug: "json-flag", Kind: "fork", StartSHA: recordedStart},
			prepare: func(t *testing.T, fx *beginFixture) {
				writeWorktreeFile(t, fx.Worktree, "sub/a.go", "package sub\n\nfunc Built() {}\n")
				built := planparser.Card{
					Number:         1,
					Slug:           "json-flag",
					Type:           planparser.CardTypeCreate,
					TypeLabelCount: 1,
					HasType:        true,
					HasIntent:      true,
					Intent:         "placeholder intent",
					TargetGroups:   []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"sub#Built"}}},
					Targets:        []string{"sub#Built"},
				}
				fx.Deps.Plan.Cards = []planparser.Card{built, fx.Deps.Plan.Cards[1]}
				fx.Deps.Batches = []batcher.Batch{{Cards: []planparser.Card{built}}}
			},
			wantStart: func(*beginFixture) string { return recordedStart },
		},
		{
			name: "a report with no begin-batch record is archived and the begin proceeds",
			prepare: func(t *testing.T, fx *beginFixture) {
				reportPath := filepath.Join(fx.Deps.Geom.ReportsDir, websterengine.ReportFileName(1, "json-flag"))
				if err := os.WriteFile(reportPath, []byte("status: OK\nhead_sha: deadbeef\n"), 0o644); err != nil {
					t.Fatalf("seed report: %v", err)
				}
			},
			wantStart: func(fx *beginFixture) string { return fx.Git.head },
			check: func(t *testing.T, fx *beginFixture, bs *websterengine.BatchState) {
				reportPath := filepath.Join(fx.Deps.Geom.ReportsDir, websterengine.ReportFileName(1, "json-flag"))
				if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
					t.Errorf("stat(report) = %v; want the report renamed away", err)
				}
				archived, err := filepath.Glob(filepath.Join(fx.Deps.Geom.ReportsDir, "01-json-flag-*.yaml"))
				if err != nil || len(archived) != 1 {
					t.Fatalf("archived reports = %v, %v; want exactly one archive under the reports dir", archived, err)
				}
			},
			wantArchived: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fx := newBeginFixture(t)
			if tt.prior != nil {
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{1: tt.prior}
			}
			if tt.prepare != nil {
				tt.prepare(t, fx)
			}

			result, err := websterengine.BeginBatch(fx.Deps, 1)
			if err != nil {
				t.Fatalf("BeginBatch(1) error = %v; want nil", err)
			}
			bs := fx.Deps.State.Batches[1]
			if bs == nil {
				t.Fatal("State.Batches[1] missing after BeginBatch")
			}
			if want := tt.wantStart(fx); result.StartSHA != want || bs.StartSHA != want {
				t.Errorf("StartSHA result=%q record=%q; want %q", result.StartSHA, bs.StartSHA, want)
			}
			if want := []string{"01-json-flag"}; !slices.Equal(bs.Cards, want) {
				t.Errorf("Batches[1].Cards = %v; want %v", bs.Cards, want)
			}
			if gotArchived := result.ArchivedReport != ""; gotArchived != tt.wantArchived {
				t.Errorf("ArchivedReport = %q; want non-empty = %v", result.ArchivedReport, tt.wantArchived)
			}
			if tt.wantArchived && filepath.Dir(result.ArchivedReport) != fx.Deps.Geom.ReportsDir {
				t.Errorf("ArchivedReport = %q; want a path under %q", result.ArchivedReport, fx.Deps.Geom.ReportsDir)
			}
			if tt.check != nil {
				tt.check(t, fx, bs)
			}
		})
	}
}

// TestBeginBatch_ReclaimsPriorRecoveryStrandBeforeOverwrite proves F9's guard: when the batch being begun as a fork carries a prior recovery record with a strand (a dead recovery keeps its substrate alive by design), BeginBatch stops that strand before the record overwrite erases its StrandGUID.
// Otherwise the unreclaimed strand would race the fresh fork on the repo.
// A stop that fails fails the begin and leaves the record as it was.
func TestBeginBatch_ReclaimsPriorRecoveryStrandBeforeOverwrite(t *testing.T) {
	fx := newBeginFixture(t)
	fx.Deps.State.Batches = map[int]*websterengine.BatchState{
		1: {Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead", StrandGUID: "dead-but-live-recovery"},
	}

	t.Run("the prior strand is stopped before the record is overwritten", func(t *testing.T) {
		if _, err := websterengine.BeginBatch(fx.Deps, 1); err != nil {
			t.Fatalf("BeginBatch() error = %v; want nil", err)
		}

		if want := []string{"dead-but-live-recovery"}; !slices.Equal(fx.Stopper.Stopped, want) {
			t.Errorf("stopped = %v; want %v stopped before the record overwrite", fx.Stopper.Stopped, want)
		}
		// The record was overwritten to a fresh fork batch.
		if bs := fx.Deps.State.Batches[1]; bs.Kind != "fork" || bs.Terminal || bs.StrandGUID != "" {
			t.Errorf("Batches[1] = %+v; want a fresh non-terminal fork record with no strand", bs)
		}
	})

	t.Run("a stop failure fails the begin and keeps the record", func(t *testing.T) {
		failing := newBeginFixture(t)
		failing.Stopper.Err = errors.New("record write refused")
		failing.Deps.State.Batches = map[int]*websterengine.BatchState{
			1: {Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead", StrandGUID: "dead-but-live-recovery"},
		}

		_, err := websterengine.BeginBatch(failing.Deps, 1)
		if !errors.Is(err, failing.Stopper.Err) {
			t.Fatalf("BeginBatch() error = %v; want it to wrap %v", err, failing.Stopper.Err)
		}
		if bs := failing.Deps.State.Batches[1]; bs.Kind != "recovery" || bs.StrandGUID != "dead-but-live-recovery" {
			t.Errorf("Batches[1] = %+v; want the recovery record kept", bs)
		}
	})
}

// TestBeginBatch_ReResolvesPlanAtDispatch covers card 32's re-resolution step:
// a clean plan dispatches as today;
// a plan with a blocking finding returns a PlanConflictError, plan_defect before any batch is begun and tree_moved after, and writes no prompt file;
// a quarry-unavailable root returns the wrapped infrastructure error rather than ErrPlanConflict;
// and an informational-only findings set dispatches normally with the advisory carried on the result.
func TestBeginBatch_ReResolvesPlanAtDispatch(t *testing.T) {
	t.Run("clean plan dispatches as today", func(t *testing.T) {
		fx := newBeginFixture(t)

		result, err := websterengine.BeginBatch(fx.Deps, 1)
		if err != nil {
			t.Fatalf("BeginBatch() error = %v; want nil", err)
		}
		if len(result.Advisories) != 0 {
			t.Errorf("Advisories = %v; want none for a clean plan", result.Advisories)
		}
	})

	t.Run("blocking finding returns a plan_defect PlanConflictError and writes no prompt file", func(t *testing.T) {
		fx := newBeginFixture(t)
		// A bare package-qualified symbol in Uses trips bare-symbol-target under the default
		// glyph-enabled language, a blocking finding.
		fx.Deps.Plan.Cards = []planparser.Card{
			{Number: 1, Slug: "json-flag", Uses: []string{"sub.Foo"}},
		}

		_, err := websterengine.BeginBatch(fx.Deps, 1)
		var conflict *websterengine.PlanConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("BeginBatch() error = %v; want errors.As to a *PlanConflictError", err)
		}
		if conflict.Batch != 1 || conflict.Cause != websterengine.ConflictCausePlanDefect {
			t.Errorf("conflict = {Batch: %d, Cause: %q}; want batch 1, cause %q", conflict.Batch, conflict.Cause, websterengine.ConflictCausePlanDefect)
		}
		if !strings.Contains(strings.Join(conflict.Findings, "\n"), "bare-symbol-target") {
			t.Errorf("Findings = %v; want the bare-symbol-target finding", conflict.Findings)
		}
		if !strings.Contains(err.Error(), "lyx webster rebaseline --card NN") {
			t.Errorf("error %q; want it to name `lyx webster rebaseline --card NN`", err.Error())
		}
		entries, readErr := os.ReadDir(fx.PromptDir)
		if readErr != nil {
			t.Fatalf("ReadDir(%q): %v", fx.PromptDir, readErr)
		}
		if len(entries) != 0 {
			t.Errorf("PromptDir entries = %v; want none written on a plan-conflict refusal", entries)
		}
	})

	t.Run("blocking finding after a batch is begun returns a tree_moved PlanConflictError", func(t *testing.T) {
		fx := newBeginFixture(t)
		fx.Deps.Plan.Cards[1].Uses = []string{"sub.Foo"}
		fx.Deps.State.Batches = map[int]*websterengine.BatchState{
			1: {Slug: "json-flag", Kind: "fork"},
		}

		_, err := websterengine.BeginBatch(fx.Deps, 2)
		var conflict *websterengine.PlanConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("BeginBatch() error = %v; want errors.As to a *PlanConflictError", err)
		}
		if conflict.Cause != websterengine.ConflictCauseTreeMoved {
			t.Errorf("Cause = %q; want %q once batch 1 is begun", conflict.Cause, websterengine.ConflictCauseTreeMoved)
		}
	})

	t.Run("quarry-unavailable root returns the wrapped infrastructure error, not ErrPlanConflict", func(t *testing.T) {
		fx := newBeginFixture(t)
		fx.Deps.Geom.WorktreeRoot = filepath.Join(t.TempDir(), "does-not-exist")

		_, err := websterengine.BeginBatch(fx.Deps, 1)
		if !errors.Is(err, planglyph.ErrQuarryUnavailable) {
			t.Fatalf("BeginBatch() error = %v; want errors.Is(err, planglyph.ErrQuarryUnavailable)", err)
		}
		if errors.Is(err, websterengine.ErrPlanConflict) {
			t.Errorf("BeginBatch() error = %v; want NOT errors.Is(err, ErrPlanConflict) for an infrastructure failure", err)
		}
	})

	t.Run("informational-only findings dispatch normally with the advisory carried on the result", func(t *testing.T) {
		fx := newBeginFixture(t)
		// A Create target naming a brand-new unit is the informational create-new-unit finding.
		second := fx.Deps.Plan.Cards[1]
		fx.Deps.Plan.Cards = []planparser.Card{
			{
				Number:         1,
				Slug:           "json-flag",
				Type:           planparser.CardTypeCreate,
				TypeLabelCount: 1,
				HasType:        true,
				HasIntent:      true,
				Intent:         "placeholder intent",
				TargetGroups:   []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"brandnew#Thing"}}},
				Targets:        []string{"brandnew#Thing"},
			},
			second,
		}

		result, err := websterengine.BeginBatch(fx.Deps, 1)
		if err != nil {
			t.Fatalf("BeginBatch() error = %v; want nil", err)
		}
		if len(result.Advisories) == 0 {
			t.Error("Advisories = []; want at least one informational advisory")
		}
	})
}

// TestBeginBatch_AlreadyBuiltCardsAreNotReResolved proves the dispatch-boundary re-resolution is
// scoped to work that has not landed. A plan describes intended change, so a card already built
// contradicts the tree by design: its Create target now exists, which the Create inversion reports
// as blocking create-already-exists. Re-resolving the whole plan on every batch therefore wedged
// every multi-batch plan carrying a Create, Delete or Rename card at its second batch.
func TestBeginBatch_AlreadyBuiltCardsAreNotReResolved(t *testing.T) {
	fx := newBeginFixture(t)
	// A symbol that genuinely exists in the worktree, so card 1's Create target resolves found.
	writeWorktreeFile(t, fx.Deps.Geom.WorktreeRoot, "sub/a.go", "package sub\n\nfunc Built() {}\n")

	built := planparser.Card{
		Number:         1,
		Slug:           "json-flag",
		Type:           planparser.CardTypeCreate,
		TypeLabelCount: 1,
		HasType:        true,
		HasIntent:      true,
		Intent:         "placeholder intent",
		TargetGroups:   []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"sub#Built"}}},
		Targets:        []string{"sub#Built"},
	}
	pending := planparser.Card{
		Number:           2,
		Slug:             "list-tests",
		Type:             planparser.CardTypeEdit,
		TypeLabelCount:   1,
		HasType:          true,
		HasIntent:        true,
		Intent:           "placeholder intent",
		HasImpactSummary: true,
		ImpactSummary:    "touches the one symbol card 1 created",
		TargetGroups:     []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: []string{"sub#Built"}}},
		Targets:          []string{"sub#Built"},
	}
	fx.Deps.Plan.Cards = []planparser.Card{built, pending}
	fx.Deps.Batches = []batcher.Batch{{Cards: []planparser.Card{built}}, {Cards: []planparser.Card{pending}}}

	t.Run("card 1 still pending blocks, since its Create target already exists", func(t *testing.T) {
		fx.Deps.State.Batches = map[int]*websterengine.BatchState{}

		_, err := websterengine.BeginBatch(fx.Deps, 2)
		if !errors.Is(err, websterengine.ErrPlanConflict) {
			t.Fatalf("BeginBatch() error = %v; want errors.Is(err, ErrPlanConflict) while card 1 is still pending", err)
		}
		if !strings.Contains(err.Error(), "create-already-exists") {
			t.Errorf("BeginBatch() error = %v; want it to name create-already-exists", err)
		}
	})

	t.Run("card 1 already built dispatches, since its own success is not a defect", func(t *testing.T) {
		fx.Deps.State.Batches = map[int]*websterengine.BatchState{
			1: {Slug: "json-flag", Kind: "fork", Terminal: true, Status: "done"},
		}

		if _, err := websterengine.BeginBatch(fx.Deps, 2); err != nil {
			t.Fatalf("BeginBatch() error = %v; want nil once card 1's batch is terminal", err)
		}
	})
}

// seedRewritingPlanDir writes a real, parseable plan directory whose first card declares a draft
// plan: handle spelled differently from what quarry.Name computes for its own declaration head, so
// begin-batch's ValidateDispatch canonicalizes it and rewrites the card file on disk. Its second
// card names a glyph that cannot resolve against the fixture's worktree, so the same call also
// reports a blocking finding — the co-occurrence R4-01/R4-02's re-baseline ordering turns on.
func seedRewritingPlanDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	plankit.Write(t, dir, plankit.Plan{
		Approved: true,
		Language: "go",
		Framing:  "Two cards: one rewrites, one blocks.",
		Cards: []plankit.Card{
			{
				Number:  1,
				Slug:    "json-flag",
				Summary: "declares a draft handle whose canonical spelling differs",
				Groups:  []plankit.Group{{Label: "Create", Targets: []string{"plan:internal/foo#Barr` -> `func Bar()"}}},
				Intent:  "Declare a draft handle whose canonical spelling differs from the draft.",
			},
			{
				Number:        2,
				Slug:          "list-tests",
				Summary:       "references a glyph that does not resolve",
				Groups:        []plankit.Group{{Label: "Edit", Targets: []string{"internal/foo#Missing"}}},
				Intent:        "Reference a glyph that does not resolve against the tree.",
				ImpactSummary: "None — the target does not exist.",
			},
		},
	})
	return dir
}

// TestBeginBatch_RestampsFingerprintEvenWhenPlanDrifts is the regression test for the round-4
// review's R4-02. ValidateDispatch's resolve pass canonicalizes handles — rewriting the plan on
// disk — and then keeps going, so one call routinely both rewrites and reports a blocking finding.
// With the re-baseline positioned after the ErrPlanConflict return, state.json kept the pre-rewrite
// fingerprint while the plan on disk carried webster's own sanctioned edit, and every later
// begin-batch refused that edit as a foreign one.
func TestBeginBatch_RestampsFingerprintEvenWhenPlanDrifts(t *testing.T) {
	fx := newBeginFixture(t)

	planDir := seedRewritingPlanDir(t)
	plan, err := planparser.ParsePlan(planDir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v", planDir, err)
	}
	fx.Deps.Plan = plan
	fx.Deps.Geom.PlanDir = planDir
	seeded := mustFingerprint(t, planDir)
	fx.Deps.State.PlanFingerprint = seeded

	_, err = websterengine.BeginBatch(fx.Deps, 1)
	if !errors.Is(err, websterengine.ErrPlanConflict) {
		t.Fatalf("BeginBatch() error = %v; want errors.Is(err, ErrPlanConflict) — the unresolvable glyph must block", err)
	}

	rewritten, readErr := os.ReadFile(filepath.Join(planDir, "01-json-flag.md"))
	if readErr != nil {
		t.Fatalf("read 01-json-flag.md: %v", readErr)
	}
	if !strings.Contains(string(rewritten), "plan:internal/foo#Bar`") {
		t.Fatalf("01-json-flag.md = %q; want the draft handle canonicalized on disk — the fixture is not exercising a rewrite at all", rewritten)
	}

	if fx.Deps.State.PlanFingerprint == seeded {
		t.Error("State.PlanFingerprint still carries its pre-call value after a call that canonicalized handles on disk; every later begin-batch would refuse webster's own sanctioned rewrite as a foreign edit")
	}
}

// TestBeginBatch_NilStateIsRefusedNotPanicked is R6-21's regression test: BeginDeps.Plan was refused
// loudly while BeginDeps.State was dereferenced unguarded a few lines below, so the stated
// precondition discipline was enforced for only one of the two fields.
func TestBeginBatch_NilStateIsRefusedNotPanicked(t *testing.T) {
	_, err := websterengine.BeginBatch(websterengine.BeginDeps{Plan: &planparser.Plan{}}, 1)
	if err == nil {
		t.Fatal("websterengine.BeginBatch(nil State) error = nil; want a refusal naming the missing field")
	}
	if !strings.Contains(err.Error(), "State is nil") {
		t.Errorf("websterengine.BeginBatch(nil State) error = %v; want it to name BeginDeps.State", err)
	}
}

// TestBeginBatch_Regression329_ReBeginKeepsForthcomingCreateTarget pins #329: card 1 creates a file that card 2 Uses,
// batch 1's first begin records it, no commit lands, and the second begin of batch 1 must not drop the Create target out of the later card's validation.
func TestBeginBatch_Regression329_ReBeginKeepsForthcomingCreateTarget(t *testing.T) {
	fx := newBeginFixture(t)

	card := func(number int, slug string, typ planparser.CardType, ref string, uses []string) planparser.Card {
		c := planparser.Card{
			Number:         number,
			Slug:           slug,
			Type:           typ,
			TypeLabelCount: 1,
			HasType:        true,
			HasIntent:      true,
			Intent:         "placeholder intent",
			TargetGroups:   []planparser.TargetGroup{{Type: typ, Refs: []string{ref}}},
			Targets:        []string{ref},
			Uses:           uses,
			HasUses:        len(uses) > 0,
		}
		if typ == planparser.CardTypeEdit {
			c.HasImpactSummary = true
			c.ImpactSummary = "placeholder impact"
		}
		return c
	}
	writeWorktreeFile(t, fx.Worktree, "sub/a.go", "package sub\n\nfunc Existing() {}\n")
	writeWorktreeFile(t, fx.Worktree, "sub/b.go", "package sub\n\nfunc Other() {}\n")

	creator := card(1, "json-flag", planparser.CardTypeCreate, "sub/new.go#", nil)
	user := card(2, "list-tests", planparser.CardTypeEdit, "sub#Existing", []string{"sub/new.go#"})
	fx.Deps.Plan.Cards = []planparser.Card{creator, user}
	fx.Deps.Batches = []batcher.Batch{{Cards: []planparser.Card{creator}}, {Cards: []planparser.Card{user}}}
	fx.Deps.State.Batches = map[int]*websterengine.BatchState{}

	if _, err := websterengine.BeginBatch(fx.Deps, 1); err != nil {
		t.Fatalf("first BeginBatch(1) error = %v; want nil", err)
	}
	if fx.Deps.State.Batches[1] == nil {
		t.Fatal("first BeginBatch(1) recorded no batch 1")
	}

	if _, err := websterengine.BeginBatch(fx.Deps, 1); err != nil {
		t.Fatalf("second BeginBatch(1) error = %v; want nil — the begun, unrecorded batch's Create target must stay forthcoming", err)
	}
}

// TestBeginBatch_WayForward_UnknownBatch proves a batch number outside the plan names `lyx webster status`,
// and that naming one of the run's batches then begins it.
func TestBeginBatch_WayForward_UnknownBatch(t *testing.T) {
	fx := newBeginFixture(t)

	_, err := websterengine.BeginBatch(fx.Deps, 99)
	if err == nil || !strings.Contains(err.Error(), "way forward: `lyx webster status` lists the run's batches") {
		t.Fatalf("BeginBatch(99) error = %v; want the status way forward", err)
	}

	if _, err := websterengine.BeginBatch(fx.Deps, 1); err != nil {
		t.Fatalf("BeginBatch(1) error = %v; want nil", err)
	}
}

// TestBeginBatch_KickstartPack asserts what begin-batch builds into the fork prompt file and its pack file when kickstart_pack is on, off or inapplicable:
// with a pack, the prompt ends by naming the pack file, and without one no pack file is written.
// It also asserts that the pack reaches no other part of the result, and the one trace line each call logs.
// It lowers the process-global log threshold to Info, so it does not call t.Parallel.
func TestBeginBatch_KickstartPack(t *testing.T) {
	logger.SetVerbosity(1)
	t.Cleanup(func() { logger.SetVerbosity(0) })

	const quarryLocate = "Locate them with one `lyx quarry resolve <glyph>...` call"
	const packHeading = "## Kick-start pack"
	const packFirstTurn = "Read all of this in one turn, in parallel, before any edit"
	const plainStepOne = "1. Read the card file."
	const perCardFreshRead = "Your first work turn reads in parallel every card file the card list below points at, and nothing else"
	const perCardStepOne = "1. Before this card's first edit, run `lyx loom card-pack <N>`"
	resolvedBaz := []string{
		packHeading,
		"- `internal/foo#Baz`",
		"  - `internal/foo/impl.go:3-3` — func Baz()",
	}

	cases := []struct {
		name     string
		language string
		// withHandle adds a Create group declaring the draft handle plan:internal/foo#Barr beside card 1's Edit group.
		withHandle bool
		// oneBatch gives card 2 an Edit group on internal/foo#Baz and puts both cards in batch 1.
		oneBatch bool
		off      bool
		prepare  func(index *fakeIndex)
		// cardHeadings are card file base names whose pointer bullet and pack subsection heading the prompt must hold.
		cardHeadings []string
		// inline sets kickstart_pack_file false before the first call.
		inline bool
		// perCard sets kickstart_pack_per_card true before the first call.
		perCard bool
		// standaloneGeom gives the anchor a directory apart from the worktree, as a standalone geometry does.
		standaloneGeom bool
		// flip makes the case call begin-batch once, flip kickstart_pack_file and call it again, and assert on the second call's files.
		flip bool
		// flipPerCard makes flip toggle kickstart_pack_per_card instead of kickstart_pack_file.
		flipPerCard bool
		// stalePack says the pack file the first call wrote is still on disk after the second, which names neither the read line nor the pack path, so the needles are checked against the prompt alone.
		stalePack bool
		// layoutBlind makes the case call begin-batch again with kickstart_pack_file and kickstart_pack_per_card both flipped and require the same prompt bytes, still no pack file and its first log needle on both calls.
		layoutBlind bool
		// noPack says begin-batch writes no pack file.
		noPack bool
		// wantPrompt and wantNotPrompt are needles the prompt file followed by the pack file has and lacks.
		wantPrompt    []string
		wantNotPrompt []string
		// wantLog are the needles the captured log has.
		wantLog []string
	}{
		{
			name:          "switch on lists the resolved member glyph",
			cardHeadings:  []string{"01-json-flag.md"},
			wantPrompt:    append([]string{packFirstTurn}, resolvedBaz...),
			wantNotPrompt: []string{quarryLocate, plainStepOne},
			wantLog:       []string{"batch=1 kickstart_pack=on resolved=1 unresolved=0 pack_layout=file"},
		},
		{
			name:         "a glyph two cards of the batch name is counted once",
			oneBatch:     true,
			cardHeadings: []string{"01-json-flag.md", "02-list-tests.md"},
			wantPrompt:   resolvedBaz,
			wantLog:      []string{"batch=1 kickstart_pack=on resolved=1 unresolved=0 pack_layout=file"},
		},
		{
			name:     "an unresolved ref two cards list is counted once",
			oneBatch: true,
			prepare: func(index *fakeIndex) {
				index.pack = func(cards []planparser.Card) []planindex.CardPack {
					packs := make([]planindex.CardPack, len(cards))
					for i, c := range cards {
						packs[i] = planindex.CardPack{Card: c, Pack: planindex.Pack{Unresolved: []planindex.PackUnresolved{{Ref: "internal/foo#Baz", Reason: "quarry could not answer"}}}}
					}
					return packs
				}
				index.packErr = fmt.Errorf("%w: pack boom", planglyph.ErrQuarryUnavailable)
			},
			wantLog: []string{"batch=1 kickstart_pack=on resolved=0 unresolved=1 pack_layout=file"},
		},
		{
			name:          "the inline layout carries the pack in the prompt and writes no pack file",
			inline:        true,
			noPack:        true,
			wantPrompt:    append([]string{"Your first turn, before any edit, reads in parallel"}, resolvedBaz...),
			wantNotPrompt: []string{"## Start here", packFirstTurn},
			wantLog:       []string{"batch=1 kickstart_pack=on resolved=1 unresolved=0 pack_layout=inline"},
		},
		{
			name:       "a flip from inline to file writes the pack file",
			inline:     true,
			flip:       true,
			wantPrompt: append([]string{packFirstTurn}, resolvedBaz...),
			wantLog: []string{
				"batch=1 kickstart_pack=on resolved=1 unresolved=0 pack_layout=inline",
				"batch=1 kickstart_pack=on resolved=1 unresolved=0 pack_layout=file",
			},
		},
		{
			name:          "a flip from file to inline leaves a prompt that names no pack file",
			flip:          true,
			stalePack:     true,
			wantPrompt:    []string{packHeading, "Your first turn, before any edit, reads in parallel"},
			wantNotPrompt: []string{"## Start here"},
			wantLog: []string{
				"batch=1 kickstart_pack=on resolved=1 unresolved=0 pack_layout=file",
				"batch=1 kickstart_pack=on resolved=1 unresolved=0 pack_layout=inline",
			},
		},
		{
			name:          "the per-card layout writes no pack file and names card-pack",
			perCard:       true,
			noPack:        true,
			wantPrompt:    []string{perCardFreshRead, perCardStepOne, "## Start here"},
			wantNotPrompt: []string{packFirstTurn, packHeading, "- `internal/foo#Baz`", "internal/foo/impl.go"},
			wantLog:       []string{"batch=1 kickstart_pack=on resolved=1 unresolved=0 pack_layout=card"},
		},
		{
			name:          "the per-card switch overrides kickstart_pack_file",
			perCard:       true,
			inline:        true,
			noPack:        true,
			wantPrompt:    []string{perCardFreshRead, perCardStepOne, "## Start here"},
			wantNotPrompt: []string{packFirstTurn, packHeading, "- `internal/foo#Baz`", "internal/foo/impl.go"},
			wantLog:       []string{"batch=1 kickstart_pack=on resolved=1 unresolved=0 pack_layout=card"},
		},
		{
			name:          "a flip from file to per-card leaves a prompt that names no pack file",
			flip:          true,
			flipPerCard:   true,
			stalePack:     true,
			wantPrompt:    []string{perCardFreshRead, perCardStepOne, "## Start here"},
			wantNotPrompt: []string{packFirstTurn, packHeading},
			wantLog: []string{
				"batch=1 kickstart_pack=on resolved=1 unresolved=0 pack_layout=file",
				"batch=1 kickstart_pack=on resolved=1 unresolved=0 pack_layout=card",
			},
		},
		{
			name:        "a flip from per-card to file writes the pack file",
			perCard:     true,
			flip:        true,
			flipPerCard: true,
			wantPrompt:  append([]string{packFirstTurn}, resolvedBaz...),
			wantLog: []string{
				"batch=1 kickstart_pack=on resolved=1 unresolved=0 pack_layout=card",
				"batch=1 kickstart_pack=on resolved=1 unresolved=0 pack_layout=file",
			},
		},
		{
			name:           "a standalone geometry keeps the file layout under per-card",
			perCard:        true,
			standaloneGeom: true,
			wantPrompt:     append([]string{packFirstTurn}, resolvedBaz...),
			wantNotPrompt:  []string{perCardStepOne},
			wantLog:        []string{"batch=1 kickstart_pack=on resolved=1 unresolved=0 pack_layout=file"},
		},
		{
			name:          "switch off renders the quarry locate step",
			off:           true,
			noPack:        true,
			layoutBlind:   true,
			wantPrompt:    []string{quarryLocate, plainStepOne},
			wantNotPrompt: []string{packHeading, packFirstTurn},
			wantLog:       []string{"batch=1 kickstart_pack=off resolved=0 unresolved=0 pack_layout=none"},
		},
		{
			name:          "a plan without a glyph language renders as switch off",
			language:      "none",
			noPack:        true,
			layoutBlind:   true,
			wantPrompt:    []string{quarryLocate, plainStepOne},
			wantNotPrompt: []string{packHeading, packFirstTurn},
			wantLog:       []string{"batch=1 kickstart_pack=language-none resolved=0 unresolved=0 pack_layout=none"},
		},
		{
			name: "a pack with no entry on any card renders as switch off",
			prepare: func(index *fakeIndex) {
				index.pack = func(cards []planparser.Card) []planindex.CardPack {
					packs := make([]planindex.CardPack, len(cards))
					for i, c := range cards {
						packs[i] = planindex.CardPack{Card: c}
					}
					return packs
				}
			},
			noPack:        true,
			layoutBlind:   true,
			wantPrompt:    []string{quarryLocate, plainStepOne},
			wantNotPrompt: []string{packHeading, packFirstTurn},
			wantLog:       []string{"batch=1 kickstart_pack=empty resolved=0 unresolved=0 pack_layout=none"},
		},
		{
			name: "an unavailable index degrades every entry to unresolved",
			prepare: func(index *fakeIndex) {
				index.pack = func(cards []planparser.Card) []planindex.CardPack {
					return []planindex.CardPack{{Card: cards[0], Pack: planindex.Pack{Unresolved: []planindex.PackUnresolved{{Ref: "internal/foo#Baz", Reason: "quarry could not answer"}}}}}
				}
				index.packErr = fmt.Errorf("%w: pack boom", planglyph.ErrQuarryUnavailable)
			},
			wantPrompt: []string{packFirstTurn, packHeading, "#### Unresolved", "- `internal/foo#Baz` — quarry could not answer"},
			wantLog: []string{
				"level=WARN",
				"pack boom",
				"batch=1 kickstart_pack=on resolved=0 unresolved=1 pack_layout=file",
			},
		},
		{
			name:          "a handle the re-resolution canonicalized is listed by its canonical spelling",
			withHandle:    true,
			wantPrompt:    append([]string{"- `plan:internal/foo#Bar`"}, resolvedBaz...),
			wantNotPrompt: []string{"plan:internal/foo#Barr"},
		},
		{
			name:       "a plan that no longer parses degrades to the loaded plan",
			withHandle: true,
			prepare:    func(index *fakeIndex) { index.breakPlanAfterValidate = true },
			wantPrompt: resolvedBaz,
			wantLog: []string{
				"level=WARN",
				"could not re-parse the plan",
				"batch=1 kickstart_pack=on resolved=1 unresolved=1 pack_layout=file",
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLogs(t)
			fx := newBeginFixture(t)
			writeWorktreeFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc Baz() {}\n")

			language := tt.language
			if language == "" {
				language = "go"
			}
			cardOne := plankit.Card{Number: 1, Slug: "json-flag", Summary: "edit a member", Intent: "edit a member.", ImpactSummary: "None.",
				Groups: []plankit.Group{{Label: "Edit", Targets: []string{"internal/foo#Baz"}}}}
			if language == "none" {
				cardOne.Groups = []plankit.Group{{Label: "Prosa", Targets: []string{"base.txt"}}}
				cardOne.ImpactSummary = ""
			}
			if tt.withHandle {
				cardOne.Groups = append([]plankit.Group{{Label: "Create", Targets: []string{"plan:internal/foo#Barr` -> `func Bar()"}}}, cardOne.Groups...)
			}
			cardTwo := plankit.Card{Number: 2, Slug: "list-tests", Summary: "list the tests", Intent: "placeholder card.", Groups: []plankit.Group{{Label: "Prosa", Targets: []string{"base.txt"}}}}
			if tt.oneBatch {
				cardTwo = plankit.Card{Number: 2, Slug: "list-tests", Summary: "list the tests", Intent: "edit the same member.", ImpactSummary: "None.",
					Groups: []plankit.Group{{Label: "Edit", Targets: []string{"internal/foo#Baz"}}}}
			}
			planDir := t.TempDir()
			plankit.Write(t, planDir, plankit.Plan{Approved: true, Language: language, Cards: []plankit.Card{cardOne, cardTwo}})
			plan, err := planparser.ParsePlan(planDir)
			if err != nil {
				t.Fatalf("ParsePlan(%q) error = %v", planDir, err)
			}
			fx.Deps.Plan = plan
			fx.Deps.Geom.PlanDir = planDir
			fx.Deps.State.PlanFingerprint = mustFingerprint(t, planDir)
			fx.Deps.Batches = []batcher.Batch{{Cards: plan.Cards[:1]}, {Cards: plan.Cards[1:]}}
			if tt.oneBatch {
				fx.Deps.Batches = []batcher.Batch{{Cards: plan.Cards}}
			}
			fx.Deps.Config.KickstartPack = !tt.off
			fx.Deps.Config.KickstartPackFile = !tt.inline
			fx.Deps.Config.KickstartPackPerCard = tt.perCard
			if tt.standaloneGeom {
				fx.Deps.Geom.AnchorRoot = t.TempDir()
			}
			if tt.prepare != nil {
				tt.prepare(fx.Deps.Geom.Index.(*fakeIndex))
			}

			result, err := websterengine.BeginBatch(fx.Deps, 1)
			if err != nil {
				t.Fatalf("BeginBatch() error = %v; want nil", err)
			}
			firstPrompt, err := os.ReadFile(result.PromptPath)
			if err != nil {
				t.Fatalf("read the first prompt file: %v", err)
			}
			if tt.flip || tt.layoutBlind {
				switch {
				case tt.layoutBlind:
					fx.Deps.Config.KickstartPackFile = !fx.Deps.Config.KickstartPackFile
					fx.Deps.Config.KickstartPackPerCard = !fx.Deps.Config.KickstartPackPerCard
				case tt.flipPerCard:
					fx.Deps.Config.KickstartPackPerCard = !fx.Deps.Config.KickstartPackPerCard
				default:
					fx.Deps.Config.KickstartPackFile = !fx.Deps.Config.KickstartPackFile
				}
				result, err = websterengine.BeginBatch(fx.Deps, 1)
				if err != nil {
					t.Fatalf("second BeginBatch() error = %v; want nil", err)
				}
			}

			prompt, err := os.ReadFile(result.PromptPath)
			if err != nil {
				t.Fatalf("read the prompt file: %v", err)
			}
			if tt.layoutBlind && string(prompt) != string(firstPrompt) {
				t.Errorf("prompt differs after flipping the layout keys; want the same bytes")
			}
			if tt.layoutBlind {
				if n := strings.Count(logs.String(), tt.wantLog[0]); n != 2 {
					t.Errorf("log holds %q %d times; want it once per call", tt.wantLog[0], n)
				}
			}
			packPath := strings.TrimSuffix(result.PromptPath, ".md") + "-pack.md"
			packFile, err := os.ReadFile(packPath)
			readLine := "Now read `" + packPath + "` and do what it says.\n"
			switch {
			case tt.stalePack:
				if err != nil {
					t.Fatalf("read the stale pack file: %v", err)
				}
				requireNotContains(t, string(prompt), readLine)
				requireNotContains(t, string(prompt), packPath)
			case tt.noPack:
				if !os.IsNotExist(err) {
					t.Errorf("read the pack file %s: error = %v; want it absent", packPath, err)
				}
				requireNotContains(t, string(prompt), readLine)
			default:
				if err != nil {
					t.Fatalf("read the pack file: %v", err)
				}
				if !strings.HasSuffix(string(prompt), readLine) {
					t.Errorf("prompt does not end with %q", readLine)
				}
			}
			text := string(prompt) + string(packFile)
			if tt.stalePack {
				text = string(prompt)
			}
			for _, needle := range tt.wantPrompt {
				requireContains(t, text, needle)
			}
			for _, needle := range tt.wantNotPrompt {
				requireNotContains(t, text, needle)
			}
			// The plan directory sits outside the worktree, so the display is the absolute plan directory.
			for _, base := range tt.cardHeadings {
				path := planDir + "/" + base
				requireContains(t, string(prompt), fmt.Sprintf("- `%s`", path))
				requireContains(t, string(packFile), fmt.Sprintf("### `%s`", path))
			}
			// The pack reaches the prompt and pack files alone.
			envelope := fmt.Sprintf("%+v", *result)
			requireNotContains(t, envelope, "Kick-start")
			requireNotContains(t, envelope, "internal/foo/impl.go")
			for _, needle := range tt.wantLog {
				requireContains(t, logs.String(), needle)
			}
		})
	}
}
