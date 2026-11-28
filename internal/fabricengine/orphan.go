// orphan.go names the machine-local first-sight record of orphan pairs and reads when a pair last committed.
// The dobby tears an orphan pair down once it has gone idle for its grace.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// orphanFirstSightFileName is the file under the hub's orphan directory that holds each orphan's first-sight time.
const orphanFirstSightFileName = "first-sight.json"

// OrphanFirstSightPath returns the machine-local record of when the dobby first noticed each orphan pair of the hub:
// <HubScratchDir(hubPath)>/<lymdirs.OrphanDirName>/first-sight.json.
// A pair removal does not take it, since it sits under the hub's `.lym` and never in a pair.
func OrphanFirstSightPath(hubPath string) string {
	return filepath.Join(HubScratchDir(hubPath), lymdirs.OrphanDirName, orphanFirstSightFileName)
}

// PairLastCommit returns the newer commit time of the tips checked out in the pair's code and records worktrees.
// A worktree that is not there counts no commit, so a pair with neither reads the zero time.
func PairLastCommit(pair *lymcwd.Location) (time.Time, error) {
	var newest time.Time
	for _, dir := range []string{pair.WorktreePath(), RecordsWorktree(pair)} {
		if !pathPresent(dir) {
			continue
		}
		repo := gitrepo.New(dir)
		tip, err := repo.CurrentSHA()
		if err != nil {
			return time.Time{}, fmt.Errorf("fabricengine: read the tip checked out at %s: %w", dir, err)
		}
		commits, err := repo.Log(tip, gitrepo.LogQuery{Limit: 1})
		if err != nil {
			return time.Time{}, fmt.Errorf("fabricengine: read the tip commit at %s: %w", dir, err)
		}
		if len(commits) > 0 && commits[0].Committed.After(newest) {
			newest = commits[0].Committed
		}
	}
	return newest, nil
}
