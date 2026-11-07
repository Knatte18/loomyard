// resetsubpathcheck.go implements the --subpath check a --reset clone runs while the existing hub is still intact.
// It stages shallow clones of the weft and warp remotes in throwaway directories.
// It applies the anchor rules CloneHub applies after its own clone,
// so a refused --subpath never costs the hub being replaced.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// checkSubpathBeforeReset refuses a requested --subpath that the clone would refuse, without touching any existing hub.
// A subpath that disagrees with the anchor recorded on the weft is refused with the adopt-path error;
// when the weft records none, a subpath must name a directory in the warp's checked-out tree,
// or it is refused with the create-path error.
// The staged clones live under cwd and are removed before it returns.
// requestedAnchor is the validated form of the explicit --subpath.
func checkSubpathBeforeReset(cwd, weftURL, warpURL, requestedAnchor string) error {
	stageDir, err := os.MkdirTemp(cwd, warpProbeDirPrefix)
	if err != nil {
		return fmt.Errorf("create staging directory in %s: %w", cwd, err)
	}
	defer func() { _ = os.RemoveAll(stageDir) }()

	warpStage := filepath.Join(stageDir, "warp")
	weftStage := filepath.Join(stageDir, "weft")
	for _, stage := range []struct{ url, dir string }{{warpURL, warpStage}, {weftURL, weftStage}} {
		_, err := gitexec.Run([]string{
			"clone", "--depth", "1", "--no-checkout", "--single-branch",
			filepath.ToSlash(stage.url), filepath.Base(stage.dir),
		}, stageDir)
		if err != nil {
			return fmt.Errorf("stage %s to check --subpath: %w", stage.url, err)
		}
	}

	warpBranch, err := checkedOutBranch(warpStage, "warp prime")
	if err != nil {
		return err
	}

	markerData, source, found, err := recordedAnchorOnWeft(weftURL, weftStage, warpBranch)
	if err != nil {
		return err
	}
	if found {
		_, err := adoptRecordedAnchor(markerData, source, requestedAnchor, true)
		return err
	}

	isDir, err := warpHasDirectory(warpStage, requestedAnchor)
	if err != nil {
		return err
	}
	if !isDir {
		return fmt.Errorf("subpath %q does not exist as a directory in the cloned warp repo", requestedAnchor)
	}
	return nil
}

// recordedAnchorOnWeft reads the anchor marker the hub clone would adopt, from the staged weft clone at weftStage.
// The prime's weft branch, RecordsBranchName(warpBranch), supplies it when the remote carries that branch;
// otherwise the weft's default HEAD supplies it, matching how the clone creates that branch.
// Failing both, the default HEAD supplies the legacy board marker only when the default branch is warpBranch itself;
// the board checkout adopts a local branch of that name and is orphan-created otherwise.
// source names where the marker was read, in adoptRecordedAnchor's wording.
func recordedAnchorOnWeft(weftURL, weftStage, warpBranch string) (data []byte, source string, found bool, err error) {
	primeBranch := RecordsBranchName(warpBranch)
	primeRef := "HEAD"
	onRemote, err := fetchWeftBranchTip(weftURL, weftStage, primeBranch)
	if err != nil {
		return nil, "", false, err
	}
	if onRemote {
		primeRef = "FETCH_HEAD"
	}
	data, found, err = readAnchorMarkerAt(weftURL, weftStage, primeRef)
	if err != nil || found {
		return data, "on " + primeBranch, found, err
	}

	stdout, err := gitexec.Run([]string{"branch", "--show-current"}, weftStage)
	if err != nil {
		return nil, "", false, wrapProbeError(weftURL, "branch --show-current", err)
	}
	if strings.TrimSpace(stdout) != warpBranch {
		return nil, "", false, nil
	}
	data, found, err = readAnchorMarkerAt(weftURL, weftStage, "HEAD")
	return data, "on weft:main", found, err
}

// readAnchorMarkerAt returns the anchor marker's content at the root of ref in the staged clone at dir.
// It reports whether the marker is present.
// An unborn HEAD carries no marker.
func readAnchorMarkerAt(weftURL, dir, ref string) ([]byte, bool, error) {
	if ref == "HEAD" {
		_, err := gitrepo.New(dir).CurrentSHA()
		if errors.Is(err, gitrepo.ErrNoCommits) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, wrapProbeError(weftURL, "rev-parse HEAD", err)
		}
	}
	listed, err := gitexec.Run([]string{"ls-tree", ref, "--name-only", "--", lymcwd.AnchorFileName}, dir)
	if err != nil {
		return nil, false, wrapProbeError(weftURL, "ls-tree", err)
	}
	if strings.TrimSpace(listed) == "" {
		return nil, false, nil
	}
	content, err := gitexec.Run([]string{"show", ref + ":" + lymcwd.AnchorFileName}, dir)
	if err != nil {
		return nil, false, wrapProbeError(weftURL, "show", err)
	}
	return []byte(content), true, nil
}

// warpHasDirectory reports whether anchor names a directory in the HEAD tree of the staged warp clone at warpStage.
// "." is always a directory, and an unborn warp has no other.
func warpHasDirectory(warpStage, anchor string) (bool, error) {
	if anchor == "." {
		return true, nil
	}
	_, err := gitrepo.New(warpStage).CurrentSHA()
	if errors.Is(err, gitrepo.ErrNoCommits) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stage warp to check --subpath: rev-parse HEAD: %w", err)
	}
	listed, err := gitexec.Run([]string{"ls-tree", "-d", "HEAD", "--name-only", "--", filepath.ToSlash(anchor)}, warpStage)
	if err != nil {
		return false, fmt.Errorf("stage warp to check --subpath: ls-tree: %w", err)
	}
	return strings.TrimSpace(listed) != "", nil
}
