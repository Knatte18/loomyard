// planwrite.go implements the PlanWrite rotate-then-delegate-then-commit decorator: a thin
// shedengine.ShedProducer that archives the stale plan directory, delegates to a wrapped producer,
// and, on a Done outcome with a nil error, invokes an injected commit closure -- mirroring
// discussionwrite.go's own file shape, per this task's mirror-the-Discussion-Write-commit Shared
// Decision.

package loomshed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// archiveTimestampFormat is the compact UTC stamp layout planWrite's rotation step formats its
// archive directory names from. It duplicates internal/shedadapters' identically-named unexported
// constant deliberately, the same way this package's entryErr/cancelErr already duplicate that
// package's own unexported helpers -- see doc.go for why the duplication is deliberate.
const archiveTimestampFormat = "20060102T150405Z"

// planWrite decorates inner with a rotate-then-delegate-then-commit sequence: it archives the plan
// directory's stale top-level .md files, calls inner.Call, and, once inner reports Done with a nil
// error, invokes commit before returning that same verdict to the caller.
//
// planWrite does not consult entryErr or cancelErr itself. inner (a *shedadapters.SingleLLMProducer
// in practice) entry-checks the context as its first act and owns the whole cancellation
// obligation; rotation running before that entry check means a run cancelled between the two
// leaves an archive directory behind and no new plan, which is acceptable because the archive is
// committed content rather than dirt and the next entry rotates an already-empty directory as a
// no-op.
type planWrite struct {
	name       string
	inner      shedengine.ShedProducer
	commit     func() error
	anchorPath string
	now        func() time.Time
}

var _ shedengine.ShedProducer = (*planWrite)(nil)

// NewPlanWrite returns a planWrite identified as name, delegating to inner and invoking commit
// once inner reports Done with a nil error. anchorPath is the absolute directory lyx is anchored
// at; planWrite resolves the plan directory from it via planparser.PlanDir. A nil now defaults to
// time.Now. The return type is shedengine.ShedProducer, the seam interface, so internal/shedrecipe
// can construct this producer from outside this package while planWrite itself stays unexported.
func NewPlanWrite(name string, inner shedengine.ShedProducer, commit func() error, anchorPath string, now func() time.Time) shedengine.ShedProducer {
	if now == nil {
		now = time.Now
	}
	return &planWrite{name: name, inner: inner, commit: commit, anchorPath: anchorPath, now: now}
}

// Call implements shedengine.ShedProducer. It rotates the stale plan directory first, returning a
// wrapped error without ever touching p.inner on rotation failure. It then calls p.inner.Call(ctx)
// exactly once and returns its three results verbatim whenever the error is non-nil or the outcome
// is anything other than shedengine.Done. Only a Done outcome with a nil error invokes p.commit
// before returning.
//
// Neither a rotation failure nor a commit failure maps to shedengine.Stuck: a filesystem or git
// fault is infrastructure, not plan quality, and a returned error persists failed and aborts while
// Stuck persists blocked and bounces.
func (p *planWrite) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := p.rotateStalePlanDir(); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: rotate stale plan directory: %w", p.name, err)
	}

	outcome, pointer, err := p.inner.Call(ctx)
	if err != nil || outcome != shedengine.Done {
		return outcome, pointer, err
	}

	if err := p.commit(); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: commit produced artifacts: %w", p.name, err)
	}

	return outcome, pointer, nil
}

// rotateStalePlanDir archives every top-level ".md" file currently in the plan directory into a
// fresh archive-<stamp>[-N] subdirectory, leaving any other entry (a directory, or a non-.md file)
// in place. It resolves the plan directory via planparser.PlanDir(p.anchorPath) -- never by naming
// the "_lyx" literal, which the Lyxdirs Single-Declarer Invariant forbids in production
// path-construction context.
//
// An absent plan directory, or one with no top-level .md file to move, is a no-op with a nil
// error and creates nothing. Only files move, never directories, so a second rotation can never
// nest a previous archive directory inside a new one.
func (p *planWrite) rotateStalePlanDir() error {
	planDir := planparser.PlanDir(p.anchorPath)

	entries, err := os.ReadDir(planDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var staleFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".md" {
			continue
		}
		staleFiles = append(staleFiles, e.Name())
	}
	if len(staleFiles) == 0 {
		return nil
	}

	stamp := p.now().UTC().Format(archiveTimestampFormat)
	archiveDir, err := firstFreePlanArchivePath(planDir, stamp)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return err
	}
	for _, name := range staleFiles {
		if err := os.Rename(filepath.Join(planDir, name), filepath.Join(archiveDir, name)); err != nil {
			return err
		}
	}
	return nil
}

// firstFreePlanArchivePath returns the first archive directory path under planDir, built via
// planparser.ArchiveDirName(stamp, suffix) for suffix in the sequence "", "-1", "-2", ..., that
// does not already exist. Any os.Stat error other than not-exist is returned as-is -- this mirrors
// firstFreeArchivePath in internal/shedadapters/archive.go.
func firstFreePlanArchivePath(planDir, stamp string) (string, error) {
	for n := 0; ; n++ {
		suffix := ""
		if n > 0 {
			suffix = fmt.Sprintf("-%d", n)
		}
		candidate := filepath.Join(planDir, planparser.ArchiveDirName(stamp, suffix))
		if _, err := os.Stat(candidate); err != nil {
			if os.IsNotExist(err) {
				return candidate, nil
			}
			return "", err
		}
	}
}
