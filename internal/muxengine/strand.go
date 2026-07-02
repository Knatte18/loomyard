// strand.go implements the three strand-mutation engine ops —
// AddStrand/UpdateStrand/RemoveStrand — plus the pure decision helpers each
// composes: parent existence/cycle validation, display-name resolution, the
// hidden<->visible transition rules, and the recursive-removal cascade.
// Each exported op acquires the op lock once, delegates to an unexported
// *Locked mutation helper, then runs the shared reconcile-apply-persist
// tail (spawn.go's reconcileApplyPersistLocked) — composing reconcile
// (card 17) and apply (card 18) exactly once per op, per the batch's
// single-layer-lock decision.

package muxengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/muxengine/render"
)

// AddSpec carries the caller-supplied inputs AddStrand needs to build a new
// Strand. Role/Round/NameOverride are formatting-only inputs consumed once
// here to resolve Name — they are never persisted as Strand fields and
// never branched on afterward (the domain-free strand contract).
type AddSpec struct {
	Role, Round, NameOverride string
	Parent                    string
	Cmd, ResumeCmd            string
	Display                   render.Display
}

// Removed reports every strand RemoveStrand actually deleted: the target
// plus its whole cascaded descendant subtree, each identified by guid and
// display name.
type Removed struct {
	Strands []struct{ GUID, Name string }
}

// strandIndex returns the index of the strand with the given guid in
// strands, or -1 if none matches.
func strandIndex(strands []Strand, guid string) int {
	for i, s := range strands {
		if s.GUID == guid {
			return i
		}
	}
	return -1
}

// strandByGUID returns the strand with the given guid and true, or a zero
// Strand and false if none matches.
func strandByGUID(strands []Strand, guid string) (Strand, bool) {
	if i := strandIndex(strands, guid); i != -1 {
		return strands[i], true
	}
	return Strand{}, false
}

// wouldFormCycle reports whether linking guid as a child of parent would
// create a cycle in strands' parent chains — parent's own chain would have
// to walk back through guid. AddStrand always passes a freshly generated
// guid that cannot already appear in strands, so in practice this never
// trips during a real add; it exists as a generic, pure, unit-testable
// guard against a corrupt or reused guid rather than an implicit assumption
// baked silently into AddStrand.
func wouldFormCycle(strands []Strand, guid, parent string) bool {
	byGUID := make(map[string]Strand, len(strands))
	for _, s := range strands {
		byGUID[s.GUID] = s
	}

	cur := parent
	for cur != "" {
		if cur == guid {
			return true
		}
		s, ok := byGUID[cur]
		if !ok {
			return false
		}
		cur = s.Parent
	}
	return false
}

// directChildren returns the GUIDs of strands whose Parent equals guid, in
// table order.
func directChildren(strands []Strand, guid string) []string {
	var out []string
	for _, s := range strands {
		if s.Parent == guid {
			out = append(out, s.GUID)
		}
	}
	return out
}

// descendantSubtree returns guid and every descendant GUID beneath it (its
// whole subtree, breadth-first), so RemoveStrand can cascade a non-leaf
// removal without orphaning children into a broken parent chain.
func descendantSubtree(strands []Strand, guid string) []string {
	childrenOf := make(map[string][]string, len(strands))
	for _, s := range strands {
		if s.Parent != "" {
			childrenOf[s.Parent] = append(childrenOf[s.Parent], s.GUID)
		}
	}

	out := []string{guid}
	queue := []string{guid}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, child := range childrenOf[cur] {
			out = append(out, child)
			queue = append(queue, child)
		}
	}
	return out
}

// resolveStrandName computes AddStrand's display name from spec: an
// explicit NameOverride wins verbatim; otherwise a non-empty Role fills the
// configured strand-name template (Round/Worktree/ShortGuid substituted
// alongside it); with neither a name nor a role, the bare short guid alone
// is the name.
func resolveStrandName(template string, spec AddSpec, guid, worktreeRoot string) string {
	if spec.NameOverride != "" {
		return spec.NameOverride
	}
	if spec.Role == "" {
		return guid[:8]
	}
	parts := map[string]string{
		"<ROLE>":       spec.Role,
		"<ROUND>":      spec.Round,
		"<WORKTREE>":   filepath.Base(worktreeRoot),
		"<SHORT_GUID>": guid[:8],
	}
	return FormatStrandName(template, parts)
}

// needsLaunchOnAdd reports whether AddStrand must realize display into a
// live pane via launchStrandLocked: every anchor except hidden. A hidden
// strand registers a record with no pane and its cmd is not run at add
// time — realization is deferred to a later surface (UpdateStrand).
func needsLaunchOnAdd(display render.Display) bool {
	return display.Anchor != render.AnchorHidden
}

// needsLaunchOnSurface reports whether an UpdateStrand call is a
// hidden->visible surface — the one transition that must realize a pane via
// launchStrandLocked (GAP A), since a hidden strand was registered with no
// pane and its cmd was never run at add time.
func needsLaunchOnSurface(wasHidden bool, display render.Display) bool {
	return wasHidden && display.Anchor != render.AnchorHidden
}

// addStrandLocked builds and registers a new Strand from spec: validates
// spec.Parent (must exist, must not form a cycle), resolves its display
// name, and — unless added anchor:hidden — realizes it into a live pane via
// the shared launchStrandLocked (GAP A). It assumes the op lock is already
// held and appends the new Strand into st.Strands in place. Hermetically
// testable for a hidden add (needsLaunchOnAdd skips launchStrandLocked
// entirely, so no psmux round trip happens); a non-hidden add always makes
// a real psmux round trip via launchStrandLocked.
func (e *Engine) addStrandLocked(st *MuxState, spec AddSpec) (Strand, error) {
	guid, err := newGUID()
	if err != nil {
		return Strand{}, fmt.Errorf("generate guid: %w", err)
	}

	if spec.Parent != "" {
		if _, ok := strandByGUID(st.Strands, spec.Parent); !ok {
			return Strand{}, fmt.Errorf("unknown parent %q", spec.Parent)
		}
		if wouldFormCycle(st.Strands, guid, spec.Parent) {
			return Strand{}, fmt.Errorf("parent %q would form a cycle", spec.Parent)
		}
	}

	st.Strands = append(st.Strands, Strand{
		GUID:      guid,
		Name:      resolveStrandName(e.cfg.StrandName, spec, guid, e.layout.WorktreeRoot),
		Worktree:  e.layout.WorktreeRoot,
		Parent:    spec.Parent,
		Cmd:       spec.Cmd,
		ResumeCmd: spec.ResumeCmd,
		Display:   spec.Display,
	})
	strand := &st.Strands[len(st.Strands)-1]

	if needsLaunchOnAdd(spec.Display) {
		if err := e.launchStrandLocked(st, strand, strand.Cmd); err != nil {
			return Strand{}, fmt.Errorf("launch strand: %w", err)
		}
	}

	return *strand, nil
}

// updateStrandLocked mutates guid's Display: rejects a visible->hidden
// transition outright (v1 cannot hide a live strand), and — on a
// hidden->visible transition — realizes the strand into a live pane via
// launchStrandLocked before returning (GAP A). It assumes the op lock is
// already held. Hermetically testable except for the actual surfacing
// launch, which always makes a real psmux round trip.
func (e *Engine) updateStrandLocked(st *MuxState, guid string, display render.Display) (Strand, error) {
	idx := strandIndex(st.Strands, guid)
	if idx == -1 {
		return Strand{}, fmt.Errorf("unknown strand %q", guid)
	}
	strand := &st.Strands[idx]

	wasHidden := strand.Display.Anchor == render.AnchorHidden
	if !wasHidden && display.Anchor == render.AnchorHidden {
		return Strand{}, fmt.Errorf("cannot hide a live strand in v1")
	}

	strand.Display = display

	if needsLaunchOnSurface(wasHidden, display) {
		if err := e.launchStrandLocked(st, strand, strand.Cmd); err != nil {
			return Strand{}, fmt.Errorf("launch strand: %w", err)
		}
	}

	return *strand, nil
}

// removeStrandLocked removes guid from st.Strands: a non-leaf without
// recursive is rejected outright; otherwise the whole descendant subtree is
// cascaded away (never orphaning children into a broken parent chain). It
// assumes the op lock is already held, never touches psmux itself, and so
// is fully hermetically testable.
func (e *Engine) removeStrandLocked(st *MuxState, guid string, recursive bool) (Removed, error) {
	if _, ok := strandByGUID(st.Strands, guid); !ok {
		return Removed{}, fmt.Errorf("unknown strand %q", guid)
	}
	if len(directChildren(st.Strands, guid)) > 0 && !recursive {
		return Removed{}, fmt.Errorf("strand has children, use --recursive")
	}

	toRemove := descendantSubtree(st.Strands, guid)
	removeSet := make(map[string]bool, len(toRemove))
	for _, g := range toRemove {
		removeSet[g] = true
	}

	var removed Removed
	remaining := make([]Strand, 0, len(st.Strands))
	for _, s := range st.Strands {
		if removeSet[s.GUID] {
			removed.Strands = append(removed.Strands, struct{ GUID, Name string }{s.GUID, s.Name})
			continue
		}
		remaining = append(remaining, s)
	}
	st.Strands = remaining

	return removed, nil
}

// AddStrand registers a new strand from spec and, unless added
// anchor:hidden, realizes it into a live pane and runs its cmd, then
// reconciles and re-applies the layout. The engine, not the caller, stamps
// Worktree and generates GUID, since it owns both this worktree's geometry
// and guid generation (the guid-dependent <SHORT_GUID> name token cannot be
// computed before the guid exists). Pre-flights the session's existence
// (mirroring Status) so running add before up fails with the same friendly
// "no mux session" error instead of a raw psmux error surfacing later from
// inside launchStrandLocked.
func (e *Engine) AddStrand(spec AddSpec) (Strand, error) {
	var result Strand
	err := e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		strand, err := e.addStrandLocked(st, spec)
		if err != nil {
			return err
		}

		if _, err := e.reconcileApplyPersistLocked(st); err != nil {
			return err
		}

		result, _ = strandByGUID(st.Strands, strand.GUID)
		return nil
	})
	return result, err
}

// UpdateStrand mutates guid's display settings, then reconciles and
// re-applies the layout. It rejects a visible->hidden transition
// ("cannot hide a live strand in v1"); a hidden->visible transition
// surfaces the strand (creates its pane, runs its cmd). UpdateStrand is
// engine-API-only in v1 — there is no CLI verb for it.
func (e *Engine) UpdateStrand(guid string, display render.Display) (Strand, error) {
	var result Strand
	err := e.withOpLock(func() error {
		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		if _, err := e.updateStrandLocked(st, guid, display); err != nil {
			return err
		}

		if _, err := e.reconcileApplyPersistLocked(st); err != nil {
			return err
		}

		result, _ = strandByGUID(st.Strands, guid)
		return nil
	})
	return result, err
}

// RemoveStrand removes guid and, when it has descendants, cascades the
// removal through its whole subtree (recursive must be true for a
// non-leaf, or the call errors instead of silently deleting descendants),
// then reconciles and re-applies the layout. Returns every strand actually
// removed. Pre-flights the session's existence (mirroring Status) so
// running remove before up fails with the same friendly "no mux session"
// error instead of a raw psmux error surfacing later from inside
// reconcileApplyPersistLocked's listPanes.
func (e *Engine) RemoveStrand(guid string, recursive bool) (Removed, error) {
	var result Removed
	err := e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		removed, err := e.removeStrandLocked(st, guid, recursive)
		if err != nil {
			return err
		}

		if _, err := e.reconcileApplyPersistLocked(st); err != nil {
			return err
		}

		result = removed
		return nil
	})
	return result, err
}
