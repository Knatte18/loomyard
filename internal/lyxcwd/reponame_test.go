// reponame_test.go covers hubSuffix trimming in buildLocation, pinning the clean-break behaviour:
// the current -LYXHUB suffix is trimmed from Location.RepoName, the retired -HUB suffix is not, and
// a hub basename with no suffix at all is left untouched.

package lyxcwd

import (
	"path/filepath"
	"testing"
)

// TestBuildLocation_RepoNameSuffixTrimming covers hubSuffix trimming in buildLocation across three hub container basenames: the current suffix, the retired suffix, and no suffix at all.
// It calls buildLocation directly with applyGate set to false, so it spawns no git and touches no disk.
//
//testtiming:keep pins that only the current hub suffix is trimmed from RepoName and the retired one is not, which the resolution scenario covering its blocks never reads
func TestBuildLocation_RepoNameSuffixTrimming(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		hubBasename  string
		wantRepoName string
	}{
		{
			name:         "current suffix is trimmed",
			hubBasename:  "loomyard-LYXHUB",
			wantRepoName: "loomyard",
		},
		{
			// This is the documented, accepted degradation of the clean break, not a bug:
			// nothing parses the retired suffix any more, and RepoName is display-only.
			name:         "retired suffix is not trimmed",
			hubBasename:  "loomyard-HUB",
			wantRepoName: "loomyard-HUB",
		},
		{
			name:         "no suffix present is a no-op",
			hubBasename:  "loomyard",
			wantRepoName: "loomyard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hubPath := filepath.Join("home", "user", tt.hubBasename)
			workTreeRoot := filepath.Join(hubPath, "worktree")
			cwd := workTreeRoot

			got, err := buildLocation(cwd, workTreeRoot, hubPath, ".", false)
			if err != nil {
				t.Fatalf("buildLocation(%q, %q, %q, %q, false) returned error: %v", cwd, workTreeRoot, hubPath, ".", err)
			}
			if got.RepoName != tt.wantRepoName {
				t.Errorf("buildLocation(...).RepoName = %q; want %q", got.RepoName, tt.wantRepoName)
			}
		})
	}
}
