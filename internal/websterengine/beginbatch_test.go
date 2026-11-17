// beginbatch_test.go exercises BeginBatch end to end.
// A temp directory backs WorktreeRoot, and a fakeGit answers the HeadSHA capture.
// A recordingStopper stands in for the strand-stop seam.
// The plan is a minimal *planparser.Plan with only Dir set.
// BeginBatch reads deps.Batches and never Plan.Cards.
// The plan dir holds a throwaway markdown file so the fingerprint gate has content to hash.
// mustFingerprint repeats the pure hashing in fingerprint.go.
// The fingerprint code is package-private.
// This file is in the external websterengine_test package and cannot call it.

package websterengine_test

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestBeginBatch_Refusals checks that each refusal names the record it saw and the one remedy for that state.
// The refused states are a pause, a plan edited after run init,
// and a report already on disk for each recorded state:
// terminal done, stuck, failed or dead, and begun non-terminal fork or recovery.
// A geometry with no code index is also refused, and its message names the missing wiring.
func TestBeginBatch_Refusals(t *testing.T) {
	tiergate.Require(t, 0)
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
		// proceeds marks a row whose call passes the rename-tool check and so may end in no error or in a refusal of its own.
		proceeds bool
		check    func(t *testing.T, fx *beginFixture)
	}{
		{
			name: "a symbol Rename pair under an empty rename tool list refuses and records no batch",
			prepare: func(t *testing.T, fx *beginFixture) {
				fx.Deps.Batches[0].Cards[0].Pairs = []planparser.MovePair{{Old: "sub#Foo", New: "plan:sub#Bar"}}
			},
			wantIs:   websterengine.ErrRenameToolMissing,
			wantText: []string{"sub#Foo", "1-json-flag", `way forward: set rename_tools with "lym config language"`, "lym webster begin-batch 1"},
			check: func(t *testing.T, fx *beginFixture) {
				if len(fx.Deps.State.Batches) != 0 {
					t.Errorf("state batches = %v; want none recorded", fx.Deps.State.Batches)
				}
			},
		},
		{
			name: "a symbol Rename pair under a listed rename tool passes the check",
			prepare: func(t *testing.T, fx *beginFixture) {
				fx.Deps.Batches[0].Cards[0].Pairs = []planparser.MovePair{{Old: "sub#Foo", New: "plan:sub#Bar"}}
				fx.Deps.RenameTools = []string{"rename Go symbols with `gopls rename`"}
			},
			proceeds: true,
		},
		{
			name: "a file Rename pair under an empty rename tool list passes the check",
			prepare: func(t *testing.T, fx *beginFixture) {
				fx.Deps.Batches[0].Cards[0].Pairs = []planparser.MovePair{{Old: "internal/a/old.go#", New: "internal/a/new.go#"}}
			},
			proceeds: true,
		},
		{
			name: "a plan edited after run init points at the reset-to-start route",
			prepare: func(t *testing.T, fx *beginFixture) {
				fx.Deps.State.PlanFingerprint = "0000000000000000000000000000000000000000000000000000000000000000"
			},
			wantIs:   websterengine.ErrFingerprintMismatch,
			wantText: []string{"1) lym webster reset --to start; 2) lym webster run"},
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
			wantText:    []string{"lym webster rebaseline --card 03"},
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
			wantText:    []string{"begun and not terminal", "way forward: `lym webster record-batch 1`, after fixing whatever its last refusal named"},
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
			wantText:    []string{"begun and not terminal", "way forward: `lym webster recover-batch 1`"},
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
			wantText:    []string{"terminal with status stuck", "way forward: `lym webster recover-batch 1`"},
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
			wantText:    []string{"terminal with status failed", "way forward: `lym webster recover-batch 1`"},
			wantNotText: []string{"record-batch"},
		},
		{
			name: "a report over a terminal dead record ends the run stuck",
			prepare: func(t *testing.T, fx *beginFixture) {
				seedReport(t, fx)
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{
					1: {Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead", Recoveries: 1},
				}
			},
			wantText:    []string{"terminal with status dead", "way forward: the recovery of batch 1 is dead and earns no retry, so end the run stuck naming the batch"},
			wantNotText: []string{"record-batch", "recover-batch", "exhausted"},
		},
		{
			name: "a report over a dead recovery a batch-start reset cleared names recover-batch once more",
			prepare: func(t *testing.T, fx *beginFixture) {
				seedReport(t, fx)
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{
					1: {Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead"},
				}
			},
			wantText:    []string{"terminal with status dead", "way forward: `lym webster recover-batch 1`, once more, since the batch earns another recovery"},
			wantNotText: []string{"exhausted", "record-batch"},
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
			wantText:    []string{"exhausted", "end the run stuck naming the batch", "lym webster reset --to batch-start --batch 01"},
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
			wantText:    []string{"terminal with status failed", "way forward: `lym webster recover-batch 1`"},
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
			wantText:    []string{"exhausted", "end the run stuck naming the batch", "lym webster reset --to batch-start --batch 01"},
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
			wantText:    []string{"terminal with status dead", "way forward: `lym webster recover-batch 1`, once more"},
			wantNotText: []string{"exhausted", "record-batch"},
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
			if tt.proceeds {
				if errors.Is(err, websterengine.ErrRenameToolMissing) {
					t.Errorf("BeginBatch() error = %v; want the rename-tool check passed", err)
				}
				return
			}
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

// TestBeginBatch_PromptFile checks the fork prompt that BeginBatch writes under PromptsDir.
// The {{.prev_digest}} placeholder is filled from the batch that runs immediately before in execution order.
// This holds whatever the batch numbers are.
// The first batch gets the first-batch sentinel instead.
// Each card's Go-derived gate command appears in the prompt.
//
//testtiming:keep pins the prompt's predecessor digest or first-batch sentinel in execution order, the prompt path and the card gate command; the covering tests render a prompt without reading these
func TestBeginBatch_PromptFile(t *testing.T) {
	tiergate.Require(t, 0)
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
			name: "the predecessor digest names the batch's pattern departures",
			prepare: func(t *testing.T, fx *beginFixture) {
				d := done(1, "json-flag", "deadbeef")
				d.Digest.Departures = []string{"1: json-flag: kept a legacy name", "1: json-flag: skipped a scan"}
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{1: d}
			},
			number: 2,
			want:   []string{"pattern departures=1: json-flag: kept a legacy name; 1: json-flag: skipped a scan"},
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
			name: "the hub's gate tier lines render verbatim",
			prepare: func(t *testing.T, fx *beginFixture) {
				fx.Deps.GateTiers = "- tier1: unit tests\n- gate card: [1]"
			},
			number: 1,
			want:   []string{"- tier1: unit tests\n- gate card: [1]\n"},
		},
		{
			name: "the hub's rename tools render",
			prepare: func(t *testing.T, fx *beginFixture) {
				fx.Deps.RenameTools = []string{"rename Go symbols with `gopls rename`"}
			},
			number: 1,
			want:   []string{"- rename Go symbols with `gopls rename`\n"},
		},
		{
			name:   "each card carries its gate call, and the batch carries the batch gate call",
			number: 1,
			want: []string{
				"  - `lym gate test --gate card --card 1-json-flag`\n",
				"- `lym gate test --gate batch --card 1-json-flag`",
			},
			wantNot: []string{"{{.", "## verify:", "-run '^(", "--card 1-json-flag --main-packages"},
		},
		{
			name:   "the last batch's gate call ends with --main-packages",
			number: 2,
			want:   []string{"- `lym gate test --gate batch --card 2-list-tests --main-packages`"},
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

	t.Run("a dangling batch list refuses the begin with no prompt written and no state recorded", func(t *testing.T) {
		t.Parallel()

		fx := newBeginFixture(t)
		fx.Deps.Gate.Gates["batch"] = gateslot.GateKey{Tiers: []int{7}, Scope: "card"}
		_, err := websterengine.BeginBatch(fx.Deps, 1)
		if err == nil || !strings.Contains(err.Error(), "lym config gate") {
			t.Fatalf("BeginBatch() error = %v; want a refusal naming lym config gate", err)
		}
		if entries, readErr := os.ReadDir(fx.PromptDir); readErr != nil || len(entries) != 0 {
			t.Errorf("prompts dir = %v, %v; want it empty", entries, readErr)
		}
		if len(fx.Deps.State.Batches) != 0 {
			t.Errorf("state batches = %v; want none recorded", fx.Deps.State.Batches)
		}
	})
}

// TestBeginBatch_Record checks the batch record BeginBatch leaves.
// A first begin sets CurrentBatch and writes a fresh fork record at the current head.
// The record carries the NN-<slug> card set.
// A first begin also creates a missing reports dir, which a shell-writing fork's report needs.
// A re-begin over a non-terminal record keeps its StartSHA, or records the head when it had none.
// It also keeps the card set, the audit warnings and the fork transcripts.
// A re-begin proceeds past the record's own already-landed Create target.
//
//testtiming:keep pins every field a first begin and each re-begin shape records, and the reports dir a first begin creates; the covering tests check only that a begin succeeds
func TestBeginBatch_Record(t *testing.T) {
	tiergate.Require(t, 0)
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
				fx.Deps.State.MasterSessionID = "master-session"
				fx.Deps.State.MasterEngine = "master-engine"
			},
			wantStart: func(fx *beginFixture) string { return fx.Git.head },
			check: func(t *testing.T, fx *beginFixture, bs *websterengine.BatchState) {
				if bs.SessionID != "master-session" || bs.Engine != "master-engine" {
					t.Errorf("Batches[1] session and engine = %q, %q; want the Master's %q, %q", bs.SessionID, bs.Engine, "master-session", "master-engine")
				}
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
			name: "a re-begin of a batch waiting on its parent resumes it in flight with its start and its recovery count",
			prior: &websterengine.BatchState{Slug: "json-flag", Kind: "fork", StartSHA: recordedStart, Status: websterengine.DigestStatusWaiting, Recoveries: 1,
				Digest: &websterengine.Digest{Batch: "01-json-flag", Status: websterengine.DigestStatusWaiting, Need: "raise the tier 2 timeout"}},
			prepare: func(t *testing.T, fx *beginFixture) {
				fx.Git.commit()
			},
			wantStart: func(*beginFixture) string { return recordedStart },
			check: func(t *testing.T, fx *beginFixture, bs *websterengine.BatchState) {
				if bs.Terminal || bs.Status != "" || bs.Digest != nil || bs.Recoveries != 1 {
					t.Errorf("Batches[1] Terminal, Status, Digest, Recoveries = %v, %q, %v, %d; want false, \"\", nil, 1", bs.Terminal, bs.Status, bs.Digest, bs.Recoveries)
				}
			},
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
			prior:     &websterengine.BatchState{Slug: "json-flag", Kind: "recovery", Recoveries: 2, RecoverySessions: []string{"r1", "r2"}, RecoveryEngines: map[string]string{"r1": "claude", "r2": "fake"}, RecoveryStartSHA: recordedStart},
			wantStart: func(fx *beginFixture) string { return fx.Git.head },
			check: func(t *testing.T, fx *beginFixture, bs *websterengine.BatchState) {
				if want := map[string]string{"r1": "claude", "r2": "fake"}; !maps.Equal(bs.RecoveryEngines, want) {
					t.Errorf("Batches[1].RecoveryEngines = %v; want %v", bs.RecoveryEngines, want)
				}
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

// TestBeginBatch_ReclaimsPriorRecoveryStrandBeforeOverwrite checks a fork begin over a prior recovery record.
// A dead recovery can keep its strand live.
// BeginBatch must stop that strand before the overwrite erases its StrandGUID.
// Otherwise the unreclaimed strand would race the fresh fork on the repo.
// A stop that fails fails the begin and leaves the record as it was.
func TestBeginBatch_ReclaimsPriorRecoveryStrandBeforeOverwrite(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestBeginBatch_ReResolvesPlanAtDispatch proves BeginBatch re-resolves the plan at dispatch.
// A clean plan dispatches normally.
// A plan with a blocking finding returns a PlanConflictError and writes no prompt file.
// Its cause is plan_defect before any batch is begun and tree_moved after.
// A quarry-unavailable root returns the wrapped infrastructure error rather than ErrPlanConflict.
// An informational-only findings set dispatches normally with the advisory carried on the result.
func TestBeginBatch_ReResolvesPlanAtDispatch(t *testing.T) {
	tiergate.Require(t, 0)
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
		// A bare package-qualified symbol in Uses trips bare-symbol-target under the default glyph-enabled language.
		// That finding is blocking.
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
		if !strings.Contains(err.Error(), "lym webster rebaseline --card NN") {
			t.Errorf("error %q; want it to name `lym webster rebaseline --card NN`", err.Error())
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

// dispatchFindingsIndex is a planindex.Index whose ValidateDispatch answers findings.
type dispatchFindingsIndex struct {
	planindex.Index
	findings []planindex.Finding
}

func (i dispatchFindingsIndex) ValidateDispatch(*planparser.Plan, string, []planparser.Card, []planparser.Card) ([]planindex.Finding, error) {
	return i.findings, nil
}

// TestBeginBatch_ConflictCauseNamesAPlanOrderConflict proves the plan_order cause.
// A refusal whose every blocking finding is delete-before-reference gets that cause,
// and its way forward tells the reader to move the delete, whether or not batches are begun.
// A set holding any other blocking finding keeps its plan_defect or tree_moved cause,
// and its way forward matches the tree.
// An informational delete-before-reference does not count.
func TestBeginBatch_ConflictCauseNamesAPlanOrderConflict(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		moveDelete = "way forward: move each named delete to a card after the card that still references it, run \"lym webster rebaseline --card NN\""
		matchTree  = "way forward: edit the plan so the named cards match the tree, run \"lym webster rebaseline --card NN\""
	)
	deleteOrder := planindex.Finding{Check: planindex.CheckDeleteBeforeReference, Card: "01-json-flag", Severity: planindex.SeverityBlocking}
	deleteOrderAdvisory := planindex.Finding{Check: planindex.CheckDeleteBeforeReference, Card: "01-json-flag", Severity: planindex.SeverityInformational}
	notFound := planindex.Finding{Check: "glyph-not-found", Card: "02-list-tests", Severity: planindex.SeverityBlocking}

	tests := []struct {
		name           string
		findings       []planindex.Finding
		begun          bool
		wantCause      string
		wantWayForward string
	}{
		{"DeleteOrderAfterABatchIsBegun", []planindex.Finding{deleteOrder}, true, websterengine.ConflictCausePlanOrder, moveDelete},
		{"DeleteOrderWithNoBatchBegun", []planindex.Finding{deleteOrder}, false, websterengine.ConflictCausePlanOrder, moveDelete},
		{"MixedFindingsAfterABatchIsBegun", []planindex.Finding{deleteOrder, notFound}, true, websterengine.ConflictCauseTreeMoved, matchTree},
		{"MixedFindingsWithNoBatchBegun", []planindex.Finding{notFound, deleteOrder}, false, websterengine.ConflictCausePlanDefect, matchTree},
		{"InformationalDeleteOrderBesideABlockingFinding", []planindex.Finding{deleteOrderAdvisory, notFound}, true, websterengine.ConflictCauseTreeMoved, matchTree},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newBeginFixture(t)
			fx.Deps.Geom.Index = dispatchFindingsIndex{Index: fx.Deps.Geom.Index, findings: tt.findings}
			batchNumber := 1
			if tt.begun {
				batchNumber = 2
				fx.Deps.State.Batches = map[int]*websterengine.BatchState{1: {Slug: "json-flag", Kind: "fork"}}
			}

			_, err := websterengine.BeginBatch(fx.Deps, batchNumber)
			var conflict *websterengine.PlanConflictError
			if !errors.As(err, &conflict) {
				t.Fatalf("BeginBatch() error = %v; want errors.As to a *PlanConflictError", err)
			}
			if conflict.Cause != tt.wantCause {
				t.Errorf("Cause = %q; want %q", conflict.Cause, tt.wantCause)
			}
			if !strings.Contains(err.Error(), tt.wantWayForward) {
				t.Errorf("error %q; want it to hold %q", err.Error(), tt.wantWayForward)
			}
		})
	}
}

// TestBeginBatch_AlreadyBuiltCardsAreNotReResolved proves re-resolution covers only work that has not landed.
// A plan describes intended change, so a built card contradicts the tree by design:
// its Create target now exists, which the Create inversion reports as blocking create-already-exists.
// Re-resolving built cards would wedge a multi-batch plan at its second batch.
// The plans it wedges are those carrying a Create, Delete or Rename card.
func TestBeginBatch_AlreadyBuiltCardsAreNotReResolved(t *testing.T) {
	tiergate.Require(t, 0)
	fx := newBeginFixture(t)
	// A symbol that exists in the worktree, so card 1's Create target resolves found.
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

// seedRewritingPlanDir writes a real, parseable plan directory.
// Its first card declares a plan: handle that quarry.Name does not compute for its declaration head.
// begin-batch's ValidateDispatch canonicalizes that handle and rewrites the card file on disk.
// Its second card names a glyph that cannot resolve against the fixture's worktree,
// so the same call also reports a blocking finding.
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

// TestBeginBatch_RestampsFingerprintEvenWhenPlanDrifts proves the fingerprint is restamped when the plan drifts.
// ValidateDispatch's resolve pass canonicalizes handles, rewriting the plan on disk.
// It then keeps going, so one call routinely both rewrites the plan and reports a blocking finding.
// The re-baseline must run before the ErrPlanConflict return;
// after it, state.json keeps the pre-rewrite fingerprint while the plan on disk carries webster's sanctioned edit,
// and every later begin-batch refuses that edit as a foreign one.
func TestBeginBatch_RestampsFingerprintEvenWhenPlanDrifts(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestBeginBatch_NilStateIsRefusedNotPanicked proves a nil BeginDeps.State is refused with an error naming the field.
func TestBeginBatch_NilStateIsRefusedNotPanicked(t *testing.T) {
	tiergate.Require(t, 0)
	_, err := websterengine.BeginBatch(websterengine.BeginDeps{Plan: &planparser.Plan{}}, 1)
	if err == nil {
		t.Fatal("websterengine.BeginBatch(nil State) error = nil; want a refusal naming the missing field")
	}
	if !strings.Contains(err.Error(), "State is nil") {
		t.Errorf("websterengine.BeginBatch(nil State) error = %v; want it to name BeginDeps.State", err)
	}
}

// TestBeginBatch_Regression329_ReBeginKeepsForthcomingCreateTarget pins a re-begin of a batch.
// Card 1 creates a file that card 2 Uses. The first begin of batch 1 records it, and no commit lands.
// The second begin of batch 1 must not drop the Create target from the later card's validation.
func TestBeginBatch_Regression329_ReBeginKeepsForthcomingCreateTarget(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestBeginBatch_WayForward_UnknownBatch proves a batch number outside the plan names `lym webster status`.
// It also proves that naming one of the run's batches then begins it.
func TestBeginBatch_WayForward_UnknownBatch(t *testing.T) {
	tiergate.Require(t, 0)
	fx := newBeginFixture(t)

	_, err := websterengine.BeginBatch(fx.Deps, 99)
	if err == nil || !strings.Contains(err.Error(), "way forward: `lym webster status` lists the run's batches") {
		t.Fatalf("BeginBatch(99) error = %v; want the status way forward", err)
	}

	if _, err := websterengine.BeginBatch(fx.Deps, 1); err != nil {
		t.Fatalf("BeginBatch(1) error = %v; want nil", err)
	}
}
