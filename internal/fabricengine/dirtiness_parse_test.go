// dirtiness_parse_test.go covers, with no git, how `git status --porcelain -z` output becomes dirty tracked paths.

package fabricengine

import (
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestParseTrackedDirtyPaths covers the NUL-separated status parse:
// an empty status lists nothing, a path comes back verbatim under the directory,
// and a rename or copy record names its destination while its trailing source record is skipped.
func TestParseTrackedDirtyPaths(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const dir = "/wt"
	tests := []struct {
		name   string
		stdout string
		want   []string
	}{
		{"CleanSource", "", nil},
		{"ModifiedFile", " M tracked.txt\x00", []string{"/wt/tracked.txt"}},
		{"StagedAndModifiedFiles", "M  a.txt\x00 M sub/b.txt\x00", []string{"/wt/a.txt", "/wt/sub/b.txt"}},
		{"PathWithSpaceAndNonASCII", " M a b/æø.txt\x00", []string{"/wt/a b/æø.txt"}},
		{"RenameSkipsItsSourceRecord", "R  new.txt\x00old.txt\x00 M other.txt\x00", []string{"/wt/new.txt", "/wt/other.txt"}},
		{"CopySkipsItsSourceRecord", "C  copy.txt\x00orig.txt\x00", []string{"/wt/copy.txt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var want []string
			for _, path := range tt.want {
				want = append(want, filepath.FromSlash(path))
			}
			if diff := cmp.Diff(want, parseTrackedDirtyPaths(filepath.FromSlash(dir), tt.stdout)); diff != "" {
				t.Errorf("parseTrackedDirtyPaths(%q) mismatch (-want +got):\n%s", tt.stdout, diff)
			}
		})
	}
}
