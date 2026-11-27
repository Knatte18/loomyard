// teardown.go implements the producer that removes a task worktree.
// It shuts the loom session down, then removes the worktree, strictly in that order, under the hub's prime lock.

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

// worktreeTeardownProducer shuts down the task worktree's loom session and then removes the worktree pair.
// It holds the hub's prime lock for the duration of both calls.
type worktreeTeardownProducer struct {
	name       string
	slug       string
	deps       TeardownDeps
	primeLock  PrimeLock
	scratchDir string
}

var _ shedengine.ShedProducer = (*worktreeTeardownProducer)(nil)

// NewWorktreeTeardown returns a shedengine.ShedProducer for the task worktree identified by slug.
// The producer acquires primeLock, calls deps.Shutdown, and calls deps.Remove only when Shutdown succeeds.
// A failed shutdown or removal is reported as a Stuck outcome, not as an error.
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
// It acquires the prime lock; a contended lock is waited for, bounded and cancellable.
// An Acquire error is returned as a hard error.
// A spent bound yields Stuck, naming the lock path and the wait.
// The release closure is deferred, so it runs on every exit path, including each Stuck one.
// A release error is logged at Warn and does not replace the verdict.
//
// It then calls deps.Shutdown.
// A Shutdown error yields Stuck, naming session shutdown as the failed half, and deps.Remove is not called on that path.
// Removing the worktree before its session has shut down is the ordering this producer exists to prevent.
// A non-empty abandoned-session return is logged and recorded under scratchDir, and does not change the verdict.
//
// It then calls deps.Remove.
// A Remove error yields Stuck, naming worktree removal as the failed half.
// The reason also states that session shutdown already succeeded, so the two halves can be told apart.
// On success Call returns Done.
func (p *worktreeTeardownProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	release, reason, err := acquirePrimeLock(ctx, p.name, p.slug, p.primeLock)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if release == nil {
		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
	}
	defer func() {
		if rerr := release(); rerr != nil {
			logger.Warn("battenshed: release prime lock failed", "producer", p.name, "slug", p.slug, "path", p.primeLock.Path, "error", rerr)
		}
	}()

	abandonedSession, shutdownErr := p.deps.Shutdown(ctx)
	if shutdownErr != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		reason := fmt.Sprintf("session shutdown failed: %s", shutdownErr.Error())
		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
	}
	recordAbandonedSession(p.name, p.slug, abandonedSession, p.scratchDir)

	if err := p.deps.Remove(ctx); err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		reason := fmt.Sprintf("worktree removal failed (session shutdown already succeeded): %s", err.Error())
		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
	}

	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}
