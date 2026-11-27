// roundcontext_test.go tests BuildRoundContext over a temp run directory.
// It also holds the helpers that lay out a complete round.

package shedadapters

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	// contextReview is a parsing review holding a design finding F1 and a decision-class finding F2.
	contextReview = "---\nverdict: BLOCKING\nfindings:\n" +
		"  - id: F1\n    severity: BLOCKING\n    class: design\n    location: a.go:1\n    summary: s1\n    fix: f1\n" +
		"  - id: F2\n    severity: MEDIUM\n    class: decision\n    location: a.go:2\n    summary: s2\n    fix: f2\n" +
		"---\n"
	// contextEngineReport is the fixer-report the engine writes itself when no fixer is spawned.
	contextEngineReport = "---\nspawned: false\ndispositions: []\n---\nnothing fixed\n"
	// contextFixerReport is a fixer-written report with an entry for each finding of contextReview.
	contextFixerReport = "---\ndispositions:\n  - id: F1\n    disposition: fixed\n  - id: F2\n    disposition: fixed\n---\n"
)

// writeCompleteRound writes round n's review, the engine's fixer-report and a snapshot record with decisions_routed.
// It overwrites whatever files the round holds, and leaves the state a round whose fix half ended leaves.
func writeCompleteRound(t *testing.T, runDir string, n int, routed bool) {
	t.Helper()
	writeRoundFileWith(t, roundReviewPath(runDir, n), contextReview)
	writeRoundFileWith(t, roundFixerReportPath(runDir, n), contextEngineReport)
	err := updateRoundSnapshot(runDir, n, func(rec *roundSnapshot, _ bool) { rec.DecisionsRouted = &routed })
	if err != nil {
		t.Fatalf("updateRoundSnapshot(round %d) = %v; want nil", n, err)
	}
}

// addedEntry renders one `Added after Discussion` entry of a decision record.
func addedEntry(title, body string) string {
	return "### Added after Discussion (parent, 2026-11-17): " + title + "\n\nDecision: " + body + "\n"
}

// writeDecisionRecord writes a decision record holding entries at path.
func writeDecisionRecord(t *testing.T, path string, entries ...string) {
	t.Helper()
	writeRoundFileWith(t, path, "# Record\n\n## Decisions\n\n"+strings.Join(entries, "\n"))
}

// recordDigests records the digests of the entries the decision record at recordPath holds in round n's snapshot record.
func recordDigests(t *testing.T, runDir string, n int, recordPath string) {
	t.Helper()
	digests, err := addedDecisionDigests(recordPath)
	if err != nil {
		t.Fatalf("addedDecisionDigests() = %v; want nil", err)
	}
	if err := updateRoundSnapshot(runDir, n, func(rec *roundSnapshot, _ bool) { rec.AddedDecisions = digests }); err != nil {
		t.Fatalf("updateRoundSnapshot(round %d) = %v; want nil", n, err)
	}
}

// recordParentAsk records an ask for round of the generation runDir holds, with the focus file that gives it its seed.
func recordParentAsk(t *testing.T, runDir string, files burlerengine.ParentReviewFiles, round int) {
	t.Helper()
	writeFocusFile(t, runDir, 1, focusFile{Round: 1, Focus: []string{"look at the seam"}})
	seed, err := roundSeed(runDir)
	if err != nil {
		t.Fatalf("roundSeed() = %v; want nil", err)
	}
	ask := burlerengine.ParentAsk{Round: round, Seed: seed, State: burlerengine.ParentAskDone}
	if _, err := burlerengine.RecordParentAsk(files, ask); err != nil {
		t.Fatalf("RecordParentAsk() = %v; want nil", err)
	}
}

func TestBuildRoundContext(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// env is what a row's setup lays out: the run directory, the decision record's path and the run's anchor.
	type env struct {
		runDir, recordPath, anchor string
	}
	// complete lays round 1 out as complete, with decisions not routed, as most rows start.
	complete := func(t *testing.T, e env) { writeCompleteRound(t, e.runDir, 1, false) }
	// then runs setup after complete.
	then := func(setup func(t *testing.T, e env)) func(t *testing.T, e env) {
		return func(t *testing.T, e env) {
			complete(t, e)
			setup(t, e)
		}
	}

	tests := []struct {
		name  string
		round int
		setup func(t *testing.T, e env)
		// withAnchor passes the run's anchor to the builder.
		withAnchor bool
		wantNil    bool
		wantErr    bool
		check      func(t *testing.T, got *burlerengine.RoundContext, e env)
	}{
		{name: "RoundOneHasNoEarlierRound", round: 1, setup: complete, wantNil: true},
		{
			name: "EarlierReviewAbsent", setup: then(func(t *testing.T, e env) { _ = os.Remove(roundReviewPath(e.runDir, 1)) }), wantNil: true,
		},
		{
			name: "EarlierReviewUnparseable", wantNil: true,
			setup: then(func(t *testing.T, e env) { writeRoundFileWith(t, roundReviewPath(e.runDir, 1), "not frontmatter") }),
		},
		{
			name: "EarlierSnapshotRecordAbsent", wantNil: true,
			setup: then(func(t *testing.T, e env) { _ = os.Remove(roundSnapshotPath(e.runDir, 1)) }),
		},
		{
			name: "EarlierRecordLacksDecisionsRouted", wantNil: true,
			setup: then(func(t *testing.T, e env) {
				if err := updateRoundSnapshot(e.runDir, 1, func(rec *roundSnapshot, _ bool) { rec.DecisionsRouted = nil }); err != nil {
					t.Fatalf("updateRoundSnapshot() = %v; want nil", err)
				}
			}),
		},
		{
			name: "FixerReportFailsTheFixCheck", wantNil: true,
			setup: then(func(t *testing.T, e env) {
				writeRoundFileWith(t, roundFixerReportPath(e.runDir, 1), "---\ndispositions:\n  - id: F1\n    disposition: fixed\n---\n")
			}),
		},
		{
			name: "FixerReportUnparseable", wantNil: true,
			setup: then(func(t *testing.T, e env) { writeRoundFileWith(t, roundFixerReportPath(e.runDir, 1), "not frontmatter") }),
		},
		{
			name: "SnapshotRecordPathIsADirectory", wantErr: true,
			setup: then(func(t *testing.T, e env) {
				if err := os.Remove(roundSnapshotPath(e.runDir, 1)); err != nil {
					t.Fatalf("Remove(record) = %v; want nil", err)
				}
				if err := os.Mkdir(roundSnapshotPath(e.runDir, 1), 0o755); err != nil {
					t.Fatalf("Mkdir(record path) = %v; want nil", err)
				}
			}),
		},
		{
			name: "EngineWrittenReportGivesAContext", setup: complete,
			check: func(t *testing.T, got *burlerengine.RoundContext, e env) {
				if len(got.Prior) != 2 || got.Prior[0].Finding.ID != "F1" || got.Prior[1].Finding.ID != "F2" {
					t.Fatalf("Prior = %+v; want F1 and F2 in review order", got.Prior)
				}
				if got.Prior[0].Disposition != nil || got.Prior[0].Held || got.Prior[1].Held {
					t.Errorf("Prior = %+v; want no disposition and nothing held when decisions are not routed", got.Prior)
				}
				if !got.Incremental || got.HasChanges || got.ChangesPath != "" || got.ParentReportPath != "" {
					t.Errorf("context = %+v; want an incremental round with no changes file and no parent report", got)
				}
				if got.PriorReviewPath != roundReviewPath(e.runDir, 1) || got.PriorFixerReportPath != roundFixerReportPath(e.runDir, 1) {
					t.Errorf("paths = %q, %q; want round 1's review and fixer-report", got.PriorReviewPath, got.PriorFixerReportPath)
				}
			},
		},
		{
			name: "FixerWrittenReportCarriesItsDispositions",
			setup: then(func(t *testing.T, e env) {
				writeRoundFileWith(t, roundFixerReportPath(e.runDir, 1), contextFixerReport)
			}),
			check: func(t *testing.T, got *burlerengine.RoundContext, _ env) {
				if d := got.Prior[0].Disposition; d == nil || d.Disposition != burlerengine.DispositionFixed {
					t.Errorf("Prior[0].Disposition = %+v; want fixed", d)
				}
			},
		},
		{
			name:  "RoutedMarksTheDecisionFindingHeld",
			setup: func(t *testing.T, e env) { writeCompleteRound(t, e.runDir, 1, true) },
			check: func(t *testing.T, got *burlerengine.RoundContext, _ env) {
				if got.Prior[0].Held || !got.Prior[1].Held {
					t.Errorf("held = %v, %v; want only the decision-class finding held", got.Prior[0].Held, got.Prior[1].Held)
				}
			},
		},
		{
			name: "HeldEntryIsNotListedAndANewOneIs",
			setup: func(t *testing.T, e env) {
				complete(t, e)
				writeDecisionRecord(t, e.recordPath, addedEntry("First call", "cache per run."))
				recordDigests(t, e.runDir, 1, e.recordPath)
				writeDecisionRecord(t, e.recordPath, addedEntry("First call", "cache per run."), addedEntry("Second call", "no retry."))
			},
			check: func(t *testing.T, got *burlerengine.RoundContext, _ env) {
				if !slices.Equal(got.AddedTitles, []string{"Second call"}) || len(got.AddedTexts) != 1 || !strings.Contains(got.AddedTexts[0], "no retry.") {
					t.Errorf("added = %v, %q; want only Second call", got.AddedTitles, got.AddedTexts)
				}
			},
		},
		{
			name: "EntryAddedAfterAFoldRemovedEarlierOnesIsListed",
			setup: func(t *testing.T, e env) {
				complete(t, e)
				writeDecisionRecord(t, e.recordPath, addedEntry("First call", "cache per run."), addedEntry("Second call", "no retry."))
				recordDigests(t, e.runDir, 1, e.recordPath)
				writeDecisionRecord(t, e.recordPath, addedEntry("Third call", "log it."))
			},
			check: func(t *testing.T, got *burlerengine.RoundContext, _ env) {
				if !slices.Equal(got.AddedTitles, []string{"Third call"}) {
					t.Errorf("AddedTitles = %v; want [Third call]", got.AddedTitles)
				}
			},
		},
		{
			name: "EntryUnderAFoldedTitleWithANewBodyIsListed",
			setup: func(t *testing.T, e env) {
				complete(t, e)
				writeDecisionRecord(t, e.recordPath, addedEntry("First call", "cache per run."))
				recordDigests(t, e.runDir, 1, e.recordPath)
				writeDecisionRecord(t, e.recordPath, addedEntry("First call", "cache per round."))
			},
			check: func(t *testing.T, got *burlerengine.RoundContext, _ env) {
				if !slices.Equal(got.AddedTitles, []string{"First call"}) || !strings.Contains(got.AddedTexts[0], "cache per round.") {
					t.Errorf("added = %v, %q; want First call with its new body", got.AddedTitles, got.AddedTexts)
				}
			},
		},
		{
			name: "AmendmentGenerationIsNotIncremental",
			setup: then(func(t *testing.T, e env) {
				if err := writeGenerationRecord(e.runDir, generationRecord{Number: 2, Amendment: true}); err != nil {
					t.Fatalf("writeGenerationRecord() = %v; want nil", err)
				}
			}),
			check: func(t *testing.T, got *burlerengine.RoundContext, _ env) {
				if got.Incremental {
					t.Error("Incremental = true; want false under an amendment generation")
				}
			},
		},
		{
			name: "ChangesFileFillsChanged",
			setup: then(func(t *testing.T, e env) {
				if err := writeRoundChanges(e.runDir, 2, []string{"a.go", "b.go"}); err != nil {
					t.Fatalf("writeRoundChanges() = %v; want nil", err)
				}
			}),
			check: func(t *testing.T, got *burlerengine.RoundContext, e env) {
				if !got.HasChanges || !slices.Equal(got.Changed, []string{"a.go", "b.go"}) || got.ChangesPath != roundChangesPath(e.runDir, 2) {
					t.Errorf("changes = %v, %v, %q; want a.go and b.go listed in round 2's changes file", got.HasChanges, got.Changed, got.ChangesPath)
				}
			},
		},
		{
			name: "ParentReportOfTheRoundsAsk", withAnchor: true,
			setup: then(func(t *testing.T, e env) {
				recordParentAsk(t, e.runDir, burlerengine.ParentReviewFilesOf(e.anchor), 2)
			}),
			check: func(t *testing.T, got *burlerengine.RoundContext, e env) {
				if want := burlerengine.ParentReviewFilesOf(e.anchor).ReportPath(1); got.ParentReportPath != want {
					t.Errorf("ParentReportPath = %q; want %q", got.ParentReportPath, want)
				}
			},
		},
		{
			name: "NoParentReportWhenTheRoundHoldsNoAsk", withAnchor: true,
			setup: then(func(t *testing.T, e env) {
				recordParentAsk(t, e.runDir, burlerengine.ParentReviewFilesOf(e.anchor), 3)
			}),
			check: func(t *testing.T, got *burlerengine.RoundContext, _ env) {
				if got.ParentReportPath != "" {
					t.Errorf("ParentReportPath = %q; want none", got.ParentReportPath)
				}
			},
		},
		{
			name: "NoParentReportWhenTheAskRecordIsUnreadable", withAnchor: true,
			setup: then(func(t *testing.T, e env) {
				writeFocusFile(t, e.runDir, 1, focusFile{Round: 1, Focus: []string{"look at the seam"}})
				if err := os.MkdirAll(burlerengine.ParentReviewFilesOf(e.anchor).Record, 0o755); err != nil {
					t.Fatalf("MkdirAll(ask record path) = %v; want nil", err)
				}
			}),
			check: func(t *testing.T, got *burlerengine.RoundContext, _ env) {
				if got.ParentReportPath != "" {
					t.Errorf("ParentReportPath = %q; want none", got.ParentReportPath)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			e := env{runDir: filepath.Join(root, "run"), recordPath: filepath.Join(root, "decisions.md"), anchor: filepath.Join(root, "anchor")}
			if err := os.MkdirAll(e.runDir, 0o755); err != nil {
				t.Fatalf("MkdirAll(run dir) = %v; want nil", err)
			}
			if tt.setup != nil {
				tt.setup(t, e)
			}
			round, anchor := tt.round, ""
			if round == 0 {
				round = 2
			}
			if tt.withAnchor {
				anchor = e.anchor
			}

			got, err := BuildRoundContext(e.runDir, round, e.recordPath, anchor)

			if (err != nil) != tt.wantErr {
				t.Fatalf("BuildRoundContext() error = %v; want an error: %v", err, tt.wantErr)
			}
			if tt.wantNil || tt.wantErr {
				if got != nil {
					t.Errorf("BuildRoundContext() = %+v; want a nil context", got)
				}
				return
			}
			if got == nil {
				t.Fatal("BuildRoundContext() = nil; want a context")
			}
			tt.check(t, got, e)
		})
	}
}
