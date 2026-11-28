// resume_test.go pins what a call of Run does with the strands and records a crash left:
// the strands it stops, the runs it finds by their output files, the fixers it attaches to and the rung it continues on.

package weaverengine_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

// saveSeed writes st as the state an earlier call left in the fixture's run directory.
func (f *runFixture) saveSeed(st *weaverengine.State) {
	f.t.Helper()
	geom := f.deps.Geom
	for _, dir := range []string{geom.RunDir, geom.ReportsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			f.t.Fatal(err)
		}
	}
	if err := weaverengine.SaveState(geom.RunDir, geom.ScratchDir, st); err != nil {
		f.t.Fatalf("SaveState() error = %v", err)
	}
}

// seedBegun saves a state whose batch number began over its one card, holding the given attempts.
func (f *runFixture) seedBegun(number int, slug string, attempts ...*weaverengine.AttemptState) *weaverengine.State {
	f.t.Helper()
	head, _ := f.git.HeadSHA("")
	cards := []string{fmt.Sprintf("%02d-%s", number, slug)}
	st := &weaverengine.State{RunGUID: "guid", Attempts: attempts}
	st.Batches = map[int]*runplan.BatchState{number: {Slug: slug, Cards: cards, StartSHA: head, Kind: "strand"}}
	st.Partition = []runplan.PartitionBatch{{Cards: cards}}
	f.saveSeed(st)
	return st
}

// seededAttempt returns attempt n of batch number on rung with its outcome, with the report path Run gives it.
func (f *runFixture) seededAttempt(number int, slug string, n, rung int, outcome string, inFlight bool) *weaverengine.AttemptState {
	return &weaverengine.AttemptState{
		Batch:      number,
		Number:     n,
		Rung:       rung,
		Outcome:    outcome,
		InFlight:   inFlight,
		ReportPath: weaverengine.AttemptReportPath(f.deps.Geom.ReportsDir, number, slug, n),
	}
}

// startStrand starts a strand over output the way a crashed call left it running, and returns its run handle.
// The scripted runner's next script decides what the strand writes.
func (f *runFixture) startStrand(output string) *shuttleengine.Run {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		f.t.Fatal(err)
	}
	spec := shuttleengine.Spec{
		Prompt:      "resume me",
		OutputFiles: []string{output},
		ModelChoice: shuttleengine.ModelChoice{Model: "model-a"},
		ConfigRole:  agentconfig.RoleWebster,
		Role:        weaverengine.BatchStrandRole,
		Segment:     segmentcolor.Webster,
		Round:       "01-one",
	}
	run, err := f.runner.StartGated(spec, nil)
	if err != nil {
		f.t.Fatalf("StartGated() error = %v", err)
	}
	return run
}

// foundRun makes the fixture's probe answer run for every spec.
func (f *runFixture) foundRun(run *shuttleengine.Run) {
	f.runner.probe = func(shuttleengine.Spec) (*shuttleengine.Run, bool, error) { return run, true, nil }
}

func TestRun_Resume(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("RecordedOutcomeStrandIsStoppedFirstAndAFailedStopStartsNothing", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		attempt := fx.seededAttempt(1, "one", 1, 0, "failed", true)
		attempt.StrandGUID = "strand-1"
		fx.seedBegun(1, "one", attempt)
		fx.runner.stopErr = errors.New("reed is gone")

		requireStuck(t, mustRun(t, fx), "strand-1", "lym reed remove strand-1")

		if len(fx.runner.specs) != 0 {
			t.Errorf("strands started = %d; want none after a failed stop", len(fx.runner.specs))
		}
	})

	t.Run("InFlightAttemptIsFoundByItsReportPathAndItsResultRecorded", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		attempt := fx.seededAttempt(1, "one", 1, 0, "", true)
		fx.seedBegun(1, "one", attempt)
		fx.runner.scripts = []attemptScript{{commit: true}}
		run := fx.startStrand(attempt.ReportPath)
		fx.foundRun(run)
		var deliveredTo []string
		fx.deps.Deliver = func(strand string, _ []string) ([]string, error) {
			deliveredTo = append(deliveredTo, strand)
			return []string{"## a decision"}, nil
		}

		requireDone(t, mustRun(t, fx), 1)

		if len(fx.runner.specs) != 1 {
			t.Errorf("strands started = %d; want only the crashed attempt's, with no second agent", len(fx.runner.specs))
		}
		if len(fx.runner.probed) == 0 || fx.runner.probed[0].OutputFiles[0] != attempt.ReportPath || fx.runner.probed[0].Timeout != 45*time.Minute {
			t.Errorf("probed specs = %v; want the probe by the attempt's report path, bounded by attempt_timeout_min", fx.runner.probed)
		}
		got := fx.loadState().Attempts[0]
		if got.Outcome != "done" || got.StrandGUID != run.StrandGUID() || !slices.Equal(got.Delivered, []string{"## a decision"}) {
			t.Errorf("attempt = outcome %q, guid %q, delivered %v; want done, the found run's guid and the sent heading", got.Outcome, got.StrandGUID, got.Delivered)
		}
		if len(deliveredTo) != 1 || deliveredTo[0] == "" {
			t.Errorf("decisions delivered to %v; want the probed strand once", deliveredTo)
		}
	})

	for _, reportOnDisk := range []bool{false, true} {
		name := "NotFoundProbeWithNoReportIsAMachineryLoss"
		if reportOnDisk {
			name = "NotFoundProbeWithTheReportOnDiskIsAMachineryLoss"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fx := newRunFixture(t, 1, "one")
			attempt := fx.seededAttempt(1, "one", 1, 0, "", true)
			fx.seedBegun(1, "one", attempt)
			if reportOnDisk {
				head, _ := fx.git.HeadSHA("")
				if err := runplan.WriteReport(attempt.ReportPath, &runplan.Report{Status: runplan.ReportStatusOK, HeadSHA: head}); err != nil {
					t.Fatal(err)
				}
			}
			fx.runner.scripts = []attemptScript{{commit: true}}

			requireDone(t, mustRun(t, fx), 1)

			st := fx.loadState()
			if st.Attempts[0].Outcome != "lost" || st.Attempts[1].Outcome != "done" {
				t.Errorf("attempts = %q, %q; want a lost one and a done retry", st.Attempts[0].Outcome, st.Attempts[1].Outcome)
			}
			if len(fx.runner.specs) != 1 || fx.runner.specs[0].Model != "model-a" {
				t.Errorf("started = %d strand(s), first model %q; want one retry on the same rung", len(fx.runner.specs), fx.runner.specs[0].Model)
			}
		})
	}

	t.Run("ProbeErrorStartsNothingAndNamesTheError", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.seedBegun(1, "one", fx.seededAttempt(1, "one", 1, 0, "", true))
		fx.runner.probe = func(shuttleengine.Spec) (*shuttleengine.Run, bool, error) {
			return nil, false, errors.New("reed state is unreadable")
		}

		requireStuck(t, mustRun(t, fx), "reed state is unreadable", "way forward")

		if len(fx.runner.specs) != 0 {
			t.Errorf("strands started = %d; want none after a probe error", len(fx.runner.specs))
		}
	})

	t.Run("LiveFixerRecordIsAttachedDeliveredToAndAwaited", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		st := fx.seedBegun(1, "one")
		st.Batches[1].Terminal, st.Batches[1].Status = true, runplan.DigestStatusDone
		st.Fixers = []*weaverengine.FixerState{{
			Number:     1,
			ReplyPath:  weaverengine.FixerReplyPath(fx.deps.Geom.ReportsDir, 1),
			Findings:   "TestBroken fails",
			StrandName: "fixer-strand",
			Record:     &gatefixer.Record{StrandGUID: "fixer-live", SpawnedAt: time.Unix(0, 0).UTC().Format(time.RFC3339)},
		}}
		fx.saveSeed(st)
		var deliveredTo []string
		fx.deps.Deliver = func(strand string, _ []string) ([]string, error) {
			deliveredTo = append(deliveredTo, strand)
			return []string{"## a decision"}, nil
		}

		result := mustRun(t, fx)

		requireDone(t, result, 1)
		if got, want := spanKinds(result), []shuttleengine.GateSpanKind{shuttleengine.GateSpanGate}; !slices.Equal(got, want) {
			t.Errorf("verify spans = %v; want the verify's run alone, with none for the attached fixer: %v", got, want)
		}
		if fx.fixer.attached != 1 || len(fx.fixer.specs) != 0 {
			t.Errorf("fixer attaches = %d, spawns = %d; want one attach and no spawn", fx.fixer.attached, len(fx.fixer.specs))
		}
		if !slices.Equal(deliveredTo, []string{"fixer-strand"}) {
			t.Errorf("decisions delivered to %v; want the fixer's strand once", deliveredTo)
		}
		if got := fx.loadState().Fixers[0]; got.Outcome != "done" || !slices.Equal(got.Delivered, []string{"## a decision"}) {
			t.Errorf("fixer = outcome %q, delivered %v; want done with the sent heading", got.Outcome, got.Delivered)
		}
	})

	t.Run("FixerWithNoRecordHasItsProbedStrandStoppedAndTheVerifyRunsAgain", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		st := fx.seedBegun(1, "one")
		st.Batches[1].Terminal, st.Batches[1].Status = true, runplan.DigestStatusDone
		replyPath := weaverengine.FixerReplyPath(fx.deps.Geom.ReportsDir, 1)
		st.Fixers = []*weaverengine.FixerState{{Number: 1, ReplyPath: replyPath, Findings: "TestBroken fails"}}
		fx.saveSeed(st)
		fx.runner.scripts = []attemptScript{{noReport: true}}
		run := fx.startStrand(replyPath)
		fx.foundRun(run)

		requireDone(t, mustRun(t, fx), 1)

		if len(fx.runner.probed) == 0 || fx.runner.probed[0].OutputFiles[0] != replyPath {
			t.Errorf("probed specs = %v; want the probe by the fixer's reply path", fx.runner.probed)
		}
		if !slices.Contains(fx.reed.RemovedGUIDs, run.StrandGUID()) {
			t.Errorf("removed strands = %v; want the probed fixer strand %q stopped", fx.reed.RemovedGUIDs, run.StrandGUID())
		}
		if got := fx.loadState().Fixers[0].Outcome; got != "lost" {
			t.Errorf("fixer outcome = %q; want lost", got)
		}
		if len(fx.verify.calls) != 1 || len(fx.fixer.specs) != 0 {
			t.Errorf("verify calls = %d, fixers spawned = %d; want the verify run again and no new fixer", len(fx.verify.calls), len(fx.fixer.specs))
		}
	})

	continues := []struct {
		name string
		// attempts are the recorded attempts of batch 1.
		attempts []struct {
			rung    int
			outcome string
		}
		// wantModel is the model of the next attempt, empty when the row stops instead.
		wantModel string
	}{
		{
			name: "FailedAttemptOnRungZeroRunsRungOne",
			attempts: []struct {
				rung    int
				outcome string
			}{{0, "failed"}},
			wantModel: "model-b",
		},
		{
			name: "LostAttemptRetriesOnItsOwnRung",
			attempts: []struct {
				rung    int
				outcome string
			}{{0, "lost"}},
			wantModel: "model-a",
		},
		{
			name: "SecondLossOnARungClimbs",
			attempts: []struct {
				rung    int
				outcome string
			}{{0, "lost"}, {0, "lost"}},
			wantModel: "model-b",
		},
		{
			name: "FailedSecondRungAttemptRunsTheThirdRung",
			attempts: []struct {
				rung    int
				outcome string
			}{{0, "failed"}, {1, "failed"}},
			wantModel: "model-c",
		},
		{
			name: "LostLastRungAttemptRetriesOnTheLastRung",
			attempts: []struct {
				rung    int
				outcome string
			}{{2, "lost"}},
			wantModel: "model-c",
		},
		{
			name: "FaultAttemptRetriesOnItsOwnRung",
			attempts: []struct {
				rung    int
				outcome string
			}{{0, "fault"}},
			wantModel: "model-a",
		},
		{
			name: "FailedLastRungAttemptStopsTheRow",
			attempts: []struct {
				rung    int
				outcome string
			}{{2, "failed"}},
		},
	}
	for _, tt := range continues {
		t.Run("Crash_"+tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newRunFixture(t, 1, "one")
			var attempts []*weaverengine.AttemptState
			for i, recorded := range tt.attempts {
				attempts = append(attempts, fx.seededAttempt(1, "one", i+1, recorded.rung, recorded.outcome, false))
			}
			seeded := fx.seedBegun(1, "one", attempts...)
			fx.runner.scripts = []attemptScript{{commit: true}}

			result := mustRun(t, fx)

			st := fx.loadState()
			if tt.wantModel == "" {
				requireStuck(t, result, "batch 01-one")
				if st.Stop == nil || st.Stop.Kind != "batch" || st.Stop.Batch != 1 || st.Stop.Rung != 2 {
					t.Errorf("Stop = %+v; want a batch stop on batch 1 at rung 2", st.Stop)
				}
				if len(fx.runner.specs) != 0 {
					t.Errorf("strands started = %d; want none", len(fx.runner.specs))
				}
				return
			}
			requireDone(t, result, 1)
			if len(fx.runner.specs) != 1 || fx.runner.specs[0].Model != tt.wantModel {
				t.Errorf("started = %d strand(s), model %q; want one on %q", len(fx.runner.specs), fx.runner.specs[0].Model, tt.wantModel)
			}
			if got, want := st.Batches[1].StartSHA, seeded.Batches[1].StartSHA; got != want || !slices.Equal(st.Batches[1].Cards, seeded.Batches[1].Cards) {
				t.Errorf("batch record = start %q cards %v; want the recorded start %q and card set kept", got, st.Batches[1].Cards, want)
			}
		})
	}
}
