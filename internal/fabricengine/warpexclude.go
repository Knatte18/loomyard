// warpexclude.go seeds the warp exclude list: the paths under a warp worktree's anchor that fabric keeps out of git in every warp worktree through the repository's `info/exclude`.
// The list is the `warp_exclude` key of fabric's config, separate from the junction name-set and never wired as a junction.

package fabricengine

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// seedWarpExcludeLines appends a `/<anchorRel>/<path>` line for each path to the `info/exclude` of the repository checked out at worktree, a `/<path>` line when anchorRel is ".".
// The lines are anchored at the repository root, since a slash-free pattern matches at every depth, and name the anchor subpath because the files the list names sit at a worktree's anchor.
// A line already present is skipped, and no line is ever removed: `info/exclude` is one file for every worktree of the repository.
// rec records KindFileWritten on the exclude file only when it changed.
func seedWarpExcludeLines(rec *Mutations, worktree, anchorRel string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	anchorPrefix := filepath.ToSlash(anchorRel)
	excludePath, changed, err := mutateGitExclude(worktree, func(content string) (string, error) {
		present := make(map[string]bool)
		for _, line := range strings.Split(content, "\n") {
			present[strings.TrimSpace(line)] = true
		}
		for _, listed := range paths {
			line := "/" + path.Join(anchorPrefix, listed)
			if present[line] {
				continue
			}
			if content != "" && !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += line + "\n"
			present[line] = true
		}
		return content, nil
	})
	if err != nil {
		return err
	}
	if changed {
		rec.Append(KindFileWritten, excludePath, "")
	}
	return nil
}

// SeedWarpExclude loads fabric's config and seeds its warp exclude list, under l's anchor, into the exclude file of the repository l's warp worktree belongs to.
// It returns the listed paths, relative to the anchor, and records no mutation: it is no fabric verb.
// Nothing on the list is written in the weft, so the weft is never seeded.
func SeedWarpExclude(l *lymcwd.Location) ([]string, error) {
	cfg, err := LoadConfig(ConfigDirs(l))
	if err != nil {
		return nil, err
	}
	paths := cfg.WarpExcludePaths()
	if err := seedWarpExcludeLines(nil, l.WorktreePath(), l.AnchorRel, paths); err != nil {
		return nil, err
	}
	return paths, nil
}

// ExcludeWayForward returns the refusal text for a path the warp exclude list does not name:
// the path cannot be written because git would show it as untracked, and it joins the list through fabric's `warp_exclude` key in `lym config fabric`.
func ExcludeWayForward(path string) string {
	return fmt.Sprintf("%s is not on fabric's warp exclude list, so it would show as untracked; way forward: add it to the warp_exclude key in \"lym config fabric\" and run again", path)
}
