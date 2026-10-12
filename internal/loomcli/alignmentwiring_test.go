// alignmentwiring_test.go pins the parent-review Aligner's wiring over a fake shuttle and a fake board read: when it is off, that Spawn is idempotent per round, and how a polled verdict maps onto the gate's poll.

package loomcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/alignment"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// idleRun is an agent that never ends on its own; Wait returns once the test is over.
type idleRun struct {
	guid    string
	release <-chan struct{}
}

func (r idleRun) Wait() (shuttleengine.Result, error) {
	<-r.release
	return shuttleengine.Result{}, nil
}

func (r idleRun) StrandGUID() string { return r.guid }

// recordingShuttle records the specs it starts and never finds a live run.
type recordingShuttle struct {
	release <-chan struct{}
	specs   []shuttleengine.Spec
}

func (s *recordingShuttle) Start(spec shuttleengine.Spec) (alignment.Run, error) {
	s.specs = append(s.specs, spec)
	return idleRun{guid: "guid", release: s.release}, nil
}

func (s *recordingShuttle) Probe(shuttleengine.Spec) (alignment.Run, bool, error) {
	return nil, false, nil
}

func (s *recordingShuttle) StopStrand(string) error { return nil }

type alignerFixture struct {
	location *lyxcwd.Location
	cfg      loomengine.Config
	registry modelspec.Registry
	stencils string
	shuttle  *recordingShuttle
	notes    []boardengine.Task
}

func newAlignerFixture(t *testing.T) *alignerFixture {
	t.Helper()
	location := hubLocation(t, "pair", ".")
	cfg, err := loomengine.LoadConfig(fabricengine.ConfigDirs(location), "loom")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := modelspec.LoadRegistry(fabricengine.ConfigDirs(location))
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	return &alignerFixture{
		location: location,
		cfg:      cfg,
		registry: registry,
		stencils: fabricengine.StencilsDir(location.HubPath),
		shuttle:  &recordingShuttle{release: release},
	}
}

func (f *alignerFixture) aligner(t *testing.T) parentreview.Aligner {
	t.Helper()
	aligner, err := newParentReviewAligner(f.location, "self", f.cfg, f.registry, f.stencils, f.shuttle, func() ([]boardengine.Task, error) { return f.notes, nil })
	if err != nil {
		t.Fatal(err)
	}
	// The origin record sits in a fabric record, which a tier 1 test has none of.
	if concrete, ok := aligner.(*parentReviewAligner); ok {
		concrete.parentBranch = func() (string, error) { return "main", nil }
	}
	return aligner
}

// answer writes the agent's verdict and report for the first started spec.
func (f *alignerFixture) answer(t *testing.T, verdict string) {
	t.Helper()
	spec := f.shuttle.specs[0]
	if err := os.WriteFile(spec.OutputFiles[0], []byte(verdict), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec.OutputFiles[1], []byte("the report\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParentReviewAligner(t *testing.T) {
	t.Parallel()

	const approve = "verdict: approve\nreason: in scope\n"

	t.Run("skip_alignment gives no aligner", func(t *testing.T) {
		t.Parallel()
		f := newAlignerFixture(t)
		f.cfg.SkipAlignment = true
		if got := f.aligner(t); got != nil {
			t.Fatalf("aligner = %v; want nil", got)
		}
	})

	t.Run("a hub with no board dir gives no aligner", func(t *testing.T) {
		t.Parallel()
		f := newAlignerFixture(t)
		if err := os.RemoveAll(fabricengine.BoardDir(f.location.HubPath)); err != nil {
			t.Fatal(err)
		}
		if got := f.aligner(t); got != nil {
			t.Fatalf("aligner = %v; want nil", got)
		}
	})

	t.Run("spawning a round twice starts one agent", func(t *testing.T) {
		t.Parallel()
		f := newAlignerFixture(t)
		aligner := f.aligner(t)
		first, err := aligner.Spawn(1)
		if err != nil {
			t.Fatal(err)
		}
		second, err := aligner.Spawn(1)
		if err != nil {
			t.Fatal(err)
		}
		if len(f.shuttle.specs) != 1 || first.Attempt == "" || first.Attempt != second.Attempt {
			t.Fatalf("starts = %d, attempts = %q and %q; want one start and one attempt", len(f.shuttle.specs), first.Attempt, second.Attempt)
		}
	})

	for _, row := range []struct {
		name        string
		answer      string
		change      func(t *testing.T, f *alignerFixture)
		wantVerdict string
		wantReason  string
	}{
		{name: "an approve over an unchanged snapshot", answer: approve, wantVerdict: alignment.VerdictApprove, wantReason: "in scope"},
		{
			name:   "an approve after the decision record changed",
			answer: approve,
			change: func(t *testing.T, f *alignerFixture) {
				record := loomengine.DiscussionDecisionRecord(f.location)
				if err := os.MkdirAll(filepath.Dir(record), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(record, []byte("a new decision\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantVerdict: alignment.VerdictEscalate,
			wantReason:  decisionRecordedReason,
		},
		{
			name:   "an approve after a note naming the slug",
			answer: approve,
			change: func(t *testing.T, f *alignerFixture) {
				f.notes = append(f.notes, boardengine.Task{ID: 7, Kind: boardengine.KindNote, Title: "hold pair on x"})
			},
			wantVerdict: alignment.VerdictEscalate,
			wantReason:  decisionRecordedReason,
		},
		{
			name:   "an approve after a note naming another slug",
			answer: approve,
			change: func(t *testing.T, f *alignerFixture) {
				f.notes = append(f.notes, boardengine.Task{ID: 7, Kind: boardengine.KindNote, Title: "hold other on x"})
			},
			wantVerdict: alignment.VerdictApprove,
			wantReason:  "in scope",
		},
		{
			name:        "an escalate carries its question",
			answer:      "verdict: escalate\nreason: overlap\nquestion: which of the two lands first?\n",
			wantVerdict: alignment.VerdictEscalate,
			wantReason:  "which of the two lands first?",
		},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			f := newAlignerFixture(t)
			aligner := f.aligner(t)
			spawned, err := aligner.Spawn(1)
			if err != nil {
				t.Fatal(err)
			}
			if poll, err := aligner.Poll(spawned); err != nil || poll.Done {
				t.Fatalf("poll before the agent answers = %+v, %v; want pending", poll, err)
			}
			f.answer(t, row.answer)
			if row.change != nil {
				row.change(t, f)
			}
			poll, err := aligner.Poll(spawned)
			if err != nil {
				t.Fatal(err)
			}
			want := parentreview.AlignmentPoll{Done: true, Verdict: row.wantVerdict, Report: f.shuttle.specs[0].OutputFiles[1], Reason: row.wantReason}
			if poll != want {
				t.Fatalf("poll = %+v; want %+v", poll, want)
			}
		})
	}
}
