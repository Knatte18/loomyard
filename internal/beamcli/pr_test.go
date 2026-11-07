// pr_test.go drives the pr open and pr update sequences over fake deps and a fake GitHub.
// It covers a pull request opened, found or recorded.
// A pull request a landing record names is settled by where GitHub reports it.
// It also covers the update and each refusal with its way forward.

package beamcli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/prkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const prURL = "https://github.com/o/r/pull/7"

// recordedPullRequest is the pull request a landing record names in the rows that start from a recorded one.
var recordedPullRequest = &pawlengine.LandingPullRequest{Number: 5, URL: "https://github.com/o/r/pull/5", PushedHead: "old", AnnounceHead: "old"}

func TestPROpen(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	githubOrigin := landingshed.Origin{URL: "https://github.com/o/r.git", Upstream: true}
	tests := []struct {
		name  string
		setup func(w *landWorld)
		// wantErr is empty for a verb that answers, else every substring the refusal names.
		wantErr []string
		// check judges the answer and the world of a verb that answers.
		check func(t *testing.T, w *landWorld, got map[string]any)
	}{
		{
			name: "NoneRecordedNoneOpenPushedCreatedAndRecordedWithBothHeads",
			setup: func(w *landWorld) {
				w.api.Script(prkit.MethodCreate, prkit.Answer{Ref: pullrequest.Ref{Number: 7, URL: prURL}})
			},
			check: func(t *testing.T, w *landWorld, got map[string]any) {
				if want := []string{"b|main|Ship the beam|Groups the work."}; !slices.Equal(w.api.Calls(prkit.MethodCreate), want) {
					t.Errorf("Create calls = %v; want %v", w.api.Calls(prkit.MethodCreate), want)
				}
				want := &pawlengine.LandingPullRequest{Number: 7, URL: prURL, PushedHead: "head1", AnnounceHead: "head1"}
				if w.pushed != 1 || w.landing["b"].PullRequest == nil || *w.landing["b"].PullRequest != *want {
					t.Errorf("pushed %d times, recorded %+v; want one push and %+v", w.pushed, w.landing["b"].PullRequest, want)
				}
				if got["created"] != true || got["url"] != prURL {
					t.Errorf("answer = %v; want created with the URL", got)
				}
			},
		},
		{
			name: "OpenDraftFoundMarkedReadyNotEditedAndRecorded",
			setup: func(w *landWorld) {
				w.api.Script(prkit.MethodFindOpen, prkit.Answer{Ref: pullrequest.Ref{Number: 8, URL: prURL}, Found: true})
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: pullrequest.Status{State: pullrequest.StateOpen, Draft: true}})
			},
			check: func(t *testing.T, w *landWorld, got map[string]any) {
				if want := []string{"8|false"}; !slices.Equal(w.api.Calls(prkit.MethodSetDraft), want) {
					t.Errorf("SetDraft calls = %v; want %v", w.api.Calls(prkit.MethodSetDraft), want)
				}
				if edits, creates := w.api.Calls(prkit.MethodEdit), w.api.Calls(prkit.MethodCreate); len(edits)+len(creates) != 0 {
					t.Errorf("Edit calls %v, Create calls %v; want a found pull request's text left as it is", edits, creates)
				}
				if recorded := w.landing["b"].PullRequest; recorded == nil || recorded.Number != 8 || got["created"] != false {
					t.Errorf("recorded %+v, answer %v; want pull request 8 recorded and not created", recorded, got)
				}
			},
		},
		{
			name: "RecordedOpenAnswersAlreadyAndCommitsWithoutAnotherGitHubCall",
			setup: func(w *landWorld) {
				w.landing["b"] = pawlengine.LandingRecord{Slug: "b", DescribedAt: landNow, PullRequest: recordedPullRequest}
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: pullrequest.Status{State: pullrequest.StateOpen}})
			},
			check: func(t *testing.T, w *landWorld, got map[string]any) {
				for _, method := range []string{prkit.MethodFindOpen, prkit.MethodCreate, prkit.MethodEdit, prkit.MethodReopen, prkit.MethodSetDraft} {
					if calls := w.api.Calls(method); len(calls) != 0 {
						t.Errorf("%s calls = %v; want none beside Status", method, calls)
					}
				}
				if got["already"] != true || w.writes != 1 || w.pushed != 0 {
					t.Errorf("answer %v, %d writes, %d pushes; want already, one commit and no push", got, w.writes, w.pushed)
				}
			},
		},
		{
			name: "RecordedClosedReopened",
			setup: func(w *landWorld) {
				w.landing["b"] = pawlengine.LandingRecord{Slug: "b", DescribedAt: landNow, PullRequest: recordedPullRequest}
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: pullrequest.Status{State: pullrequest.StateClosed}})
			},
			check: func(t *testing.T, w *landWorld, got map[string]any) {
				if want := []string{"5"}; !slices.Equal(w.api.Calls(prkit.MethodReopen), want) || got["reopened"] != true {
					t.Errorf("Reopen calls %v, answer %v; want pull request 5 reopened", w.api.Calls(prkit.MethodReopen), got)
				}
			},
		},
		{
			name: "RecordedMergedChangesNothingAndNamesReady",
			setup: func(w *landWorld) {
				w.landing["b"] = pawlengine.LandingRecord{Slug: "b", DescribedAt: landNow, PullRequest: recordedPullRequest}
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: pullrequest.Status{State: pullrequest.StateMerged}})
			},
			check: func(t *testing.T, w *landWorld, got map[string]any) {
				if reopens := w.api.Calls(prkit.MethodReopen); len(reopens) != 0 || w.writes != 0 || w.pushed != 0 {
					t.Errorf("Reopen calls %v, %d writes, %d pushes; want nothing changed", reopens, w.writes, w.pushed)
				}
				if !strings.Contains(got["way_forward"].(string), "lym beam ready b") {
					t.Errorf("answer = %v; want it to name `lym beam ready b`", got)
				}
			},
		},
		{
			name: "FailingRecordWriteAfterACreatedPullRequestNamesItsURL",
			setup: func(w *landWorld) {
				w.api.Script(prkit.MethodCreate, prkit.Answer{Ref: pullrequest.Ref{Number: 7, URL: prURL}})
				w.writeErr = errors.New("the commit failed")
			},
			wantErr: []string{prURL, "the commit failed", "lym beam pr open b"},
		},
		{
			name:    "UnusableTokenRefusedNamingTheCause",
			setup:   func(w *landWorld) { w.apiErr = githubclient.ErrTokenUnresolvable },
			wantErr: []string{"no GitHub token", "lym beam pr open b"},
		},
		{
			name:    "FailingPushRefusedNamingTheRerun",
			setup:   func(w *landWorld) { w.pushErr = errors.New("the push was rejected") },
			wantErr: []string{"the push was rejected", "lym beam pr open b"},
		},
		{
			name:    "FailingCreateRefusedNamingTheRerun",
			setup:   func(w *landWorld) { w.api.Script(prkit.MethodCreate, prkit.Answer{Err: errors.New("GitHub is down")}) },
			wantErr: []string{"GitHub is down", "lym beam pr open b"},
		},
		{
			name:    "NoDescriptionStampedRefusedNamingDescribe",
			setup:   func(w *landWorld) { w.landing["b"] = pawlengine.LandingRecord{Slug: "b"} },
			wantErr: []string{"no change description", "lym beam describe b"},
		},
		{
			name:    "NoLandingRecordRefusedNamingDescribe",
			setup:   func(w *landWorld) { delete(w.landing, "b") },
			wantErr: []string{"no change description", "lym beam describe b"},
		},
		{
			name:    "NoCommitsBeyondTheParentRefusedNamingReady",
			setup:   func(w *landWorld) { w.hasCommits = false },
			wantErr: []string{"no commits beyond its parent", "lym beam ready b"},
		},
		{
			name:    "NoOriginRemoteRefused",
			setup:   func(w *landWorld) { w.origin = landingshed.Origin{Upstream: true} },
			wantErr: []string{"no origin remote", "GitHub origin", "lym beam ready b"},
		},
		{
			name:    "OriginOffGitHubRefused",
			setup:   func(w *landWorld) { w.origin = landingshed.Origin{URL: "ssh://git@gitlab.com/o/r.git", Upstream: true} },
			wantErr: []string{"gitlab.com", "lym beam ready b"},
		},
		{
			name:    "OriginNamingNoRepositoryRefused",
			setup:   func(w *landWorld) { w.origin = landingshed.Origin{URL: "https://github.com/onlyowner", Upstream: true} },
			wantErr: []string{"names no repository", "lym beam ready b"},
		},
		{
			name:    "ParentWithNoUpstreamRefused",
			setup:   func(w *landWorld) { w.origin = landingshed.Origin{URL: githubOrigin.URL} },
			wantErr: []string{"no upstream", "lym beam ready b"},
		},
		{
			name: "PushSkipRefused",
			setup: func(w *landWorld) {
				w.origin = landingshed.Origin{URL: githubOrigin.URL, Upstream: true, PushSkipped: true}
			},
			wantErr: []string{"push skip", "lym beam ready b"},
		},
		{
			name:    "StandingReadyRecordRefused",
			setup:   func(w *landWorld) { w.ready["b"] = true },
			wantErr: []string{"lym beam pr open:", "final", "lym batten status b"},
		},
		{
			name: "RunNotAtBeamAwaitRefused",
			setup: func(w *landWorld) {
				w.children["b"] = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Finalize"}
			},
			wantErr: []string{"lym beam pr open:", "Finalize", "lym batten status b"},
		},
		{
			name: "BeamOwnedByAnotherMachineRefusedNamingTheVerbAndAdopt",
			setup: func(w *landWorld) {
				w.owners.Entries["b"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: there}
			},
			wantErr: []string{"lym beam pr open: the beam b is owned by machine there", "lym adopt b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newLandWorld()
			w.ready["b"] = false
			w.api.Script(prkit.MethodStatus, prkit.Answer{})
			tt.setup(w)

			got, err := prOpen(context.Background(), w.deps(), "b")

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("prOpen() = nil; want a refusal naming %v", tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("prOpen() error = %q; want it to name %q", err.Error(), want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("prOpen() = %v; want an answer", err)
			}
			tt.check(t, w, got)
		})
	}
}

func TestPRUpdate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	recorded := func(w *landWorld) {
		pull := *recordedPullRequest
		w.landing["b"] = pawlengine.LandingRecord{Slug: "b", DescribedAt: landNow, PullRequest: &pull}
	}
	tests := []struct {
		name  string
		setup func(w *landWorld)
		// wantErr is empty for a verb that answers, else every substring the refusal names.
		wantErr []string
		// wantSetDraft is the SetDraft calls of a verb that answers.
		wantSetDraft []string
	}{
		{
			name: "PushedEditedDraftMarkedReadyAndHeadRecorded",
			setup: func(w *landWorld) {
				recorded(w)
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: pullrequest.Status{State: pullrequest.StateOpen, Draft: true}})
			},
			wantSetDraft: []string{"5|false"},
		},
		{
			name: "PullRequestReadyForReviewIsNotMarkedAgain",
			setup: func(w *landWorld) {
				recorded(w)
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: pullrequest.Status{State: pullrequest.StateOpen}})
			},
		},
		{
			name:    "NoPullRequestRecordedRefusedNamingOpen",
			setup:   func(w *landWorld) {},
			wantErr: []string{"lym beam pr update:", "no pull request recorded", "lym beam pr open b"},
		},
		{
			name: "FailingEditRefusedNamingTheRerun",
			setup: func(w *landWorld) {
				recorded(w)
				w.api.Script(prkit.MethodEdit, prkit.Answer{Err: errors.New("GitHub is down")})
			},
			wantErr: []string{"GitHub is down", "lym beam pr update b"},
		},
		{
			name: "StandingReadyRecordRefused",
			setup: func(w *landWorld) {
				recorded(w)
				w.ready["b"] = true
			},
			wantErr: []string{"lym beam pr update:", "final"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newLandWorld()
			w.ready["b"] = false
			w.pushHead = "head2"
			tt.setup(w)

			got, err := prUpdate(context.Background(), w.deps(), "b")

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("prUpdate() = nil; want a refusal naming %v", tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("prUpdate() error = %q; want it to name %q", err.Error(), want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("prUpdate() = %v; want an answer", err)
			}
			if want := []string{"5|Ship the beam|Groups the work."}; !slices.Equal(w.api.Calls(prkit.MethodEdit), want) {
				t.Errorf("Edit calls = %v; want %v", w.api.Calls(prkit.MethodEdit), want)
			}
			if !slices.Equal(w.api.Calls(prkit.MethodSetDraft), tt.wantSetDraft) {
				t.Errorf("SetDraft calls = %v; want %v", w.api.Calls(prkit.MethodSetDraft), tt.wantSetDraft)
			}
			pull := w.landing["b"].PullRequest
			if pull == nil || pull.PushedHead != "head2" || pull.AnnounceHead != "old" || got["head"] != "head2" {
				t.Errorf("recorded %+v, answer %v; want PushedHead head2 and AnnounceHead left at old", pull, got)
			}
		})
	}
}

// recordPullRequest makes the landing record of beam "b" name a copy of recordedPullRequest.
func recordPullRequest(w *landWorld) {
	pull := *recordedPullRequest
	w.landing["b"] = pawlengine.LandingRecord{Slug: "b", DescribedAt: landNow, PullRequest: &pull}
}

// openPullRequestAtBeamHead is the Status of an open pull request whose head is the fake beam's.
var openPullRequestAtBeamHead = pullrequest.Status{State: pullrequest.StateOpen, HeadSHA: "head1", MergeableState: "clean"}

func TestPRStatus(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		setup func(w *landWorld)
		check func(t *testing.T, w *landWorld, got map[string]any)
	}{
		{
			name: "NoneRecordedAtBeamAwaitNamesOpen",
			check: func(t *testing.T, w *landWorld, got map[string]any) {
				if got["recorded"] != false || !strings.Contains(got["way_forward"].(string), "lym beam pr open b") {
					t.Errorf("answer = %v; want recorded false naming `lym beam pr open b`", got)
				}
				if calls := w.api.Calls(prkit.MethodStatus); len(calls) != 0 {
					t.Errorf("Status calls = %v; want none with no pull request recorded", calls)
				}
			},
		},
		{
			name: "NoneRecordedAtFinalizeNamesFinalizesDecision",
			setup: func(w *landWorld) {
				w.children["b"] = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Finalize"}
			},
			check: func(t *testing.T, _ *landWorld, got map[string]any) {
				wayForward := got["way_forward"].(string)
				if got["recorded"] != false || !strings.Contains(wayForward, "Finalize decides") || strings.Contains(wayForward, "lym beam pr open") {
					t.Errorf("answer = %v; want recorded false naming Finalize's decision and no `lym beam pr open`", got)
				}
			},
		},
		{
			name: "OpenCleanPullRequestAtBeamHeadIsMergeableNow",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: openPullRequestAtBeamHead})
			},
			check: func(t *testing.T, w *landWorld, got map[string]any) {
				if got["mergeable_now"] != true || got["state"] != "open" || got["head"] != "head1" {
					t.Errorf("answer = %v; want an open pull request at head1 mergeable now", got)
				}
				if _, causes := got["causes"]; causes {
					t.Errorf("answer = %v; want no causes for a pull request that merges", got)
				}
				if want := []string{"main|head1"}; !slices.Equal(w.api.Calls(prkit.MethodRequiredChecks), want) {
					t.Errorf("RequiredChecks calls = %v; want %v", w.api.Calls(prkit.MethodRequiredChecks), want)
				}
			},
		},
		{
			name: "BehindPullRequestNamesBehind",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				behind := openPullRequestAtBeamHead
				behind.MergeableState = "behind"
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: behind})
			},
			check: func(t *testing.T, _ *landWorld, got map[string]any) {
				causes, _ := got["causes"].([]string)
				if got["mergeable_now"] != false || !strings.Contains(strings.Join(causes, "|"), "behind") {
					t.Errorf("answer = %v; want not mergeable now, naming behind", got)
				}
			},
		},
		{
			name: "RunAtFinalizeIsStillAnswered",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				w.ready["b"] = true
				w.children["b"] = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Finalize"}
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: openPullRequestAtBeamHead})
			},
			check: func(t *testing.T, _ *landWorld, got map[string]any) {
				if got["recorded"] != true || got["mergeable_now"] != true {
					t.Errorf("answer = %v; want the recorded pull request read at Finalize", got)
				}
			},
		},
		{
			name: "UnknownMergeableStateIsReadAgainUntilComputed",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				unknown := openPullRequestAtBeamHead
				unknown.MergeableState = "unknown"
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: unknown}, prkit.Answer{Status: openPullRequestAtBeamHead})
			},
			check: func(t *testing.T, w *landWorld, got map[string]any) {
				if got["mergeable_now"] != true || w.slept != time.Second {
					t.Errorf("answer = %v after sleeping %v; want mergeable now after one second", got, w.slept)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newLandWorld()
			if tt.setup != nil {
				tt.setup(w)
			}

			got, err := prStatus(context.Background(), w.deps(), "b")
			if err != nil {
				t.Fatalf("prStatus() = %v; want an answer", err)
			}
			tt.check(t, w, got)
		})
	}
}

func TestPRClose(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		setup func(w *landWorld)
		// wantErr is empty for a verb that answers, else every substring the refusal names.
		wantErr []string
		// wantCloses is the Close calls of a verb that answers, and wantClosed the answer's closed flag.
		wantCloses []string
		// wantWayForward holds every substring the answer's way forward names, and noWayForward every one it must not.
		wantWayForward []string
		noWayForward   []string
	}{
		{
			name: "OpenPullRequestUnderAnOffParentClosedWithTheOperatorsChoiceAndDropped",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: openPullRequestAtBeamHead})
			},
			wantCloses:     []string{"5|" + landWithoutPullRequestComment},
			wantWayForward: []string{"lym beam ready b", "without a pull request"},
		},
		{
			name: "ClosedPullRequestMakesNoCloseCallAndIsDropped",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: pullrequest.Status{State: pullrequest.StateClosed}})
			},
		},
		{
			name: "MergedPullRequestMakesNoCloseCallAndIsDropped",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: pullrequest.Status{State: pullrequest.StateMerged}})
			},
		},
		{
			name: "MergeParentReportsFinalizeLandsThroughAPullRequest",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				w.mode = "merge"
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: openPullRequestAtBeamHead})
			},
			wantCloses:     []string{"5|" + closeOnlyComment},
			wantWayForward: []string{"Finalize still lands through a pull request"},
			noWayForward:   []string{"refuses"},
		},
		{
			name: "RequiredParentReportsReadyRefusesUntilOpen",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				w.mode = "required"
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: openPullRequestAtBeamHead})
			},
			wantCloses:     []string{"5|" + closeOnlyComment},
			wantWayForward: []string{"`lym beam ready b` refuses", "lym beam pr open b"},
		},
		{
			name: "RulesRequiredOnParentReportsReadyRefusesUntilOpen",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				w.mode = "on"
				w.rulesRequired = true
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: openPullRequestAtBeamHead})
			},
			wantCloses:     []string{"5|" + closeOnlyComment},
			wantWayForward: []string{"GitHub's rules", "`lym beam ready b` refuses", "lym beam pr open b"},
		},
		{
			name: "RequiredParentWithNoCommitsBeyondItsParentCommentsTheOperatorsChoiceAndReportsNoRefusal",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				w.mode = "required"
				w.hasCommits = false
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: openPullRequestAtBeamHead})
			},
			wantCloses:     []string{"5|" + landWithoutPullRequestComment},
			wantWayForward: []string{"lym beam ready b"},
			noWayForward:   []string{"refuses"},
		},
		{
			name: "FailingModeReadStillClosesAndDropsAndReportsTheError",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				w.modeErr = errors.New("beam.yaml is unreadable")
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: openPullRequestAtBeamHead})
			},
			wantCloses:     []string{"5|" + closeOnlyComment},
			wantWayForward: []string{"beam.yaml is unreadable"},
		},
		{
			name: "FailingBeamHasCommitsReadStillClosesAndDropsAndReportsTheError",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				w.commitsErr = errors.New("the pair is unreadable")
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: openPullRequestAtBeamHead})
			},
			wantCloses:     []string{"5|" + closeOnlyComment},
			wantWayForward: []string{"the pair is unreadable"},
		},
		{
			name:    "NoPullRequestRecordedRefusedNamingOpen",
			wantErr: []string{"lym beam pr close:", "no pull request recorded", "lym beam pr open b"},
		},
		{
			name: "StandingReadyRecordRefused",
			setup: func(w *landWorld) {
				recordPullRequest(w)
				w.ready["b"] = true
			},
			wantErr: []string{"lym beam pr close:", "final", "lym batten status b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newLandWorld()
			w.ready["b"] = false
			if tt.setup != nil {
				tt.setup(w)
			}

			got, err := prClose(context.Background(), w.deps(), "b")

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("prClose() = nil; want a refusal naming %v", tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("prClose() error = %q; want it to name %q", err.Error(), want)
					}
				}
				if closes := w.api.Calls(prkit.MethodClose); len(closes) != 0 || w.writes != 0 {
					t.Errorf("Close calls %v, %d writes on a refusal; want none", closes, w.writes)
				}
				return
			}
			if err != nil {
				t.Fatalf("prClose() = %v; want an answer", err)
			}
			if closes := w.api.Calls(prkit.MethodClose); !slices.Equal(closes, tt.wantCloses) {
				t.Errorf("Close calls = %v; want %v", closes, tt.wantCloses)
			}
			if got["closed"] != (len(tt.wantCloses) > 0) || got["dropped"] != true {
				t.Errorf("answer = %v; want closed %t and dropped", got, len(tt.wantCloses) > 0)
			}
			if w.landing["b"].PullRequest != nil || w.writes != 1 {
				t.Errorf("landing record %+v after %d writes; want the pull request dropped by one committed write", w.landing["b"], w.writes)
			}
			wayForward := got["way_forward"].(string)
			for _, want := range tt.wantWayForward {
				if !strings.Contains(wayForward, want) {
					t.Errorf("way forward = %q; want it to name %q", wayForward, want)
				}
			}
			for _, unwanted := range tt.noWayForward {
				if strings.Contains(wayForward, unwanted) {
					t.Errorf("way forward = %q; want it not to name %q", wayForward, unwanted)
				}
			}
		})
	}
}

func TestParentRuleRequired(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	githubOrigin := landingshed.Origin{URL: "https://github.com/o/r.git", Upstream: true}
	tests := []struct {
		name   string
		origin landingshed.Origin
		// openErr is what the fake open fails with, rules the scripted RequiresPullRequest answer.
		openErr error
		rules   prkit.Answer
		// cancelled runs the read under a context that has ended.
		cancelled bool
		want      bool
		wantErr   bool
		// wantOpens and wantRuleReads are how many times open and RequiresPullRequest ran.
		wantOpens     int
		wantRuleReads int
	}{
		{name: "OriginNotOnGitHubReadsNoRuleAndNeverOpens", origin: landingshed.Origin{URL: "ssh://git@gitlab.com/o/r.git", Upstream: true}},
		{name: "OriginWithNoParentUpstreamReadsNoRuleAndNeverOpens", origin: landingshed.Origin{URL: githubOrigin.URL}},
		{name: "UnusableTokenFromOpenReadsNoRule", origin: githubOrigin, openErr: githubclient.ErrTokenUnresolvable, wantOpens: 1},
		{
			name:          "RejectedTokenFromTheCallReadsNoRule",
			origin:        githubOrigin,
			rules:         prkit.Answer{Err: fmt.Errorf("call: %w", githubclient.ErrTokenRejected)},
			wantOpens:     1,
			wantRuleReads: 1,
		},
		{
			name:          "NonTransientFailureReadsNoRule",
			origin:        githubOrigin,
			rules:         prkit.Answer{Err: errors.New("GitHub answered 404")},
			wantOpens:     1,
			wantRuleReads: 1,
		},
		{
			name:          "TransientFailureIsReturned",
			origin:        githubOrigin,
			rules:         prkit.Answer{Err: fmt.Errorf("call: %w", context.DeadlineExceeded)},
			wantErr:       true,
			wantOpens:     1,
			wantRuleReads: 1,
		},
		{
			name:          "CancelledContextIsReturned",
			origin:        githubOrigin,
			rules:         prkit.Answer{Err: errors.New("the call stopped")},
			cancelled:     true,
			wantErr:       true,
			wantOpens:     1,
			wantRuleReads: 1,
		},
		{name: "RulesThatRequireAPullRequestReadTrue", origin: githubOrigin, rules: prkit.Answer{Found: true}, want: true, wantOpens: 1, wantRuleReads: 1},
		{name: "RulesThatDoNotRequireOneReadFalse", origin: githubOrigin, wantOpens: 1, wantRuleReads: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			api := &prkit.API{}
			api.Script(prkit.MethodRequiresPullRequest, tt.rules)
			opens := 0
			open := func(string) (pullrequest.API, error) {
				opens++
				if tt.openErr != nil {
					return nil, tt.openErr
				}
				return api, nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancelled {
				cancel()
			}

			got, err := parentRuleRequired(ctx, tt.origin, "main", open)

			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("parentRuleRequired() = %t, %v; want %t, error %t", got, err, tt.want, tt.wantErr)
			}
			if reads := api.Calls(prkit.MethodRequiresPullRequest); opens != tt.wantOpens || len(reads) != tt.wantRuleReads {
				t.Errorf("open ran %d times and RequiresPullRequest %v; want %d opens and %d reads", opens, reads, tt.wantOpens, tt.wantRuleReads)
			}
		})
	}
}
