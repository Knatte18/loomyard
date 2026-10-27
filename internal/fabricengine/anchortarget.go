// anchortarget.go judges what a change of a worktree's anchor starts from: the requested directory against the warp checkout, and the weft's paths against a clean tree.
// Each judgment refuses before anything is touched, and each refusal names its next step.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// warpCostNote is what a refusal tells the operator about the warp change it asks for.
const warpCostNote = "a code commit on the checked-out warp branch, shared by every machine that checks it out"

// validateAnchorTarget refuses a target anchor the pair's warp checkout cannot hold.
// The target must pass lymcwd.ValidateAnchorRel and be a directory the warp checkout's HEAD tracks.
// None of its segments may be a link or a name fabric reserves: a wired name in names, `.lym`, `.scratch` or a hub-reserved name.
// It may hold no real wired name either, since a junction cannot be wired over one.
// A directory HEAD lacks while the warp's fetched upstream tracks it names the update of the warp checkout, and one neither tracks names the commit that brings it in, each with its cost.
// It reads the warp checkout and changes nothing.
func validateAnchorTarget(pair *lymcwd.Location, target string, names []string) error {
	anchor, err := lymcwd.ValidateAnchorRel(target)
	if err != nil {
		return fmt.Errorf("%w; choose another directory of the warp checkout", err)
	}
	warp := WorktreePath(pair, pair.WorktreeName)

	if anchor != "." {
		reserved := slices.Concat(names, []string{lymdirs.DotLymDirName, scratchDirName}, HubReservedNames())
		prefix := warp
		for _, segment := range strings.Split(filepath.ToSlash(anchor), "/") {
			prefix = filepath.Join(prefix, segment)
			isLink, err := fslink.IsLink(prefix)
			if err != nil {
				return fmt.Errorf("inspect %s: %w", prefix, err)
			}
			switch {
			case slices.Contains(reserved, segment):
				return fmt.Errorf("the anchor %s has the segment %q, which fabric reserves; choose another directory of the warp checkout", anchor, segment)
			case isLink:
				return fmt.Errorf("the anchor %s passes through the link %s; choose another directory of the warp checkout", anchor, prefix)
			}
		}
		if err := requireTrackedAnchorDir(warp, anchor); err != nil {
			return err
		}
	}

	for _, name := range names {
		path := filepath.Join(warp, anchor, name)
		isLink, err := fslink.IsLink(path)
		if err != nil {
			return fmt.Errorf("inspect %s: %w", path, err)
		}
		if isLink {
			continue
		}
		if _, statErr := os.Lstat(path); statErr == nil {
			return fmt.Errorf("the warp checkout holds a real %s at %s, which a junction cannot be wired over; move it out of %s in the warp checkout and commit that (%s), then run the verb again",
				name, path, anchor, warpCostNote)
		}
	}
	return nil
}

// requireTrackedAnchorDir refuses an anchor the warp checkout at warp does not track as a directory at HEAD.
func requireTrackedAnchorDir(warp, anchor string) error {
	tracked, err := warpDirTracked(warp, "HEAD", anchor)
	if err != nil || tracked {
		return err
	}
	if upstream, hasUpstream, err := upstreamSHAAt(warp); err != nil {
		return err
	} else if hasUpstream {
		if tracked, err := warpDirTracked(warp, upstream, anchor); err != nil {
			return err
		} else if tracked {
			return fmt.Errorf("the directory %s is not tracked at the warp checkout's HEAD, but its fetched upstream tracks it; update the warp checkout by hand to a commit that tracks it (%s), then run the verb again",
				anchor, warpCostNote)
		}
	}
	return fmt.Errorf("the directory %s is tracked neither at the warp checkout's HEAD nor at its fetched upstream; commit the directory in the warp checkout first (%s), then run the verb again",
		anchor, warpCostNote)
}

// warpDirTracked reports whether rev of the warp repository at warp holds anchor as a directory.
func warpDirTracked(warp, rev, anchor string) (bool, error) {
	listed, err := gitexec.Run([]string{"ls-tree", "-d", "--name-only", rev, "--", filepath.ToSlash(anchor)}, warp)
	if err != nil {
		return false, fmt.Errorf("look for %s at %s in %s: %w", anchor, rev, warp, err)
	}
	return strings.TrimSpace(listed) != "", nil
}

// anchorNamePathspecs returns the slash paths of the wired names under anchor, never `.lym`, which no commit holds.
func anchorNamePathspecs(anchor string, names []string) []string {
	anchor = filepath.ToSlash(anchor)
	var specs []string
	for _, name := range names {
		if name != lymdirs.DotLymDirName {
			specs = append(specs, anchorJoin(anchor, name))
		}
	}
	return specs
}

// refuseUnsettledMoveSource refuses a weft whose wired names under anchor, or whose marker, hold a change that is not committed.
// An untracked path under the wired names counts, the never-tracked `.lym` bar.
// The refusal names the commit or the restore that clears the change.
func refuseUnsettledMoveSource(weftPath, slug, anchor string, names []string) error {
	specs := append(anchorNamePathspecs(anchor, names), lymcwd.AnchorFileName)
	listing, err := gitexec.RunWithEnv(append([]string{"status", "--porcelain", "-z", "--untracked-files=all", "--"}, specs...), weftPath, []string{"GIT_LITERAL_PATHSPECS=1"})
	if err != nil {
		return fmt.Errorf("list the uncommitted changes of %s: %w", weftPath, err)
	}
	var contentPaths []string
	markerChanged := false
	records := strings.Split(listing, "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 4 {
			continue
		}
		if strings.ContainsAny(record[:2], "RC") {
			i++
		}
		if record[3:] == lymcwd.AnchorFileName {
			markerChanged = true
			continue
		}
		contentPaths = append(contentPaths, record[3:])
	}
	switch {
	case len(contentPaths) > 0:
		return fmt.Errorf("the weft of %s holds uncommitted or untracked changes under %s (%s); commit them with `lym fabric commit` or remove the files, then run the verb again",
			slug, anchor, strings.Join(contentPaths, ", "))
	case markerChanged:
		return fmt.Errorf("the weft of %s holds an uncommitted change of %s; drop it with `git -C %s restore %s`, then run the verb again",
			slug, lymcwd.AnchorFileName, weftPath, lymcwd.AnchorFileName)
	}
	return nil
}
