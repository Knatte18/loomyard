// planwrite.go implements the Plan-Write row's two halves: NewPlanWrite, the post-Done commit
// decorator it shares with Discussion-Write, and NewPlanDirRotator, the stale-plan-directory rotation
// that runs as the wrapped SingleLLMProducer's fresh-spawn preparation rather than as a step ahead of
// it.

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

// archiveTimestampFormat is the compact UTC stamp layout the plan-directory rotation formats its
// archive directory names from. It duplicates internal/shedadapters' identically-named unexported
// constant deliberately, the same way this package's entryErr/cancelErr already duplicate that
// package's own unexported helpers -- see doc.go for why the duplication is deliberate.
const archiveTimestampFormat = "20060102T150405Z"

// planWrite decorates inner with a post-Done commit step: once inner.Call reports Done with a nil
// error, planWrite invokes commit before returning that same verdict to the caller.
//
// It carries no rotation of its own, deliberately. Rotating the plan directory is destructive to the
// very files a live plan agent may be mid-write on, so it belongs after the wrapped
// SingleLLMProducer's attach probe, never before its Call -- see NewPlanDirRotator.
//
// planWrite does not consult entryErr or cancelErr itself. inner (a
// *shedadapters.SingleLLMProducer in practice) already entry-checks the context as its first act, so
// a second check here would be duplicate work at the same seam; the wrapped producer owns the whole
// cancellation obligation.
//
// It is deliberately a distinct type from discussionWrite rather than a shared one, even though the
// two now carry identical logic: internal/loomrecipe's shape_test asserts the concrete producer type
// built for each recipe row, so collapsing them would retire a guard that currently tells the
// Plan-Write row's producer from the Discussion-Write row's.
type planWrite struct {
	name   string
	inner  shedengine.ShedProducer
	commit func() error
}

var _ shedengine.ShedProducer = (*planWrite)(nil)

// NewPlanWrite returns a planWrite identified as name, delegating to inner and invoking commit once
// inner reports Done with a nil error. The return type is shedengine.ShedProducer, the seam
// interface, so internal/shedrecipe can construct this producer from outside this package while
// planWrite itself stays unexported.
func NewPlanWrite(name string, inner shedengine.ShedProducer, commit func() error) shedengine.ShedProducer {
	return &planWrite{name: name, inner: inner, commit: commit}
}

// Call implements shedengine.ShedProducer: it calls p.inner.Call(ctx) exactly once and returns its
// three results verbatim whenever the error is non-nil or the outcome is anything other than
// shedengine.Done. Only a Done outcome with a nil error invokes p.commit before returning.
//
// A commit failure maps to a returned error, never to shedengine.Stuck: a git fault is infrastructure
// rather than plan quality, and a returned error persists failed and aborts while Stuck persists
// blocked and bounces.
func (p *planWrite) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	outcome, pointer, err := p.inner.Call(ctx)
	if err != nil || outcome != shedengine.Done {
		return outcome, pointer, err
	}

	if err := p.commit(); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: commit produced artifacts: %w", p.name, err)
	}

	return outcome, pointer, nil
}

// NewPlanDirRotator returns the closure that archives the plan directory's stale top-level ".md"
// files into a fresh archive-<stamp>[-N] subdirectory under it, resolving that directory from
// anchorPath via planparser.PlanDir. A nil now defaults to time.Now.
//
// The returned closure is handed to shedadapters.NewSingleLLMProducer as its prepareFreshSpawn hook,
// so it runs only once the attach probe has established that no live agent is writing into that
// directory. Running it unconditionally ahead of the producer -- which is what this rotation used to
// do, as a step inside the decorator's own Call -- moves 00-overview.md, the Plan spec's sole
// declared output file, out from under an agent the very next line then attaches to: shuttle's Wait
// polls for bare existence at the spec's paths, so a plan that was finished never classifies Done
// and times out into a hard run failure instead.
//
// Only files move, never directories, so a second rotation can never nest a previous archive
// directory inside a new one. An absent plan directory, or one with no top-level .md file to move,
// is a no-op with a nil error and creates nothing.
func NewPlanDirRotator(anchorPath string, now func() time.Time) func() error {
	if now == nil {
		now = time.Now
	}
	return func() error {
		if err := rotateStalePlanDir(anchorPath, now); err != nil {
			return fmt.Errorf("loomshed: rotate stale plan directory: %w", err)
		}
		return nil
	}
}

// rotateStalePlanDir archives every top-level ".md" file currently in the plan directory resolved
// from anchorPath into a fresh archive-<stamp>[-N] subdirectory, leaving any other entry (a
// directory, or a non-.md file) in place.
// It resolves the plan directory via planparser.PlanDir(anchorPath) -- never by naming the "_lyx"
// literal, which the Lyxdirs Single-Declarer Invariant forbids in production path-construction
// context.
func rotateStalePlanDir(anchorPath string, now func() time.Time) error {
	planDir := planparser.PlanDir(anchorPath)

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

	stamp := now().UTC().Format(archiveTimestampFormat)
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
