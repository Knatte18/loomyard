// pausedsend.go holds the refusal of a send into a session a pause covers.
// The file defines the ErrPaused sentinel and the PausedError, which carries the judgment.
// It also holds the runner's coverage reading of a send's target.

package shuttleengine

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// ErrPaused marks a send refused because a live pause entry covers the target session: nothing was typed.
// It is its own sentinel and never wraps a busy-session error, so a paused send is never mistaken for a busy one.
var ErrPaused = errors.New("shuttle: a pause covers the session")

// PausedError is the refusal of a send into a session a live pause entry covers.
// Slug is the slug the coverage read judged.
// Entry is the covering entry.
// Err is the full error, whose text names the resume that lifts the hold.
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

// targetSlug returns the slug of the target strand's own name as reed reports it.
// For a prime name it returns the empty slug, which only a hub entry covers.
// name is the strand name the caller already holds.
// Only when it is empty does targetSlug ask reed for the strand's name by guid.
// A name that does not parse, and a strand reed does not list, falls back to the runner's geometry slug.
// A failed reed status does the same, and warns once per change of error.
func (r *Runner) targetSlug(guid, name string) string {
	slug, _ := r.resolveTargetSlug(guid, name)
	return slug
}

// resolveTargetSlug returns the target slug for guid and name.
// resolved reports whether the slug came from a parsed strand name, not the runner's geometry slug.
func (r *Runner) resolveTargetSlug(guid, name string) (slug string, resolved bool) {
	if name == "" {
		status, err := r.reed.Status()
		r.warnOnChange(&r.statusReadWarned, "shuttle: reed status unreadable, judging a send's pause coverage by the runner's slug", err, "strandGUID", guid)
		if err != nil {
			return r.pause.Slug, false
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
		return r.pause.Slug, false
	}
	return parsed.Slug, true
}

// pausedTarget returns a *PausedError when a live pause entry covers the strand that guid names.
// It judges the strand's own slug at the runner's clock.
// A runner with no pause geometry reads uncovered.
// An unreadable record also reads uncovered.
// It warns once per change of error, so a corrupt file never freezes sends.
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

// warnOnChange logs msg at Warn when err is non-nil and its text differs from *last.
// It then keeps err's text in *last, and a nil err clears it.
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

// pausedRefusal returns the refusal of a send into a session a pause covers.
// It returns nil for a context built with no coverage reading.
func (sc sendContext) pausedRefusal() error {
	if sc.paused == nil {
		return nil
	}
	return sc.paused()
}
