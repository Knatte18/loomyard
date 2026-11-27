// leftoverpairs.go reads and settles, for the dobby, the pairs no run's own teardown takes.
// It lists the hub's orphan pairs,
// and it gives a run cancelled with keep before cancel intents existed its finished keep cancel intent.

package battencli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// MarkKeptCancels gives each run whose parent worktree is geom's a committed, finished keep cancel intent.
// The intent names this machine.
// It marks a run when its batten record is in shed state cancelled with no cancel intent and its pair stands here.
// A cancel with keep left such a record before cancel intents existed,
// and the run FSM reads it as neither cancelled nor kept.
// A record whose finished keep intent this machine wrote, but whose commit failed, is committed again.
// No intent is written while a merge is in progress on the prime's records side.
// A refused commit therefore leaves nothing written.
// Each record is committed and pushed through the synchronous records push; a push origin cannot take now is owed.
// It returns the slugs it committed, and stops at the first run whose write, commit or push fails, naming it.
func MarkKeptCancels(prime *lymcwd.Location, geom dobbyengine.Geometry, parents *DobbyParents) ([]string, error) {
	runs, err := dobbyParentRuns(prime, geom, parents)
	if err != nil {
		return nil, err
	}
	var marked []string
	machine := ""
	for _, run := range runs {
		if run.status.State != shedengine.StateCancelled {
			continue
		}
		stands, err := pairStands(prime, run.slug)
		if err != nil {
			return marked, err
		}
		if !stands {
			continue
		}
		if machine == "" {
			if machine, err = machinename.Name(); err != nil {
				return marked, err
			}
		}
		deps := battenCommitStatusDeps(prime, run.slug)
		if intent := run.status.Cancel; intent == nil {
			written, err := writeKeptCancel(prime, deps, run.slug, machine)
			if err != nil {
				return marked, err
			}
			if !written {
				continue
			}
		} else {
			if intent.Remove || intent.Finish == nil || intent.Machine != machine {
				continue
			}
			// An intent already written waits for the next call while a merge is in progress, as it may be committed already.
			active, err := deps.MergeActive()
			if err != nil {
				return marked, fmt.Errorf("read the prime's records merge state for %s: %w", run.slug, err)
			}
			if active {
				continue
			}
		}
		committed, err := commitCancelRecord(deps, run.slug)
		if err != nil {
			return marked, fmt.Errorf("commit and push the kept cancel of %s: %w", run.slug, err)
		}
		if committed {
			marked = append(marked, run.slug)
		}
	}
	return marked, nil
}

// writeKeptCancel writes slug's finished keep cancel intent naming machine, and reports whether it wrote one.
// It writes nothing while a merge is in progress on the prime's records side.
func writeKeptCancel(prime *lymcwd.Location, deps commitStatusDeps, slug, machine string) (bool, error) {
	if err := refuseWhileMergeActive(deps); err != nil {
		return false, fmt.Errorf("record the kept cancel of %s: %w", slug, err)
	}
	if err := os.MkdirAll(filepath.Dir(shedrun.StatusLock(prime, slug)), 0o755); err != nil {
		return false, err
	}
	written, err := shedrun.WriteKeptCancel(prime, slug, machine, time.Now())
	if err != nil {
		return false, fmt.Errorf("record the kept cancel of %s: %w", slug, err)
	}
	return written, nil
}

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
