// teardown.go implements the producer that removes a task worktree.
// It shuts the loom session down, asks for the pair to be pushed, then removes the worktree once it is level.
// The order is strict.
// It holds the hub's prime lock for the shutdown and for the removal, and none for the push.

package battenshed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// abandonedSessionFileName is the fixed name of the abandoned-session record inside a run's scratch directory.
const abandonedSessionFileName = "abandoned-session"

// AbandonedSessionFile returns the path under scratchDir where the abandoned session's name is recorded.
// It is exported so that the readers and the writers of the record share one declaration of its name.
func AbandonedSessionFile(scratchDir string) string {
	return filepath.Join(scratchDir, abandonedSessionFileName)
}

// recordAbandonedSession logs and records that session shutdown abandoned a session rather than ending it cleanly.
// When it did not, it clears any stale record.
// The record outlives the process that observed the abandonment,
// so a step-driven lifecycle can report it as well as a run-driven one.
// A write or remove failure is logged rather than escalated:
// failing to record how a teardown ended must not change whether it succeeded.
func recordAbandonedSession(producer, slug, session, scratchDir string) {
	path := AbandonedSessionFile(scratchDir)
	if session == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			logger.Warn("battenshed: clear stale abandoned-session record failed", "producer", producer, "slug", slug, "path", path, "error", err)
		}
		return
	}

	logger.Warn("battenshed: session shutdown abandoned a session rather than ending it cleanly", "producer", producer, "slug", slug, "session", session)
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		logger.Warn("battenshed: create scratch directory for abandoned-session record failed", "producer", producer, "scratchDir", scratchDir, "error", err)
		return
	}
	if err := os.WriteFile(path, []byte(session+"\n"), 0o644); err != nil {
		logger.Warn("battenshed: write abandoned-session record failed", "producer", producer, "path", path, "error", err)
	}
}

// worktreeTeardownProducer shuts down the task worktree's loom session and asks for the pair to be pushed.
// It then removes the worktree pair if it is level.
// It holds the hub's prime lock for the shutdown and again for the removal, and none for the push.
type worktreeTeardownProducer struct {
	name       string
	slug       string
	deps       TeardownDeps
	primeLock  PrimeLock
	scratchDir string
}

var _ shedengine.ShedProducer = (*worktreeTeardownProducer)(nil)

// NewWorktreeTeardown returns a shedengine.ShedProducer for the task worktree identified by slug.
// The producer takes primeLock and calls deps.Shutdown, then releases the lock.
// It then calls deps.Push with no prime lock held.
// It takes primeLock again and reads deps.Level once.
// A true read calls deps.Remove; a false read ends the row Done with the pair standing, to be removed later,
// so the row waits on no network and never loops.
// A failed shutdown, push, Level read or removal is reported as a Stuck outcome, not as an error.
// deps.Remove is called only after a successful shutdown and a level read.
func NewWorktreeTeardown(name, slug string, deps TeardownDeps, primeLock PrimeLock, scratchDir string) shedengine.ShedProducer {
	return &worktreeTeardownProducer{
		name:       name,
		slug:       slug,
		deps:       deps,
		primeLock:  primeLock,
		scratchDir: scratchDir,
	}
}

// Call implements shedengine.ShedProducer.
// Each of its two prime-lock holds waits for a contended lock, bounded and cancellable.
// An Acquire error is returned as a hard error.
// A spent bound yields Stuck, naming the lock path and the wait.
// The release closure is deferred, so it runs on every exit path, including each Stuck one.
// A release error is logged at Warn and does not replace the verdict.
//
// The first hold calls deps.Shutdown.
// A Shutdown error yields Stuck, naming session shutdown as the failed half, and deps.Remove is not called on that path.
// Removing the worktree before its session has shut down is the ordering this producer exists to prevent.
// A non-empty abandoned-session return is logged and recorded under scratchDir, and does not change the verdict.
//
// A Push error yields Stuck with its text and the resume, and keeps the pair.
// A Level error yields Stuck naming the read and the resume.
// A Level that reads false yields Done and keeps the pair.
//
// A Remove error yields Stuck, naming worktree removal as the failed half.
// The reason also states that session shutdown already succeeded, so the two halves can be told apart.
// On success Call returns Done.
func (p *worktreeTeardownProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	stuck := func(reason string) (shedengine.Outcome, shedengine.OutputPointer, error) {
		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
	}

	reason, err := p.underPrimeLock(ctx, func() (string, error) { return p.shutDownSession(ctx) })
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if reason != "" {
		return stuck(reason)
	}

	reason, err = p.pushPair(ctx)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if reason != "" {
		return stuck(reason)
	}
	reason, err = p.underPrimeLock(ctx, func() (string, error) {
		level, levelReason, levelErr := p.pairIsLevel(ctx)
		if levelErr != nil || levelReason != "" {
			return levelReason, levelErr
		}
		if !level {
			logger.Info("battenshed: the pair is not level yet, so it stays standing to be removed later", "producer", p.name, "slug", p.slug)
			return "", nil
		}
		return p.removePair(ctx)
	})
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if reason != "" {
		return stuck(reason)
	}

	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// underPrimeLock runs step holding the prime lock, waiting for a contended lock.
// It returns the lock's own stuck reason when the wait's bound is spent, and step's results otherwise.
// The lock is released on every path out of step.
func (p *worktreeTeardownProducer) underPrimeLock(ctx context.Context, step func() (stuckReason string, err error)) (string, error) {
	release, reason, err := acquirePrimeLock(ctx, p.name, p.slug, p.primeLock)
	if err != nil {
		return "", err
	}
	if release == nil {
		return reason, nil
	}
	defer func() {
		if rerr := release(); rerr != nil {
			logger.Warn("battenshed: release prime lock failed", "producer", p.name, "slug", p.slug, "path", p.primeLock.Path, "error", rerr)
		}
	}()
	return step()
}

// shutDownSession calls deps.Shutdown and records the abandoned session it reports.
// It returns the stuck reason of a failed shutdown, and the cancellation error when ctx ended.
func (p *worktreeTeardownProducer) shutDownSession(ctx context.Context) (string, error) {
	abandonedSession, shutdownErr := p.deps.Shutdown(ctx)
	if shutdownErr != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", cerr
		}
		return fmt.Sprintf("session shutdown failed: %s", shutdownErr.Error()), nil
	}
	recordAbandonedSession(p.name, p.slug, abandonedSession, p.scratchDir)
	return "", nil
}

// pushPair calls deps.Push, which a nil Push skips.
// It returns the stuck reason of a push that cannot go on.
func (p *worktreeTeardownProducer) pushPair(ctx context.Context) (stuckReason string, err error) {
	if p.deps.Push == nil {
		return "", nil
	}
	pushErr := p.deps.Push(ctx)
	if pushErr == nil {
		return "", nil
	}
	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", cerr
	}
	return fmt.Sprintf("the push step failed and the pair is kept: %s; once its way forward is done, resume with `lym batten run %s`", pushErr.Error(), p.slug), nil
}

// pairIsLevel calls deps.Level, which a nil Level reads as level.
// It returns the stuck reason of a failed read.
func (p *worktreeTeardownProducer) pairIsLevel(ctx context.Context) (level bool, stuckReason string, err error) {
	if p.deps.Level == nil {
		return true, "", nil
	}
	level, levelErr := p.deps.Level()
	if levelErr != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return false, "", cerr
		}
		return false, fmt.Sprintf("reading whether the pair may be removed failed and the pair is kept: %s; resume with `lym batten run %s`", levelErr.Error(), p.slug), nil
	}
	return level, "", nil
}

// removePair calls deps.Remove.
// It returns the stuck reason of a failed removal, which states that session shutdown already succeeded.
func (p *worktreeTeardownProducer) removePair(ctx context.Context) (string, error) {
	if err := p.deps.Remove(ctx); err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", cerr
		}
		return fmt.Sprintf("worktree removal failed (session shutdown already succeeded): %s", err.Error()), nil
	}
	return "", nil
}
