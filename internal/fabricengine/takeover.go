// takeover.go reads, from a records branch's fetched upstream, whether another machine took a run over.
// It also holds this machine's hand-over marks of the runs it gave up.
// A rejected records push consults it before any replay, so a run that moved is never pushed again from here.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
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

// isTakeoverStop reports whether err is a *TakeoverError or wraps ErrHandedOver.
func isTakeoverStop(err error) bool {
	var takeover *TakeoverError
	return errors.As(err, &takeover) || errors.Is(err, ErrHandedOver)
}

// Holds reports whether name is one of the names that read as this machine, its current name or a former one.
func (n MachineNames) Holds(name string) bool {
	return name != "" && (name == n.Current || slices.Contains(n.Former, name))
}

// isZero reports whether n carries no name, which turns the takeover read off.
func (n MachineNames) isZero() bool {
	return n.Current == "" && len(n.Former) == 0
}

// HandOverMark records that this machine handed a run over to the machine of a takeover entry.
type HandOverMark struct {
	// Machine is the machine the takeover entry names.
	Machine string `json:"machine"`
	// At is the time the takeover entry was recorded.
	At string `json:"at"`
	// Written is the time this machine wrote the mark.
	Written time.Time `json:"written"`
}

// HandOverMarkPath returns the file that holds this machine's hand-over mark of slug:
// <HubScratchDir(hubPath)>/<lymdirs.HandOverDirName>/<slug>.json.
func HandOverMarkPath(hubPath, slug string) string {
	return filepath.Join(HubScratchDir(hubPath), lymdirs.HandOverDirName, slug+".json")
}

// ReadHandOverMark reads this machine's hand-over mark of slug, accepting keys the mark does not declare.
// An absent mark reads not found.
func ReadHandOverMark(hubPath, slug string) (mark HandOverMark, found bool, err error) {
	path := HandOverMarkPath(hubPath, slug)
	mark, found, err = state.ReadJSON[HandOverMark](path, path+".lock")
	if err != nil {
		return HandOverMark{}, false, fmt.Errorf("fabricengine: read the hand-over mark of %q: %w", slug, err)
	}
	return mark, found, nil
}

// ReadUpstreamTakeover reads the takeover entry of the run slug.
// It reads at the fetched upstream tip of the records worktree at weftPath.
// The run's status file is read under the anchor that tip records.
// It comes from the slug's run directory or the legacy `self` directory.
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
func takeoverStop(l *lymcwd.Location, weftPath, slug string, here MachineNames) error {
	if here.isZero() || slug == "" {
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

// warpLocationOfRecords returns the location of the code worktree that the records worktree at weftPath pairs with.
// In a one-repository hub that worktree is the records worktree itself.
func warpLocationOfRecords(weftPath string) (*lymcwd.Location, error) {
	warpSlug, ok := pairSlugOfRecords(weftPath)
	if !ok {
		return nil, fmt.Errorf("fabricengine: %s is no weft sibling worktree", weftPath)
	}
	return lymcwd.ResolveWorktree(filepath.Join(filepath.Dir(weftPath), warpSlug))
}
