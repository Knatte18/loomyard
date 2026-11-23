// clone_onerepository_test.go proves CloneHub's refusals of a one-URL clone, before it creates anything.
// Each row aims the clone at a bare repository built here and asserts the way forward the refusal names.
// It also proves the refusal of a clone into a directory that is not there.
// It shares the fixture helpers of clone_adopt_test.go and the TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCloneHub_OneURLRefusals asserts that each refused one-URL clone names its way forward and creates nothing.
// The container holds no hub directory and no probe directory after any refusal.
func TestCloneHub_OneURLRefusals(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const codeURL = "https://example.com/code"

	tests := []struct {
		name string
		// prepare shapes the origin the clone is aimed at and returns the bare repository's path.
		prepare func(t *testing.T, fixtures string) string
		// singleRepo, shortname and secondURL set the clone's options; the origin is its first URL.
		singleRepo bool
		shortname  string
		secondURL  bool
		// wantErrorHas are substrings of the refusal.
		wantErrorHas []string
	}{
		{"EarlierLayoutWithoutFlag", withEarlierLayoutBranch, false, "", false,
			[]string{"delete lym-bolt on origin", "then clone again"}},
		{"EarlierLayoutWithFlag", withEarlierLayoutBranch, true, "tst", false,
			[]string{"delete lym-bolt on origin", "then clone again"}},
		{"SeedThatStoppedBeforeItsBoard", func(t *testing.T, fixtures string) string {
			bare := makeBareRemote(t, fixtures, "stopped-seed")
			gitkit.Git(t, bare, "update-ref", "refs/lym/heads/main-weft", "main")
			return bare
		}, false, "", false, []string{"a one-repository seed stopped before its board landed", "lym fabric clone --single-repo --shortname <shortname>"}},
		{"BoardAlreadyOnOrigin", func(t *testing.T, fixtures string) string {
			bare := makeBareRemote(t, fixtures, "board-present")
			gitkit.Git(t, bare, "update-ref", "refs/lym/heads/main", "main")
			return bare
		}, true, "tst", false, []string{"without --single-repo"}},
		{"RecordsRepository", func(t *testing.T, fixtures string) string {
			bare := makeBareRemote(t, fixtures, "records-repository")
			commitFileOnBranch(t, fixtures, bare, "main", fabricengine.WarpBindingFileName, codeURL+"\n")
			return bare
		}, true, "tst", false, []string{"lym fabric clone <records-url> <code-url>"}},
		{"TwoRepositoryWeftTrackingTheLegacyAnchor", func(t *testing.T, fixtures string) string {
			bare := makeBareRemote(t, fixtures, "records-with-anchor")
			commitFileOnBranch(t, fixtures, bare, "main", fabricengine.WarpBindingFileName, codeURL+"\n")
			commitFileOnBranch(t, fixtures, bare, "main", lymcwd.AnchorFileName, ".\n")
			return bare
		}, true, "tst", false, []string{"lym fabric clone <records-url> <code-url>"}},
		{"DefaultBranchTracksTheAnchor", func(t *testing.T, fixtures string) string {
			bare := makeBareRemote(t, fixtures, "tracks-anchor")
			commitFileOnBranch(t, fixtures, bare, "main", lymcwd.AnchorFileName, ".\n")
			return bare
		}, true, "tst", false, []string{"remove " + lymcwd.AnchorFileName + " and " + lymdirs.LymDirName + " from the code branches"}},
		{"DefaultBranchTracksTheLymDirectory", func(t *testing.T, fixtures string) string {
			bare := makeBareRemote(t, fixtures, "tracks-lym-dir")
			commitFileOnBranch(t, fixtures, bare, "main", lymdirs.LymDirName+"/placeholder.txt", "x\n")
			return bare
		}, true, "tst", false, []string{"remove " + lymcwd.AnchorFileName + " and " + lymdirs.LymDirName + " from the code branches"}},
		{"NoShortname", func(t *testing.T, fixtures string) string {
			return makeBareRemote(t, fixtures, "no-shortname")
		}, true, "", false, []string{"--shortname"}},
		{"SecondURL", func(t *testing.T, fixtures string) string {
			return makeBareRemote(t, fixtures, "second-url")
		}, true, "tst", true, []string{"give --single-repo one URL, the code repository"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			origin := filepath.ToSlash(tt.prepare(t, t.TempDir()))
			cloneParent := t.TempDir()
			opts := fabricengine.CloneOptions{WeftURL: origin, SingleRepo: tt.singleRepo, Shortname: tt.shortname}
			if tt.secondURL {
				opts.WarpURL = origin
			}

			_, err := fabricengine.CloneHub(cloneParent, opts)
			if err == nil {
				t.Fatalf("CloneHub(%+v) = nil error; want a refusal", opts)
			}
			for _, want := range tt.wantErrorHas {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("CloneHub(%+v) error = %q; want it to contain %q", opts, err.Error(), want)
				}
			}
			entries, readErr := os.ReadDir(cloneParent)
			if readErr != nil {
				t.Fatalf("read clone parent %s: %v", cloneParent, readErr)
			}
			if len(entries) != 0 {
				t.Errorf("clone parent holds %d entries after the refusal; want no hub and no probe directory", len(entries))
			}
		})
	}
}

// TestCloneHub_RefusesAParentDirectoryThatIsNotThere asserts that a clone into a missing directory names that directory.
// It creates nothing, and its error is not the failed start of a git that was never at fault.
func TestCloneHub_RefusesAParentDirectoryThatIsNotThere(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	cloneParent := filepath.Join(t.TempDir(), "missing")

	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{WeftURL: "https://example.com/code"})

	if err == nil {
		t.Fatalf("CloneHub(%s) = nil error; want a refusal", cloneParent)
	}
	for _, want := range []string{cloneParent, "pass --into an existing directory"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("CloneHub(%s) error = %q; want it to contain %q", cloneParent, err.Error(), want)
		}
	}
	if strings.Contains(err.Error(), "fork/exec") {
		t.Errorf("CloneHub(%s) error = %q; want no failed start of git", cloneParent, err.Error())
	}
	if _, statErr := os.Stat(cloneParent); !os.IsNotExist(statErr) {
		t.Errorf("stat %s after the refusal = %v; want it still missing", cloneParent, statErr)
	}
}

// withEarlierLayoutBranch builds a bare repository whose branches include the board branch of the earlier layout.
func withEarlierLayoutBranch(t *testing.T, fixtures string) string {
	t.Helper()
	bare := makeBareRemote(t, fixtures, "earlier-layout")
	gitkit.Git(t, bare, "branch", "lym-bolt", "main")
	return bare
}
