// bolt.go defines Bolt, the handle over the unpaired weft:main area (today
// consumed only by _board): it names that area as a self-contained unit
// through Commit/Push/Sync, so a caller outside this package never has to
// spell "weft" to reach it.

package fabricengine

import "path/filepath"

// Bolt is a handle over a single weft:main-backed repo path with no paired warp
// side — the exception to fabric's one-repo illusion. It never constructs any
// geometry token itself.
type Bolt struct {
	path string
}

// NewBolt returns a Bolt over the repo at repoPath.
func NewBolt(repoPath string) *Bolt {
	return &Bolt{path: repoPath}
}

// Commit stages and commits every change in the Bolt's repo under message,
// with no warp trailer and no correspondence recording.
func (b *Bolt) Commit(message string, opts SyncOptions) (sha string, committed bool, err error) {
	return commitWeftAt(b.path, message, opts)
}

// Push pushes any unpushed commits in the Bolt's repo.
func (b *Bolt) Push(opts SyncOptions) error {
	return pushWeftAt(b.path, opts)
}

// Sync drives step to completion under an absorbing push lock, looping while
// step reports progress.
func (b *Bolt) Sync(step func() (progressed bool, err error)) error {
	return coalescePush(filepath.Join(b.path, "board.push.lock"), step)
}
