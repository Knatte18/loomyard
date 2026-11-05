// raddlefile_test.go exercises the Raddle-File producer over recording fakes for every seam and a run's files in a temp directory.

package loomshed_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/raddleparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// raddleFileRecord is raddleRecord with the two sections a part's record copies whole.
var raddleFileRecord = raddleRecord + "\n## Constraints\n\nNone.\n\n## Auto-mode assumptions\n\nNone.\n"

// raddleFileProposal returns a proposal of the shape that passes the checks against raddleFileRecord: the tapestry shape's one part builds every decision, and the other shapes keep the two parts, the project shapes adding their project.
func raddleFileProposal(shape string) raddleparser.Proposal {
	proposal := validRaddleProposal()
	proposal.Shape = shape
	for i := range proposal.Parts {
		proposal.Parts[i].Contract = raddleparser.Contract{Takes: "takes " + proposal.Parts[i].Slug, Delivers: "delivers " + proposal.Parts[i].Slug, Exports: "Exports" + proposal.Parts[i].Slug}
	}
	switch shape {
	case raddleparser.ShapeTapestry:
		first, second := proposal.Parts[0], proposal.Parts[1]
		first.Decisions = append(first.Decisions, second.Decisions...)
		first.OpenRisks = append(first.OpenRisks, second.OpenRisks...)
		first.Criteria = append(first.Criteria, second.Criteria...)
		first.PatternChanges = append(first.PatternChanges, second.PatternChanges...)
		first.Packages = append(first.Packages, second.Packages...)
		proposal.Parts = []raddleparser.Part{first}
	case raddleparser.ShapeProject, raddleparser.ShapeBeam:
		proposal.Project = &raddleparser.Project{Slug: "proj-1", Title: "Project", Type: "feature"}
	}
	return proposal
}

// raddleFileFixture is a Raddle-File producer over files in a temp directory and recording seams.
type raddleFileFixture struct {
	t        *testing.T
	deps     loomshed.RaddleFileDeps
	proposal raddleparser.Proposal
	created  time.Time

	calls    []string
	failing  map[string]error
	findings []raddleparser.Finding
	recheck  raddleparser.Recheck
	filings  []boardengine.RaddleFiling
	released []string
	notices  []string

	stored     bool
	storedHash string

	alreadyFiled bool
	askedSketch  string
	askedSlugs   []string
}

// newRaddleFileFixture writes the proposal, the cold proposal, the record and the run's state, with the approval of the proposal recorded when approved is set.
func newRaddleFileFixture(t *testing.T, proposal raddleparser.Proposal, mode loomshed.RaddleMode, approved bool) *raddleFileFixture {
	t.Helper()
	dir := t.TempDir()
	f := &raddleFileFixture{t: t, proposal: proposal, created: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC), failing: map[string]error{}}
	f.deps = loomshed.RaddleFileDeps{
		ProposalPath:   filepath.Join(dir, "proposal.yaml"),
		ColdPath:       filepath.Join(dir, "cold-proposal.yaml"),
		RecordPath:     filepath.Join(dir, "decision.md"),
		StageDir:       filepath.Join(dir, "stage"),
		RecordsDir:     filepath.Join(dir, "records"),
		PartRecordPath: func(slug string) string { return filepath.Join(dir, "records", "decision-"+slug+".md") },
		StatePath:      filepath.Join(dir, "state.json"),
		StateLockPath:  filepath.Join(dir, "state.json.lock"),
		Check: func() ([]raddleparser.Finding, error) {
			return f.findings, f.seam("check")
		},
		Recheck: func() (raddleparser.Recheck, error) {
			return f.recheck, f.seam("recheck")
		},
		Entry: func() (loomshed.RaddleRunEntry, error) {
			return loomshed.RaddleRunEntry{Slug: "sketch-x", Created: f.created, ModuleLabels: []string{"mod-a"}, Project: "grouping"}, nil
		},
		StoreDone: func(created time.Time, hash string) (bool, error) {
			return f.stored && created.Equal(f.created) && hash == f.storedHash, nil
		},
		AlreadyFiled: func(sketch string, slugs []string) (bool, error) {
			f.askedSketch, f.askedSlugs = sketch, slugs
			return f.alreadyFiled, nil
		},
		File: func(filing boardengine.RaddleFiling) error {
			if err := f.seam("file"); err != nil {
				return err
			}
			f.filings = append(f.filings, filing)
			f.stored, f.storedHash = true, filing.Hash
			return nil
		},
		CreateBeam: func(string) error { return f.seam("create_beam") },
		Release: func(slugs []string) error {
			f.released = slugs
			return f.seam("release")
		},
		Notify: func(line string) error {
			if err := f.seam("notify"); err != nil {
				return err
			}
			f.notices = append(f.notices, line)
			return nil
		},
		EndEntry: func() error { return f.seam("end_entry") },
		Commit:   func() error { return f.seam("commit") },
	}

	f.writeProposal(proposal)
	f.write(f.deps.ColdPath, "cold")
	f.write(f.deps.RecordPath, raddleFileRecord)
	f.updateState(func(stored *loomshed.RaddleState) {
		stored.Mode = mode
		if approved {
			stored.Approval = &loomshed.RaddleApprovalRecord{Hash: raddleparser.Hash(marshalRaddleProposal(t, proposal)), Session: 1}
		}
	})
	return f
}

// seam records a call of the named seam and returns the error scripted for it.
func (f *raddleFileFixture) seam(name string) error {
	f.calls = append(f.calls, name)
	return f.failing[name]
}

func (f *raddleFileFixture) write(path, content string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatalf("create the directory of %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		f.t.Fatalf("write %s: %v", path, err)
	}
}

func (f *raddleFileFixture) read(path string) string {
	f.t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func (f *raddleFileFixture) writeProposal(proposal raddleparser.Proposal) {
	f.t.Helper()
	f.proposal = proposal
	f.write(f.deps.ProposalPath, string(marshalRaddleProposal(f.t, proposal)))
}

func (f *raddleFileFixture) updateState(mutate func(*loomshed.RaddleState)) {
	f.t.Helper()
	if _, err := loomshed.UpdateRaddleState(f.deps.StatePath, f.deps.StateLockPath, mutate); err != nil {
		f.t.Fatalf("UpdateRaddleState() = %v; want nil", err)
	}
}

func (f *raddleFileFixture) state() loomshed.RaddleState {
	f.t.Helper()
	stored, err := loomshed.ReadRaddleState(f.deps.StatePath, f.deps.StateLockPath)
	if err != nil {
		f.t.Fatalf("ReadRaddleState() = %v; want nil", err)
	}
	return stored
}

func (f *raddleFileFixture) call() (shedengine.Outcome, shedengine.OutputPointer, error) {
	return loomshed.NewRaddleFile("Raddle-File", f.deps).Call(context.Background())
}

// count returns how many times the named seam was called.
func (f *raddleFileFixture) count(name string) int {
	calls := 0
	for _, call := range f.calls {
		if call == name {
			calls++
		}
	}
	return calls
}

// steps returns the calls of the seams that are steps of the row, leaving the checks out.
func (f *raddleFileFixture) steps() []string {
	return slices.DeleteFunc(slices.Clone(f.calls), func(name string) bool { return name == "check" || name == "recheck" })
}

// requireDone fails the test unless the call reported Done.
func requireDone(t *testing.T, outcome shedengine.Outcome, err error) {
	t.Helper()
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = %q, %v; want Done", outcome, err)
	}
}

func TestRaddleFile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	moved := []raddleparser.Finding{{Check: raddleparser.CheckRevisedMoved, Detail: `the entry "other" was revised`}}

	t.Run("EachShapeFilesOnceWithItsStepsInOrder", func(t *testing.T) {
		t.Parallel()
		inOrder := []string{"commit", "file", "notify", "end_entry"}
		beamOrder := []string{"commit", "file", "create_beam", "release", "notify", "end_entry"}
		for _, tt := range []struct {
			shape string
			want  []string
		}{
			{raddleparser.ShapeTapestry, inOrder},
			{raddleparser.ShapeTasks, inOrder},
			{raddleparser.ShapeProject, inOrder},
			{raddleparser.ShapeBeam, beamOrder},
		} {
			t.Run(tt.shape, func(t *testing.T) {
				t.Parallel()
				f := newRaddleFileFixture(t, raddleFileProposal(tt.shape), loomshed.RaddleModeAuto, false)
				outcome, _, err := f.call()
				requireDone(t, outcome, err)

				if got := f.steps(); !slices.Equal(got, tt.want) {
					t.Fatalf("steps = %v; want %v", got, tt.want)
				}
				filing := f.filings[0]
				if filing.Release != (tt.shape != raddleparser.ShapeBeam) || filing.Project != "grouping" || filing.RunEntry != "sketch-x" || filing.Hash != raddleparser.Hash(marshalRaddleProposal(t, f.proposal)) {
					t.Errorf("filing release/project/run entry/hash = %v/%q/%q/%q; want release off only for a beam, the Sketch's project, the run's entry and the proposal's hash", filing.Release, filing.Project, filing.RunEntry, filing.Hash)
				}
				if tt.shape == raddleparser.ShapeBeam && !slices.Equal(f.released, []string{"part-a", "part-b"}) {
					t.Errorf("released = %v; want both parts", f.released)
				}
				if got := f.state(); !got.NoticeSent {
					t.Errorf("state = %+v; want the notice marked sent", got)
				}

				isProject := tt.shape == raddleparser.ShapeProject || tt.shape == raddleparser.ShapeBeam
				if (filing.ProjectEntry != nil) != isProject {
					t.Fatalf("project entry = %+v; want one only for the project shapes", filing.ProjectEntry)
				}
				if isProject {
					if want := []string{"feature", "mod-a"}; !slices.Equal(filing.ProjectEntry.Labels, want) || filing.ProjectEntry.Kind != boardengine.KindProject || string(filing.ProjectEntry.Record) != raddleFileRecord {
						t.Errorf("project entry = %+v; want labels %v, kind project and the Sketch's record whole", filing.ProjectEntry, want)
					}
				}
				for _, part := range filing.Parts {
					if want := []string{"feature", "mod-a"}; !slices.Equal(part.Labels, want) || part.Kind != boardengine.KindTask {
						t.Errorf("part %s labels/kind = %v/%q; want %v and a task", part.Slug, part.Labels, part.Kind, want)
					}
					for _, want := range []string{"`sketch-x`", "takes " + part.Slug, "delivers " + part.Slug, "Exports" + part.Slug, "(about 10 changed lines)"} {
						if !strings.Contains(part.Body, want) {
							t.Errorf("part %s body = %q; want it to contain %q", part.Slug, part.Body, want)
						}
					}
				}

				switch tt.shape {
				case raddleparser.ShapeTapestry:
					if got := string(filing.Parts[0].Record); got != raddleFileRecord {
						t.Errorf("the tapestry part's record = %q; want the Sketch's record whole", got)
					}
				case raddleparser.ShapeTasks:
					first := string(filing.Parts[0].Record)
					for _, want := range []string{"## Goal", "## Scope", "### D1", "Risk one"} {
						if !strings.Contains(first, want) {
							t.Errorf("part-a record = %q; want it to contain %q", first, want)
						}
					}
					if strings.Contains(first, "### D2") || strings.Contains(first, "Risk two") {
						t.Errorf("part-a record = %q; want only its own decisions and bullets", first)
					}
				}
			})
		}
	})

	t.Run("AFailureAtFilingSendsTheProposalBackAndDeletesItsOutputs", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			approved   bool
			setup      func(f *raddleFileFixture)
			wantReason string
			wantFiled  int
		}{
			{name: "AFailingCheck", approved: true, setup: func(f *raddleFileFixture) {
				f.findings = []raddleparser.Finding{{Check: raddleparser.CheckSlugTaken, Detail: "taken"}}
			}, wantReason: raddleparser.CheckSlugTaken},
			{name: "AProposalEditedAfterApproval", approved: true, setup: func(f *raddleFileFixture) {
				edited := f.proposal
				edited.Cost.Line = "edited after approval"
				f.writeProposal(edited)
			}, wantReason: "edited after it was approved"},
			{name: "AProposalWithNoApproval", approved: false, setup: func(*raddleFileFixture) {}, wantReason: "no recorded approval"},
			{name: "AMovedBoard", approved: true, setup: func(f *raddleFileFixture) {
				f.recheck.Failures = moved
			}, wantReason: raddleparser.CheckRevisedMoved},
			{name: "TheBoardWriteRefusingItsRecheck", approved: true, setup: func(f *raddleFileFixture) {
				f.failing["file"] = fmt.Errorf("%w: %s", boardengine.ErrRaddleRecheck, "slug-taken")
			}, wantReason: "slug-taken", wantFiled: 1},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newRaddleFileFixture(t, raddleFileProposal(raddleparser.ShapeTasks), loomshed.RaddleModeInteractive, tt.approved)
				f.write(filepath.Join(f.deps.StageDir, "hash"), "stale")
				f.write(f.deps.PartRecordPath("part-a"), "stale")
				tt.setup(f)

				outcome, pointer, err := f.call()
				if err != nil || outcome != shedengine.Stuck || pointer.Halt {
					t.Fatalf("Call() = %q, %+v, %v; want Stuck without Halt", outcome, pointer, err)
				}
				if !strings.Contains(pointer.Reason, tt.wantReason) {
					t.Errorf("Reason = %q; want it to name %q", pointer.Reason, tt.wantReason)
				}
				got := f.state()
				if got.Returns != 1 || !strings.Contains(got.ReturnReason, tt.wantReason) || got.Approval != nil {
					t.Errorf("state = %+v; want one return with the reason recorded and the approval cleared", got)
				}
				for _, dir := range []string{f.deps.StageDir, f.deps.RecordsDir} {
					if _, err := os.Stat(dir); !os.IsNotExist(err) {
						t.Errorf("%s stat = %v; want it deleted", dir, err)
					}
				}
				if f.count("file") != tt.wantFiled {
					t.Errorf("File calls = %d; want %d", f.count("file"), tt.wantFiled)
				}
			})
		}
	})

	t.Run("AThirdFailureHaltsForTheParentNamingTheGoto", func(t *testing.T) {
		t.Parallel()
		f := newRaddleFileFixture(t, raddleFileProposal(raddleparser.ShapeTasks), loomshed.RaddleModeAuto, false)
		f.recheck.Failures = moved
		f.updateState(func(stored *loomshed.RaddleState) { stored.Returns = 2 })

		outcome, pointer, err := f.call()
		if err != nil || outcome != shedengine.Stuck || !pointer.Halt {
			t.Fatalf("Call() = %q, %+v, %v; want Stuck with Halt", outcome, pointer, err)
		}
		for _, want := range []string{raddleparser.CheckRevisedMoved, "lym loom goto --to Raddle-Write", "used its 2 returns"} {
			if !strings.Contains(pointer.Reason, want) {
				t.Errorf("Reason = %q; want it to contain %q", pointer.Reason, want)
			}
		}
		if got := f.state(); got.Returns != 2 {
			t.Errorf("returns = %d; want 2, not reset", got.Returns)
		}
	})

	t.Run("AFileErrorOtherThanTheRecheckIsReturned", func(t *testing.T) {
		t.Parallel()
		f := newRaddleFileFixture(t, raddleFileProposal(raddleparser.ShapeTasks), loomshed.RaddleModeAuto, false)
		f.failing["file"] = errors.New("board write failed")

		outcome, _, err := f.call()
		if err == nil || !strings.Contains(err.Error(), "board write failed") || outcome != "" {
			t.Errorf("Call() = %q, %v; want the board's error", outcome, err)
		}
		if got := f.state(); got.Returns != 0 {
			t.Errorf("returns = %d; want 0, since the failure is no return", got.Returns)
		}
	})

	t.Run("AFailureAfterTheFilingHaltsAndTheRetryFinishesWithoutTheChecks", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			seam       string
			wantStep   string
			wantNotify int
		}{
			{"create_beam", "make the project a beam", 1},
			{"release", "release the filed parts", 1},
			{"notify", "notify the parent", 2},
			{"end_entry", "end the run's board entry", 1},
		} {
			t.Run(tt.seam, func(t *testing.T) {
				t.Parallel()
				f := newRaddleFileFixture(t, raddleFileProposal(raddleparser.ShapeBeam), loomshed.RaddleModeAuto, false)
				f.failing[tt.seam] = errors.New("seam down")

				outcome, pointer, err := f.call()
				if err != nil || outcome != shedengine.Stuck || !pointer.Halt {
					t.Fatalf("Call() = %q, %+v, %v; want Stuck with Halt", outcome, pointer, err)
				}
				for _, want := range []string{tt.wantStep, "seam down", "lym loom resume"} {
					if !strings.Contains(pointer.Reason, want) {
						t.Errorf("Reason = %q; want it to contain %q", pointer.Reason, want)
					}
				}

				// The filing stands, so the retry judges nothing of the proposal or the board again, which would now fail.
				delete(f.failing, tt.seam)
				f.findings = []raddleparser.Finding{{Check: raddleparser.CheckSlugTaken, Detail: "filed parts now hold the slugs"}}
				f.recheck.Failures = moved
				outcome, _, err = f.call()
				requireDone(t, outcome, err)
				if f.count("file") != 1 || f.count("check") != 1 || f.count("recheck") != 1 || f.count("commit") != 1 {
					t.Errorf("calls = %v; want one check, re-check, commit and filing across both calls", f.calls)
				}
				if f.count("notify") != tt.wantNotify || f.count("end_entry") != 1+btoi(tt.seam == "end_entry") {
					t.Errorf("calls = %v; want %d notify calls and the entry ended once the seam works", f.calls, tt.wantNotify)
				}
			})
		}
	})

	t.Run("AHalfDoneFilingIsFinishedWithoutTheChecks", func(t *testing.T) {
		t.Parallel()
		f := newRaddleFileFixture(t, raddleFileProposal(raddleparser.ShapeTasks), loomshed.RaddleModeAuto, false)
		f.alreadyFiled = true
		// The stage a filing that crashed after its board write leaves: every staged file, the proposal's hash last.
		f.write(filepath.Join(f.deps.StageDir, "proposal.yaml"), string(marshalRaddleProposal(t, f.proposal)))
		f.write(filepath.Join(f.deps.StageDir, "cold-proposal.yaml"), "cold")
		f.write(filepath.Join(f.deps.StageDir, "decision-record.md"), raddleFileRecord)
		f.write(filepath.Join(f.deps.StageDir, "hash"), raddleparser.Hash(marshalRaddleProposal(t, f.proposal)))
		f.recheck.Landed = []string{"gone"}
		// The filed entries already hold the slugs, so a check or a re-check run over them would send the proposal back.
		f.findings = []raddleparser.Finding{{Check: raddleparser.CheckSlugTaken, Detail: "filed parts now hold the slugs"}}
		f.recheck.Failures = moved

		outcome, _, err := f.call()
		requireDone(t, outcome, err)

		if f.count("check") != 0 || f.count("recheck") != 1 || f.count("commit") != 1 || f.count("file") != 1 {
			t.Errorf("calls = %v; want no check, one re-check read for the landed entries, and the commit and the filing once so the store is written", f.calls)
		}
		if len(f.notices) != 1 || !strings.Contains(f.notices[0], "gone") {
			t.Errorf("notices = %q; want one naming the landed entry gone", f.notices)
		}
		if f.askedSketch != f.proposal.Sketch || !slices.Equal(f.askedSlugs, []string{"part-a", "part-b"}) {
			t.Errorf("AlreadyFiled(%q, %v); want the proposal's Sketch and its part slugs", f.askedSketch, f.askedSlugs)
		}
		if got := f.state(); got.Returns != 0 {
			t.Errorf("returns = %d; want 0, since the half-done filing is no return", got.Returns)
		}
	})

	t.Run("AHalfDoneFilingOfAnEditedProposalIsCheckedAgain", func(t *testing.T) {
		t.Parallel()
		f := newRaddleFileFixture(t, raddleFileProposal(raddleparser.ShapeTasks), loomshed.RaddleModeAuto, false)
		f.alreadyFiled = true
		f.write(filepath.Join(f.deps.StageDir, "hash"), "hash of the proposal that was filed")
		f.findings = []raddleparser.Finding{{Check: raddleparser.CheckSlugTaken, Detail: "filed parts now hold the slugs"}}

		outcome, pointer, err := f.call()

		if err != nil || outcome != shedengine.Stuck || pointer.Halt || !strings.Contains(pointer.Reason, raddleparser.CheckSlugTaken) {
			t.Fatalf("Call() = %q, %+v, %v; want Stuck without Halt naming the failed check", outcome, pointer, err)
		}
		if f.count("file") != 0 || f.askedSlugs != nil {
			t.Errorf("calls = %v, AlreadyFiled slugs = %v; want no filing and no question to the board, since the stage holds another proposal", f.calls, f.askedSlugs)
		}
	})

	t.Run("ACrashBeforeTheFilingRewritesTheOutputsOfAnEarlierProposal", func(t *testing.T) {
		t.Parallel()
		f := newRaddleFileFixture(t, raddleFileProposal(raddleparser.ShapeTasks), loomshed.RaddleModeInteractive, true)
		f.failing["commit"] = errors.New("git commit failed")
		if _, _, err := f.call(); err == nil {
			t.Fatal("Call() = nil error; want the commit fault")
		}

		edited := f.proposal
		edited.Cost.Line = "revised after the crash"
		f.writeProposal(edited)
		newData := marshalRaddleProposal(t, edited)
		f.updateState(func(stored *loomshed.RaddleState) {
			stored.Approval = &loomshed.RaddleApprovalRecord{Hash: raddleparser.Hash(newData), Session: 2}
		})
		delete(f.failing, "commit")

		outcome, _, err := f.call()
		requireDone(t, outcome, err)
		if got := f.read(filepath.Join(f.deps.StageDir, "proposal.yaml")); got != string(newData) {
			t.Errorf("staged proposal = %q; want the revised proposal", got)
		}
		if got, want := f.read(f.deps.PartRecordPath("part-a")+".hash"), raddleparser.Hash(newData); got != want {
			t.Errorf("part record hash = %q; want %q", got, want)
		}
		if got := string(f.filings[0].Proposal); got != string(newData) {
			t.Errorf("filed proposal = %q; want the revised proposal", got)
		}
	})

	t.Run("AReturnedProposalIsNotFiledUntilApprovedAgain", func(t *testing.T) {
		t.Parallel()
		f := newRaddleFileFixture(t, raddleFileProposal(raddleparser.ShapeTasks), loomshed.RaddleModeInteractive, true)
		f.recheck.Failures = moved
		if outcome, _, err := f.call(); err != nil || outcome != shedengine.Stuck {
			t.Fatalf("first Call() = %q, %v; want Stuck", outcome, err)
		}

		f.recheck.Failures = nil
		outcome, pointer, err := f.call()
		if err != nil || outcome != shedengine.Stuck || !strings.Contains(pointer.Reason, "no recorded approval") || len(f.filings) != 0 {
			t.Fatalf("second Call() = %q, %+v, %v with %d filings; want Stuck naming the missing approval and nothing filed", outcome, pointer, err, len(f.filings))
		}

		f.updateState(func(stored *loomshed.RaddleState) {
			stored.Approval = &loomshed.RaddleApprovalRecord{Hash: raddleparser.Hash(marshalRaddleProposal(t, f.proposal)), Session: 2}
		})
		outcome, _, err = f.call()
		requireDone(t, outcome, err)
	})

	t.Run("LandedEntriesAreNamedInTheNoticeAndFailNothing", func(t *testing.T) {
		t.Parallel()
		f := newRaddleFileFixture(t, raddleFileProposal(raddleparser.ShapeTasks), loomshed.RaddleModeAuto, false)
		f.recheck.Landed = []string{"gone"}

		outcome, _, err := f.call()
		requireDone(t, outcome, err)
		if len(f.notices) != 1 || !strings.Contains(f.notices[0], "gone") {
			t.Errorf("notices = %q; want one naming the landed entry gone", f.notices)
		}
	})
}

// btoi returns 1 for true and 0 for false.
func btoi(value bool) int {
	if value {
		return 1
	}
	return 0
}
