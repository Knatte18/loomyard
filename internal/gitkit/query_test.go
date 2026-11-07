// query_test.go covers the git version parsing behind SkipBelowGitVersion.

package gitkit

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestGitVersionGate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name        string
		output      string
		wantVersion string
		minimum     string
		wantBelow   bool
	}{
		{"Plain", "git version 2.53.0\n", "2.53.0", "2.48", false},
		{"WindowsSuffix", "git version 2.47.1.windows.1\n", "2.47.1", "2.48", true},
		{"AppleSuffix", "git version 2.39.5 (Apple Git-154)\n", "2.39.5", "2.48", true},
		{"ExactMinimum", "git version 2.48.0\n", "2.48.0", "2.48", false},
		{"NumericNotLexical", "git version 2.100.0\n", "2.100.0", "2.48", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseGitVersion(tt.output)
			if got != tt.wantVersion {
				t.Errorf("parseGitVersion(%q) = %q; want %q", tt.output, got, tt.wantVersion)
			}
			if below := compareDottedVersions(got, tt.minimum) < 0; below != tt.wantBelow {
				t.Errorf("compareDottedVersions(%q, %q) < 0 = %t; want %t", got, tt.minimum, below, tt.wantBelow)
			}
		})
	}
}
