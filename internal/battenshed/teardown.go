// teardown.go implements NewWorktreeTeardown, the producer that shuts down the loom session and
// removes the task worktree, strictly in that order, under the hub's prime lock.

package battenshed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// abandonedSessionFileName is the fixed name the abandoned-session record carries inside a run's
// told scratch directory.
const abandonedSessionFileName = "abandoned-session"

// AbandonedSessionFile returns the path recordAbandonedSession writes the abandoned session's name
// to, under scratchDir.
// It is exported for the same reason StuckReasonFile is: the file's reader and writer share one
// declarer of its name.
func AbandonedSessionFile(scratchDir string) string {
	return filepath.Join(scratchDir, abandonedSessionFileName)
}

// recordAbandonedSession logs and records that session shutdown abandoned a session rather than
// ending it cleanly, and clears any stale record when it did not.
// The record outlives the process that observed it, which is what lets a step-driven lifecycle
// report the value as well as a run-driven one.
// A write or remove failure is logged rather than escalated: failing to record how a teardown ended
// must not change whether it succeeded.
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

// worktreeTeardownProducer shuts down the task worktree's loom session and then removes the
// worktree pair, holding the hub's prime lock for the duration of both calls.
type worktreeTeardownProducer struct {
	name       string
	slug       string
	deps       TeardownDeps
	primeLock  PrimeLock
	scratchDir string
}

var _ shedengine.ShedProducer = (*worktreeTeardownProducer)(nil)

// NewWorktreeTeardown returns a shedengine.ShedProducer that acquires primeLock, calls
// deps.Shutdown, and -- only on Shutdown's success -- calls deps.Remove, for the task worktree
// identified by slug.
func NewWorktreeTeardown(name, slug string, deps TeardownDeps, primeLock PrimeLock, scratchDir string) shedengine.ShedProducer {
	return &worktreeTeardownProducer{
		name:       name,
		slug:       slug,
		deps:       deps,
		primeLock:  primeLock,
		scratchDir: scratchDir,
	}
}

// Call implements shedengine.ShedProducer. It acquires the prime lock exactly as
// worktreeCreateProducer.Call does -- an Acquire error is a returned hard error, ok == false is
// Stuck naming primeLock.Path, and the release closure is deferred so it runs on every exit path
// including every Stuck one, with a release error logged at Warn rather than replacing the
// verdict.
//
// It then calls deps.Shutdown. A Shutdown error is Stuck naming session shutdown as the failed
// half, and deps.Remove is not called at all on that path -- abandoning that ordering is the
// single thing this one-row producer exists to prevent. A non-empty abandonedSession return is
// logged and recorded under scratchDir (recordAbandonedSession), and does not change the verdict.
//
// It then calls deps.Remove. A Remove error is Stuck naming worktree removal as the failed half
// and stating that session shutdown already succeeded, so the two halves are distinguishable in
// the reason text. On success the row returns Done.
func (p *worktreeTeardownProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	release, ok, err := p.primeLock.Acquire()
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("battenshed: %s: acquire prime lock %q: %w", p.name, p.primeLock.Path, err)
	}
	if !ok {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		reason := fmt.Sprintf("prime lock %q is already held; another batten producer is creating or tearing down a task worktree", p.primeLock.Path)
		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{}, nil
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
		return shedengine.Stuck, shedengine.OutputPointer{}, nil
	}
	recordAbandonedSession(p.name, p.slug, abandonedSession, p.scratchDir)

	if err := p.deps.Remove(ctx); err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		reason := fmt.Sprintf("worktree removal failed (session shutdown already succeeded): %s", err.Error())
		reportStuck(p.name, reason, p.scratchDir, "slug", p.slug)
		return shedengine.Stuck, shedengine.OutputPointer{}, nil
	}

	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}
