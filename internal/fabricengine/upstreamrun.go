// upstreamrun.go reads a pair's own run status and its anchor at the tree of its weft's fetched upstream tip.
// A weft that stopped taking its upstream in over a marker change keeps a checked-out status that goes stale, and this read is the one that does not.

package fabricengine

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// UpstreamAnchor returns the anchor the tree of the pair's weft's fetched upstream tip records, and found false when the weft has no upstream.
// A tip that records no marker answers the anchor its content implies, with pair's anchor standing in for a tree that holds no wired name in names.
func UpstreamAnchor(pair *lymcwd.Location, names []string) (anchor string, found bool, err error) {
	weftPath := RecordsWorktreePath(pair, pair.WorktreeName)
	upstream, hasUpstream, err := upstreamSHAAt(weftPath)
	if err != nil || !hasUpstream {
		return "", false, err
	}
	anchor, err = treeAnchor(weftPath, upstream, names, pair.AnchorRel)
	if err != nil {
		return "", false, err
	}
	return anchor, true, nil
}

// UpstreamRunUnfinished reports whether the pair's own run status, read at the tree of its weft's fetched upstream tip, names an unfinished run.
// A run is unfinished when its status holds a state other than done; a tree with no status reads as finished.
// The status sits under the anchor the tip's own marker names, with pair's anchor standing in for a tip that records none.
// It is read from the directory of the pair's slug or, when the tree lacks that directory, from the legacy `self` directory.
// A weft with no fetched upstream is read at HEAD.
// names are the wired names the tip's anchor is derived from.
func UpstreamRunUnfinished(pair *lymcwd.Location, names []string) (unfinished bool, err error) {
	weftPath := RecordsWorktreePath(pair, pair.WorktreeName)
	tip := "HEAD"
	if upstream, hasUpstream, err := upstreamSHAAt(weftPath); err != nil {
		return false, err
	} else if hasUpstream {
		tip = upstream
	}
	anchor, err := treeAnchor(weftPath, tip, names, pair.AnchorRel)
	if err != nil {
		return false, err
	}

	_, statusRel, err := shedrun.TreeRunFileRels(pair.WorktreeName)
	if err != nil {
		return false, err
	}
	runDir := path.Dir(path.Join(filepath.ToSlash(anchor), statusRel))
	listed, err := gitexec.Run([]string{"ls-tree", "-d", "--name-only", tip, "--", runDir}, weftPath)
	if err != nil {
		return false, fmt.Errorf("look for %s at %s in %s: %w", runDir, tip, weftPath, err)
	}
	if listed == "" {
		if _, statusRel, err = shedrun.TreeRunFileRels(shedrun.SelfRunID); err != nil {
			return false, err
		}
	}

	statusPath := path.Join(filepath.ToSlash(anchor), statusRel)
	data, err := showFile(weftPath, tip, statusPath)
	if err != nil {
		return false, fmt.Errorf("read %s at %s in %s: %w", statusPath, tip, weftPath, err)
	}
	if data == nil {
		return false, nil
	}
	var status shedengine.Status
	if err := json.Unmarshal(data, &status); err != nil {
		return false, fmt.Errorf("parse %s at %s in %s: %w", statusPath, tip, weftPath, err)
	}
	return status.State != shedengine.StateDone, nil
}
