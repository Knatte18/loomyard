// pushside_test.go tests the rules of one side's push: the actions they list and the verdict they end in.

package fabriccore_test

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/testkit/fabrickit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPushSide drives the rule over made-up snapshots with a backoff of two delays.
// Each action takes the next class of its row, and the last class repeats.
func TestPushSide(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		done        = fabriccore.OutcomeDone
		failed      = fabriccore.OutcomeFailed
		rejected    = fabriccore.OutcomeRejected
		transport   = fabriccore.OutcomeTransportFailure
		protected   = fabriccore.OutcomeProtected
		unrejected  = fabriccore.OutcomeProtectedUnrejected
		originAhead = fabriccore.OutcomeOriginAhead
		sideRead    = "read under_lock push_side"
		originRead  = "read origin push_side"
		judgeRead   = "read origin push_judgment"
	)
	type classes = []fabriccore.OutcomeClass
	boom := errors.New("boom")
	verdict := func(kind fabriccore.PushVerdictKind) fabriccore.PushVerdict {
		return fabriccore.PushVerdict{Kind: kind, Outcome: -1, Rejection: -1}
	}
	record := func(class fabriclayout.PushFailureClass, outcome int) fabriccore.PushVerdict {
		return fabriccore.PushVerdict{Kind: fabriccore.PushVerdictRecord, Class: class, Outcome: outcome, Rejection: -1}
	}
	with := func(v fabriccore.PushVerdict, change func(*fabriccore.PushVerdict)) fabriccore.PushVerdict {
		change(&v)
		return v
	}
	landed := verdict(fabriccore.PushVerdictLanded)
	nothingLacking := func(outcome int) fabriccore.PushVerdict {
		return with(record(fabriclayout.PushFailureOther, outcome), func(v *fabriccore.PushVerdict) { v.NothingLacking = true })
	}
	judged := &fabriccore.PushJudgmentFacts{Read: true}
	diverged := &fabriccore.PushJudgmentFacts{Read: true, OriginHoldsMore: true}
	divergence := func(outcome int) fabriccore.PushVerdict {
		return with(record(fabriclayout.PushFailureNotFastForward, outcome), func(v *fabriccore.PushVerdict) { v.LocalCommits = true })
	}
	view := func(v fabriccore.PushOriginView) func(*fabriccore.PushSideFacts) {
		return func(f *fabriccore.PushSideFacts) {
			v.Read = true
			f.Origin = v
		}
	}
	rewound := func(change func(*fabriccore.PushSideFacts)) func(*fabriccore.PushSideFacts) {
		return func(f *fabriccore.PushSideFacts) {
			f.Side = fabriclayout.RepoCode
			f.Origin = fabriccore.PushOriginView{Read: true, Head: "o1", Rewound: true, Dropped: []string{"c1"}}
			if change != nil {
				change(f)
			}
		}
	}
	boltPath := "/hub/" + fabriclayout.BoltDirName
	board := func(change func(*fabriccore.PushBoardFacts)) func(*fabriccore.PushSideFacts) {
		return func(f *fabriccore.PushSideFacts) {
			f.Side = fabriclayout.RepoBolt
			f.Board = fabriccore.PushBoardFacts{
				Unseeded: true, Path: boltPath, HubPath: "/hub", Remote: "origin", DiscardWorkFlag: "discard-work",
				OriginFound: true, OriginTip: "o1", LocalTip: "l1", Shared: true,
				Upstreams: []fabriccore.PushUpstreamSetting{{Branch: "main", MergeRef: "refs/x/main"}},
			}
			change(&f.Board)
		}
	}
	// repeated is the trace of one action followed by a wait and the action again after each delay.
	repeated := func(action string) []string {
		return []string{action, "wait 1s", action, "wait 2s", action}
	}
	trace := func(parts ...[]string) []string { return slices.Concat(parts...) }

	tests := []struct {
		name string
		// facts changes the base facts: a records side with an upstream, origin holding it and one unpushed commit.
		facts func(*fabriccore.PushSideFacts)
		// originDown fails every lookup on origin in transit.
		originDown bool
		classes    classes
		judgment   *fabriccore.PushJudgmentFacts
		wantTrace  []string
		want       fabriccore.PushVerdict
		// wantRefusal is a part of the refusal the rule ends in.
		wantRefusal string
	}{
		{name: "NothingReadAsksForTheSide", facts: func(f *fabriccore.PushSideFacts) { f.Read = false }, wantTrace: []string{sideRead}},
		{name: "TransportLoopPushesOncePerDelayAndStops", classes: classes{transport},
			wantTrace: repeated("push"), want: record(fabriclayout.PushFailureTransport, 4)},
		{name: "ExhaustedPushIsPushedOnce", classes: classes{fabriccore.OutcomeRetriesExhausted},
			wantTrace: []string{"push"}, want: record(fabriclayout.PushFailureTransport, 0)},
		{name: "SingleAttemptPushesOnce", facts: func(f *fabriccore.PushSideFacts) { f.SingleAttempt = true }, classes: classes{transport},
			wantTrace: []string{"push"}, want: record(fabriclayout.PushFailureTransport, 0)},
		{name: "RejectionRetryRunsOncePlusOncePerDelay", classes: classes{rejected}, judgment: judged,
			wantTrace: trace([]string{"push"}, repeated("seam_call"), []string{judgeRead}), want: nothingLacking(5)},
		{name: "ProtectedFirstPushRunsOneRetryThenTheJudgment", classes: classes{protected}, judgment: judged,
			wantTrace: []string{"push", "seam_call", judgeRead}, want: nothingLacking(1)},
		{name: "ProtectedRejectionWithOriginAheadIsNotFastForward", classes: classes{rejected, protected}, judgment: diverged,
			wantTrace: []string{"push", "seam_call", judgeRead}, want: divergence(1)},
		{name: "ProtectedUnrejectedRetryIsOtherUnjoinedAndUnjudged", classes: classes{rejected, unrejected},
			wantTrace: []string{"push", "seam_call"}, want: record(fabriclayout.PushFailureOther, 1)},
		{name: "FailedRetryKeepsRetryingAndJoinsTheRejection", classes: classes{rejected, failed}, judgment: judged,
			wantTrace: trace([]string{"push"}, repeated("seam_call"), []string{judgeRead}),
			want:      with(nothingLacking(5), func(v *fabriccore.PushVerdict) { v.Rejection = 0 })},
		{name: "OriginAheadStopsAndKeepsTheFirstRejection", classes: classes{rejected, originAhead}, judgment: diverged,
			wantTrace: []string{"push", "seam_call", judgeRead}, want: divergence(0)},
		{name: "AuthRefusedRetryStopsAtOnce", classes: classes{rejected, fabriccore.OutcomeAuthRefused},
			wantTrace: []string{"push", "seam_call"}, want: record(fabriclayout.PushFailureAuth, 1)},
		{name: "AnchorStopRetryStopsAtOnce", classes: classes{rejected, fabriccore.OutcomeAnchorStop},
			wantTrace: []string{"push", "seam_call"}, want: record(fabriclayout.PushFailureAnchorChange, 1)},
		{name: "ReplayConflictRetryStopsAtOnce", classes: classes{rejected, fabriccore.OutcomeReplayConflict},
			wantTrace: []string{"push", "seam_call"}, want: divergence(1)},
		{name: "DestructiveRefusalRetryStopsAtOnce", classes: classes{rejected, fabriccore.OutcomeDestructiveRefusal},
			wantTrace: []string{"push", "seam_call"}, want: divergence(1)},
		{name: "TransportRetryStopsAtOnce", classes: classes{rejected, transport},
			wantTrace: []string{"push", "seam_call"}, want: record(fabriclayout.PushFailureTransport, 1)},
		{name: "TakeoverRetryIsReturnedUnrecorded", classes: classes{rejected, fabriccore.OutcomeTakeover},
			wantTrace: []string{"push", "seam_call"},
			want:      with(verdict(fabriccore.PushVerdictReturn), func(v *fabriccore.PushVerdict) { v.Outcome = 1 })},
		{name: "DeferredRetryDefers", classes: classes{rejected, fabriccore.OutcomeDeferred},
			wantTrace: []string{"push", "seam_call"}, want: verdict(fabriccore.PushVerdictDeferred)},
		{name: "LandedRetryLands", classes: classes{rejected, done}, wantTrace: []string{"push", "seam_call"}, want: landed},
		{name: "FirstPushFailingForNoNamedReasonIsOther", classes: classes{failed},
			wantTrace: []string{"push"}, want: record(fabriclayout.PushFailureOther, 0)},
		{name: "RefusedResetsUnmadeCommitIsNotFastForward", classes: classes{rejected, originAhead},
			judgment:  &fabriccore.PushJudgmentFacts{Read: true, Unmade: "u1", UnmadeAt: "o2"},
			wantTrace: []string{"push", "seam_call", judgeRead},
			want: with(record(fabriclayout.PushFailureNotFastForward, 0), func(v *fabriccore.PushVerdict) {
				v.Commits, v.OriginHead = []string{"u1"}, "o2"
			})},
		{name: "FailedJudgmentReadIsOtherWithItsError", classes: classes{rejected, originAhead},
			judgment:  &fabriccore.PushJudgmentFacts{Read: true, Err: boom},
			wantTrace: []string{"push", "seam_call", judgeRead},
			want:      with(record(fabriclayout.PushFailureOther, 0), func(v *fabriccore.PushVerdict) { v.Err = boom })},

		{name: "GoneBranchIsLookedUpOncePerDelayThenRecorded", facts: view(fabriccore.PushOriginView{Gone: true}),
			wantTrace: []string{"wait 1s", "wait 2s"}, want: record(fabriclayout.PushFailureGone, -1)},
		{name: "UnreachableOriginIsLookedUpOncePerDelayThenRecorded", originDown: true,
			facts:     func(f *fabriccore.PushSideFacts) { f.Origin = fabriccore.PushOriginView{} },
			wantTrace: []string{originRead, "wait 1s", "wait 2s"}, want: record(fabriclayout.PushFailureTransport, -1)},
		{name: "SingleAttemptReturnsAnUnreachableOriginAtOnce", originDown: true,
			facts:     func(f *fabriccore.PushSideFacts) { f.Origin, f.SingleAttempt = fabriccore.PushOriginView{}, true },
			wantTrace: []string{originRead}, want: record(fabriclayout.PushFailureTransport, -1)},
		{name: "EndedBranchDropsItsTrackingRef", facts: view(fabriccore.PushOriginView{Gone: true, Ended: true}),
			wantTrace: []string{"ref_delete"}, want: verdict(fabriccore.PushVerdictDone)},
		{name: "FailedTrackingRefDeleteIsReturned", facts: view(fabriccore.PushOriginView{Gone: true, Ended: true}), classes: classes{failed},
			wantTrace: []string{"ref_delete"},
			want:      with(verdict(fabriccore.PushVerdictReturn), func(v *fabriccore.PushVerdict) { v.Outcome = 0 })},
		{name: "UncheckedBranchIsPushed", facts: view(fabriccore.PushOriginView{Unchecked: true}), wantTrace: []string{"push"}, want: landed},
		{name: "FailedJudgmentOfAGoneEntryIsReturned", facts: view(fabriccore.PushOriginView{Gone: true, JudgeErr: boom}),
			want: with(verdict(fabriccore.PushVerdictReturn), func(v *fabriccore.PushVerdict) { v.Err = boom })},
		{name: "RewoundTaskBranchIsHeldBack", facts: rewound(nil),
			want: with(record(fabriclayout.PushFailureRewound, -1), func(v *fabriccore.PushVerdict) {
				v.Commits, v.OriginHead = []string{"c1"}, "o1"
			})},
		{name: "PrimeSkipsTheRewindCheck", facts: rewound(func(f *fabriccore.PushSideFacts) { f.Prime = true }),
			wantTrace: []string{"push"}, want: landed},
		{name: "PrunedTrackingRefSkipsTheRewindCheck",
			facts:     rewound(func(f *fabriccore.PushSideFacts) { f.Upstream = fabriccore.PushUpstreamPruned }),
			wantTrace: []string{"push"}, want: landed},
		{name: "BranchNotRewoundIsPushed", facts: rewound(func(f *fabriccore.PushSideFacts) { f.Origin.Rewound = false }),
			wantTrace: []string{"push"}, want: landed},
		{name: "UnreadableBranchNameIsPushedWithNoOriginRead",
			facts:     func(f *fabriccore.PushSideFacts) { f.Branch, f.Origin = "", fabriccore.PushOriginView{} },
			wantTrace: []string{"push"}, want: verdict(fabriccore.PushVerdictDone)},
		{name: "BranchWithNoUpstreamIsPushedWithNoOriginRead",
			facts: func(f *fabriccore.PushSideFacts) {
				f.Upstream, f.Origin = fabriccore.PushUpstreamNone, fabriccore.PushOriginView{}
			},
			wantTrace: []string{"push"}, want: landed},
		{name: "FailedUpstreamReadIsReturned", facts: func(f *fabriccore.PushSideFacts) { f.UpstreamErr = boom },
			want: with(verdict(fabriccore.PushVerdictReturn), func(v *fabriccore.PushVerdict) { v.Err = boom })},
		{name: "FailedNamespaceReadIsOtherBeforeOriginIsRead",
			facts: func(f *fabriccore.PushSideFacts) { f.NamespaceErr, f.Origin = boom, fabriccore.PushOriginView{} },
			want:  with(record(fabriclayout.PushFailureOther, -1), func(v *fabriccore.PushVerdict) { v.Err = boom })},
		{name: "UnpushedCommitsAreReadAfterOrigin", facts: func(f *fabriccore.PushSideFacts) { f.UnpushedRead = false },
			wantTrace: []string{sideRead}},
		{name: "BoltSideWithNothingUnpushedIsCleared",
			facts: func(f *fabriccore.PushSideFacts) { f.Side, f.Unpushed = fabriclayout.RepoBolt, false },
			want:  verdict(fabriccore.PushVerdictCleared)},

		{name: "BoardNeverSeededIsRefused", facts: board(func(b *fabriccore.PushBoardFacts) { b.OriginFound = false }),
			wantRefusal: "again with `--reset --discard-work`"},
		{name: "BoardWithNoCommitsBesideAnotherIsRefused", facts: board(func(b *fabriccore.PushBoardFacts) { b.NoCommits = true }),
			wantRefusal: "while this board holds no commits, so another clone seeded origin; way forward: remove the hub at /hub"},
		{name: "BoardSharingNoHistoryIsRefused", facts: board(func(b *fabriccore.PushBoardFacts) { b.Shared = false }),
			wantRefusal: "without `--single-repo` to join that hub"},
		{name: "SeededBoardWithoutUpstreamsIsRefused", facts: board(func(*fabriccore.PushBoardFacts) {}),
			wantRefusal: "way forward: set them with `git -C " + boltPath + " config branch.main.remote origin`, " +
				"`git -C " + boltPath + " config branch.main.merge refs/x/main`, then run the same command again"},
		{name: "BoardWhoseOriginCannotBeReadIsRefused", facts: board(func(b *fabriccore.PushBoardFacts) { b.OriginReadErr = boom }),
			wantRefusal: "boom; way forward: run the same command again once origin answers"},
		{name: "BoardThatCannotBeFetchedIsRefused", facts: board(func(b *fabriccore.PushBoardFacts) { b.FetchErr = boom }),
			wantRefusal: "origin's board could not be fetched to compare it with this board: boom"},
		{name: "BoardThatCannotBeComparedIsRefused", facts: board(func(b *fabriccore.PushBoardFacts) { b.CompareErr = boom }),
			wantRefusal: "origin's board could not be compared with this board: boom"},
		{name: "BoardWhoseRecordsBranchCannotBeReadIsRefused", facts: board(func(b *fabriccore.PushBoardFacts) { b.RecordsBranchErr = boom }),
			wantRefusal: "could not be read from the records store at " + boltPath + " to name the upstream settings: boom"},
		{name: "BoltSideWithAnUnreadLayoutReturnsItsError",
			facts: func(f *fabriccore.PushSideFacts) { f.Side, f.NamespaceErr = fabriclayout.RepoBolt, boom },
			want:  with(verdict(fabriccore.PushVerdictReturn), func(v *fabriccore.PushVerdict) { v.Err = boom })},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			snapshot := fabrickit.NewSnapshot().Build()
			snapshot.Backoff = []time.Duration{time.Second, 2 * time.Second}
			snapshot.Push = fabriccore.PushSideFacts{
				Read: true, Side: fabriclayout.RepoRecords, Branch: "task", Upstream: fabriccore.PushUpstreamTracked,
				Origin: fabriccore.PushOriginView{Read: true}, UnpushedRead: true, Unpushed: true,
			}
			if tt.facts != nil {
				tt.facts(&snapshot.Push)
			}
			rowClasses := tt.classes
			if len(rowClasses) == 0 {
				rowClasses = classes{done}
			}
			failOrigin := func() {
				snapshot.ReadFailures = append(snapshot.ReadFailures,
					fabriccore.ReadFailure{Phase: fabriccore.PhaseOrigin, Field: fabriccore.FieldPushSide, Class: transport})
			}

			var got []string
			ran := 0
			var answer fabriccore.Answer
			for range 20 {
				answer = fabriccore.PushSide(snapshot)
				if answer.Phase != "" {
					got = append(got, fmt.Sprintf("read %s %s", answer.Phase, answer.Read[0]))
					switch {
					case answer.Read[0] == fabriccore.FieldPushJudgment && tt.judgment != nil:
						snapshot.Push.Judgment = *tt.judgment
						continue
					case answer.Read[0] == fabriccore.FieldPushSide && tt.originDown:
						failOrigin()
						// Each wait that follows is traced, and the lookup after it fails again.
						for range 20 {
							next := fabriccore.PushSide(snapshot)
							if len(next.Actions) == 0 {
								break
							}
							got = append(got, "wait "+next.Actions[0].Delay.String())
							snapshot.Outcomes = append(snapshot.Outcomes, fabriccore.Outcome{Action: next.Actions[0], Class: done})
							failOrigin()
						}
					}
					break
				}
				if len(answer.Actions) == 0 {
					break
				}
				for _, action := range answer.Actions {
					class := done
					if action.Kind == fabriccore.ActionWait {
						got = append(got, "wait "+action.Delay.String())
					} else {
						got = append(got, string(action.Kind))
					}
					if action.Kind != fabriccore.ActionWait {
						class = rowClasses[min(ran, len(rowClasses)-1)]
						ran++
					}
					snapshot.Outcomes = append(snapshot.Outcomes, fabriccore.Outcome{Action: action, Class: class})
					if class != done {
						break
					}
				}
			}

			if !slices.Equal(got, tt.wantTrace) {
				t.Errorf("PushSide() trace = %q; want %q", got, tt.wantTrace)
			}
			if tt.wantRefusal != "" {
				if answer.Refusal == nil || !strings.Contains(answer.Refusal.Error(), tt.wantRefusal) {
					t.Errorf("PushSide() refusal = %v; want one holding %q", answer.Refusal, tt.wantRefusal)
				}
				return
			}
			if tt.want.Kind == "" {
				return
			}
			if verdict := fabriccore.JudgePush(snapshot); !reflect.DeepEqual(verdict, tt.want) {
				t.Errorf("JudgePush() = %+v; want %+v", verdict, tt.want)
			}
		})
	}
}
