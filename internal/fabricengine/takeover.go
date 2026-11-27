// takeover.go reads, from a records branch's fetched upstream, whether another machine took a run over.
// It also holds this machine's hand-over marks of the runs it gave up.
// A rejected records push consults it before any replay, so a run that moved is never pushed again from here.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// ErrHandedOver is wrapped by the error of a push that was not run because this machine already handed the run over.
var ErrHandedOver = errors.New("fabricengine: the run was handed over to another machine, so nothing more is pushed for it")

// TakeoverError is the error of a rejected records push whose run origin records as taken over by another machine.
// Nothing was replayed, pushed or recorded as a failed push.
type TakeoverError struct {
	// Slug is the run the takeover entry names.
	Slug string
	// Entry is the takeover entry origin holds for the run.
	Entry shedengine.Takeover
	// RecordsWorktree is the records worktree whose push met the entry.
	RecordsWorktree string
}

// Error names the run and the machine that took it over, and the way forward.
func (e *TakeoverError) Error() string {
	return fmt.Sprintf("fabricengine: the run %q was taken over by machine %q (adopt subject %q, at %s), so the records at %s are not pushed from here; way forward: this machine hands the run over, and `lym adopt` takes it back",
		e.Slug, e.Entry.Machine, e.Entry.Subject, e.Entry.At, e.RecordsWorktree)
}

// TakeoversError is the error of a rejected push of the prime's records that met takeover entries of batten runs.
// Each run named is one whose local commits origin lacks and whose entry on origin names another machine.
// Nothing was replayed, pushed or recorded as a failed push.
type TakeoversError struct {
	// Takeovers holds one *TakeoverError per taken-over run, in the order the runs' unpushed commits were made.
	Takeovers []*TakeoverError
}

// Error joins the error of each taken-over run.
func (e *TakeoversError) Error() string {
	messages := make([]string, 0, len(e.Takeovers))
	for _, takeover := range e.Takeovers {
		messages = append(messages, takeover.Error())
	}
	return strings.Join(messages, "; ")
}

// ErrTakeoverPassOwed is wrapped by the error of a push of the prime's records whose takeover read failed.
// Nothing was replayed or pushed, and the push is owed: the next attempt reads every takeover again.
var ErrTakeoverPassOwed = errors.New("fabricengine: the takeover read of the prime's records push failed, so the push is owed")

// isTakeoverStop reports whether err stops a push for a takeover.
// It is a *TakeoverError or a *TakeoversError, or wraps ErrHandedOver or ErrTakeoverPassOwed.
func isTakeoverStop(err error) bool {
	var takeover *TakeoverError
	var takeovers *TakeoversError
	return errors.As(err, &takeover) || errors.As(err, &takeovers) ||
		errors.Is(err, ErrHandedOver) || errors.Is(err, ErrTakeoverPassOwed)
}

// ReadHandOverMark reads this machine's hand-over mark of slug, accepting keys the mark does not declare.
// An absent mark reads not found.
func ReadHandOverMark(hubPath, slug string) (mark fabriclayout.HandOverMark, found bool, err error) {
	path := fabriclayout.HandOverMarkPath(hubPath, slug)
	mark, found, err = state.ReadJSON[fabriclayout.HandOverMark](path, path+".lock")
	if err != nil {
		return fabriclayout.HandOverMark{}, false, fmt.Errorf("fabricengine: read the hand-over mark of %q: %w", slug, err)
	}
	return mark, found, nil
}

// ReadUpstreamTakeover reads the takeover entry of the run slug.
// It reads at the fetched upstream tip of the records worktree at weftPath.
// The run's status file is read under the anchor that tip records.
// It comes from the slug's run directory.
// In a pair that is the child record.
// On the prime's records branch it is the record of the run slug the caller names.
// It reports found false when the worktree has no upstream, or the tip holds no record or no takeover entry.
// l is any location of the hub, which names the wired directories the anchor is derived from.
func ReadUpstreamTakeover(l *lymcwd.Location, weftPath, slug string) (entry shedengine.Takeover, found bool, err error) {
	upstream, hasUpstream, err := upstreamSHAAt(weftPath)
	if err != nil || !hasUpstream {
		return shedengine.Takeover{}, false, err
	}
	status, found, err := originStatusAtTip(l, weftPath, upstream, slug)
	if err != nil || !found || status.Takeover == nil {
		return shedengine.Takeover{}, false, err
	}
	return *status.Takeover, true, nil
}

// takeoverStop judges whether the run slug was taken over by another machine, from the fetched tip of the records branch.
// It does nothing for a zero here or an empty slug.
// No entry, or an entry naming a machine that here holds, returns nil.
// An entry naming another machine returns an error.
// When this machine's hand-over mark of the slug names that machine, the error wraps ErrHandedOver.
// Otherwise it is a *TakeoverError.
func takeoverStop(l *lymcwd.Location, weftPath, slug string, here fabriclayout.MachineNames) error {
	if here.IsZero() || slug == "" {
		return nil
	}
	entry, found, err := ReadUpstreamTakeover(l, weftPath, slug)
	if err != nil {
		return err
	}
	if !found || here.Holds(entry.Machine) {
		return nil
	}
	mark, marked, err := ReadHandOverMark(l.HubPath, slug)
	if err != nil {
		return err
	}
	if marked && mark.Machine == entry.Machine {
		return fmt.Errorf("%w: machine %q holds the run %q", ErrHandedOver, entry.Machine, slug)
	}
	return &TakeoverError{Slug: slug, Entry: entry, RecordsWorktree: weftPath}
}

// primeTakeovers reads the takeover entry of each run in ahead.
// It reads from the fetched upstream tip of the prime's records branch.
// ahead holds the slugs the Run-Transition trailers of the local commits origin lacks name, oldest first, each once.
// A run with no entry, or whose entry names a machine here holds, is left alone.
// A taken-over run whose hand-over mark names the entry's machine is listed in handedOver.
// Any other taken-over run is returned in pending as its *TakeoverError.
func primeTakeovers(l *lymcwd.Location, weftPath string, ahead []string, here fabriclayout.MachineNames) (pending []*TakeoverError, handedOver []string, err error) {
	for _, slug := range ahead {
		entry, found, err := ReadUpstreamTakeover(l, weftPath, slug)
		if err != nil {
			return nil, nil, err
		}
		if !found || here.Holds(entry.Machine) {
			continue
		}
		mark, marked, err := ReadHandOverMark(l.HubPath, slug)
		if err != nil {
			return nil, nil, err
		}
		if marked && mark.Machine == entry.Machine {
			handedOver = append(handedOver, slug)
			continue
		}
		pending = append(pending, &TakeoverError{Slug: slug, Entry: entry, RecordsWorktree: weftPath})
	}
	return pending, handedOver, nil
}

// primePushTakeovers reads the takeovers a push of the prime's records branch at weftPath meets, after its fetch.
// It reads the run runSlug, when one is named.
// It also reads every run that a local commit origin lacks carries a Run-Transition trailer of.
// It returns the runs already handed over, whose commits the replay sets aside.
// A pending takeover returns a *TakeoversError holding every pending one.
// A failed read returns an error wrapping ErrTakeoverPassOwed.
func primePushTakeovers(l *lymcwd.Location, weftPath, runSlug string, here fabriclayout.MachineNames) (handedOver []string, err error) {
	slugs, err := unpushedTransitionSlugs(weftPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTakeoverPassOwed, err)
	}
	if runSlug != "" && !slices.Contains(slugs, runSlug) {
		slugs = append(slugs, runSlug)
	}
	pending, handedOver, err := primeTakeovers(l, weftPath, slugs, here)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTakeoverPassOwed, err)
	}
	if len(pending) > 0 {
		return nil, &TakeoversError{Takeovers: pending}
	}
	return handedOver, nil
}

// unpushedTransitionSlugs returns the slugs that the Run-Transition trailers name.
// It reads the commits at weftPath that its upstream lacks, and lists the slugs oldest first and each once.
// A branch with no upstream names none.
func unpushedTransitionSlugs(weftPath string) ([]string, error) {
	upstream, hasUpstream, err := upstreamSHAAt(weftPath)
	if err != nil || !hasUpstream {
		return nil, err
	}
	repo := gitrepo.New(weftPath)
	head, err := repo.CurrentSHA()
	if err != nil {
		return nil, err
	}
	commits, err := repo.CommitsNotIn(head, upstream)
	if err != nil {
		return nil, err
	}
	slices.Reverse(commits)
	var slugs []string
	for _, sha := range commits {
		detail, err := repo.CommitDetail(sha)
		if err != nil {
			return nil, err
		}
		if slug := trailerValue(detail.Message, fabriclayout.TransitionTrailerKey); slug != "" && !slices.Contains(slugs, slug) {
			slugs = append(slugs, slug)
		}
	}
	return slugs, nil
}

// warpLocationOfRecords returns the location of the code worktree that the records worktree at weftPath pairs with.
func warpLocationOfRecords(weftPath string) (*lymcwd.Location, error) {
	warpSlug, ok := fabriclayout.WeftWarpSlug(filepath.Base(weftPath))
	if !ok {
		return nil, fmt.Errorf("fabricengine: %s is no weft sibling worktree", weftPath)
	}
	return lymcwd.ResolveWorktree(filepath.Join(filepath.Dir(weftPath), warpSlug))
}
