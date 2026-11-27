// darnwrite_test.go exercises the Darn-Write row's producer at Tier 1:
// a fake session and in-memory seams recording the order of their calls.

package loomshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/permkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// darnStopReport is the stop report the fixture's description file holds.
const darnStopReport = "The entry is outdated: landed work removed the module it names."

// scratchDirToken stands in a case's expected prior work for the fixture's scratch directory, a per-test temporary path.
const scratchDirToken = "<scratch>"

// darnFixture is one Darn-Write producer setup.
// It runs over a temporary directory holding a record, a support log and a description.
// Its seams log their calls, and its fake session reports a fixed verdict.
type darnFixture struct {
	deps     DarnDeps
	events   []string
	told     []DarnTold
	outcome  shedengine.Outcome
	pointer  shedengine.OutputPointer
	sessErr  error
	findings []discussionparser.Finding
	checkErr error
	// committed is CommittedRecord's answer.
	committed bool
	// writeMarker makes the fake session create the stop marker.
	writeMarker bool
	// writeConditionMarker makes the fake session create the condition stop marker.
	writeConditionMarker bool
	// emptyReport makes the fake session leave the description file empty.
	emptyReport bool
}

func newDarnFixture(t *testing.T) *darnFixture {
	t.Helper()
	dir := t.TempDir()
	description := filepath.Join(dir, "description.md")
	for path, text := range map[string]string{
		filepath.Join(dir, "decision-record.md"): "# record\n",
		filepath.Join(dir, "support-log.md"):     "# log\n",
		description:                              darnStopReport + "\n",
	} {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}
	}
	f := &darnFixture{outcome: shedengine.Done, pointer: shedengine.OutputPointer{Path: description}}
	f.deps = DarnDeps{
		RecordPath:     filepath.Join(dir, "decision-record.md"),
		SupportLogPath: filepath.Join(dir, "support-log.md"),
		StopMarkerPath: filepath.Join(dir, "stop-marker"),

		ConditionStopMarkerPath: filepath.Join(dir, "condition-stop-marker"),
		CommittedRecord:         func() (bool, error) { return f.committed, nil },
		Commit:                  func() error { f.events = append(f.events, "commit"); return nil },
		LatestOutcome:           func() (DarnOutcome, bool, error) { return DarnOutcome{}, false, nil },
		BranchCommits:           func() ([]string, error) { return nil, nil },
		ScratchDir:              filepath.Join(dir, "scratch"),
		CheckRecord: func() ([]discussionparser.Finding, error) {
			f.events = append(f.events, "check")
			return f.findings, f.checkErr
		},
	}
	return f
}

func (f *darnFixture) session(told DarnTold) shedengine.ShedProducer {
	f.told = append(f.told, told)
	return darnSession{f: f}
}

type darnSession struct{ f *darnFixture }

func (s darnSession) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	s.f.events = append(s.f.events, "session")
	if s.f.writeMarker {
		if err := os.WriteFile(s.f.deps.StopMarkerPath, nil, 0o644); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	if s.f.writeConditionMarker {
		if err := os.WriteFile(s.f.deps.ConditionStopMarkerPath, nil, 0o644); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	if s.f.emptyReport {
		if err := os.WriteFile(s.f.pointer.Path, nil, 0o644); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	return s.f.outcome, s.f.pointer, s.f.sessErr
}

func TestDarnWrite_Call(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	cases := []struct {
		name       string
		mutate     func(*darnFixture)
		session    bool
		wantResult shedengine.Outcome
		wantErr    string
		wantReason string
		// wantReasonHas lists further words the reason must hold.
		wantReasonHas []string
		// wantReasonLacks is a text the reason must not hold.
		wantReasonLacks string
		wantEvents      string
		// wantTold is the told text, with scratchDirToken standing for the fixture's scratch directory.
		wantTold DarnTold
		// scratchFiles are written under a directory the fixture's scratch directory links to, as the `.scratch` link does.
		scratchFiles []string
		// unreadableScratchDir names a directory of scratchFiles whose permission bits are cleared before the call.
		unreadableScratchDir string
		// danglingScratch makes the fixture's scratch directory a link to a path that does not exist.
		danglingScratch bool
		// staleMarker leaves a stop marker from an earlier spawn in place before the call.
		staleMarker bool
		// wantNoMarker asserts the stop marker is absent after the call.
		wantNoMarker bool
		// staleConditionMarker leaves a condition stop marker from an earlier spawn in place before the call.
		staleConditionMarker bool
		// wantNoConditionMarker asserts the condition stop marker is absent after the call.
		wantNoConditionMarker bool
	}{
		{
			name:       "first spawn tells nothing, commits and checks the record",
			session:    true,
			wantResult: shedengine.Done,
			wantEvents: "session,commit,check",
		},
		{
			name: "prior work names the earlier outcome and its kept findings",
			mutate: func(f *darnFixture) {
				f.deps.LatestOutcome = func() (DarnOutcome, bool, error) {
					return DarnOutcome{Outcome: shedengine.Stuck, FindingsPath: "/run/Darn-Write.findings.md"}, true, nil
				}
			},
			session:    true,
			wantResult: shedengine.Done,
			wantEvents: "session,commit,check",
			wantTold: DarnTold{PriorWork: "An earlier spawn of this task already began the change, and its work is committed on the task branch.\n" +
				"Read that work first and continue it instead of starting over.\n" +
				"That spawn ended stuck.\n" +
				"Its last gate findings are kept at /run/Darn-Write.findings.md: read them and fix what they name.\n" +
				decisionsWinLine},
		},
		{
			name: "an interrupted spawn that recorded no outcome is named by its commits and scratch files",
			mutate: func(f *darnFixture) {
				f.deps.BranchCommits = func() ([]string, error) {
					return []string{"b2c3d4e solver: count branches", "a1b2c3d solver: add the branch table"}, nil
				}
			},
			scratchFiles: []string{"runs/second.txt", "measure.md"},
			session:      true,
			wantResult:   shedengine.Done,
			wantEvents:   "session,commit,check",
			wantTold: DarnTold{PriorWork: "An earlier spawn of this task was interrupted before it recorded an outcome, and its work may be present on the task branch and in the scratch directory.\n" +
				"Read that work first and continue it instead of starting over.\n" +
				"The task branch holds these commits beyond its base, newest first:\n" +
				"- b2c3d4e solver: count branches\n" +
				"- a1b2c3d solver: add the branch table\n" +
				"These entries survive under the scratch directory <scratch>:\n" +
				"- measure.md\n" +
				"- runs/\n" +
				decisionsWinLine},
		},
		{
			name:                 "an unreadable scratch subdirectory is listed by name and not read",
			scratchFiles:         []string{"locked/hidden.txt", "measure.md"},
			unreadableScratchDir: "locked",
			session:              true,
			wantResult:           shedengine.Done,
			wantEvents:           "session,commit,check",
			wantTold: DarnTold{PriorWork: "No earlier spawn of this task recorded an outcome, and the task branch holds no commits beyond its base.\n" +
				"An interrupted spawn or another agent of this task may have left the scratch entries below.\n" +
				"Read those that bear on the change before you redo or overwrite them.\n" +
				"These entries survive under the scratch directory <scratch>:\n" +
				"- locked/\n" +
				"- measure.md\n" +
				decisionsWinLine},
		},
		{
			name:            "a dangling scratch link tells nothing",
			danglingScratch: true,
			session:         true,
			wantResult:      shedengine.Done,
			wantEvents:      "session,commit,check",
		},
		{
			name:    "a scratch path that cannot be listed is an error",
			mutate:  func(f *darnFixture) { f.deps.ScratchDir = f.deps.RecordPath },
			wantErr: "list the scratch directory",
		},
		{
			name: "branch commits read failure is an error",
			mutate: func(f *darnFixture) {
				f.deps.BranchCommits = func() ([]string, error) { return nil, errors.New("merge base unreadable") }
			},
			wantErr: "merge base unreadable",
		},
		{
			name: "gate-failed Stuck commits, keeps the record and appends the way forward",
			mutate: func(f *darnFixture) {
				f.outcome = shedengine.Stuck
				f.pointer = shedengine.OutputPointer{Path: "/work/description.md", Reason: "verify gate failed: see /work/verify-1.log"}
			},
			session:         true,
			wantResult:      shedengine.Stuck,
			wantReason:      "verify gate failed: see /work/verify-1.log; " + darnResumeWayForward,
			wantReasonHas:   []string{"lym loom resume"},
			wantReasonLacks: "Parent-Align",
			wantEvents:      "session,commit",
		},
		{
			name: "a guard finding after the session halts Stuck naming the findings and the way forward, with one spawn",
			mutate: func(f *darnFixture) {
				f.findings = []discussionparser.Finding{{Check: "discussion-section-missing", Path: "/work/decision-record.md", Detail: "no ## Goal section"}}
			},
			session:         true,
			wantResult:      shedengine.Stuck,
			wantReason:      "no ## Goal section",
			wantReasonHas:   []string{"git -C _lym checkout HEAD -- discussion/<file>", "lym fabric commit", "lym loom resume"},
			wantReasonLacks: darnResumeWayForward,
			wantEvents:      "session,commit,check",
		},
		{
			name: "a gate-failed Stuck is not guarded",
			mutate: func(f *darnFixture) {
				f.outcome = shedengine.Stuck
				f.findings = []discussionparser.Finding{{Check: "discussion-section-missing"}}
			},
			session:       true,
			wantResult:    shedengine.Stuck,
			wantReasonHas: []string{"lym loom resume"},
			wantEvents:    "session,commit",
		},
		{
			name:       "guard check failure is an error",
			mutate:     func(f *darnFixture) { f.checkErr = errors.New("history unavailable") },
			session:    true,
			wantErr:    "history unavailable",
			wantEvents: "session,commit,check",
		},
		{
			name: "Stuck without a pointer is neither committed nor amended",
			mutate: func(f *darnFixture) {
				f.outcome = shedengine.Stuck
				f.pointer = shedengine.OutputPointer{Reason: "no output"}
			},
			session:    true,
			wantResult: shedengine.Stuck,
			wantReason: "no output",
			wantEvents: "session",
		},
		{
			name: "latest outcome read failure is an error",
			mutate: func(f *darnFixture) {
				f.deps.LatestOutcome = func() (DarnOutcome, bool, error) { return DarnOutcome{}, false, errors.New("status gone") }
			},
			wantErr: "status gone",
		},
		{
			name: "commit failure is an error",
			mutate: func(f *darnFixture) {
				f.deps.Commit = func() error { return errors.New("git fault") }
			},
			session:    true,
			wantErr:    "git fault",
			wantEvents: "session",
		},
		{
			name:       "session error is returned without a commit",
			mutate:     func(f *darnFixture) { f.sessErr = errors.New("shuttle died") },
			session:    true,
			wantErr:    "shuttle died",
			wantEvents: "session",
		},
		{
			name:            "a missing record halts before any session naming the goto and the resume",
			mutate:          func(f *darnFixture) { f.deps.RecordPath += ".absent" },
			wantResult:      shedengine.Stuck,
			wantReasonHas:   []string{"decision record", "lym loom goto --to Record-Write", "lym loom resume"},
			wantReasonLacks: "git -C _lym checkout",
		},
		{
			name: "a missing record the committed history holds names the restore from HEAD",
			mutate: func(f *darnFixture) {
				f.deps.RecordPath += ".absent"
				f.committed = true
			},
			wantResult:      shedengine.Stuck,
			wantReasonHas:   []string{"git -C _lym checkout HEAD -- discussion/decision-record.md", "lym loom resume"},
			wantReasonLacks: "lym loom goto",
		},
		{
			name:          "a missing support log beside a present record names the goto and the resume",
			mutate:        func(f *darnFixture) { f.deps.SupportLogPath += ".absent" },
			wantResult:    shedengine.Stuck,
			wantReasonHas: []string{"support log", "lym loom goto --to Record-Write", "lym loom resume"},
		},
		{
			name: "committed record read failure is an error",
			mutate: func(f *darnFixture) {
				f.deps.RecordPath += ".absent"
				f.deps.CommittedRecord = func() (bool, error) { return false, errors.New("git down") }
			},
			wantErr: "git down",
		},
		{
			name:          "a stop marker after a Done session halts with the stop report and skips the record guard",
			mutate:        func(f *darnFixture) { f.writeMarker = true },
			session:       true,
			wantResult:    shedengine.Stuck,
			wantReason:    darnStopReport,
			wantReasonHas: []string{"lym loom decision add", "lym loom resume", "lym board upsert", "kind note"},
			wantEvents:    "session,commit",
		},
		{
			name: "a stop marker after a gate-failed Stuck halts with the stop report in place of the resume way forward",
			mutate: func(f *darnFixture) {
				f.writeMarker = true
				f.outcome = shedengine.Stuck
				f.pointer.Reason = "description gate failed"
			},
			session:         true,
			wantResult:      shedengine.Stuck,
			wantReason:      darnStopReport,
			wantReasonHas:   []string{"lym loom decision add", "lym loom resume"},
			wantReasonLacks: darnResumeWayForward,
			wantEvents:      "session,commit",
		},
		{
			name: "a stop marker on a respawn whose earlier spawn committed work halts the same way",
			mutate: func(f *darnFixture) {
				f.writeMarker = true
				f.deps.LatestOutcome = func() (DarnOutcome, bool, error) { return DarnOutcome{Outcome: shedengine.Done}, true, nil }
			},
			session:       true,
			wantResult:    shedengine.Stuck,
			wantReason:    darnStopReport,
			wantReasonHas: []string{"lym loom decision add", "lym loom resume"},
			wantEvents:    "session,commit",
			wantTold: DarnTold{PriorWork: "An earlier spawn of this task already began the change, and its work is committed on the task branch.\n" +
				"Read that work first and continue it instead of starting over.\n" +
				"That spawn ended done.\n" +
				decisionsWinLine},
		},
		{
			name: "a stop marker with no report file halts naming the missing report",
			mutate: func(f *darnFixture) {
				f.writeMarker = true
				f.pointer.Path += ".absent"
			},
			session:       true,
			wantResult:    shedengine.Stuck,
			wantReason:    darnStopReportMissing,
			wantReasonHas: []string{"lym loom decision add"},
			wantEvents:    "session,commit",
		},
		{
			name: "a stop marker with the condition stop marker halts with the stop report alone",
			mutate: func(f *darnFixture) {
				f.writeMarker = true
				f.writeConditionMarker = true
			},
			session:         true,
			wantResult:      shedengine.Stuck,
			wantReason:      darnStopReport,
			wantReasonLacks: "lym loom resume",
			wantEvents:      "session,commit",
		},
		{
			name: "a condition stop with an empty report halts with a fixed reason naming the stop marker",
			mutate: func(f *darnFixture) {
				f.writeMarker = true
				f.writeConditionMarker = true
				f.emptyReport = true
			},
			session:         true,
			wantResult:      shedengine.Stuck,
			wantReasonHas:   []string{"stopped on the task body's condition and wrote no report", "stop-marker"},
			wantReasonLacks: "lym loom resume",
			wantEvents:      "session,commit",
		},
		{
			name:                  "a stale condition stop marker is removed before the session and does not halt",
			staleConditionMarker:  true,
			session:               true,
			wantResult:            shedengine.Done,
			wantEvents:            "session,commit,check",
			wantNoConditionMarker: true,
		},
		{
			name:       "a condition stop marker alone does not halt",
			mutate:     func(f *darnFixture) { f.writeConditionMarker = true },
			session:    true,
			wantResult: shedengine.Done,
			wantEvents: "session,commit,check",
		},
		{
			name:         "a stale stop marker is removed before the session and does not halt",
			staleMarker:  true,
			session:      true,
			wantResult:   shedengine.Done,
			wantEvents:   "session,commit,check",
			wantNoMarker: true,
		},
		{
			name:    "unwired Commit seam is named",
			mutate:  func(f *darnFixture) { f.deps.Commit = nil },
			wantErr: "no Commit seam wired",
		},
		{
			name:    "unwired CommittedRecord seam is named",
			mutate:  func(f *darnFixture) { f.deps.CommittedRecord = nil },
			wantErr: "no CommittedRecord seam wired",
		},
		{
			name:    "unwired RecordPath is named",
			mutate:  func(f *darnFixture) { f.deps.RecordPath = "" },
			wantErr: "no RecordPath seam wired",
		},
		{
			name:    "unwired SupportLogPath is named",
			mutate:  func(f *darnFixture) { f.deps.SupportLogPath = "" },
			wantErr: "no SupportLogPath seam wired",
		},
		{
			name:    "unwired StopMarkerPath is named",
			mutate:  func(f *darnFixture) { f.deps.StopMarkerPath = "" },
			wantErr: "no StopMarkerPath seam wired",
		},
		{
			name:    "unwired ConditionStopMarkerPath is named",
			mutate:  func(f *darnFixture) { f.deps.ConditionStopMarkerPath = "" },
			wantErr: "no ConditionStopMarkerPath seam wired",
		},
		{
			name:    "unwired LatestOutcome seam is named",
			mutate:  func(f *darnFixture) { f.deps.LatestOutcome = nil },
			wantErr: "no LatestOutcome seam wired",
		},
		{
			name:    "unwired CheckRecord seam is named",
			mutate:  func(f *darnFixture) { f.deps.CheckRecord = nil },
			wantErr: "no CheckRecord seam wired",
		},
		{
			name:    "unwired BranchCommits seam is named",
			mutate:  func(f *darnFixture) { f.deps.BranchCommits = nil },
			wantErr: "no BranchCommits seam wired",
		},
		{
			name:    "unwired ScratchDir is named",
			mutate:  func(f *darnFixture) { f.deps.ScratchDir = "" },
			wantErr: "no ScratchDir seam wired",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newDarnFixture(t)
			if tc.mutate != nil {
				tc.mutate(f)
			}
			if tc.staleMarker {
				if err := os.WriteFile(f.deps.StopMarkerPath, nil, 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", f.deps.StopMarkerPath, err)
				}
			}
			if tc.staleConditionMarker {
				if err := os.WriteFile(f.deps.ConditionStopMarkerPath, nil, 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", f.deps.ConditionStopMarkerPath, err)
				}
			}
			if len(tc.scratchFiles) > 0 {
				writeLinkedScratch(t, f.deps.ScratchDir, tc.scratchFiles)
			}
			if tc.unreadableScratchDir != "" {
				permkit.Chmod(t, filepath.Join(f.deps.ScratchDir, tc.unreadableScratchDir), 0)
			}
			if tc.danglingScratch {
				if err := os.Symlink(f.deps.ScratchDir+"-absent", f.deps.ScratchDir); err != nil {
					t.Fatalf("Symlink(%q) = %v; want nil", f.deps.ScratchDir, err)
				}
			}
			outcome, pointer, err := NewDarnWrite(NameDarnWrite, f.session, f.deps).Call(context.Background())

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Call() error = %v; want it to contain %q", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			} else if outcome != tc.wantResult {
				t.Errorf("Call() outcome = %q; want %q", outcome, tc.wantResult)
			}
			if tc.wantReason != "" && !strings.Contains(pointer.Reason, tc.wantReason) {
				t.Errorf("Call() reason = %q; want it to contain %q", pointer.Reason, tc.wantReason)
			}
			for _, word := range tc.wantReasonHas {
				if !strings.Contains(pointer.Reason, word) {
					t.Errorf("Call() reason = %q; want it to contain %q", pointer.Reason, word)
				}
			}
			if tc.wantReasonLacks != "" && strings.Contains(pointer.Reason, tc.wantReasonLacks) {
				t.Errorf("Call() reason = %q; want it not to contain %q", pointer.Reason, tc.wantReasonLacks)
			}
			if pointer.Reroute {
				t.Errorf("Call() Reroute = true; want false")
			}
			if got := strings.Join(f.events, ","); got != tc.wantEvents {
				t.Errorf("events = %q; want %q", got, tc.wantEvents)
			}
			if _, err := os.Stat(f.deps.StopMarkerPath); tc.wantNoMarker && !errors.Is(err, os.ErrNotExist) {
				t.Errorf("Stat(stop marker) = %v; want it removed", err)
			}
			if _, err := os.Stat(f.deps.ConditionStopMarkerPath); tc.wantNoConditionMarker && !errors.Is(err, os.ErrNotExist) {
				t.Errorf("Stat(condition stop marker) = %v; want it removed", err)
			}
			if !tc.session {
				if len(f.told) != 0 {
					t.Errorf("session built %d times; want none", len(f.told))
				}
				return
			}
			wantTold := DarnTold{PriorWork: strings.ReplaceAll(tc.wantTold.PriorWork, scratchDirToken, f.deps.ScratchDir)}
			if len(f.told) != 1 || f.told[0] != wantTold {
				t.Errorf("told = %+v; want one %+v", f.told, wantTold)
			}
		})
	}

	t.Run("a resume whose session writes the marker again halts again", func(t *testing.T) {
		t.Parallel()
		f := newDarnFixture(t)
		f.writeMarker = true
		p := NewDarnWrite(NameDarnWrite, f.session, f.deps)
		for attempt := 1; attempt <= 2; attempt++ {
			outcome, pointer, err := p.Call(context.Background())
			if err != nil || outcome != shedengine.Stuck || !strings.Contains(pointer.Reason, darnStopReport) {
				t.Fatalf("Call() attempt %d = (%q, %q, %v); want Stuck with the stop report", attempt, outcome, pointer.Reason, err)
			}
		}
	})
}

// writeLinkedScratch writes each of files, slash-separated, under a new directory beside link.
// It then makes link a link to that directory.
func writeLinkedScratch(t *testing.T, link string, files []string) {
	t.Helper()
	target := link + "-target"
	for _, file := range files {
		path := filepath.Join(target, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("partial\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink(%q, %q) = %v; want nil", target, link, err)
	}
}

func TestDarnWrite_CancelledContextRefusesBeforeAnySeam(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	f := newDarnFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := NewDarnWrite(NameDarnWrite, f.session, f.deps).Call(ctx); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Call() error = %v; want context.Canceled", err)
	}
	if len(f.events) != 0 {
		t.Errorf("events = %v; want none", f.events)
	}
}

func TestRenderPriorWork(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	manyFiles := make([]string, priorWorkListLimit+2)
	for i := range manyFiles {
		manyFiles[i] = "f.txt"
	}
	cases := []struct {
		name   string
		latest DarnOutcome
		found  bool
		work   branchWork
		want   string
	}{
		{"nothing to name", DarnOutcome{}, false, branchWork{}, ""},
		{"findings of an entry the read did not find are ignored", DarnOutcome{FindingsPath: "/run/stale.md"}, false, branchWork{}, ""},
		{"an earlier entry with no kept findings", DarnOutcome{Outcome: shedengine.Done}, true, branchWork{}, "An earlier spawn of this task already began the change, and its work is committed on the task branch.\nRead that work first and continue it instead of starting over.\nThat spawn ended done.\n" + decisionsWinLine},
		{
			"an earlier entry lists the commits and scratch files after its findings, as maybe a later spawn's",
			DarnOutcome{Outcome: shedengine.Stuck, FindingsPath: "/run/f.md"},
			true,
			branchWork{commits: []string{"a1b2c3d solver: add the branch table"}, scratchDir: "/w/.scratch", scratchEntries: []string{"measure.md"}},
			"An earlier spawn of this task already began the change, and its work is committed on the task branch.\n" +
				"Read that work first and continue it instead of starting over.\n" +
				"That spawn ended stuck.\n" +
				"Its last gate findings are kept at /run/f.md: read them and fix what they name.\n" +
				"A later spawn may have been interrupted before it recorded an outcome, so what is listed below can go beyond the spawn that ended so.\n" +
				"The task branch holds these commits beyond its base, newest first:\n" +
				"- a1b2c3d solver: add the branch table\n" +
				"These entries survive under the scratch directory /w/.scratch:\n" +
				"- measure.md\n" +
				decisionsWinLine,
		},
		{
			"scratch files alone claim no interrupted spawn and a long list is cut with a count",
			DarnOutcome{},
			false,
			branchWork{scratchDir: "/w/.scratch", scratchEntries: manyFiles},
			"No earlier spawn of this task recorded an outcome, and the task branch holds no commits beyond its base.\n" +
				"An interrupted spawn or another agent of this task may have left the scratch entries below.\n" +
				"Read those that bear on the change before you redo or overwrite them.\n" +
				"These entries survive under the scratch directory /w/.scratch:\n" +
				strings.Repeat("- f.txt\n", priorWorkListLimit) +
				"- and 2 more\n" +
				decisionsWinLine,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := renderPriorWork(tc.latest, tc.found, tc.work); got != tc.want {
				t.Errorf("renderPriorWork() = %q; want %q", got, tc.want)
			}
		})
	}
}
