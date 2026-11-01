//go:build tier0

// fixloop_test.go pins the fix loop of a failed adopt check over fake seams and a temp subject directory: the cases of failedEnd, the fix task's slug and its body.

package adoptcli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fronttask"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// fixFixture is a failedEnd over a temp subject directory, with every seam faked and recording its calls.
type fixFixture struct {
	t    *testing.T
	deps fixDeps
	dir  string

	tasks       []boardengine.Task
	runRecords  map[string]shedengine.Status
	eligible    map[string]bool
	runSlugs    []string
	branchSlugs []string
	vocabulary  []string

	created    []map[string]any
	createdFor []string
	released   []string
	createErr  error
	releaseErr error
	cancelled  []string
	cancelErr  error
	removed    []string
	removeErr  error
}

// newFixFixture returns a fixture for the beam `alpha` on machine `boxA`, with a try limit of 3 and a vocabulary knowing `gate`.
func newFixFixture(t *testing.T) *fixFixture {
	t.Helper()
	root := t.TempDir()
	f := &fixFixture{t: t, dir: filepath.Join(root, "alpha"), runRecords: map[string]shedengine.Status{}, eligible: map[string]bool{}, vocabulary: []string{"bug", "gate"}}
	f.deps = fixDeps{
		Subject:  "alpha",
		Dir:      f.dir,
		Project:  "alpha",
		Machine:  "boxA",
		TryLimit: 3,
		Tasks:    func() ([]boardengine.Task, error) { return f.tasks, nil },
		RunRecord: func(slug string) (shedengine.Status, bool, error) {
			status, found := f.runRecords[slug]
			return status, found, nil
		},
		EscalationEligible: func(slug string) bool { return f.eligible[slug] },
		RunRecordSlugs:     func() ([]string, error) { return f.runSlugs, nil },
		TaskBranchSlugs:    func(string) ([]string, error) { return f.branchSlugs, nil },
		Known:              func(label string) bool { return slices.Contains(f.vocabulary, label) },
		FrontDeps: fronttask.Deps{
			Tasks: func() ([]boardengine.Task, error) { return f.tasks, nil },
			Create: func(fields map[string]any, onLand string) (boardengine.Task, error) {
				if f.createErr != nil {
					return boardengine.Task{}, f.createErr
				}
				f.created = append(f.created, fields)
				f.createdFor = append(f.createdFor, onLand)
				return boardengine.Task{}, nil
			},
			Release: func(slug string) error {
				f.released = append(f.released, slug)
				return f.releaseErr
			},
			RemoveEntry: func(slug string) error {
				if f.removeErr != nil {
					return f.removeErr
				}
				f.removed = append(f.removed, slug)
				f.tasks = slices.DeleteFunc(f.tasks, func(task boardengine.Task) bool { return task.Slug == slug })
				return nil
			},
			HasRun: func(slug string) (bool, error) {
				for _, task := range f.tasks {
					if task.Slug == slug {
						return boardengine.IsRunStatus(task.Status), nil
					}
				}
				return false, nil
			},
			Cancel: func(_ context.Context, slug string) error {
				f.cancelled = append(f.cancelled, slug)
				return f.cancelErr
			},
			RecordPath: filepath.Join(root, "front-records.json"),
		},
	}
	return f
}

// failedRecord returns the record of a check that failed at tier 0 with one failed test.
func failedRecord() CheckRecord {
	tier := 0
	return CheckRecord{
		Subject: "alpha", State: checkFailed, FailedTier: &tier,
		Tiers: []CheckTier{{Tier: 0, Outcome: checkFailed, Failed: 1, Failures: []gateslot.TestFailure{{Package: "example.com/a", Test: "TestA", FirstLine: "a_test.go:12: got 1; want 2"}}}},
	}
}

func (f *fixFixture) tries() int {
	f.t.Helper()
	count, err := readTries(f.dir)
	if err != nil {
		f.t.Fatalf("readTries: %v", err)
	}
	return count
}

func (f *fixFixture) setTries(count int) {
	f.t.Helper()
	if err := writeTries(f.dir, count); err != nil {
		f.t.Fatalf("writeTries: %v", err)
	}
}

// ownFix is the subject's fix task `adopt-fix-alpha-t1` with status.
func ownFix(status *string) boardengine.Task { return fixTask("adopt-fix-alpha-t1", status) }

func TestFailedEnd(t *testing.T) {
	t.Parallel()

	t.Run("below the limit with no fix task it files one released task and counts the try", func(t *testing.T) {
		t.Parallel()
		f := newFixFixture(t)
		if err := appendHolds(f.dir, []string{"t1"}); err != nil {
			t.Fatal(err)
		}

		lines := failedEnd(f.deps, failedRecord())
		if len(f.created) != 1 {
			t.Fatalf("created = %v; want one fix task (lines %q)", f.created, lines)
		}
		fields := f.created[0]
		for key, want := range map[string]any{"slug": "adopt-fix-alpha-t1", "title": "Fix the adopt check of alpha on boxA", "kind": boardengine.KindTask, "recipe": "darn", "profile": "light", "project": "alpha"} {
			if fields[key] != want {
				t.Errorf("fields[%q] = %v; want %v", key, fields[key], want)
			}
		}
		if labels, _ := fields["labels"].([]string); !slices.Equal(labels, []string{"bug", "gate"}) {
			t.Errorf("labels = %v; want bug and gate", fields["labels"])
		}
		if body, _ := fields["body"].(string); !strings.Contains(body, "Stop condition: the failure is a machine failure.") || !strings.Contains(body, "a_test.go:12: got 1; want 2") {
			t.Errorf("body = %q; want the failed test and the stop condition", body)
		}
		if f.createdFor[0] != fronttask.ActionAdoptCheck || !slices.Equal(f.released, []string{"adopt-fix-alpha-t1"}) {
			t.Errorf("on_land = %q, released = %q; want adopt-check and the slug released", f.createdFor, f.released)
		}
		if got := f.tries(); got != 1 {
			t.Errorf("tries = %d; want 1", got)
		}
		if len(lines) != 1 || !strings.Contains(lines[0], "adopt-fix-alpha-t1") || !strings.Contains(lines[0], "the only run that starts") {
			t.Errorf("lines = %q; want one line naming the filed slug and that it is the only run that starts", lines)
		}
	})

	t.Run("the prime's fix task is filed with no project", func(t *testing.T) {
		t.Parallel()
		f := newFixFixture(t)
		f.deps.Subject, f.deps.Project = "root", ""

		failedEnd(f.deps, failedRecord())
		if len(f.created) != 1 {
			t.Fatalf("created = %v; want one fix task", f.created)
		}
		if _, hasProject := f.created[0]["project"]; hasProject || f.created[0]["slug"] != "adopt-fix-root-t1" {
			t.Errorf("fields = %v; want slug adopt-fix-root-t1 and no project", f.created[0])
		}
	})

	t.Run("a vocabulary without gate files the task with the label bug alone", func(t *testing.T) {
		t.Parallel()
		f := newFixFixture(t)
		f.vocabulary = []string{"bug"}

		failedEnd(f.deps, failedRecord())
		if len(f.created) != 1 {
			t.Fatalf("created = %v; want one fix task", f.created)
		}
		if labels, _ := f.created[0]["labels"].([]string); !slices.Equal(labels, []string{"bug"}) {
			t.Errorf("labels = %v; want bug alone", f.created[0]["labels"])
		}
	})

	t.Run("at the limit it files none, removes none and says the tries ran out", func(t *testing.T) {
		t.Parallel()
		f := newFixFixture(t)
		f.setTries(3)

		lines := failedEnd(f.deps, failedRecord())
		if len(f.created) != 0 || len(f.removed) != 0 {
			t.Errorf("created = %v, removed = %q; want neither", f.created, f.removed)
		}
		if len(lines) != 1 || !mentions(lines, "ran out", "tier0", "example.com/a TestA", "suite.log", "lym adopt alpha") {
			t.Errorf("lines = %q; want the tries ran out, the failing tier and test, suite.log and lym adopt alpha", lines)
		}
	})

	t.Run("an open front task of another action files none and names its landing", func(t *testing.T) {
		t.Parallel()
		f := newFixFixture(t)
		other := fixTask("other-front", nil)
		other.OnLand = "something-else"
		f.tasks = []boardengine.Task{other}

		lines := failedEnd(f.deps, failedRecord())
		if len(f.created) != 0 || !mentions(lines, "other-front", "lym adopt alpha") {
			t.Errorf("created = %v, lines = %q; want none filed and a line naming other-front and lym adopt alpha", f.created, lines)
		}
	})

	t.Run("the own fix task open and healthy files none, removes none and names its state and hand steps", func(t *testing.T) {
		t.Parallel()
		running := boardengine.RunStatus("running", "Code-Write")
		tests := []struct {
			name      string
			status    *string
			record    *shedengine.Status
			eligible  bool
			wantState string
			wantHand  string
		}{
			{"ReleasedWithTheEmptyStatus", nil, nil, false, "released", ""},
			{"RunStatusWithNoReadableRunRecord", &running, nil, false, "starting", ""},
			{"Running", &running, &shedengine.Status{State: shedengine.StateRunning}, false, "running", ""},
			{"Paused", &running, &shedengine.Status{State: shedengine.StatePaused}, false, "paused", "`lym batten run adopt-fix-alpha-t1` from the prime"},
			{"AwaitingAnAdmittedEscalation", &running, &shedengine.Status{State: shedengine.StateAwaiting}, true, "awaiting", ""},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newFixFixture(t)
				f.tasks = []boardengine.Task{ownFix(tt.status)}
				if tt.record != nil {
					f.runRecords["adopt-fix-alpha-t1"] = *tt.record
				}
				f.eligible["adopt-fix-alpha-t1"] = tt.eligible

				lines := failedEnd(f.deps, failedRecord())
				if len(f.created) != 0 || len(f.removed) != 0 || len(f.cancelled) != 0 {
					t.Errorf("created = %v, removed = %q, cancelled = %q; want none", f.created, f.removed, f.cancelled)
				}
				if len(lines) != 1 || !mentions(lines, "adopt-fix-alpha-t1", tt.wantState, "its landing starts the next check", tt.wantHand, "`lym batten cancel adopt-fix-alpha-t1 --remove`, then `lym board remove adopt-fix-alpha-t1`") {
					t.Errorf("lines = %q; want the task, its state %q and the hand steps", lines, tt.wantState)
				}
			})
		}
	})

	t.Run("a draft or halted own fix task is removed and a new one filed under a higher slug", func(t *testing.T) {
		t.Parallel()
		running := boardengine.RunStatus("running", "Code-Write")
		draft := boardengine.StatusDraft
		tests := []struct {
			name       string
			status     *string
			record     *shedengine.Status
			wantCancel bool
		}{
			{"Draft", &draft, nil, false},
			{"Blocked", &running, &shedengine.Status{State: shedengine.StateBlocked}, true},
			{"Failed", &running, &shedengine.Status{State: shedengine.StateFailed}, true},
			{"AwaitingWithNoAdmittedEscalation", &running, &shedengine.Status{State: shedengine.StateAwaiting}, true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newFixFixture(t)
				f.tasks = []boardengine.Task{ownFix(tt.status)}
				if tt.record != nil {
					f.runRecords["adopt-fix-alpha-t1"] = *tt.record
				}
				f.setTries(1)

				lines := failedEnd(f.deps, failedRecord())
				if !slices.Equal(f.removed, []string{"adopt-fix-alpha-t1"}) || (len(f.cancelled) == 1) != tt.wantCancel {
					t.Errorf("removed = %q, cancelled = %q; want the task removed, cancelled: %v", f.removed, f.cancelled, tt.wantCancel)
				}
				if len(f.created) != 1 || f.created[0]["slug"] != "adopt-fix-alpha-t2" || f.tries() != 2 {
					t.Errorf("created = %v, tries = %d (lines %q); want adopt-fix-alpha-t2 filed and the count 2", f.created, f.tries(), lines)
				}
			})
		}
	})

	t.Run("a halted own fix task at the limit, and one beside another open front task, is neither removed nor joined by a filing", func(t *testing.T) {
		t.Parallel()
		running := boardengine.RunStatus("blocked", "Code-Write")
		atLimit := newFixFixture(t)
		atLimit.tasks = []boardengine.Task{ownFix(&running)}
		atLimit.runRecords["adopt-fix-alpha-t1"] = shedengine.Status{State: shedengine.StateBlocked}
		atLimit.setTries(3)
		beside := newFixFixture(t)
		other := fixTask("other-front", nil)
		other.OnLand = "something-else"
		beside.tasks = []boardengine.Task{ownFix(&running), other}
		beside.runRecords["adopt-fix-alpha-t1"] = shedengine.Status{State: shedengine.StateBlocked}

		limitLines := failedEnd(atLimit.deps, failedRecord())
		besideLines := failedEnd(beside.deps, failedRecord())
		for name, f := range map[string]*fixFixture{"AtTheLimit": atLimit, "BesideAnotherFront": beside} {
			if len(f.created) != 0 || len(f.removed) != 0 || len(f.cancelled) != 0 {
				t.Errorf("%s: created = %v, removed = %q, cancelled = %q; want none", name, f.created, f.removed, f.cancelled)
			}
		}
		if !mentions(limitLines, "adopt-fix-alpha-t1", "halted", "lym adopt alpha") || !mentions(besideLines, "other-front") {
			t.Errorf("limit lines = %q, beside lines = %q; want the halted task named at the limit and the other front task named beside it", limitLines, besideLines)
		}
	})

	t.Run("a refused filing names its error and the re-run and leaves the count, a release that fails after the count still counts", func(t *testing.T) {
		t.Parallel()
		refused := newFixFixture(t)
		refused.createErr = errors.New("label unknown")
		dying := newFixFixture(t)
		dying.releaseErr = errors.New("board locked")

		refusedLines := failedEnd(refused.deps, failedRecord())
		dyingLines := failedEnd(dying.deps, failedRecord())
		if !mentions(refusedLines, "label unknown", "lym adopt alpha") || refused.tries() != 0 {
			t.Errorf("refused lines = %q, tries = %d; want the error and the re-run, and the count left at 0", refusedLines, refused.tries())
		}
		if !mentions(dyingLines, "board locked", "lym adopt alpha") || dying.tries() != 1 {
			t.Errorf("dying lines = %q, tries = %d; want the error and the re-run, and the count 1", dyingLines, dying.tries())
		}
	})

	t.Run("a failed removal files none, a failed cancel names the hand cancel alone and a failed entry removal adds the re-run", func(t *testing.T) {
		t.Parallel()
		running := boardengine.RunStatus("blocked", "Code-Write")
		cancel := newFixFixture(t)
		cancel.tasks = []boardengine.Task{ownFix(&running)}
		cancel.runRecords["adopt-fix-alpha-t1"] = shedengine.Status{State: shedengine.StateBlocked}
		cancel.cancelErr = errors.New("driver alive")
		entry := newFixFixture(t)
		draft := boardengine.StatusDraft
		entry.tasks = []boardengine.Task{ownFix(&draft)}
		entry.removeErr = errors.New("board locked")

		cancelLines := failedEnd(cancel.deps, failedRecord())
		entryLines := failedEnd(entry.deps, failedRecord())
		if len(cancel.created) != 0 || len(entry.created) != 0 {
			t.Errorf("created = %v and %v; want none", cancel.created, entry.created)
		}
		if !mentions(cancelLines, "driver alive", "`lym batten cancel adopt-fix-alpha-t1 --remove`") || mentions(cancelLines, "re-run lym adopt") {
			t.Errorf("cancel lines = %q; want the hand cancel first and no re-run", cancelLines)
		}
		if !mentions(entryLines, "board locked", "`lym board remove adopt-fix-alpha-t1`", "re-run lym adopt alpha") {
			t.Errorf("entry lines = %q; want the hand removal and the re-run", entryLines)
		}
	})

	t.Run("a slug numbered after a landed fix task kept only as a run record or a task branch is higher", func(t *testing.T) {
		t.Parallel()
		f := newFixFixture(t)
		f.runSlugs = []string{"adopt-fix-alpha-t3", "unrelated"}
		f.branchSlugs = []string{"adopt-fix-alpha-t5"}

		failedEnd(f.deps, failedRecord())
		if len(f.created) != 1 || f.created[0]["slug"] != "adopt-fix-alpha-t6" {
			t.Errorf("created = %v; want adopt-fix-alpha-t6", f.created)
		}
	})
}

func TestFixSlug(t *testing.T) {
	t.Parallel()

	t.Run("a short subject's prefix is adopt-fix and the subject", func(t *testing.T) {
		t.Parallel()
		if got := fixSlugPrefix("alpha"); got != "adopt-fix-alpha" {
			t.Errorf("fixSlugPrefix(alpha) = %q; want adopt-fix-alpha", got)
		}
		// The longest subject that still fits with the -t999 suffix.
		fits := strings.Repeat("a", boardengine.MaxSlugLength-len("adopt-fix-")-len("-t999"))
		if got := fixSlugPrefix(fits); got != "adopt-fix-"+fits {
			t.Errorf("fixSlugPrefix(%q) = %q; want it uncut", fits, got)
		}
	})

	t.Run("long subjects sharing their cut get two prefixes within the limit and no doubled hyphen", func(t *testing.T) {
		t.Parallel()
		subjects := []string{"a-very-long-beam-slug-name-one", "a-very-long-beam-slug-name-two", "abcdefghi-jklmnopqrstuvwxyz"}
		seen := map[string]string{}
		for _, subject := range subjects {
			prefix := fixSlugPrefix(subject)
			if len(prefix)+len("-t999") > boardengine.MaxSlugLength || strings.Contains(prefix, "--") {
				t.Errorf("fixSlugPrefix(%q) = %q; want it to leave room for -t999 within %d with no doubled hyphen", subject, prefix, boardengine.MaxSlugLength)
			}
			if other, dup := seen[prefix]; dup {
				t.Errorf("fixSlugPrefix(%q) = fixSlugPrefix(%q) = %q; want distinct prefixes", subject, other, prefix)
			}
			seen[prefix] = subject
		}
	})

	t.Run("the next slug is one past the highest number among the taken slugs of its prefix", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			taken []string
			want  string
		}{
			{"NoneTaken", nil, "adopt-fix-alpha-t1"},
			{"HighestPlusOne", []string{"adopt-fix-alpha-t1", "adopt-fix-alpha-t4", "adopt-fix-alpha-t2"}, "adopt-fix-alpha-t5"},
			{"OtherPrefixesAndShapesIgnored", []string{"adopt-fix-beta-t9", "adopt-fix-alpha-t2-t7", "adopt-fix-alpha-tx", "adopt-fix-alpha-t03", "alpha"}, "adopt-fix-alpha-t1"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if got := nextFixSlug("adopt-fix-alpha", tt.taken); got != tt.want {
					t.Errorf("nextFixSlug(adopt-fix-alpha, %q) = %q; want %q", tt.taken, got, tt.want)
				}
			})
		}
	})
}

func TestFixBody(t *testing.T) {
	t.Parallel()
	record := failedRecord()
	record.Tiers[0].Tier = 1
	record.Tiers[0].Failures = append(record.Tiers[0].Failures, gateslot.TestFailure{Package: "example.com/b", Test: "TestB", FirstLine: "not in the log"})
	var logLines []string
	for i := range 40 {
		logLines = append(logLines, fmt.Sprintf("log line %d", i))
	}
	logLines[20] = "a_test.go:12: got 1; want 2"
	suiteLog := strings.Join(logLines, "\n")

	body := fixBody("alpha", record, suiteLog)
	for _, want := range []string{
		"tier1",
		"- example.com/a TestA: a_test.go:12: got 1; want 2",
		"log line 18\nlog line 19\na_test.go:12: got 1; want 2",
		"Stop condition: the failure is a machine failure. Write a stop report naming the exact steps the operator must take on this machine and, as its way forward, a re-run of lym adopt alpha once the machine is settled, never lym loom resume.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body = %q; want it to hold %q", body, want)
		}
	}
	if strings.Contains(body, "log line 31") || strings.Contains(body, "log line 17") {
		t.Errorf("body = %q; want the excerpt bounded to %d lines from 2 before the failed test's first line", body, fixExcerptLines)
	}
	if strings.Count(body, "```")/2 != 1 {
		t.Errorf("body = %q; want one excerpt, since the second test's first line is not in the log", body)
	}

	many := failedRecord()
	many.Tiers[0].Failures = nil
	for i := range maxListedFailures + 3 {
		many.Tiers[0].Failures = append(many.Tiers[0].Failures, gateslot.TestFailure{Package: "example.com/p", Test: fmt.Sprintf("Test%d", i)})
	}
	if got := fixBody("alpha", many, ""); !strings.Contains(got, "and 3 more") || strings.Contains(got, fmt.Sprintf("Test%d", maxListedFailures)) {
		t.Errorf("body = %q; want the failed tests capped at %d with the remainder counted", got, maxListedFailures)
	}
}
