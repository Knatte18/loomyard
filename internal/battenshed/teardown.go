// teardown.go implements the producer that removes a task worktree.
// It shuts the loom session down, pushes the pair to origin, then removes the worktree, strictly in that order.
// It holds the hub's prime lock for the shutdown and for the removal, and none while it pushes.

package battenshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// abandonedSessionFileName is the fixed name of the abandoned-session record inside a run's scratch directory.
const abandonedSessionFileName = "abandoned-session"

// maxTeardownPushRounds bounds the pushes the teardown makes while origin still lacks a tip of the pair.
const maxTeardownPushRounds = 3

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

// worktreeTeardownProducer shuts down the task worktree's loom session and pushes the pair.
// It then removes the worktree pair.
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
// It then calls deps.Push with no prime lock held, so the push's own waits never hold the lock.
// It takes primeLock again and reads deps.Level: a false read means a tip moved since the push,
// so it releases the lock and pushes again, up to maxTeardownPushRounds pushes.
// A true read calls deps.Remove and releases the lock.
// A push that reports the pair's entry ended skips the Level read, since nothing is left to push.
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
// A Push error wrapping ErrPushEntryEnded goes on to the removal at once.
// One wrapping ErrPushHandedOver yields Stuck naming the hand-over.
// The pair is already removed and the run is another machine's.
// Any other Push error yields Stuck with its text and the resume, and keeps the pair.
// A Level error yields Stuck naming the read and the resume.
// A Level that still reads false after the last of maxTeardownPushRounds pushes yields Stuck naming the resume,
// so a push that lands nothing never loops.
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

	for round, removed := 1, false; !removed; round++ {
		reason, entryEnded, err := p.pushPair(ctx)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if reason != "" {
			return stuck(reason)
		}
		reason, err = p.underPrimeLock(ctx, func() (string, error) {
			level := entryEnded
			if !entryEnded {
				var levelReason string
				var levelErr error
				if level, levelReason, levelErr = p.pairIsLevel(ctx); levelErr != nil || levelReason != "" {
					return levelReason, levelErr
				}
			}
			if !level {
				if round >= maxTeardownPushRounds {
					return fmt.Sprintf("origin still lacks a tip of the pair after %d pushes, so the pair is kept; once the tips settle, resume with `lym batten run %s`", round, p.slug), nil
				}
				return "", nil
			}
			removed = true
			return p.removePair(ctx)
		})
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if reason != "" {
			return stuck(reason)
		}
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
// It returns the stuck reason of a push that cannot go on, and whether origin ended the pair's entry.
// A push error wrapping ErrPushEntryEnded is no failure: nothing is left to push.
func (p *worktreeTeardownProducer) pushPair(ctx context.Context) (stuckReason string, entryEnded bool, err error) {
	if p.deps.Push == nil {
		return "", false, nil
	}
	pushErr := p.deps.Push(ctx)
	if pushErr == nil {
		return "", false, nil
	}
	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", false, cerr
	}
	switch {
	case errors.Is(pushErr, ErrPushEntryEnded):
		return "", true, nil
	case errors.Is(pushErr, ErrPushHandedOver):
		return fmt.Sprintf("the run was handed over to another machine and its pair is already removed here, so no resume applies here: %s", pushErr.Error()), false, nil
	}
	return fmt.Sprintf("the push step failed and the pair is kept: %s; once its way forward is done, resume with `lym batten run %s`", pushErr.Error(), p.slug), false, nil
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
		return false, fmt.Sprintf("reading whether origin holds the pair's tips failed and the pair is kept: %s; resume with `lym batten run %s`", levelErr.Error(), p.slug), nil
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
