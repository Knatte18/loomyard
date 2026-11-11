// goclient_test.go pins the program claim a Go creel client signs with.

package creelcli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestGoClient_DropRunOnAHubWithNoJournalAnswersNothingAndConnectsToNoDaemon(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// The client has no connection behind it, so any call it made would panic.
	client := &GoClient{journalDir: filepath.Join(t.TempDir(), "journal")}
	dropped, err := client.DropRun("pay")
	if dropped != nil || err != nil {
		t.Errorf("DropRun on a hub with no journal directory = %v, %v; want no messages and no error", dropped, err)
	}
}

func TestProgramClaim(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name          string
		slug, program string
		want          string
		wantErr       string
	}{
		{"EmptySlugClaimsTheTwoSegmentProgram", "", "BATTEN", "ly:BATTEN", ""},
		{"SlugClaimsTheScopedProgram", "pay", "DOBBY", "ly:pay:DOBBY", ""},
		{"BadProgramNameIsWrapped", "", "batten", "", `the program name "batten" is not usable`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			claim, err := programClaim(tt.slug, tt.program)("ly")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("programClaim(%q, %q) error = %v; want it to contain %q", tt.slug, tt.program, err, tt.wantErr)
				}
				return
			}
			if err != nil || claim.Program != tt.want || claim.StrandName != "" {
				t.Errorf("programClaim(%q, %q) = %+v, %v; want program %q and no strand", tt.slug, tt.program, claim, err, tt.want)
			}
		})
	}
}
