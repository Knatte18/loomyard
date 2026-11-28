// originjudge_test.go tests the rules of judging branches against origin: the reads, the verdicts and the repair steps.

package fabriccore_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestOriginJudge(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	allSeams := map[fabriccore.RequiredSeam]bool{
		fabriccore.SeamRunRecordPaths: true,
		fabriccore.SeamRunLocks:       true,
		fabriccore.SeamStatusCodec:    true,
	}
	without := func(seam fabriccore.RequiredSeam) map[fabriccore.RequiredSeam]bool {
		seams := map[fabriccore.RequiredSeam]bool{}
		for name := range allSeams {
			seams[name] = name != seam
		}
		return seams
	}
	readOrigin := fabriccore.Answer{Phase: fabriccore.PhaseOrigin, Read: []fabriccore.Field{fabriccore.FieldOriginRefs}}

	t.Run("RefusesAnUnsuppliedSeamBeforeOriginIsRead", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			scope   fabriclayout.ReconcileScope
			missing fabriccore.RequiredSeam
			refuses bool
		}{
			{"RunRecordPaths", fabriclayout.ReconcileScope{JudgeOnly: true}, fabriccore.SeamRunRecordPaths, true},
			{"StatusCodec", fabriclayout.ReconcileScope{Run: "task"}, fabriccore.SeamStatusCodec, true},
			{"RunLocksOfAMovingHubScope", fabriclayout.ReconcileScope{}, fabriccore.SeamRunLocks, true},
			{"NoRunLocksForAJudgeOnlyScope", fabriclayout.ReconcileScope{JudgeOnly: true}, fabriccore.SeamRunLocks, false},
			{"NoRunLocksForARunScope", fabriclayout.ReconcileScope{Run: "task"}, fabriccore.SeamRunLocks, false},
		}
		for _, tt := range tests {
			got := fabriccore.OriginJudge(fabriccore.Snapshot{
				Seams:     without(tt.missing),
				Reconcile: fabriccore.ReconcileFacts{Scope: tt.scope, Attempts: 1},
			})
			if !tt.refuses {
				if !reflect.DeepEqual(got, readOrigin) {
					t.Errorf("%s: OriginJudge() = %+v; want %+v", tt.name, got, readOrigin)
				}
				continue
			}
			if got.Refusal == nil || !strings.Contains(got.Refusal.Reason, string(tt.missing)) || got.Phase != "" {
				t.Errorf("%s: OriginJudge() = %+v; want a refusal naming %s", tt.name, got, tt.missing)
			}
		}
	})

	t.Run("ReadsOriginAgainWhileAttemptsRemain", func(t *testing.T) {
		t.Parallel()
		wait := fabriccore.Answer{
			Actions: []fabriccore.Action{{Kind: fabriccore.ActionWait, Delay: time.Second}},
			Reread:  []fabriccore.Field{fabriccore.FieldOriginRefs},
		}
		tests := []struct {
			name     string
			attempts int
			want     []fabriccore.Answer
		}{
			{"ThreeAttempts", 3, []fabriccore.Answer{readOrigin, wait, wait, {}}},
			{"OneAttemptHasNoRetry", 1, []fabriccore.Answer{readOrigin, {}}},
		}
		for _, tt := range tests {
			snapshot := fabriccore.Snapshot{Seams: allSeams, Reconcile: fabriccore.ReconcileFacts{Attempts: tt.attempts, RetryInterval: time.Second}}
			for failures, want := range tt.want {
				if got := fabriccore.OriginJudge(snapshot); !reflect.DeepEqual(got, want) {
					t.Errorf("%s: OriginJudge() after %d failed reads = %+v; want %+v", tt.name, failures, got, want)
				}
				snapshot.ReadFailures = append(snapshot.ReadFailures, fabriccore.ReadFailure{Field: fabriccore.FieldOriginRefs})
			}
		}
	})

	t.Run("FetchesOnlyWhenAHeadMovedAndThenReadsTheBranches", func(t *testing.T) {
		t.Parallel()
		fetch := fabriccore.Answer{Phase: fabriccore.PhaseOrigin, Read: []fabriccore.Field{fabriccore.FieldMovedHeads}}
		branches := fabriccore.Answer{Phase: fabriccore.PhaseOrigin, Read: []fabriccore.Field{fabriccore.FieldBranchFacts}}
		tests := []struct {
			name  string
			facts fabriccore.ReconcileFacts
			want  fabriccore.Answer
		}{
			{"AMovedHead", fabriccore.ReconcileFacts{Targets: []fabriccore.BranchFacts{{Tracking: "a", OriginHead: "b"}}}, fetch},
			{"AMovedHeadAlreadyFetched", fabriccore.ReconcileFacts{Fetched: true, Targets: []fabriccore.BranchFacts{{Tracking: "a", OriginHead: "b"}}}, branches},
			{"AnUnmovedAndAGoneHead", fabriccore.ReconcileFacts{Targets: []fabriccore.BranchFacts{{Tracking: "a", OriginHead: "a"}, {Tracking: "a"}}}, branches},
			{"JudgedBranches", fabriccore.ReconcileFacts{Judged: true}, fabriccore.Answer{}},
		}
		for _, tt := range tests {
			got := fabriccore.OriginJudge(fabriccore.Snapshot{
				Seams:     allSeams,
				Stores:    map[fabriccore.Store]fabriccore.StoreRefs{fabriccore.StoreCode: {OriginRead: true}},
				Reconcile: tt.facts,
			})
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s: OriginJudge() = %+v; want %+v", tt.name, got, tt.want)
			}
		}
	})

	t.Run("JudgeBranch", func(t *testing.T) {
		t.Parallel()
		const code, records = fabriclayout.RepoCode, fabriclayout.RepoRecords
		tests := []struct {
			name        string
			facts       fabriccore.BranchFacts
			want        fabriclayout.BranchState
			wantCommits []string
		}{
			{"NoOriginHeadIsGone", fabriccore.BranchFacts{Repo: code}, fabriclayout.BranchGone, nil},
			{"EqualTipsAreLevel", fabriccore.BranchFacts{Repo: code, OriginHead: "o", LocalTip: "o"}, fabriclayout.BranchLevel, nil},
			{"RewoundWinsOverAhead", fabriccore.BranchFacts{Repo: code, OriginHead: "o", LocalTip: "l", Rewound: true, Dropped: []string{"d"}, Ahead: true}, fabriclayout.BranchRewound, []string{"d"}},
			{"BehindIsFastForward", fabriccore.BranchFacts{Repo: code, OriginHead: "o", LocalTip: "l", Behind: true}, fabriclayout.BranchFastForward, nil},
			{"Ahead", fabriccore.BranchFacts{Repo: code, OriginHead: "o", LocalTip: "l", Ahead: true}, fabriclayout.BranchAhead, nil},
			{"ARecordsBranchNeitherSideContains", fabriccore.BranchFacts{Repo: records, OriginHead: "o", LocalTip: "l"}, fabriclayout.BranchNeedsReplay, nil},
			{"ABoltBranchNeitherSideContains", fabriccore.BranchFacts{Repo: fabriclayout.RepoBolt, OriginHead: "o", LocalTip: "l"}, fabriclayout.BranchNeedsReplay, nil},
			{"ARewrittenCodeBranch", fabriccore.BranchFacts{Repo: code, OriginHead: "o", LocalTip: "l", Rewritten: true}, fabriclayout.BranchRewritten, nil},
			{"ACodeBranchBothSidesMoved", fabriccore.BranchFacts{Repo: code, OriginHead: "o", LocalTip: "l"}, fabriclayout.BranchDiverged, nil},
		}
		for _, tt := range tests {
			got := fabriccore.JudgeBranch(tt.facts)
			if got.State != tt.want || !slices.Equal(got.Commits, tt.wantCommits) || got.Repo != tt.facts.Repo || got.OriginHead != tt.facts.OriginHead {
				t.Errorf("%s: JudgeBranch(%+v) = %+v; want state %q with commits %v", tt.name, tt.facts, got, tt.want, tt.wantCommits)
			}
		}
	})
}

func TestClassifyEntries(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("ClassifyEntryPrecedence", func(t *testing.T) {
		t.Parallel()
		states := func(s ...fabriclayout.BranchState) []fabriclayout.BranchState { return s }
		tests := []struct {
			name  string
			facts fabriccore.EntryFacts
			want  fabriclayout.EntryState
		}{
			{"EndedOverAStoppingBranch", fabriccore.EntryFacts{Ended: true, Branches: states(fabriclayout.BranchDirty)}, fabriclayout.EntryEnded},
			{"EndedOverAD3Stop", fabriccore.EntryFacts{Ended: true, D3Stop: true}, fabriclayout.EntryEnded},
			{"EndedOverOwnedElsewhere", fabriccore.EntryFacts{Ended: true, OwnedElsewhere: true}, fabriclayout.EntryEnded},
			{"OwnedElsewhereOverStoppingBranches", fabriccore.EntryFacts{OwnedElsewhere: true, Branches: states(fabriclayout.BranchDiverged, fabriclayout.BranchDirty)}, fabriclayout.EntryOwnedElsewhere},
			{"OwnedElsewhereOverAD3Stop", fabriccore.EntryFacts{OwnedElsewhere: true, D3Stop: true}, fabriclayout.EntryOwnedElsewhere},
			{"AD3StopOverLevelBranches", fabriccore.EntryFacts{D3Stop: true, Branches: states(fabriclayout.BranchLevel)}, fabriclayout.EntryStopped},
			{"ADivergedBranch", fabriccore.EntryFacts{Branches: states(fabriclayout.BranchLevel, fabriclayout.BranchDiverged)}, fabriclayout.EntryStopped},
			{"ARewoundBranch", fabriccore.EntryFacts{Branches: states(fabriclayout.BranchRewound)}, fabriclayout.EntryStopped},
			{"ARewrittenBranch", fabriccore.EntryFacts{Branches: states(fabriclayout.BranchRewritten)}, fabriclayout.EntryStopped},
			{"AGoneBranch", fabriccore.EntryFacts{Branches: states(fabriclayout.BranchGone)}, fabriclayout.EntryStopped},
			{"ABusyBranch", fabriccore.EntryFacts{Branches: states(fabriclayout.BranchBusy)}, fabriclayout.EntryStopped},
			{"ALivePairsBranch", fabriccore.EntryFacts{Branches: states(fabriclayout.BranchLive)}, fabriclayout.EntryStopped},
			{"AnAnchorStoppedBranch", fabriccore.EntryFacts{Branches: states(fabriclayout.BranchAnchorStopped)}, fabriclayout.EntryStopped},
			{"BranchesThatNeverStop", fabriccore.EntryFacts{Branches: states(fabriclayout.BranchLevel, fabriclayout.BranchFastForward, fabriclayout.BranchAhead, fabriclayout.BranchNeedsReplay)}, fabriclayout.EntryActive},
			{"NoBranch", fabriccore.EntryFacts{}, fabriclayout.EntryActive},
		}
		for _, tt := range tests {
			if got := fabriccore.ClassifyEntry(tt.facts); got != tt.want {
				t.Errorf("%s: ClassifyEntry(%+v) = %q; want %q", tt.name, tt.facts, got, tt.want)
			}
		}
	})

	bolt := fabriclayout.BranchOutcome{Repo: fabriclayout.RepoBolt, State: fabriclayout.BranchDiverged, WayForward: "settle bolt"}
	primeRecords := fabriclayout.BranchOutcome{Repo: fabriclayout.RepoRecords, Prime: true, State: fabriclayout.BranchDirty, WayForward: "settle the prime"}
	busyBolt := fabriclayout.BranchOutcome{Repo: fabriclayout.RepoBolt, State: fabriclayout.BranchBusy, WayForward: "wait"}
	taskCode := fabriclayout.BranchOutcome{Repo: fabriclayout.RepoCode, Slug: "task", State: fabriclayout.BranchGone, WayForward: "settle the task"}
	busyTaskRecords := fabriclayout.BranchOutcome{Repo: fabriclayout.RepoRecords, Slug: "task", State: fabriclayout.BranchBusy, WayForward: "wait"}
	active := fabriclayout.EntryOutcome{Slug: "task", State: fabriclayout.EntryActive}
	stopped := func(wayForward string) fabriclayout.EntryOutcome {
		return fabriclayout.EntryOutcome{Slug: "task", State: fabriclayout.EntryStopped, WayForward: wayForward}
	}

	tests := []struct {
		name     string
		scope    fabriclayout.ReconcileScope
		branches []fabriclayout.BranchOutcome
		owners   map[string]string
		subjects map[string]string
		want     fabriclayout.EntryOutcome
	}{
		{name: "AStoppedBoltStopsTheEntry", branches: []fabriclayout.BranchOutcome{bolt}, want: stopped("settle bolt")},
		{name: "AStoppedPrimeRecordsBranchStopsTheEntry", branches: []fabriclayout.BranchOutcome{primeRecords}, want: stopped("settle the prime")},
		{name: "ABusyBoltStopsNoEntry", branches: []fabriclayout.BranchOutcome{busyBolt}, want: active},
		{name: "TheEntrysOwnWayForwardWinsOverTheSharedStops", branches: []fabriclayout.BranchOutcome{bolt, taskCode}, want: stopped("settle the task")},
		{name: "ABusyRecordsBranchOfTheRunsOwnPairStopsNothing", scope: fabriclayout.ReconcileScope{Run: "task"}, branches: []fabriclayout.BranchOutcome{busyTaskRecords}, want: active},
		{name: "ABusyRecordsBranchOfAPairStopsItInAHubScope", branches: []fabriclayout.BranchOutcome{busyTaskRecords}, want: stopped("wait")},
		{
			name:     "AnEntryOwnedElsewhereNamesTheAdoptOfItsSubject",
			branches: []fabriclayout.BranchOutcome{taskCode},
			owners:   map[string]string{"task": "other"},
			subjects: map[string]string{"task": "beam"},
			want: fabriclayout.EntryOutcome{Slug: "task", State: fabriclayout.EntryOwnedElsewhere, Owner: "other", AdoptSubject: "beam",
				WayForward: "`lym adopt beam` from this machine's prime takes it over"},
		},
		{
			name:   "AnEntryOwnedElsewhereWithNoSubjectNamesItsSlug",
			owners: map[string]string{"task": "other"},
			want: fabriclayout.EntryOutcome{Slug: "task", State: fabriclayout.EntryOwnedElsewhere, Owner: "other",
				WayForward: "`lym adopt task` from this machine's prime takes it over"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := fabriccore.ClassifyEntries(tt.scope, []string{"task"}, tt.branches, tt.owners, tt.subjects)
			if want := []fabriclayout.EntryOutcome{tt.want}; !reflect.DeepEqual(got, want) {
				t.Errorf("ClassifyEntries() = %+v; want %+v", got, want)
			}
		})
	}
}
