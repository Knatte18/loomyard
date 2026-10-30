//go:build tier0

// pullrequestmode_test.go covers ResolvePullRequest: the nine mode and field combinations, the warning for an off field under a required parent, each origin fact that blocks a pull request, and an empty task branch.

package landingshed

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestResolvePullRequest stays serial: logcapture redirects the process-global logger.
func TestResolvePullRequest(t *testing.T) {
	const warnLine = "the entry's pull_request is off but the parent's beam.yaml pull_request is required"
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
		}
		for _, tt := range tests {
			t.Run(tt.parent+"_"+tt.field, func(t *testing.T) {
				logs := logcapture.CaptureVerbose(t)
				got := ResolvePullRequest("slug", tt.parent, tt.field, githubOrigin, true)
				if got.Open != tt.wantOpen || got.Halt != "" || got.Reason == "" {
					t.Errorf("ResolvePullRequest(%q, %q) = %+v; want Open %v, no halt and a reason", tt.parent, tt.field, got, tt.wantOpen)
				}
				wantWarn := tt.parent == "required" && tt.field == "off"
				if gotWarn := strings.Contains(logs.String(), warnLine); gotWarn != wantWarn {
					t.Errorf("ResolvePullRequest(%q, %q) warned = %v; want %v\n%s", tt.parent, tt.field, gotWarn, wantWarn, logs.String())
				}
			})
		}
		off := ResolvePullRequest("slug", "off", "", githubOrigin, true)
		if want := "pull request off: the parent's beam.yaml pull_request is off and the entry's pull_request is empty"; off.Reason != want {
			t.Errorf("off reason = %q; want %q", off.Reason, want)
		}
	})

	t.Run("Origins", func(t *testing.T) {
		const haltStart = "a pull request is required here ("
		const wayForward = "way forward: give the hub a GitHub origin, an upstream for the parent branch and no push skip, then run lym loom resume"
		const cancelClause = ", or run lym batten cancel slug and start a new run with the entry's pull_request changed"
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
			{"PushSkipUnderOnIsNo", "on", "", Origin{URL: githubOrigin.URL, Upstream: true, PushSkipped: true}, false, "push skip", false},
			{"PushSkipUnderRequiredHalts", "required", "", Origin{URL: githubOrigin.URL, Upstream: true, PushSkipped: true}, true, "push skip", false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				logcapture.CaptureVerbose(t)
				got := ResolvePullRequest("slug", tt.parent, tt.field, tt.origin, true)
				if got.Open {
					t.Fatalf("ResolvePullRequest() = %+v; want no pull request", got)
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
				got := ResolvePullRequest("slug", tt.parent, tt.field, tt.origin, false)
				if got.Open || got.Halt != "" || !strings.Contains(got.Reason, tt.wantIn) {
					t.Errorf("ResolvePullRequest(%q, %q, %+v, false) = %+v; want no pull request, no halt and a reason holding %q", tt.parent, tt.field, tt.origin, got, tt.wantIn)
				}
			})
		}
	})
}
