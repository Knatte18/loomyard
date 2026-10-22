// pausedsend.go holds the refusal of a send into a session a pause covers: the ErrPaused sentinel, the PausedError that carries the judgment, and the runner's coverage reading of a send's target.

package shuttleengine

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// ErrPaused marks a send refused because a live pause entry covers the target session: nothing was typed.
// It is its own sentinel and never wraps ErrSessionBusy, so a caller that holds a busy send does not mistake a paused one for it.
var ErrPaused = errors.New("shuttle: a pause covers the session")

// PausedError is the refusal of a send into a session a live pause entry covers.
// Slug is the slug the coverage read judged, Entry the covering entry, and Err the full error, whose text names the resume that lifts the hold.
// A caller that renders its own retry reads Entry and Slug.
type PausedError struct {
	Slug  string
	Entry pauserecord.Entry
	Err   error
}

// Error returns the message of Err.
func (e *PausedError) Error() string {
	return e.Err.Error()
}

// Unwrap returns Err, so errors.Is(err, ErrPaused) holds.
func (e *PausedError) Unwrap() error {
	return e.Err
}

// targetSlug returns the slug of the target strand's own name as reed reports it, the empty slug for a prime name, which only a hub entry covers.
// name is the strand name the caller already holds; only when it is empty does targetSlug ask reed for the strand's name by guid.
// A name that does not parse and a strand reed does not list fall back to the runner's geometry slug, as does a failed reed status, which warns once per change of error.
func (r *Runner) targetSlug(guid, name string) string {
	if name == "" {
		status, err := r.reed.Status()
		r.warnOnChange(&r.statusReadWarned, "shuttle: reed status unreadable, judging a send's pause coverage by the runner's slug", err, "strandGUID", guid)
		if err != nil {
			return r.pause.Slug
		}
		for _, strand := range status.Strands {
			if strand.GUID == guid {
				name = strand.Name
				break
			}
		}
	}
	parsed, err := agentname.Parse(name)
	if err != nil {
		return r.pause.Slug
	}
	return parsed.Slug
}

// pausedTarget returns a *PausedError when a live pause entry covers the strand guid names, judged by the strand's own slug at the runner's clock.
// A runner told no pause geometry reads uncovered, and so does an unreadable record, which warns once per change of error so a corrupt file never freezes sends.
// It only reads: it types nothing, records nothing and never waits.
func (r *Runner) pausedTarget(guid, name string) error {
	if r.pause.RecordPath == "" {
		return nil
	}
	slug := r.targetSlug(guid, name)
	coverage, err := pauserecord.ReadCoverage(r.pause.RecordPath, slug, r.clock.Now())
	r.warnOnChange(&r.coverageReadWarned, "shuttle: pause record unreadable, reading a send's target as uncovered", err, "strandGUID", guid, "path", r.pause.RecordPath)
	if err != nil || !coverage.Covered {
		return nil
	}
	return &PausedError{
		Slug:  slug,
		Entry: coverage.Entry,
		Err:   fmt.Errorf("%w: strand %q: %w", ErrPaused, guid, pauserecord.CoveredError(coverage.Entry, slug, "retry the send")),
	}
}

// warnOnChange logs msg at Warn when err is non-nil and its text differs from *last, then keeps err's text in *last, cleared by a nil err.
func (r *Runner) warnOnChange(last *string, msg string, err error, args ...any) {
	r.pauseWarnMu.Lock()
	defer r.pauseWarnMu.Unlock()
	if err == nil {
		*last = ""
		return
	}
	if err.Error() == *last {
		return
	}
	*last = err.Error()
	logger.Warn(msg, append(args, "error", err)...)
}

// pausedRefusal returns the refusal of a send into a session a pause covers, nil for a context built with no coverage reading.
func (sc sendContext) pausedRefusal() error {
	if sc.paused == nil {
		return nil
	}
	return sc.paused()
}
