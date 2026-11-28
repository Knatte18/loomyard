// onerepositoryclone_integration_test.go drives CloneAndWire over local bare remotes for the one-repository layout.
// It covers a fresh origin cloned with SingleRepo and the refusals that leave no hub behind.
// It also covers the one-URL clone over an origin that already holds the board branch.
// Last, it covers the board commit's exclusion of hub scratch.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// oneRepositoryFixtureName is the name of the code repository every row clones, which is also its hub's name.
const oneRepositoryFixtureName = "clonecli-warp"

// cloneOneRepository clones the bare at bareURL with SingleRepo into parent through fabriccli.CloneAndWire.
func cloneOneRepository(bareURL, parent string, singleRepo bool) (fabricengine.CloneResult, error) {
	return fabriccli.CloneAndWire(parent, fabricengine.CloneOptions{WeftURL: filepath.ToSlash(bareURL), Shortname: "tst", SingleRepo: singleRepo})
}

// plantPreReceive installs a pre-receive hook in bare that prints message on stderr and refuses every push.
func plantPreReceive(t *testing.T, bare, message string) {
	t.Helper()

	hook := "#!/bin/sh\necho '" + message + "' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bare, "hooks", "pre-receive"), []byte(hook), 0o755); err != nil {
		t.Fatalf("plant the pre-receive hook in %s: %v", bare, err)
	}
}

// requireNothingLeft fails the test when parent holds anything, a hub directory or a probe clone.
func requireNothingLeft(t *testing.T, parent string) {
	t.Helper()

	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("read %s: %v", parent, err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if len(names) > 0 {
		t.Errorf("the clone parent holds %v; want a refused clone to leave nothing behind", names)
	}
}

// requireRealDirectory fails the test unless path is a directory that is not a link.
func requireRealDirectory(t *testing.T, path string) {
	t.Helper()

	info, err := os.Lstat(path)
	if err != nil {
		t.Errorf("Lstat(%s) error = %v; want a real directory", path, err)
		return
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("%s has mode %v; want a real directory", path, info.Mode())
	}
}

// TestCloneAndWire_OneRepository drives the one-repository clone over bare remotes built with gitkit.
// A hook planted in a bare stands for a remote that refuses the clone's push.
func TestCloneAndWire_OneRepository(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("SingleRepoOnAFreshOrigin", func(t *testing.T) {
		t.Parallel()
		bare := makeCLICloneWarpBare(t, t.TempDir(), oneRepositoryFixtureName)
		res, err := cloneOneRepository(bare, t.TempDir(), true)
		if err != nil {
			t.Fatalf("CloneAndWire(SingleRepo) error = %v; want nil", err)
		}
		if !res.OneRepository {
			t.Errorf("CloneResult.OneRepository = false; want true")
		}
		prime := res.PrimeCwd

		if got := gitkit.Git(t, bare, "show", "main:"+lymcwd.AnchorFileName); got != "." {
			t.Errorf("origin's main holds %s %q; want %q", lymcwd.AnchorFileName, got, ".")
		}
		originTip := gitkit.RevParse(t, bare, "main")
		if !gitkit.IsAncestor(t, prime, originTip, "HEAD") {
			t.Errorf("origin's main %s is not in the prime's history; want the prime's own commits pushed", originTip)
		}
		if subjects := gitkit.Git(t, bare, "log", "--format=%s", "main"); !strings.Contains(subjects, "fabric clone: record anchor") {
			t.Errorf("origin's main history is %q; want it to hold the commit that records the anchor", subjects)
		}
		if !gitkit.BranchExists(t, bare, fabricengine.BoltBranchName) {
			t.Errorf("origin has no %s branch; want the board branch pushed", fabricengine.BoltBranchName)
		}
		if got := gitkit.CurrentBranch(t, res.BoardDir); got != fabricengine.BoltBranchName {
			t.Errorf("the board is on %q; want %q", got, fabricengine.BoltBranchName)
		}
		if got := gitkit.Git(t, prime, "config", "--get", fabricengine.PrimeBranchConfigKey); got != "main" {
			t.Errorf("%s = %q; want %q", fabricengine.PrimeBranchConfigKey, got, "main")
		}

		for _, name := range []string{lymdirs.LymDirName, lymdirs.DotLymDirName} {
			requireRealDirectory(t, filepath.Join(prime, name))
		}
		lines := gitkit.ExcludeLines(t, prime)
		wantExcluded := "/" + lymdirs.DotLymDirName
		found := false
		for _, line := range lines {
			found = found || line == wantExcluded
			for _, forbidden := range []string{lymdirs.LymDirName, lymcwd.AnchorFileName, ".weft", "lock"} {
				if strings.Contains(line, forbidden) {
					t.Errorf("the exclude file holds %q; want no %s line", line, forbidden)
				}
			}
		}
		if !found {
			t.Errorf("the exclude file holds %v; want it to hold %q", lines, wantExcluded)
		}
	})

	t.Run("SingleRepoRefusesAnOriginHoldingLymBolt", func(t *testing.T) {
		t.Parallel()
		bare := makeCLICloneWarpBare(t, t.TempDir(), oneRepositoryFixtureName)
		gitkit.Git(t, bare, "branch", fabricengine.BoltBranchName, "main")
		parent := t.TempDir()

		_, err := cloneOneRepository(bare, parent, true)
		if err == nil || !strings.Contains(err.Error(), "--single-repo refused") || !strings.Contains(err.Error(), fabricengine.BoltBranchName) || !strings.Contains(err.Error(), "way forward:") {
			t.Fatalf("CloneAndWire(SingleRepo) error = %v; want a refusal naming the flag, %s and a way forward", err, fabricengine.BoltBranchName)
		}
		requireNothingLeft(t, parent)
	})

	t.Run("SingleRepoRefusesAWeftBinding", func(t *testing.T) {
		t.Parallel()
		fixtures := t.TempDir()
		bare := makeCLICloneWarpBare(t, fixtures, oneRepositoryFixtureName)
		work := filepath.Join(fixtures, "work")
		gitkit.Git(t, fixtures, "clone", bare, work)
		gitkit.CommitFile(t, work, fabricengine.WarpBindingFileName, "https://example.com/code.git\n", "bind a records repository")
		gitkit.Git(t, work, "push", "origin", "main")
		parent := t.TempDir()

		_, err := cloneOneRepository(bare, parent, true)
		if err == nil || !strings.Contains(err.Error(), "--single-repo refused") || !strings.Contains(err.Error(), fabricengine.WarpBindingFileName) || !strings.Contains(err.Error(), "way forward:") {
			t.Fatalf("CloneAndWire(SingleRepo) error = %v; want a refusal naming the flag, %s and a way forward", err, fabricengine.WarpBindingFileName)
		}
		requireNothingLeft(t, parent)
	})

	t.Run("SingleRepoRefusesASecondURL", func(t *testing.T) {
		t.Parallel()
		fixtures := t.TempDir()
		bare := makeCLICloneWarpBare(t, fixtures, oneRepositoryFixtureName)
		second := makeCLICloneWarpBare(t, fixtures, "clonecli-second")
		parent := t.TempDir()

		_, err := fabriccli.CloneAndWire(parent, fabricengine.CloneOptions{WeftURL: filepath.ToSlash(bare), WarpURL: filepath.ToSlash(second), Shortname: "tst", SingleRepo: true})
		if err == nil || !strings.Contains(err.Error(), "--single-repo") || !strings.Contains(err.Error(), "way forward:") {
			t.Fatalf("CloneAndWire(SingleRepo, two URLs) error = %v; want a refusal naming the flag and a way forward", err)
		}
		requireNothingLeft(t, parent)
	})

	pushRefusals := []struct {
		name     string
		message  string
		wantText []string
		wantNot  string
	}{
		{
			name:     "ProtectedMarkerPushLeavesNothing",
			message:  "remote: error: GH006: Protected branch update failed for refs/heads/main.",
			wantText: []string{"protected", "allow direct pushes to main", "way forward:"},
		},
		{
			name:     "RefusedMarkerPushLeavesNothing",
			message:  "remote: error: the repository takes no pushes today",
			wantText: []string{"the repository takes no pushes today", "settle what the error names", "way forward:"},
			wantNot:  "protected",
		},
	}
	for _, tt := range pushRefusals {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bare := makeCLICloneWarpBare(t, t.TempDir(), oneRepositoryFixtureName)
			mainBefore := gitkit.RevParse(t, bare, "main")
			plantPreReceive(t, bare, tt.message)
			parent := t.TempDir()

			_, err := cloneOneRepository(bare, parent, true)
			if err == nil {
				t.Fatalf("CloneAndWire(SingleRepo) error = nil; want the refused push")
			}
			for _, want := range tt.wantText {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("CloneAndWire(SingleRepo) error = %q; want it to contain %q", err, want)
				}
			}
			if tt.wantNot != "" && strings.Contains(strings.ToLower(err.Error()), tt.wantNot) {
				t.Errorf("CloneAndWire(SingleRepo) error = %q; want it not to contain %q", err, tt.wantNot)
			}
			if gitkit.BranchExists(t, bare, fabricengine.BoltBranchName) {
				t.Errorf("origin has a %s branch after a refused push; want none", fabricengine.BoltBranchName)
			}
			if got := gitkit.RevParse(t, bare, "main"); got != mainBefore {
				t.Errorf("origin's main = %s after a refused push; want it to stay %s", got, mainBefore)
			}
			requireNothingLeft(t, parent)
		})
	}

	t.Run("OneURLOverLymBolt", func(t *testing.T) {
		t.Parallel()
		bare := makeCLICloneWarpBare(t, t.TempDir(), oneRepositoryFixtureName)
		if _, err := cloneOneRepository(bare, t.TempDir(), true); err != nil {
			t.Fatalf("CloneAndWire(SingleRepo) error = %v; want nil", err)
		}

		res, err := cloneOneRepository(bare, t.TempDir(), false)
		if err != nil {
			t.Fatalf("CloneAndWire(one URL over %s) error = %v; want nil", fabricengine.BoltBranchName, err)
		}
		if !res.OneRepository {
			t.Errorf("CloneResult.OneRepository = false; want the one-repository layout")
		}
		if got := gitkit.Git(t, res.PrimeCwd, "config", "--get", fabricengine.PrimeBranchConfigKey); got != "main" {
			t.Errorf("%s = %q; want %q", fabricengine.PrimeBranchConfigKey, got, "main")
		}
		if got := gitkit.CurrentBranch(t, res.BoardDir); got != fabricengine.BoltBranchName {
			t.Errorf("the board is on %q; want %q", got, fabricengine.BoltBranchName)
		}
		if got, want := gitkit.Git(t, res.BoardDir, "rev-parse", "--abbrev-ref", fabricengine.BoltBranchName+"@{upstream}"), "origin/"+fabricengine.BoltBranchName; got != want {
			t.Errorf("the board's upstream = %q; want %q", got, want)
		}
		requireRealDirectory(t, filepath.Join(res.PrimeCwd, lymdirs.DotLymDirName))
	})

	t.Run("OneURLOverLymBoltWithoutAMarkerRefuses", func(t *testing.T) {
		t.Parallel()
		bare := makeCLICloneWarpBare(t, t.TempDir(), oneRepositoryFixtureName)
		gitkit.Git(t, bare, "branch", fabricengine.BoltBranchName, "main")
		parent := t.TempDir()

		_, err := cloneOneRepository(bare, parent, false)
		if err == nil || !strings.Contains(err.Error(), lymcwd.AnchorFileName) || !strings.Contains(err.Error(), "way forward:") {
			t.Fatalf("CloneAndWire(one URL over %s) error = %v; want a refusal naming %s and a way forward", fabricengine.BoltBranchName, err, lymcwd.AnchorFileName)
		}
		requireNothingLeft(t, parent)
	})

	t.Run("BoardCommitNeverStagesHubScratch", func(t *testing.T) {
		t.Parallel()
		bare := makeCLICloneWarpBare(t, t.TempDir(), oneRepositoryFixtureName)
		res, err := cloneOneRepository(bare, t.TempDir(), true)
		if err != nil {
			t.Fatalf("CloneAndWire(SingleRepo) error = %v; want nil", err)
		}
		scratch := filepath.Join(fabricengine.HubScratchDir(res.HubPath), "hub-scratch.txt")
		if err := os.WriteFile(scratch, []byte("machine-local\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", scratch, err)
		}
		if err := os.WriteFile(filepath.Join(res.BoardDir, "board-note.md"), []byte("tracked\n"), 0o644); err != nil {
			t.Fatalf("write the board note: %v", err)
		}

		if _, committed, err := fabricengine.NewBolt(res.BoardDir).Commit("test: board commit", fabricengine.SyncOptions{}); err != nil || !committed {
			t.Fatalf("Bolt.Commit() = committed %t, error %v; want true, nil", committed, err)
		}
		for _, tracked := range gitkit.LsFiles(t, res.BoardDir) {
			if strings.HasPrefix(tracked, lymdirs.DotLymDirName+"/") {
				t.Errorf("the board tracks %s; want hub scratch left out of the commit", tracked)
			}
		}
	})
}
