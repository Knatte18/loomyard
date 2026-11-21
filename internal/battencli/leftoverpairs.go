// leftoverpairs.go reads, for the dobby, the pairs no run's own teardown takes.

package battencli

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// unreadableOrphanTip keeps one failure episode per orphan candidate whose last commit does not read.
// It is keyed by slug, so a persistent failure logs one Warn, not one line per dobby probe tick.
var unreadableOrphanTip logger.EpisodeSet

// DobbyOrphans lists the hub's orphan pairs.
// An orphan is a task pair on disk that no board entry at boardDir and no batten run directory on the prime names.
// The prime is no pair, and a beam's pair is named by its board entry.
// Each carries the newer commit time of its two branch tips and whether a process registered under its slug lives.
// A worktree that does not resolve, or whose last commit does not read, is left out with one Warn.
func DobbyOrphans(prime *lymcwd.Location, boardDir string) ([]dobbyengine.OrphanView, error) {
	worktrees, err := fabricengine.List(prime.WorktreePath())
	if err != nil {
		return nil, err
	}
	tasks, err := boardengine.ReadTasks(boardDir)
	if err != nil {
		return nil, fmt.Errorf("read the board: %w", err)
	}
	onBoard := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		onBoard[task.Slug] = true
	}
	var orphans []dobbyengine.OrphanView
	for _, worktree := range worktrees {
		if worktree.Main || worktree.Prunable {
			continue
		}
		pair, err := lymcwd.ResolveWorktree(worktree.Path)
		if err != nil {
			logger.Warn("battencli: a worktree that does not resolve is left out of the orphan pairs", "worktree", worktree.Path, "error", err)
			continue
		}
		slug := pair.WorktreeName
		if onBoard[slug] {
			continue
		}
		if _, err := os.Stat(shedrun.RunDir(prime, slug)); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read the batten run directory of %s: %w", slug, err)
		}
		lastCommit, err := fabricengine.PairLastCommit(pair)
		if err != nil {
			unreadableOrphanTip.Fail(slug, "battencli: a pair whose last commit does not read is left out of the orphan pairs", "slug", slug, "error", err)
			continue
		}
		unreadableOrphanTip.Recover(slug, "battencli: a pair's last commit reads again and it is listed as an orphan again", "slug", slug)
		live, err := runproc.List(fabricengine.RunProcDir(prime.HubPath), slug)
		if err != nil {
			return nil, fmt.Errorf("read the processes of %s: %w", slug, err)
		}
		orphans = append(orphans, dobbyengine.OrphanView{Slug: slug, LastCommit: lastCommit, ProcessLive: len(live) > 0})
	}
	return orphans, nil
}
