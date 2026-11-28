// pullrequestmode_test.go covers ResolvePullRequest: each beam.pull_request mode;
// a parent branch GitHub's rules guard;
// each origin fact that blocks a pull request; a beam's landing record; and an empty task branch.

package landingshed

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestResolvePullRequest(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	githubOrigin := Origin{URL: "https://github.com/o/r.git", Upstream: true}
	local := Origin{URL: "/srv/hub.git", Upstream: true}

	t.Run("Modes", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			mode          string
			wantOpen      bool
			wantAtOnce    bool
			wantTrueMerge bool
		}{
			{"review", true, false, false},
			{"off", false, false, false},
			{"trail", true, true, false},
			{"merge-in", false, false, true},
		}
		for _, tt := range tests {
			t.Run(tt.mode, func(t *testing.T) {
				t.Parallel()
				got := ResolvePullRequest(tt.mode, NotABeam, false, githubOrigin, true)
				if got.Open != tt.wantOpen || got.MergeAtOnce != tt.wantAtOnce || got.TrueMerge != tt.wantTrueMerge || got.Halt != "" || got.Reason == "" {
					t.Errorf("ResolvePullRequest(%q) = %+v; want Open %v, MergeAtOnce %v, TrueMerge %v, no halt and a reason",
						tt.mode, got, tt.wantOpen, tt.wantAtOnce, tt.wantTrueMerge)
				}
			})
		}
		off := ResolvePullRequest("off", NotABeam, false, githubOrigin, true)
		if want := "pull request off: beam.pull_request for this landing is off"; off.Reason != want {
			t.Errorf("off reason = %q; want %q", off.Reason, want)
		}
	})

	t.Run("MergeInWantsNoPullRequestWhateverTheRules", func(t *testing.T) {
		t.Parallel()
		for _, rule := range []bool{false, true} {
			got := ResolvePullRequest("merge-in", NotABeam, rule, githubOrigin, true)
			if got.Open || got.MergeAtOnce || got.Halt != "" || !got.TrueMerge || got.Reason == "" {
				t.Errorf("ResolvePullRequest(merge-in, rule %v) = %+v; want a no marked TrueMerge, with a reason", rule, got)
			}
		}
	})

	t.Run("ParentRule", func(t *testing.T) {
		t.Parallel()
		const ruleDemand = "GitHub's rules for the parent branch require a pull request"
		tests := []struct {
			name           string
			mode           string
			origin         Origin
			taskHasCommits bool
			wantOpen       bool
			// wantIn is a substring the halt holds when one is wanted, and the reason otherwise.
			wantIn   string
			wantHalt bool
		}{
			{name: "OffOpensOne", mode: "off", origin: githubOrigin, taskHasCommits: true, wantOpen: true, wantIn: "GitHub's rules for the parent branch require one"},
			{name: "ReviewOpensOne", mode: "review", origin: githubOrigin, taskHasCommits: true, wantOpen: true, wantIn: "GitHub's rules for the parent branch require one"},
			{name: "AMissingOriginFactHalts", mode: "off", origin: local, taskHasCommits: true, wantHalt: true, wantIn: ruleDemand},
			{name: "AnEmptyTaskBranchIsNo", mode: "off", origin: githubOrigin, wantIn: "the task branch has no commits beyond its parent"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := ResolvePullRequest(tt.mode, NotABeam, true, tt.origin, tt.taskHasCommits)
				if got.Open != tt.wantOpen || (got.Halt != "") != tt.wantHalt {
					t.Fatalf("ResolvePullRequest(%q, rule) = %+v; want Open %v, halt %v", tt.mode, got, tt.wantOpen, tt.wantHalt)
				}
				text := got.Reason
				if tt.wantHalt {
					text = got.Halt
				}
				if !strings.Contains(text, tt.wantIn) {
					t.Errorf("ResolvePullRequest() text = %q; want it to hold %q", text, tt.wantIn)
				}
			})
		}
	})

	t.Run("Origins", func(t *testing.T) {
		t.Parallel()
		const haltStart = "a pull request is required here ("
		const wayForward = "way forward: give the hub a GitHub origin, an upstream for the parent branch and no push skip, then run lym loom resume"
		if missing := OriginMissing(githubOrigin); missing != "" {
			t.Errorf("OriginMissing(%+v) = %q; want none for a complete GitHub origin", githubOrigin, missing)
		}
		tests := []struct {
			name     string
			mode     string
			origin   Origin
			wantHalt bool
			// wantIn is a substring the reason, or the halt when one is wanted, holds.
			wantIn string
		}{
			{"LocalPathUnderReviewIsNo", "review", local, false, "/srv/hub.git"},
			{"LocalPathUnderTrailHalts", "trail", local, true, "beam.pull_request for this landing is trail"},
			{"OwnerOnlyHalts", "review", Origin{URL: "https://github.com/onlyowner", Upstream: true}, true, "names no repository"},
			{"EmptyUserInfoHalts", "review", Origin{URL: "https://@github.com/o/r", Upstream: true}, true, "names no repository"},
			{"SSHWithPortHalts", "review", Origin{URL: "ssh://git@github.com:22/o/r.git", Upstream: true}, true, "names no repository"},
			{"GitSchemeHalts", "review", Origin{URL: "git://github.com/o/r", Upstream: true}, true, "names no repository"},
			{"UpperCaseHostHalts", "review", Origin{URL: "https://GitHub.com/o/r", Upstream: true}, true, "names no repository"},
			{"OtherHostUnderReviewIsNo", "review", Origin{URL: "ssh://git@gitlab.com/o/r.git", Upstream: true}, false, "gitlab.com"},
			{"NoOriginUnderReviewIsNo", "review", Origin{Upstream: true}, false, "no origin remote"},
			{"NoUpstreamUnderReviewIsNo", "review", Origin{URL: githubOrigin.URL}, false, "no upstream"},
			{"NoUpstreamUnderTrailHalts", "trail", Origin{URL: githubOrigin.URL}, true, "no upstream"},
			{"PushSkipUnderReviewIsNo", "review", Origin{URL: githubOrigin.URL, Upstream: true, PushSkipped: true}, false, "push skip"},
			{"PushSkipUnderTrailHalts", "trail", Origin{URL: githubOrigin.URL, Upstream: true, PushSkipped: true}, true, "push skip"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := ResolvePullRequest(tt.mode, NotABeam, false, tt.origin, true)
				if got.Open {
					t.Fatalf("ResolvePullRequest() = %+v; want no pull request", got)
				}
				text := got.Reason
				if tt.wantHalt {
					text = got.Halt
				}
				if missing := OriginMissing(tt.origin); missing == "" || !strings.Contains(text, missing) {
					t.Errorf("OriginMissing(%+v) = %q; want the missing fact the resolver's text %q names", tt.origin, missing, text)
				}
				if tt.wantHalt {
					if !strings.HasPrefix(got.Halt, haltStart) || !strings.Contains(got.Halt, tt.wantIn) || !strings.Contains(got.Halt, wayForward) {
						t.Errorf("Halt = %q; want it to start %q, hold %q and the way forward", got.Halt, haltStart, tt.wantIn)
					}
					if strings.Contains(got.Halt, "lym batten cancel") {
						t.Errorf("Halt = %q; want no cancel clause", got.Halt)
					}
					return
				}
				if got.Halt != "" || !strings.Contains(got.Reason, tt.wantIn) {
					t.Errorf("ResolvePullRequest() = %+v; want no halt and a reason holding %q", got, tt.wantIn)
				}
			})
		}
	})

	t.Run("Beam", func(t *testing.T) {
		t.Parallel()
		const recordDemand = "the beam's landing record holds a pull request"
		const noRecord = "the beam's landing record holds no pull request"
		tests := []struct {
			name     string
			mode     string
			beam     BeamRecord
			rule     bool
			origin   Origin
			wantOpen bool
			wantHalt bool
			// wantIn is a substring the reason, or the halt when one is wanted, holds.
			wantIn string
		}{
			{"RecordedPullRequestIsYesUnderOff", "off", BeamWithPullRequest, false, githubOrigin, true, false, recordDemand},
			{"RecordedPullRequestIsYesUnderReview", "review", BeamWithPullRequest, false, githubOrigin, true, false, recordDemand},
			{"RecordedPullRequestIsYesUnderMergeIn", "merge-in", BeamWithPullRequest, false, githubOrigin, true, false, recordDemand},
			{"NoneRecordedUnderReviewIsNo", "review", BeamWithoutPullRequest, false, githubOrigin, false, false, noRecord},
			{"NoneRecordedUnderOffIsNo", "off", BeamWithoutPullRequest, false, githubOrigin, false, false, noRecord},
			{"NoneRecordedUnderMergeInReadsAsOff", "merge-in", BeamWithoutPullRequest, false, githubOrigin, false, false, noRecord},
			{"NoneRecordedUnderTrailIsYes", "trail", BeamWithoutPullRequest, false, githubOrigin, true, false, "beam.pull_request for this landing is trail"},
			{"NoneRecordedUnderAParentRuleIsYes", "off", BeamWithoutPullRequest, true, githubOrigin, true, false, "GitHub's rules for the parent branch require one"},
			{"NoneRecordedUnderMergeInAndAParentRuleIsYes", "merge-in", BeamWithoutPullRequest, true, githubOrigin, true, false, "GitHub's rules for the parent branch require one"},
			{"RecordedPullRequestWithALocalOriginHalts", "off", BeamWithPullRequest, false, local, false, true, recordDemand},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := ResolvePullRequest(tt.mode, tt.beam, tt.rule, tt.origin, true)
				if got.Open != tt.wantOpen || (got.Halt != "") != tt.wantHalt || got.MergeAtOnce != tt.wantOpen || got.TrueMerge {
					t.Fatalf("ResolvePullRequest(%q, beam %d) = %+v; want Open and MergeAtOnce %v, halt %v, no TrueMerge", tt.mode, tt.beam, got, tt.wantOpen, tt.wantHalt)
				}
				text := got.Reason
				if tt.wantHalt {
					text = got.Halt
				}
				if !strings.Contains(text, tt.wantIn) {
					t.Errorf("ResolvePullRequest() text = %q; want it to hold %q", text, tt.wantIn)
				}
				if strings.Contains(got.Halt, "lym batten cancel") {
					t.Errorf("ResolvePullRequest() halt = %q; want no cancel clause for a beam", got.Halt)
				}
			})
		}
	})

	t.Run("EmptyTaskBranch", func(t *testing.T) {
		t.Parallel()
		const emptyClause = "the task branch has no commits beyond its parent"
		tests := []struct {
			name   string
			mode   string
			origin Origin
			// wantIn is a substring the reason holds.
			wantIn string
		}{
			{"ReviewIsNo", "review", githubOrigin, emptyClause},
			{"TrailIsNo", "trail", githubOrigin, emptyClause},
			{"TrailWithALocalOriginIsNoRatherThanAHalt", "trail", local, emptyClause},
			{"GitHubOriginNamingNoRepositoryIsNoRatherThanAHalt", "review", Origin{URL: "https://github.com/onlyowner", Upstream: true}, emptyClause},
			{"OffKeepsItsOffReason", "off", githubOrigin, "pull request off:"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := ResolvePullRequest(tt.mode, NotABeam, false, tt.origin, false)
				if got.Open || got.Halt != "" || !strings.Contains(got.Reason, tt.wantIn) {
					t.Errorf("ResolvePullRequest(%q, %+v, false) = %+v; want no pull request, no halt and a reason holding %q", tt.mode, tt.origin, got, tt.wantIn)
				}
			})
		}
	})
}
