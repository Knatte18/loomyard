// prepush_test.go pins the pre-push check's verdicts over stub seams.
// It covers when a push goes or stops and which pause it takes;
// the notices it posts, the skip condition, and how a stop reaches the seams it guards.

package loomcli

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPrePushCheckGuard asserts each origin verdict maps to a push or a stop, with the right pause and notice.
func TestPrePushCheckGuard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const slug = "task"
	ownNamed := func(name string, state fabricengine.BranchState, moved bool) fabricengine.BranchOutcome {
		return fabricengine.BranchOutcome{Branch: name, Repo: fabricengine.RepoCode, Slug: slug, State: state, Moved: moved}
	}
	own := func(state fabricengine.BranchState, moved bool) fabricengine.BranchOutcome {
		return ownNamed(slug, state, moved)
	}
	parent := func(state fabricengine.BranchState) fabricengine.BranchOutcome {
		return fabricengine.BranchOutcome{Branch: "main", Repo: fabricengine.RepoCode, Prime: true, State: state}
	}
	boltMain := fabricengine.BranchOutcome{Branch: "main", Repo: fabricengine.RepoBolt, State: fabricengine.BranchFastForward, Moved: true}
	entry := func(state fabricengine.EntryState) fabricengine.EntryOutcome {
		return fabricengine.EntryOutcome{Slug: slug, State: state}
	}
	tests := []struct {
		name         string
		skip         bool
		skipErr      error
		report       fabricengine.OriginReport
		reconcileErr error
		parentErr    error
		landed       bool
		landedErr    error
		pauseErr     error
		notifyErr    error
		repeat       bool
		wantStop     bool
		wantCalls    []string
		wantNotices  []string
	}{
		{name: "Skipped", skip: true},
		{name: "SkipUnreadable", skipErr: errors.New("status locked")},
		{name: "ReconcileFails", reconcileErr: errors.New("ls-remote refused"), wantCalls: []string{"reconcile"}},
		{name: "Unreachable", report: fabricengine.OriginReport{Unreachable: true}, wantCalls: []string{"reconcile"}},
		{
			name:      "ActiveNothingMoved",
			report:    fabricengine.OriginReport{Branches: []fabricengine.BranchOutcome{own(fabricengine.BranchAhead, false)}, Entries: []fabricengine.EntryOutcome{entry(fabricengine.EntryActive)}},
			wantCalls: []string{"reconcile"},
		},
		{
			name: "ActiveOwnBranchMoved",
			report: fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{own(fabricengine.BranchLevel, true), boltMain},
				Entries:  []fabricengine.EntryOutcome{entry(fabricengine.EntryActive)},
			},
			wantCalls:   []string{"reconcile"},
			wantNotices: []string{"task: origin moved task; the run took the new tips before its push"},
		},
		{
			name: "ActiveSeveralOwnBranchesMoved",
			report: fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{own(fabricengine.BranchLevel, true), ownNamed("task-records", fabricengine.BranchAhead, true)},
				Entries:  []fabricengine.EntryOutcome{entry(fabricengine.EntryActive)},
			},
			wantCalls:   []string{"reconcile"},
			wantNotices: []string{"task: origin moved task, task-records; the run took the new tips before its push"},
		},
		{
			name: "ActiveParentBranchFastForward",
			report: fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{parent(fabricengine.BranchFastForward)},
				Entries:  []fabricengine.EntryOutcome{entry(fabricengine.EntryActive)},
			},
			wantCalls:   []string{"reconcile"},
			wantNotices: []string{"task: origin moved the parent branch main (fast_forward) past this machine's tip"},
		},
		{
			name: "ActiveParentBranchDiverged",
			report: fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{parent(fabricengine.BranchDiverged)},
				Entries:  []fabricengine.EntryOutcome{entry(fabricengine.EntryActive)},
			},
			wantCalls:   []string{"reconcile"},
			wantNotices: []string{"task: origin moved the parent branch main (diverged) past this machine's tip"},
		},
		{
			name: "ActiveParentBranchLevelAndBoltBranchOfTheSameNameMoved",
			report: fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{parent(fabricengine.BranchLevel), boltMain},
				Entries:  []fabricengine.EntryOutcome{entry(fabricengine.EntryActive)},
			},
			wantCalls: []string{"reconcile"},
		},
		{
			name: "ActiveParentBranchUnreadable",
			report: fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{parent(fabricengine.BranchFastForward)},
				Entries:  []fabricengine.EntryOutcome{entry(fabricengine.EntryActive)},
			},
			parentErr: errors.New("no origin record"),
			wantCalls: []string{"reconcile"},
		},
		{
			name: "ActiveParentBranchNoticePostedOncePerTip",
			report: fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{parent(fabricengine.BranchFastForward)},
				Entries:  []fabricengine.EntryOutcome{entry(fabricengine.EntryActive)},
			},
			repeat:      true,
			wantCalls:   []string{"reconcile", "reconcile"},
			wantNotices: []string{"task: origin moved the parent branch main (fast_forward) past this machine's tip"},
		},
		{
			name: "StoppedLeavesOutAParentBranchOriginDidNotMove",
			report: fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{own(fabricengine.BranchDiverged, false), parent(fabricengine.BranchLive)},
				Entries:  []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryStopped, WayForward: "merge origin's task"}},
			},
			wantStop:    true,
			wantCalls:   []string{"reconcile", "pause"},
			wantNotices: []string{"task: paused before its push: origin moved task (diverged): merge origin's task; then `lym tender resume task`"},
		},
		{
			name:        "NoticePostFails",
			report:      fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryOwnedElsewhere, Owner: "neve"}}},
			notifyErr:   errors.New("no orch"),
			wantStop:    true,
			wantCalls:   []string{"reconcile", "pauseOwned"},
			wantNotices: []string{"task: paused before its push: the run is owned by machine \"neve\"; take it over with `lym adopt task` from this machine's prime"},
		},
		{
			name:      "EntryOfAnotherSlug",
			report:    fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: "other", State: fabricengine.EntryStopped, WayForward: "lym fabric pull"}}},
			wantCalls: []string{"reconcile"},
		},
		{
			name:        "OwnedElsewhere",
			report:      fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryOwnedElsewhere, Owner: "neve", AdoptSubject: "beam"}}},
			wantStop:    true,
			wantCalls:   []string{"reconcile", "pauseOwned"},
			wantNotices: []string{"task: paused before its push: the run is owned by machine \"neve\"; take it over with `lym adopt beam` from this machine's prime"},
		},
		{
			name:        "OwnedElsewhereNoSubject",
			report:      fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryOwnedElsewhere, Owner: "neve"}}},
			wantStop:    true,
			wantCalls:   []string{"reconcile", "pauseOwned"},
			wantNotices: []string{"task: paused before its push: the run is owned by machine \"neve\"; take it over with `lym adopt task` from this machine's prime"},
		},
		{
			name: "StoppedOwnBranch",
			report: fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{own(fabricengine.BranchDiverged, false)},
				Entries:  []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryStopped, WayForward: "merge origin's task"}},
			},
			wantStop:    true,
			wantCalls:   []string{"reconcile", "pause"},
			wantNotices: []string{"task: paused before its push: origin moved task (diverged): merge origin's task; then `lym tender resume task`"},
		},
		{
			name: "StoppedNamesOnlyTheStoppingOwnBranch",
			report: fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{
					ownNamed("task-level", fabricengine.BranchLevel, false),
					own(fabricengine.BranchDiverged, false),
					ownNamed("task-ahead", fabricengine.BranchAhead, false),
				},
				Entries: []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryStopped, WayForward: "merge origin's task"}},
			},
			wantStop:    true,
			wantCalls:   []string{"reconcile", "pause"},
			wantNotices: []string{"task: paused before its push: origin moved task (diverged): merge origin's task; then `lym tender resume task`"},
		},
		{
			name: "StoppedNamesTheParentBranch",
			report: fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{own(fabricengine.BranchLevel, false), parent(fabricengine.BranchDiverged)},
				Entries:  []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryStopped, WayForward: "merge origin's main"}},
			},
			wantStop:    true,
			wantCalls:   []string{"reconcile", "pause"},
			wantNotices: []string{"task: paused before its push: origin moved main (diverged): merge origin's main; then `lym tender resume task`"},
		},
		{
			name:        "StoppedSharedBranch",
			report:      fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryStopped, WayForward: "lym fabric pull"}}},
			wantStop:    true,
			wantCalls:   []string{"reconcile", "pause"},
			wantNotices: []string{"task: paused before its push: origin moved a branch every run shares: lym fabric pull; then `lym tender resume task`"},
		},
		{
			name: "StoppedSharedBranchBesideOwnNeedsReplay",
			report: fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{ownNamed("task-records", fabricengine.BranchNeedsReplay, false)},
				Entries:  []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryStopped, WayForward: "lym fabric pull"}},
			},
			wantStop:    true,
			wantCalls:   []string{"reconcile", "pause"},
			wantNotices: []string{"task: paused before its push: origin moved a branch every run shares: lym fabric pull; then `lym tender resume task`"},
		},
		{
			name: "StoppedSharedBranchBesideOwnBusyRecords",
			report: fabricengine.OriginReport{
				Branches: []fabricengine.BranchOutcome{{Branch: "task-records", Repo: fabricengine.RepoRecords, Slug: slug, State: fabricengine.BranchBusy}},
				Entries:  []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryStopped, WayForward: "lym fabric pull"}},
			},
			wantStop:    true,
			wantCalls:   []string{"reconcile", "pause"},
			wantNotices: []string{"task: paused before its push: origin moved a branch every run shares: lym fabric pull; then `lym tender resume task`"},
		},
		{
			name:      "EndedAfterOwnLanding",
			report:    fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryEnded, How: "done"}}},
			landed:    true,
			wantCalls: []string{"reconcile", "landed"},
		},
		{
			name:        "EndedElsewhere",
			report:      fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryEnded, How: "removed"}}},
			wantStop:    true,
			wantCalls:   []string{"reconcile", "landed", "pause"},
			wantNotices: []string{"task: paused before its push: origin shows the run removed; `lym cleanup --apply` from the prime removes its pair"},
		},
		{
			name:      "EndedWithUnreadableLanding",
			report:    fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryEnded, How: "removed"}}},
			landedErr: errors.New("prime unreadable"),
			wantCalls: []string{"reconcile", "landed"},
		},
		{
			name:        "PauseFails",
			report:      fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryOwnedElsewhere, Owner: "neve"}}},
			pauseErr:    errors.New("record locked"),
			wantStop:    true,
			wantCalls:   []string{"reconcile", "pauseOwned"},
			wantNotices: []string{"task: paused before its push: the run is owned by machine \"neve\"; take it over with `lym adopt task` from this machine's prime; the pause failed (record locked), so the run's next push stops again"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls, notices []string
			announced := map[string]bool{}
			check := prePushCheck{
				slug: slug,
				skip: func() (bool, error) { return tt.skip, tt.skipErr },
				reconcile: func() (fabricengine.OriginReport, error) {
					calls = append(calls, "reconcile")
					return tt.report, tt.reconcileErr
				},
				parentBranch: func() (string, error) { return "main", tt.parentErr },
				parentNoticed: func(key string) bool {
					seen := announced[key]
					announced[key] = true
					return seen
				},
				landed: func() (bool, error) {
					calls = append(calls, "landed")
					return tt.landed, tt.landedErr
				},
				pauseOwned: func() error {
					calls = append(calls, "pauseOwned")
					return tt.pauseErr
				},
				pause: func() error {
					calls = append(calls, "pause")
					return tt.pauseErr
				},
				notify: func(line string) error {
					notices = append(notices, line)
					return tt.notifyErr
				},
			}

			err := check.guard()
			if tt.repeat {
				err = check.guard()
			}

			if got := errors.Is(err, errPushStopped); got != tt.wantStop {
				t.Errorf("guard() = %v; want stopped %t", err, tt.wantStop)
			}
			if !reflect.DeepEqual(calls, tt.wantCalls) {
				t.Errorf("calls = %q; want %q", calls, tt.wantCalls)
			}
			if !reflect.DeepEqual(notices, tt.wantNotices) {
				t.Errorf("notices = %q; want %q", notices, tt.wantNotices)
			}
		})
	}
}

// TestAnnouncedBefore asserts a key is announced once: the first call records it and reads as new;
// a repeat reads as announced, and a different key reads as new again.
func TestAnnouncedBefore(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	path := filepath.Join(t.TempDir(), "scratch", "parent-notice")
	steps := []struct {
		key  string
		want bool
	}{
		{"main fast_forward abc", false},
		{"main fast_forward abc", true},
		{"main fast_forward def", false},
		{"main fast_forward def", true},
	}

	for _, step := range steps {
		if got := announcedBefore(path, step.key); got != step.want {
			t.Errorf("announcedBefore(%q) = %t; want %t", step.key, got, step.want)
		}
	}
}

// TestSkipsCheck asserts which runs keep the pre-push check off:
// pushes or git off, the prime, a run a pause covers, and a run at Finalize; the other runs do not.
func TestSkipsCheck(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name             string
		opts             fabricengine.SyncOptions
		prime            bool
		primeErr         error
		paused           bool
		producer         string
		producerErr      error
		want             bool
		wantErr          bool
		wantProducerRead bool
	}{
		{name: "PushesOff", opts: fabricengine.SyncOptions{SkipPush: true}, want: true},
		{name: "GitOff", opts: fabricengine.SyncOptions{SkipGit: true}, want: true},
		{name: "Prime", prime: true, want: true},
		{name: "PrimeUnreadable", primeErr: errors.New("no hub"), wantErr: true},
		{name: "PauseCoversTheRun", paused: true, want: true},
		{name: "AtFinalize", producer: loomshed.NameFinalize, want: true, wantProducerRead: true},
		{name: "AtAnotherProducer", producer: "Code-Review", wantProducerRead: true},
		{name: "NoStatusFile", wantProducerRead: true},
		{name: "StatusUnreadable", producerErr: errors.New("status locked"), wantErr: true, wantProducerRead: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			producerRead := false

			got, err := skipsCheck(
				tt.opts,
				func() (bool, error) { return tt.prime, tt.primeErr },
				func() bool { return tt.paused },
				func() (string, error) {
					producerRead = true
					return tt.producer, tt.producerErr
				},
			)

			if got != tt.want || (err != nil) != tt.wantErr {
				t.Errorf("skipsCheck() = %t, %v; want %t, error %t", got, err, tt.want, tt.wantErr)
			}
			if producerRead != tt.wantProducerRead {
				t.Errorf("the status was read = %t; want %t", producerRead, tt.wantProducerRead)
			}
		})
	}
}

// stoppingPrePushCheck returns a check that stops every push: its run's entry is owned by another machine.
func stoppingPrePushCheck() prePushCheck {
	return prePushCheck{
		slug: "task",
		skip: func() (bool, error) { return false, nil },
		reconcile: func() (fabricengine.OriginReport, error) {
			return fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: "task", State: fabricengine.EntryOwnedElsewhere, Owner: "neve"}}}, nil
		},
		pauseOwned: func() error { return nil },
		notify:     func(string) error { return nil },
	}
}

// TestPrePushCheckWiredIntoDeps asserts the status deps and the PR-Review row run behind their check:
// a stopping check stops Commit, Push and PushTask with its error and leaves Spawn a nil no-op;
// none of them reaches git.
func TestPrePushCheckWiredIntoDeps(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	location := &lymcwd.Location{RepoName: "repo", HubPath: t.TempDir(), WorktreeName: "task", AnchorRel: "."}

	statusDeps := commitStatusDepsGuardedBy(stoppingPrePushCheck(), location, shedrun.SelfRunID)
	if _, err := statusDeps.Commit("message"); !errors.Is(err, errPushStopped) {
		t.Errorf("Commit() error = %v; want it to wrap errPushStopped", err)
	}
	if err := statusDeps.Spawn(); err != nil {
		t.Errorf("Spawn() error = %v; want nil, the stopped push left its commits local", err)
	}
	if err := statusDeps.Push(); !errors.Is(err, errPushStopped) {
		t.Errorf("Push() error = %v; want it to wrap errPushStopped", err)
	}

	pullRequest := pullRequestDepsGuardedBy(stoppingPrePushCheck(), location, "main", false, &shuttleengine.Runner{})
	if err := pullRequest.PushTask(); !errors.Is(err, errPushStopped) {
		t.Errorf("PushTask() error = %v; want it to wrap errPushStopped", err)
	}
}

// TestPrePushCheckGuardedSeams asserts a stop reaches the commit, spawn and push seams it guards as documented;
// a passing check runs each seam and returns its result.
func TestPrePushCheckGuardedSeams(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	passing := prePushCheck{slug: "task", skip: func() (bool, error) { return true, nil }}
	stopping := stoppingPrePushCheck()
	boom := errors.New("boom")
	seams := map[string]func(check prePushCheck, call func() error) error{
		"Commit": func(check prePushCheck, call func() error) error {
			_, err := check.guardedCommit(func(string) (bool, error) { return true, call() })("message")
			return err
		},
		"Spawn": func(check prePushCheck, call func() error) error { return check.guardedSpawn(call)() },
		"Push":  func(check prePushCheck, call func() error) error { return check.guardedPush(call)() },
	}
	tests := []struct {
		seam        string
		wantStopErr bool
	}{
		{seam: "Commit", wantStopErr: true},
		{seam: "Spawn"},
		{seam: "Push", wantStopErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.seam+"_StoppedCheck", func(t *testing.T) {
			t.Parallel()
			calls := 0

			err := seams[tt.seam](stopping, func() error { calls++; return boom })

			if got := errors.Is(err, errPushStopped); got != tt.wantStopErr || (!tt.wantStopErr && err != nil) {
				t.Errorf("%s with a stopped check = %v; want stop error %t", tt.seam, err, tt.wantStopErr)
			}
			if calls != 0 {
				t.Errorf("%s ran its work %d times after a stop; want 0", tt.seam, calls)
			}
		})
		t.Run(tt.seam+"_PassingCheck", func(t *testing.T) {
			t.Parallel()
			calls := 0

			err := seams[tt.seam](passing, func() error { calls++; return boom })

			if !errors.Is(err, boom) {
				t.Errorf("%s with a passing check = %v; want the work's error", tt.seam, err)
			}
			if calls != 1 {
				t.Errorf("%s ran its work %d times; want 1", tt.seam, calls)
			}
		})
	}
}
