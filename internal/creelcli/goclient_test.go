//go:build tier0

// goclient_test.go pins the program claim a Go creel client signs with.

package creelcli

import (
	"strings"
	"testing"
)

func TestProgramClaim(t *testing.T) {
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
