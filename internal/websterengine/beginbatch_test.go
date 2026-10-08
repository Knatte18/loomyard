// beginbatch_test.go exercises BeginBatch end to end (Tier 1 — see docs/benchmarks/running-tests.md): a temp directory backs WorktreeRoot with a fakeGit answering the HeadSHA capture, while the reed query seam is a shuttlefake.Reed.
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
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
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
	Reed      *shuttlefake.Reed
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
	reed := &shuttlefake.Reed{}

	// webster's prompts are read from disk at call time now, so the fixture's
	// hub must carry them before BeginBatch reaches RenderForkPrompt.
	hubPath := filepath.Dir(worktree)
	seedHubStencils(t, hubPath)

	deps := websterengine.BeginDeps{
		Plan:    plan,
		Batches: batches,
		State:   &websterengine.State{PlanFingerprint: fp, MasterStrand: "master-strand-1"},
		Config:  websterengine.Config{SelfFixCap: 2},
		Reed:    reed,
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

	return &beginFixture{Deps: deps, Reed: reed, Git: git, Worktree: worktree, PlanDir: planDir, PromptDir: promptsDir}
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
			name: "a pause request",
			prepare: func(t *testing.T, fx *beginFixture) {
				if err := websterengine.RequestPause(fx.Deps.Geom.ScratchDir); err != nil {
					t.Fatalf("RequestPause() error = %v; want nil", err)
				}
			},
			wantIs: websterengine.ErrPaused,
		},
		{
			name: "a plan edited after run init points at the reset-to-start route",
			prepare: func(t *testing.T, fx *beginFixture) {
				fx.Deps.State.PlanFingerprint = "0000000000000000000000000000000000000000000000000000000000000000"
			},
			wantIs:   websterengine.ErrFingerprintMismatch,
			wantText: []string{"1) lyx webster reset --to start; 2) lyx webster run"},
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
			wantText:    []string{"terminal with status dead", "way forward: the recovery of batch 1 is exhausted, so end the run stuck naming the batch"},
			wantNotText: []string{"record-batch", "recover-batch"},
		},
		{
			name: "a geometry with no code index names the missing wiring",
			prepare: func(t *testing.T, fx *beginFixture) {
				fx.Deps.Geom.Index = nil
			},
			wantText: []string{"Geometry.Index"},
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
			name: "each card carries its gate command",
			prepare: func(t *testing.T, fx *beginFixture) {
				if err := os.MkdirAll(filepath.Join(fx.Worktree, "internal", "alpha"), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				fx.Deps.Batches[0].Cards[0].TargetGroups = []planparser.TargetGroup{
					{Type: planparser.CardTypeEdit, Refs: []string{"internal/alpha/alpha.go#"}},
				}
			},
			number: 1,
			want:   []string{"`go build ./... && go test ./... && go test -tags integration ./internal/alpha && lyx loom lint-comments`"},
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
		})
	}
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
			name:      "a re-begin keeps the audit warnings and fork transcripts",
			prior:     &websterengine.BatchState{Slug: "json-flag", Kind: "fork", AuditWarnings: []websterengine.AuditWarning{warning}, ForkTranscripts: []string{"subagents/f1.jsonl"}},
			wantStart: func(fx *beginFixture) string { return fx.Git.head },
			check: func(t *testing.T, fx *beginFixture, bs *websterengine.BatchState) {
				if len(bs.AuditWarnings) != 1 || bs.AuditWarnings[0] != warning {
					t.Errorf("Batches[1].AuditWarnings = %v; want [%v]", bs.AuditWarnings, warning)
				}
				if !slices.Equal(bs.ForkTranscripts, []string{"subagents/f1.jsonl"}) {
					t.Errorf("Batches[1].ForkTranscripts = %v; want the prior record's transcripts", bs.ForkTranscripts)
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

// TestBeginBatch_ReclaimsPriorRecoveryStrandBeforeOverwrite proves F9's guard: when the batch being
// begun as a fork carries a prior recovery record whose strand the reed still reports live (a dead
// recovery keeps its substrate alive by design), BeginBatch stops that strand before the record
// overwrite erases its StrandGUID — otherwise the unreclaimed strand would race the fresh fork on
// the repo.
func TestBeginBatch_ReclaimsPriorRecoveryStrandBeforeOverwrite(t *testing.T) {
	fx := newBeginFixture(t)
	fx.Deps.State.Batches = map[int]*websterengine.BatchState{
		1: {Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead", StrandGUID: "dead-but-live-recovery"},
	}
	fx.Reed.Strands = []reedengine.StrandStatus{{GUID: "dead-but-live-recovery", Live: true}}

	if _, err := websterengine.BeginBatch(fx.Deps, 1); err != nil {
		t.Fatalf("BeginBatch() error = %v; want nil", err)
	}

	if len(fx.Reed.RemovedGUIDs) != 1 || fx.Reed.RemovedGUIDs[0] != "dead-but-live-recovery" {
		t.Errorf("RemovedGUIDs = %v; want exactly [dead-but-live-recovery] stopped before the record overwrite", fx.Reed.RemovedGUIDs)
	}
	// The record was overwritten to a fresh fork batch.
	if bs := fx.Deps.State.Batches[1]; bs.Kind != "fork" || bs.Terminal || bs.StrandGUID != "" {
		t.Errorf("Batches[1] = %+v; want a fresh non-terminal fork record with no strand", bs)
	}
}

// TestBeginBatch_ReResolvesPlanAtDispatch covers card 32's re-resolution step: a clean plan
// dispatches as today; a plan with one blocking finding returns ErrPlanDrifted and writes no
// prompt file; a quarry-unavailable root returns the wrapped infrastructure error rather than
// ErrPlanDrifted; and an informational-only findings set dispatches normally with the advisory
// carried on the result.
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

	t.Run("blocking finding returns ErrPlanDrifted and writes no prompt file", func(t *testing.T) {
		fx := newBeginFixture(t)
		// A bare package-qualified symbol in Uses trips bare-symbol-target under the default
		// glyph-enabled language, a blocking finding.
		fx.Deps.Plan.Cards = []planparser.Card{
			{Number: 1, Slug: "json-flag", Uses: []string{"sub.Foo"}},
		}

		_, err := websterengine.BeginBatch(fx.Deps, 1)
		if !errors.Is(err, websterengine.ErrPlanDrifted) {
			t.Fatalf("BeginBatch() error = %v; want errors.Is(err, ErrPlanDrifted)", err)
		}
		if !strings.Contains(err.Error(), "lyx webster rebaseline --card NN") {
			t.Errorf("error %q; want it to name `lyx webster rebaseline --card NN`", err.Error())
		}
		entries, readErr := os.ReadDir(fx.PromptDir)
		if readErr != nil {
			t.Fatalf("ReadDir(%q): %v", fx.PromptDir, readErr)
		}
		if len(entries) != 0 {
			t.Errorf("PromptDir entries = %v; want none written on a plan-drifted refusal", entries)
		}
	})

	t.Run("quarry-unavailable root returns the wrapped infrastructure error, not ErrPlanDrifted", func(t *testing.T) {
		fx := newBeginFixture(t)
		fx.Deps.Geom.WorktreeRoot = filepath.Join(t.TempDir(), "does-not-exist")

		_, err := websterengine.BeginBatch(fx.Deps, 1)
		if !errors.Is(err, planglyph.ErrQuarryUnavailable) {
			t.Fatalf("BeginBatch() error = %v; want errors.Is(err, planglyph.ErrQuarryUnavailable)", err)
		}
		if errors.Is(err, websterengine.ErrPlanDrifted) {
			t.Errorf("BeginBatch() error = %v; want NOT errors.Is(err, ErrPlanDrifted) for an infrastructure failure", err)
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
		if !errors.Is(err, websterengine.ErrPlanDrifted) {
			t.Fatalf("BeginBatch() error = %v; want errors.Is(err, ErrPlanDrifted) while card 1 is still pending", err)
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
// With the re-baseline positioned after the ErrPlanDrifted return, state.json kept the pre-rewrite
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
	if !errors.Is(err, websterengine.ErrPlanDrifted) {
		t.Fatalf("BeginBatch() error = %v; want errors.Is(err, ErrPlanDrifted) — the unresolvable glyph must block", err)
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
