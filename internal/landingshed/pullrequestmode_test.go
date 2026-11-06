// pullrequestmode_test.go covers ResolvePullRequest: the mode and field combinations, the warning for an off field under a required or merge parent, a parent branch GitHub's rules guard, each origin fact that blocks a pull request, and an empty task branch.

package landingshed

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestResolvePullRequest stays serial: logcapture redirects the process-global logger.
func TestResolvePullRequest(t *testing.T) {
	tiergate.Require(t, 0)
	const warnLine = "the entry's pull_request is off but the parent's beam.yaml pull_request is required or merge, or GitHub's rules for the parent branch require a pull request"
	githubOrigin := Origin{URL: "https://github.com/o/r.git", Upstream: true}

	t.Run("ModeAndFieldCombinations", func(t *testing.T) {
		tests := []struct {
			parent   string
			field    string
			wantOpen bool
		}{
			{"on", "", true}, {"on", "on", true}, {"on", "off", false},
			{"off", "", false}, {"off", "on", true}, {"off", "off", false},
			{"required", "", true}, {"required", "on", true}, {"required", "off", true},
			{"merge", "", true}, {"merge", "on", true}, {"merge", "off", true},
		}
		for _, tt := range tests {
			t.Run(tt.parent+"_"+tt.field, func(t *testing.T) {
				logs := logcapture.CaptureVerbose(t)
				got := ResolvePullRequest("slug", tt.parent, tt.field, NotABeam, false, githubOrigin, true)
				if got.Open != tt.wantOpen || got.Halt != "" || got.Reason == "" {
					t.Errorf("ResolvePullRequest(%q, %q) = %+v; want Open %v, no halt and a reason", tt.parent, tt.field, got, tt.wantOpen)
				}
				if wantAtOnce := tt.parent == "merge"; got.MergeAtOnce != wantAtOnce {
					t.Errorf("ResolvePullRequest(%q, %q).MergeAtOnce = %v; want %v", tt.parent, tt.field, got.MergeAtOnce, wantAtOnce)
				}
				wantWarn := (tt.parent == "required" || tt.parent == "merge") && tt.field == "off"
				if gotWarn := strings.Contains(logs.String(), warnLine); gotWarn != wantWarn {
					t.Errorf("ResolvePullRequest(%q, %q) warned = %v; want %v\n%s", tt.parent, tt.field, gotWarn, wantWarn, logs.String())
				}
			})
		}
		off := ResolvePullRequest("slug", "off", "", NotABeam, false, githubOrigin, true)
		if want := "pull request off: the parent's beam.yaml pull_request is off and the entry's pull_request is empty"; off.Reason != want {
			t.Errorf("off reason = %q; want %q", off.Reason, want)
		}
	})

	t.Run("MergeInFieldWantsNoPullRequestUnderAnyParentOrRule", func(t *testing.T) {
		for _, parent := range []string{"on", "off", "required", "merge"} {
			for _, rule := range []bool{false, true} {
				got := ResolvePullRequest("slug", parent, "merge-in", NotABeam, rule, githubOrigin, true)
				if got.Open || got.MergeAtOnce || got.Halt != "" || !got.TrueMerge || got.Reason == "" {
					t.Errorf("ResolvePullRequest(%q, merge-in, rule %v) = %+v; want a no marked TrueMerge, with a reason", parent, rule, got)
				}
			}
		}
		if got := ResolvePullRequest("slug", "required", "merge-in", BeamWithoutPullRequest, false, githubOrigin, true); got.TrueMerge || !got.Open {
			t.Errorf("ResolvePullRequest(required, merge-in, beam) = %+v; want a beam to ignore the field", got)
		}
	})

	t.Run("ParentRule", func(t *testing.T) {
		const ruleDemand = "GitHub's rules for the parent branch require a pull request"
		local := Origin{URL: "/srv/hub.git", Upstream: true}
		tests := []struct {
			name           string
			parent         string
			field          string
			origin         Origin
			taskHasCommits bool
			wantOpen       bool
			// wantIn is a substring the halt holds when it is not empty, and otherwise the reason.
			wantIn   string
			wantHalt bool
			wantWarn bool
		}{
			{name: "OffParentOpensOne", parent: "off", origin: githubOrigin, taskHasCommits: true, wantOpen: true, wantIn: "GitHub's rules for the parent branch require one"},
			{name: "FieldOffStillOpensOneAndWarns", parent: "on", field: "off", origin: githubOrigin, taskHasCommits: true, wantOpen: true, wantIn: "GitHub's rules for the parent branch require one", wantWarn: true},
			{name: "AMissingOriginFactHaltsWithNoCancelClause", parent: "off", field: "on", origin: local, taskHasCommits: true, wantHalt: true, wantIn: ruleDemand},
			{name: "AnEmptyTaskBranchIsNo", parent: "off", origin: githubOrigin, wantIn: "the task branch has no commits beyond its parent"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				logs := logcapture.CaptureVerbose(t)
				got := ResolvePullRequest("slug", tt.parent, tt.field, NotABeam, true, tt.origin, tt.taskHasCommits)
				if got.Open != tt.wantOpen || (got.Halt != "") != tt.wantHalt {
					t.Fatalf("ResolvePullRequest(%q, %q, rule) = %+v; want Open %v, halt %v", tt.parent, tt.field, got, tt.wantOpen, tt.wantHalt)
				}
				text := got.Reason
				if tt.wantHalt {
					text = got.Halt
				}
				if !strings.Contains(text, tt.wantIn) {
					t.Errorf("ResolvePullRequest() text = %q; want it to hold %q", text, tt.wantIn)
				}
				if strings.Contains(got.Halt, "lym batten cancel") {
					t.Errorf("Halt = %q; want no cancel clause under a rule that requires a pull request", got.Halt)
				}
				if gotWarn := strings.Contains(logs.String(), warnLine); gotWarn != tt.wantWarn {
					t.Errorf("warned = %v; want %v\n%s", gotWarn, tt.wantWarn, logs.String())
				}
			})
		}
	})

	t.Run("Origins", func(t *testing.T) {
		const haltStart = "a pull request is required here ("
		const wayForward = "way forward: give the hub a GitHub origin, an upstream for the parent branch and no push skip, then run lym loom resume"
		const cancelClause = ", or run lym batten cancel slug and start a new run with the entry's pull_request changed"
		if missing := OriginMissing(githubOrigin); missing != "" {
			t.Errorf("OriginMissing(%+v) = %q; want none for a complete GitHub origin", githubOrigin, missing)
		}
		local := Origin{URL: "/srv/hub.git", Upstream: true}
		tests := []struct {
			name     string
			parent   string
			field    string
			origin   Origin
			wantHalt bool
			// wantIn is a substring the reason, or the halt when one is wanted, holds.
			wantIn string
			// wantCancel is whether the halt carries the cancel clause.
			wantCancel bool
		}{
			{"LocalPathUnderOnIsNo", "on", "", local, false, "/srv/hub.git", false},
			{"LocalPathUnderRequiredHalts", "required", "", local, true, "the parent's beam.yaml pull_request is required", false},
			{"LocalPathUnderFieldOnHalts", "on", "on", local, true, "the entry's pull_request is on", true},
			{"LocalPathUnderFieldOnAndOffParentHalts", "off", "on", local, true, "the entry's pull_request is on", true},
			{"OwnerOnlyHalts", "on", "", Origin{URL: "https://github.com/onlyowner", Upstream: true}, true, "names no repository", false},
			{"EmptyUserInfoHalts", "on", "", Origin{URL: "https://@github.com/o/r", Upstream: true}, true, "names no repository", false},
			{"SSHWithPortHalts", "on", "", Origin{URL: "ssh://git@github.com:22/o/r.git", Upstream: true}, true, "names no repository", false},
			{"GitSchemeHalts", "on", "", Origin{URL: "git://github.com/o/r", Upstream: true}, true, "names no repository", false},
			{"UpperCaseHostHalts", "on", "", Origin{URL: "https://GitHub.com/o/r", Upstream: true}, true, "names no repository", false},
			{"OtherHostUnderOnIsNo", "on", "", Origin{URL: "ssh://git@gitlab.com/o/r.git", Upstream: true}, false, "gitlab.com", false},
			{"NoOriginUnderOnIsNo", "on", "", Origin{Upstream: true}, false, "no origin remote", false},
			{"NoUpstreamUnderOnIsNo", "on", "", Origin{URL: githubOrigin.URL}, false, "no upstream", false},
			{"NoUpstreamUnderRequiredHalts", "required", "", Origin{URL: githubOrigin.URL}, true, "no upstream", false},
			{"NoUpstreamUnderMergeHalts", "merge", "", Origin{URL: githubOrigin.URL}, true, "the parent's beam.yaml pull_request is merge", false},
			{"PushSkipUnderOnIsNo", "on", "", Origin{URL: githubOrigin.URL, Upstream: true, PushSkipped: true}, false, "push skip", false},
			{"PushSkipUnderRequiredHalts", "required", "", Origin{URL: githubOrigin.URL, Upstream: true, PushSkipped: true}, true, "push skip", false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				logcapture.CaptureVerbose(t)
				got := ResolvePullRequest("slug", tt.parent, tt.field, NotABeam, false, tt.origin, true)
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
					if gotCancel := strings.Contains(got.Halt, cancelClause); gotCancel != tt.wantCancel {
						t.Errorf("Halt = %q; cancel clause present = %v, want %v", got.Halt, gotCancel, tt.wantCancel)
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
		const recordDemand = "the beam's landing record holds a pull request"
		const noRecord = "the beam's landing record holds no pull request"
		local := Origin{URL: "/srv/hub.git", Upstream: true}
		tests := []struct {
			name     string
			parent   string
			field    string
			beam     BeamRecord
			rule     bool
			origin   Origin
			wantOpen bool
			wantHalt bool
			// wantIn is a substring the reason, or the halt when one is wanted, holds.
			wantIn string
		}{
			{"RecordedPullRequestIsYesUnderOff", "off", "", BeamWithPullRequest, false, githubOrigin, true, false, recordDemand},
			{"RecordedPullRequestIsYesUnderOn", "on", "", BeamWithPullRequest, false, githubOrigin, true, false, recordDemand},
			{"NoneRecordedUnderOnIsNo", "on", "", BeamWithoutPullRequest, false, githubOrigin, false, false, noRecord},
			{"NoneRecordedUnderOffIsNo", "off", "", BeamWithoutPullRequest, false, githubOrigin, false, false, noRecord},
			{"NoneRecordedUnderRequiredIsYes", "required", "", BeamWithoutPullRequest, false, githubOrigin, true, false, "the parent's beam.yaml pull_request is required"},
			{"NoneRecordedUnderMergeIsYes", "merge", "", BeamWithoutPullRequest, false, githubOrigin, true, false, "the parent's beam.yaml pull_request is merge"},
			{"NoneRecordedUnderAParentRuleIsYes", "off", "", BeamWithoutPullRequest, true, githubOrigin, true, false, "GitHub's rules for the parent branch require one"},
			{"EntryFieldOnIsNeverRead", "off", "on", BeamWithoutPullRequest, false, githubOrigin, false, false, noRecord},
			{"EntryFieldOffIsNeverReadUnderRequired", "required", "off", BeamWithoutPullRequest, false, githubOrigin, true, false, "required"},
			{"RecordedPullRequestWithALocalOriginHalts", "off", "", BeamWithPullRequest, false, local, false, true, recordDemand},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				logs := logcapture.CaptureVerbose(t)
				got := ResolvePullRequest("slug", tt.parent, tt.field, tt.beam, tt.rule, tt.origin, true)
				if got.Open != tt.wantOpen || (got.Halt != "") != tt.wantHalt || got.MergeAtOnce != tt.wantOpen {
					t.Fatalf("ResolvePullRequest(%q, %q, beam %d) = %+v; want Open and MergeAtOnce %v, halt %v", tt.parent, tt.field, tt.beam, got, tt.wantOpen, tt.wantHalt)
				}
				text := got.Reason
				if tt.wantHalt {
					text = got.Halt
				}
				if !strings.Contains(text, tt.wantIn) {
					t.Errorf("ResolvePullRequest() text = %q; want it to hold %q", text, tt.wantIn)
				}
				if strings.Contains(got.Halt, "lym batten cancel") || strings.Contains(logs.String(), warnLine) {
					t.Errorf("ResolvePullRequest() halt = %q, log = %q; want no cancel clause and no entry-field warning for a beam", got.Halt, logs.String())
				}
			})
		}
	})

	t.Run("EmptyTaskBranch", func(t *testing.T) {
		const emptyClause = "the task branch has no commits beyond its parent"
		tests := []struct {
			name   string
			parent string
			field  string
			origin Origin
			// wantIn is a substring the reason holds.
			wantIn string
		}{
			{"OnParentIsNo", "on", "", githubOrigin, emptyClause},
			{"FieldOnIsNo", "off", "on", githubOrigin, emptyClause},
			{"RequiredParentIsNo", "required", "", githubOrigin, emptyClause},
			{"RequiredParentWithALocalOriginIsNoRatherThanAHalt", "required", "", Origin{URL: "/srv/hub.git", Upstream: true}, emptyClause},
			{"GitHubOriginNamingNoRepositoryIsNoRatherThanAHalt", "on", "", Origin{URL: "https://github.com/onlyowner", Upstream: true}, emptyClause},
			{"OffParentKeepsItsOffReason", "off", "", githubOrigin, "pull request off:"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				logcapture.CaptureVerbose(t)
				got := ResolvePullRequest("slug", tt.parent, tt.field, NotABeam, false, tt.origin, false)
				if got.Open || got.Halt != "" || !strings.Contains(got.Reason, tt.wantIn) {
					t.Errorf("ResolvePullRequest(%q, %q, %+v, false) = %+v; want no pull request, no halt and a reason holding %q", tt.parent, tt.field, tt.origin, got, tt.wantIn)
				}
			})
		}
	})
}
