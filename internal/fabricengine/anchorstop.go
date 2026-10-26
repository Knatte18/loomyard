// anchorstop.go compares a weft's anchor marker with its upstream's and defines the stop that a difference, or a marker left empty, raises.
// The weft pull, the fast-forward behind a landing and the replaying push all stop on it instead of crossing an anchor change.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// ErrAnchorChange is wrapped by every stop that keeps a weft from crossing a change of its anchor marker, or from working with a marker it cannot read.
var ErrAnchorChange = errors.New("fabricengine: weft anchor marker change")

// PushFailureAnchorChange means the push was held back because the upstream carries another anchor marker than the weft, or the weft's own marker is empty.
// The record carries the way forward the stop named.
const PushFailureAnchorChange PushFailureClass = "anchor_change"

// AnchorStopError is the stop of a weft operation that would have crossed an anchor change.
// Reason says what differs, and WayForward names the commands that settle it.
// It reports errors.Is(err, ErrAnchorChange), and its message is the reason alone: a caller adds the way forward where it prints one.
type AnchorStopError struct {
	Reason     string
	WayForward string
}

// Error returns the reason of the stop.
func (e *AnchorStopError) Error() string {
	return "fabricengine: " + e.Reason
}

// Unwrap returns ErrAnchorChange, so errors.Is sees every anchor stop as one.
func (e *AnchorStopError) Unwrap() error {
	return ErrAnchorChange
}

// weftStop returns the stop as a *WeftStopError for branch, whose Err wraps ErrAnchorChange.
func (e *AnchorStopError) weftStop(branch string) *WeftStopError {
	return &WeftStopError{
		Branch:     branch,
		Err:        fmt.Errorf("%w: %s", ErrAnchorChange, e.Reason),
		WayForward: e.WayForward,
	}
}

// weftAnchorStop compares the anchor of the weft tip head with that of the upstream tip upstream, each read by treeAnchor with names and fallback.
// It stops when upstream has commits head lacks and the two anchors differ:
// taking upstream in, by fast-forward or by replay, would then leave a marker other than the checked-out one or carry local commits across a marker change.
// A head that already contains upstream never stops, so a re-anchor not yet pushed goes through.
// wantAnchor is the anchor that settles the stop: upstream's when head's anchor equals the merge base's, since only upstream moved it, and head's otherwise.
// With reset, for a caller that replaces the checked-out tree with upstream's, ancestry is ignored:
// any difference between the two anchors stops, and wantAnchor is upstream's.
func weftAnchorStop(weftPath, head, upstream string, names []string, fallback string, reset bool) (wantAnchor string, stop bool, err error) {
	repo := gitrepo.New(weftPath)
	headSHA, err := repo.ResolveSHA(head)
	if err != nil {
		return "", false, err
	}
	upstreamSHA, err := repo.ResolveSHA(upstream)
	if err != nil {
		return "", false, err
	}

	headAnchor, err := treeAnchor(weftPath, headSHA, names, fallback)
	if err != nil {
		return "", false, err
	}
	upstreamAnchor, err := treeAnchor(weftPath, upstreamSHA, names, fallback)
	if err != nil {
		return "", false, err
	}
	if headAnchor == upstreamAnchor {
		return "", false, nil
	}
	if reset {
		return upstreamAnchor, true, nil
	}

	taken, err := repo.CommitsNotIn(upstreamSHA, headSHA)
	if err != nil {
		return "", false, err
	}
	if len(taken) == 0 {
		return "", false, nil
	}
	baseSHA, err := repo.MergeBase(headSHA, upstreamSHA)
	if err != nil {
		return "", false, err
	}
	baseAnchor, err := treeAnchor(weftPath, baseSHA, names, fallback)
	if err != nil {
		return "", false, err
	}
	if headAnchor == baseAnchor {
		return upstreamAnchor, true, nil
	}
	return headAnchor, true, nil
}

// emptyMarkerStop returns the stop for a weft worktree whose marker file is present but blank, and nil when it is not.
// Its way forward is the refusal's own: restore the committed marker, or write the anchor and commit it.
func emptyMarkerStop(weftPath string) *AnchorStopError {
	err := refuseEmptyOwnMarker(weftPath)
	if err == nil {
		return nil
	}
	return &AnchorStopError{
		Reason:     fmt.Sprintf("the weft's %s is empty", lyxcwd.AnchorFileName),
		WayForward: err.Error(),
	}
}

// refuseDropLocalAnchorChange refuses to move the weft worktree at weftPath of the pair slug onto origin's tip tracking while the anchors of the two differ.
// A local weft ahead of origin with a re-anchor not yet pushed differs as well, since the comparison ignores ancestry.
// An empty own marker is refused first.
// Both refusals wrap ErrAnchorChange, and the difference names the anchor change and ends with re-running the same adopt under --drop-local, since the pair may be a moved run that no adopt names.
func refuseDropLocalAnchorChange(l *lyxcwd.Location, slug, weftPath, tracking string) error {
	if stop := emptyMarkerStop(weftPath); stop != nil {
		return fmt.Errorf("%w: %s", stop, stop.WayForward)
	}
	pair, err := lyxcwd.NamedWorktree(l.HubPath, slug)
	if err != nil {
		return err
	}
	names, err := RepoWiredNames(pair)
	if err != nil {
		return fmt.Errorf("load the wired names to compare anchor markers: %w", err)
	}
	want, stop, err := weftAnchorStop(weftPath, "HEAD", tracking, names, pair.AnchorRel, true)
	if err != nil || !stop {
		return err
	}
	return fmt.Errorf("%w: the weft of %s and origin's tip carry different anchors, origin's being %q, so moving it to origin's tip would cross an anchor change; run `lyx fabric anchor %s --set %s`, then re-run the same `lyx adopt` with `--drop-local`",
		ErrAnchorChange, slug, want, slug, want)
}

// upstreamAnchorStop compares the weft worktree at weftPath, as checked out, with the upstream tip upstream.
// The pair's fallback anchor and wired names are read from its own recorded anchor, and codeWorktree is where the way forward runs.
// It returns nil when the weft may take upstream in.
func upstreamAnchorStop(weftPath, codeWorktree, upstream string) (*AnchorStopError, error) {
	slug, ok := WeftWarpSlug(filepath.Base(weftPath))
	if !ok {
		return nil, fmt.Errorf("fabricengine: %s is no weft sibling worktree", weftPath)
	}
	pair, err := lyxcwd.NamedWorktree(filepath.Dir(weftPath), slug)
	if err != nil {
		return nil, err
	}
	names, err := RepoWiredNames(pair)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: load the wired names to compare anchor markers: %w", err)
	}
	want, stop, err := weftAnchorStop(weftPath, "HEAD", upstream, names, pair.AnchorRel, false)
	if err != nil || !stop {
		return nil, err
	}
	return &AnchorStopError{
		Reason: "origin's weft branch carries another anchor marker than this checkout, so its commits cannot be taken in without crossing an anchor change",
		WayForward: fmt.Sprintf("run `lyx fabric anchor %s --set %s` in %s, which integrates the other marker, then re-run this command",
			slug, want, codeWorktree),
	}, nil
}
