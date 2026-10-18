// identity_test.go pins how a request's sender is resolved from the peer's process ancestry and its claim.

package creelengine

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveSender(t *testing.T) {
	t.Parallel()
	const (
		linux = "linux"
		mac   = "darwin"
	)
	live := func(name string, pid int) Strand { return Strand{Name: name, Live: true, PanePID: pid} }
	strands := []Strand{live(alpha, 100), live(beta, 200), live(orch, 300)}
	// ancestry maps a process to its parent: 1001 and 1002 sit under alpha's pane, 3001 under the orch's, 5000 under no pane.
	ancestry := map[int]int{1001: 1000, 1000: 100, 100: 10, 3001: 300, 300: 10, 5000: 4000, 4000: 1}
	parentPID := func(pid int) (int, bool) {
		parent, ok := ancestry[pid]
		return parent, ok
	}
	envelope := Claim{Terminal: "pts/4", ParentProcess: "claude", WorkingDir: "/home/me/x"}
	external, err := ExternalSender("ly", "pts/4", "claude", "/home/me/x")
	if err != nil {
		t.Fatalf("ExternalSender: %v", err)
	}
	withClaim := func(base Claim, strandName, program string) Claim {
		base.StrandName, base.Program = strandName, program
		return base
	}

	tests := []struct {
		name       string
		goos       string
		peer       int
		claim      Claim
		strands    []Strand
		strandsErr error
		want       Sender
		wantErr    string
	}{
		{"Linux_PaneProcessWithNoEnvironmentIsItsStrand", linux, 1001, Claim{}, strands, nil, StrandSender(alpha), ""},
		{"Linux_PaneProcessWithMatchingEnvironmentIsItsStrand", linux, 1001, Claim{StrandName: alpha}, strands, nil, StrandSender(alpha), ""},
		{"Linux_PeerThatIsThePaneProcessIsItsStrand", linux, 100, Claim{}, strands, nil, StrandSender(alpha), ""},
		{"Linux_DisagreeingStrandNameIsRefused", linux, 1001, Claim{StrandName: beta}, strands, nil, Sender{}, "runs in the pane of " + alpha},
		{"Linux_OutsidePeerClaimingAStrandNameIsRefused", linux, 5000, Claim{StrandName: alpha}, strands, nil, Sender{}, "runs in no strand pane"},
		{"Linux_ProgramClaimFromANonOrchPaneIsRefused", linux, 1001, withClaim(Claim{}, "", "ly:BATTEN"), strands, nil, Sender{}, "only an orch strand's pane"},
		{"Linux_ProgramClaimFromAnOrchPaneIsAccepted", linux, 3001, withClaim(Claim{}, orch, "ly:BATTEN"), strands, nil, ProgramSender("ly:BATTEN"), ""},
		{"Linux_ProgramClaimFromOutsideEveryPaneIsAccepted", linux, 5000, withClaim(Claim{}, "", "ly:BATTEN"), strands, nil, ProgramSender("ly:BATTEN"), ""},
		{"Linux_ProgramClaimUnderAnotherShortnameIsRefused", linux, 5000, withClaim(Claim{}, "", "other:BATTEN"), strands, nil, Sender{}, "shortname is ly"},
		{"Linux_ProgramClaimOfExternalIsRefused", linux, 5000, withClaim(Claim{}, "", "ly:EXTERNAL"), strands, nil, Sender{}, "reserved"},
		{"Linux_MalformedProgramClaimIsRefused", linux, 5000, withClaim(Claim{}, "", "ly:batten"), strands, nil, Sender{}, "program"},
		{"Linux_PeerInNoKnownPaneWhileAPanePIDIsUnknownIsRefused", linux, 5000, Claim{}, []Strand{live(alpha, 100), live(beta, 0)}, nil, Sender{}, "pane process of " + beta},
		{"Linux_PeerInNoKnownPaneWhileTheStrandListFailsIsRefused", linux, 5000, Claim{}, nil, errors.New("reed state of /hub/x is unreadable"), Sender{}, "reed state of /hub/x is unreadable"},
		{"Linux_PeerInAKnownPaneResolvesDespiteAnUnknownPanePID", linux, 1001, Claim{}, []Strand{live(alpha, 100), live(beta, 0)}, nil, StrandSender(alpha), ""},
		{"Linux_DormantStrandPanePIDIsNotRequired", linux, 5000, envelope, []Strand{live(alpha, 100), {Name: beta}}, nil, external, ""},
		{"Linux_PlainOutsidePeerIsExternalWithItsEnvelope", linux, 5000, envelope, strands, nil, external, ""},
		{"Off_EnvironmentStrandNameOfThisHubIsTheSender", mac, 0, Claim{StrandName: alpha}, strands, nil, StrandSender(alpha), ""},
		{"Off_StrandNameOfAnotherShortnameFallsThrough", mac, 0, withClaim(envelope, "other:x:driver", ""), strands, nil, external, ""},
		{"Off_ProgramClaimIsAccepted", mac, 0, withClaim(Claim{}, "", "ly:BATTEN"), strands, nil, ProgramSender("ly:BATTEN"), ""},
		{"Off_ProgramClaimOfExternalIsRefused", mac, 0, withClaim(Claim{}, "", "ly:EXTERNAL"), strands, nil, Sender{}, "reserved"},
		{"Off_NothingClaimedIsExternal", mac, 0, envelope, strands, nil, external, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveSender(tt.peer, tt.claim, "ly", tt.strands, tt.strandsErr, parentPID, tt.goos)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveSender = %+v, %v; want an error containing %q", got, err, tt.wantErr)
				}
				// Only the refusal of a claim from a process in no pane wraps the sentinel: the other-pane and unknown-pane-process refusals do not.
				outside := strings.Contains(tt.wantErr, "runs in no strand pane")
				if got := errors.Is(err, ErrStrandClaimOutsidePane); got != outside {
					t.Errorf("errors.Is(%v, ErrStrandClaimOutsidePane) = %v; want %v", err, got, outside)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveSender: %v", err)
			}
			if got != tt.want {
				t.Errorf("resolveSender = %+v; want %+v", got, tt.want)
			}
		})
	}

	t.Run("InvalidShortnameCannotNameAnExternalSender", func(t *testing.T) {
		t.Parallel()
		for _, goos := range []string{linux, mac} {
			got, err := resolveSender(5000, envelope, "LY", strands, nil, parentPID, goos)
			if err == nil || !strings.Contains(err.Error(), "shortname") {
				t.Errorf("resolveSender(%s, shortname LY) = %+v, %v; want a shortname error", goos, got, err)
			}
		}
	})
}
