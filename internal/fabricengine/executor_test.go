// executor_test.go tests the executor loop and its error classes.
// It is in package fabricengine because the loop is unexported and no verb drives it.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/testkit/fabrickit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// provokedTransportError returns git's error for a listing of an origin that refuses the connection.
func provokedTransportError(t *testing.T) error {
	t.Helper()
	_, err := gitexec.Run([]string{"ls-remote", "http://127.0.0.1:1/unreachable.git"}, t.TempDir())
	if err == nil {
		t.Fatal("ls-remote of an unreachable origin succeeded; want an error")
	}
	return err
}

func TestRunRule(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	transport := provokedTransportError(t)
	boom := errors.New("boom")
	diskError := errors.New("disk error")
	lockError := errors.New("lock held")
	mixedReadError := errors.Join(
		&fieldReadError{Field: fabriccore.FieldOriginRefs, Store: fabriccore.StoreCode, Err: transport},
		&fieldReadError{Field: fabriccore.FieldOriginRefs, Store: fabriccore.StoreRecords, Err: diskError},
	)
	emptyReason := &fabriccore.Refusal{}
	stop := &fabriccore.Refusal{Reason: "stop"}
	facts := fabrickit.NewSnapshot().
		LocalRef(fabriccore.StoreCode, "refs/heads/task", "abc").
		Anchor("/hub/task", ".").
		Build()
	phaseOf := func(phase fabriccore.Phase, field fabriccore.Field) fabriccore.Answer {
		return fabriccore.Answer{Phase: phase, Read: []fabriccore.Field{field}}
	}
	unreachableStore := func(store fabriccore.Store) error {
		return &fieldReadError{Field: fabriccore.FieldOriginRefs, Store: store, Err: transport}
	}
	unreachable := func(store fabriccore.Store) fabriccore.ReadFailure {
		return fabriccore.ReadFailure{
			Phase:  fabriccore.PhaseOrigin,
			Field:  fabriccore.FieldOriginRefs,
			Store:  store,
			Class:  fabriccore.OutcomeTransportFailure,
			Detail: transport.Error(),
		}
	}
	action := func(kind fabriccore.ActionKind) fabriccore.Action { return fabriccore.Action{Kind: kind} }
	done := func(kind fabriccore.ActionKind) fabriccore.Outcome {
		return fabriccore.Outcome{Action: action(kind), Class: fabriccore.OutcomeDone}
	}
	failed := func(kind fabriccore.ActionKind, detail string) fabriccore.Outcome {
		return fabriccore.Outcome{Action: action(kind), Class: fabriccore.OutcomeFailed, Detail: detail}
	}

	tests := []struct {
		name  string
		start fabriccore.Answer
		rule  fabriccore.Rule
		// readErrs and runErrs fail the read call at that index and the runner of that kind.
		readErrs []error
		runErrs  map[fabriccore.ActionKind]error
		lockErr  error
		// noLock passes runRule no lock function.
		noLock       bool
		wantErr      error
		wantCalls    []string
		wantPhases   []fabriccore.Phase
		wantOutcomes []fabriccore.Outcome
		wantFailures []fabriccore.ReadFailure
	}{
		{
			name:    "StartRefusalEndsAtOnce",
			start:   fabriccore.Answer{Refusal: emptyReason},
			wantErr: emptyReason,
		},
		{
			name:  "PhaseThenActionsThenRereadUnderThatPhase",
			start: phaseOf(fabriccore.PhasePreLock, fabriccore.FieldLocalRefs),
			rule: func(s fabriccore.Snapshot) fabriccore.Answer {
				if len(s.Outcomes) > 0 {
					return fabriccore.Answer{}
				}
				return fabriccore.Answer{
					Actions: []fabriccore.Action{action(fabriccore.ActionRefCreate), action(fabriccore.ActionPush)},
					Reread:  []fabriccore.Field{fabriccore.FieldAnchors},
				}
			},
			wantCalls: []string{
				"lock pre_lock", "read pre_lock [local_refs]",
				"run ref_create", "run push", "read pre_lock [anchors]",
				"release pre_lock",
			},
			wantPhases:   []fabriccore.Phase{fabriccore.PhasePreLock},
			wantOutcomes: []fabriccore.Outcome{done(fabriccore.ActionRefCreate), done(fabriccore.ActionPush)},
		},
		{
			name: "StartActionsRereadUnderNoPhase",
			start: fabriccore.Answer{
				Actions: []fabriccore.Action{action(fabriccore.ActionTag)},
				Reread:  []fabriccore.Field{fabriccore.FieldLocalRefs},
			},
			wantCalls:    []string{"run tag", "read  [local_refs]"},
			wantOutcomes: []fabriccore.Outcome{done(fabriccore.ActionTag)},
		},
		{
			name: "FailedActionSkipsTheRestAndReachesTheRule",
			start: fabriccore.Answer{
				Actions: []fabriccore.Action{action(fabriccore.ActionPush), action(fabriccore.ActionCommit)},
			},
			rule: func(s fabriccore.Snapshot) fabriccore.Answer {
				if s.Outcomes[0].Class == fabriccore.OutcomeFailed {
					return fabriccore.Answer{Refusal: stop}
				}
				return fabriccore.Answer{}
			},
			runErrs:      map[fabriccore.ActionKind]error{fabriccore.ActionPush: boom},
			wantErr:      stop,
			wantCalls:    []string{"run push", "read  []"},
			wantOutcomes: []fabriccore.Outcome{failed(fabriccore.ActionPush, "boom")},
		},
		{
			name: "WaitSleepsItsDelay",
			start: fabriccore.Answer{
				Actions: []fabriccore.Action{{Kind: fabriccore.ActionWait, Delay: 5 * time.Second}},
			},
			wantCalls: []string{"sleep 5s", "read  []"},
			wantOutcomes: []fabriccore.Outcome{{
				Action: fabriccore.Action{Kind: fabriccore.ActionWait, Delay: 5 * time.Second},
				Class:  fabriccore.OutcomeDone,
			}},
		},
		{
			name:  "TransportReadAndRereadFailuresAreRecordedAndReachTheRule",
			start: phaseOf(fabriccore.PhaseOrigin, fabriccore.FieldOriginRefs),
			rule: func(s fabriccore.Snapshot) fabriccore.Answer {
				switch {
				case len(s.Phases) == 0 && len(s.ReadFailures) == 2:
					return phaseOf(fabriccore.PhaseOrigin, fabriccore.FieldOriginRefs)
				case len(s.Outcomes) == 0:
					return fabriccore.Answer{
						Actions: []fabriccore.Action{action(fabriccore.ActionPush)},
						Reread:  []fabriccore.Field{fabriccore.FieldOriginRefs},
					}
				case len(s.ReadFailures) == 3:
					return fabriccore.Answer{Refusal: stop}
				}
				return fabriccore.Answer{}
			},
			readErrs: []error{
				errors.Join(unreachableStore(fabriccore.StoreCode), unreachableStore(fabriccore.StoreRecords)),
				nil,
				unreachableStore(fabriccore.StoreBolt),
			},
			wantErr: stop,
			wantCalls: []string{
				"lock origin", "read origin [origin_refs]", "read origin [origin_refs]",
				"run push", "read origin [origin_refs]", "release origin",
			},
			wantPhases:   []fabriccore.Phase{fabriccore.PhaseOrigin},
			wantOutcomes: []fabriccore.Outcome{done(fabriccore.ActionPush)},
			wantFailures: []fabriccore.ReadFailure{
				unreachable(fabriccore.StoreCode), unreachable(fabriccore.StoreRecords), unreachable(fabriccore.StoreBolt),
			},
		},
		{
			name:      "ReadErrorMixingATransportAndAnotherFailureIsReturned",
			start:     phaseOf(fabriccore.PhaseOrigin, fabriccore.FieldOriginRefs),
			readErrs:  []error{mixedReadError},
			noLock:    true,
			wantErr:   mixedReadError,
			wantCalls: []string{"read origin [origin_refs]"},
		},
		{
			name: "OtherRereadErrorKeepsTheOutcomes",
			start: fabriccore.Answer{
				Actions: []fabriccore.Action{action(fabriccore.ActionCommit)},
				Reread:  []fabriccore.Field{fabriccore.FieldAnchors},
			},
			readErrs:     []error{diskError},
			runErrs:      map[fabriccore.ActionKind]error{fabriccore.ActionCommit: boom},
			wantErr:      diskError,
			wantCalls:    []string{"run commit", "read  [anchors]"},
			wantOutcomes: []fabriccore.Outcome{failed(fabriccore.ActionCommit, "boom")},
		},
		{
			name:  "LocksPrecedeTheirReadsAndReleaseInReverse",
			start: phaseOf(fabriccore.PhasePreLock, fabriccore.FieldWorktrees),
			rule: func(s fabriccore.Snapshot) fabriccore.Answer {
				switch {
				case len(s.Phases) == 1:
					return phaseOf(fabriccore.PhaseUnderLock, fabriccore.FieldPendingTeardowns)
				case s.Stores == nil:
					// A phase already read is asked for once more.
					return phaseOf(fabriccore.PhasePreLock, fabriccore.FieldLocalRefs)
				}
				return fabriccore.Answer{Refusal: stop}
			},
			wantErr: stop,
			wantCalls: []string{
				"lock pre_lock", "read pre_lock [worktrees]",
				"lock under_lock", "read under_lock [pending_teardowns]",
				"read pre_lock [local_refs]",
				"release under_lock", "release pre_lock",
			},
			wantPhases: []fabriccore.Phase{fabriccore.PhasePreLock, fabriccore.PhaseUnderLock},
		},
		{
			name:      "LockErrorReadsNothing",
			start:     phaseOf(fabriccore.PhaseUnderLock, fabriccore.FieldLocalRefs),
			lockErr:   lockError,
			wantErr:   lockError,
			wantCalls: []string{"lock under_lock"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls []string
			record := func(format string, args ...any) { calls = append(calls, fmt.Sprintf(format, args...)) }
			reads := 0
			read := func(phase fabriccore.Phase, fields []fabriccore.Field, snapshot *fabriccore.Snapshot) error {
				record("read %s %v", phase, fields)
				reads++
				if reads <= len(tt.readErrs) {
					return tt.readErrs[reads-1]
				}
				if slices.Contains(fields, fabriccore.FieldLocalRefs) {
					snapshot.Stores = facts.Stores
				}
				if slices.Contains(fields, fabriccore.FieldAnchors) {
					snapshot.Anchors = facts.Anchors
				}
				return nil
			}
			lock := func(phase fabriccore.Phase) (func(), error) {
				record("lock %s", phase)
				return func() { record("release %s", phase) }, tt.lockErr
			}
			if tt.noLock {
				lock = nil
			}
			runners := map[fabriccore.ActionKind]actionRunner{}
			for _, kind := range []fabriccore.ActionKind{
				fabriccore.ActionRefCreate, fabriccore.ActionPush, fabriccore.ActionTag, fabriccore.ActionCommit,
			} {
				runners[kind] = func(a fabriccore.Action) error {
					record("run %s", a.Kind)
					return tt.runErrs[a.Kind]
				}
			}
			rule := tt.rule
			if rule == nil {
				rule = func(fabriccore.Snapshot) fabriccore.Answer { return fabriccore.Answer{} }
			}

			got, err := runRule(rule, tt.start, read, lock, runners, func(d time.Duration) { record("sleep %s", d) })

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("runRule() error = %v; want %v", err, tt.wantErr)
			}
			if !slices.Equal(calls, tt.wantCalls) {
				t.Errorf("runRule() calls = %q; want %q", calls, tt.wantCalls)
			}
			if !slices.Equal(got.Phases, tt.wantPhases) {
				t.Errorf("runRule() phases = %v; want %v", got.Phases, tt.wantPhases)
			}
			if !reflect.DeepEqual(got.Outcomes, tt.wantOutcomes) {
				t.Errorf("runRule() outcomes = %+v; want %+v", got.Outcomes, tt.wantOutcomes)
			}
			if !slices.Equal(got.ReadFailures, tt.wantFailures) {
				t.Errorf("runRule() read failures = %+v; want %+v", got.ReadFailures, tt.wantFailures)
			}
		})
	}
}

func TestOutcomeClassOf(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	root := t.TempDir()
	// cloneOfNewBare makes a bare repository holding one commit and returns it with a clone of it.
	cloneOfNewBare := func(name string) (bare, clone string) {
		bare = filepath.Join(root, name+".git")
		clone = filepath.Join(root, name)
		gitkit.MustRun(t, root, "git", "init", "--bare", "--initial-branch=main", bare)
		gitkit.MustRun(t, root, "git", "clone", bare, clone)
		gitkit.CommitFile(t, clone, "a.txt", "base", "base")
		gitkit.MustRun(t, clone, "git", "push", "-u", "origin", "HEAD:main")
		return bare, clone
	}

	divergedBare, behind := cloneOfNewBare("diverged")
	ahead := filepath.Join(root, "ahead")
	gitkit.MustRun(t, root, "git", "clone", divergedBare, ahead)
	gitkit.CommitFile(t, ahead, "b.txt", "theirs", "theirs")
	gitkit.MustRun(t, ahead, "git", "push", "origin", "main")
	gitkit.CommitFile(t, behind, "c.txt", "ours", "ours")
	rejected := gitrepo.New(behind).PushRebaseFree(gitrepo.RefNamespace{})

	refusingBare, refused := cloneOfNewBare("refusing")
	hook := "#!/bin/sh\necho 'Permission to hub denied to tester.' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(refusingBare, "hooks", "pre-receive"), []byte(hook), 0o755); err != nil {
		t.Fatalf("write pre-receive hook: %v", err)
	}
	gitkit.CommitFile(t, refused, "b.txt", "refused", "refused")
	authRefused := gitrepo.New(refused).PushRebaseFree(gitrepo.RefNamespace{})

	tests := []struct {
		name string
		err  error
		want fabriccore.OutcomeClass
	}{
		{"Nil", nil, fabriccore.OutcomeDone},
		{"Transport", provokedTransportError(t), fabriccore.OutcomeTransportFailure},
		{"Rejected", rejected, fabriccore.OutcomeRejected},
		{"AuthRefused", authRefused, fabriccore.OutcomeAuthRefused},
		{"AnchorStop", &AnchorStopError{Reason: "anchors differ"}, fabriccore.OutcomeAnchorStop},
		{"DestructiveRefusal", &destructiveRefusal{Check: CheckDirtiness}, fabriccore.OutcomeDestructiveRefusal},
		{"TakeoverAheadOfRejected", errors.Join(gitrepo.ErrPushRejected, ErrHandedOver), fabriccore.OutcomeTakeover},
		{"ProtectedAheadOfRejected", fmt.Errorf("%w: GH006: Protected branch update failed", gitrepo.ErrPushRejected), fabriccore.OutcomeProtected},
		{"ProtectedRefusalThatIsNoRejection", errors.New("git push: GH006: Protected branch update failed"), fabriccore.OutcomeProtectedUnrejected},
		{"ReplayConflict", &replayConflict{}, fabriccore.OutcomeReplayConflict},
		{"ExhaustedAheadOfTransport", fmt.Errorf("%w: %w", gitrepo.ErrPushRetriesExhausted, provokedTransportError(t)), fabriccore.OutcomeRetriesExhausted},
		{"OriginAhead", errOriginAhead, fabriccore.OutcomeOriginAhead},
		{"RetryDeferred", errRetryDeferred, fabriccore.OutcomeDeferred},
		{"Other", errors.New("other"), fabriccore.OutcomeFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := outcomeClassOf(tt.err); got != tt.want {
				t.Errorf("outcomeClassOf(%v) = %q; want %q", tt.err, got, tt.want)
			}
		})
	}
}
