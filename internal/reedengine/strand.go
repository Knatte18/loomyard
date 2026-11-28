// strand.go implements the strand-mutation engine ops AddStrand, UpdateStrand and RemoveStrand.
// It also holds the pure helpers they compose:
// parent validation, display-name resolution, hidden/visible transitions, and the recursive-removal cascade.
// Each exported op acquires the op lock once and calls an unexported *Locked mutation helper.
// It then runs the shared tail, reconcileApplyPersistLocked in spawn.go, so reconcile and apply run once per op.

package reedengine

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
)

// ErrUnknownStrand is the sentinel wrapped by a refusal for a guid that names no strand.
// A caller uses errors.Is to tell a strand that is already gone from other failures.
var ErrUnknownStrand = errors.New("unknown strand")

// AddSpec carries the caller-supplied inputs AddStrand needs to build a new Strand.
type AddSpec struct {
	// Role is the role segment of the strand's name, numbered `-2`, `-3` against the worktree's strands when taken;
	// empty means reed's default role, defaultRole.
	Role string
	// NameOverride is an explicit name, a role segment or a full name, and is never renumbered:
	// an add naming a name another strand holds is refused.
	NameOverride   string
	Parent         string
	Cmd, ResumeCmd string
	// SessionID is opaque caller metadata that reed never reads.
	// It is stamped verbatim onto the appended Strand, and reed never interprets or branches on it.
	SessionID string
	Display   render.Display
	// IfAbsent is opt-in: false (the default) adds the strand unconditionally.
	// When true, AddStrand first matches NameOverride against this worktree's persisted strands.
	// A matched strand is a no-op, unless it is dead, in which case it is relaunched.
	// When nothing matches, the strand is added as usual.
	// NameOverride must be non-empty when IfAbsent is set.
	IfAbsent bool
	// DoneWhen is copied verbatim onto the new strand's DoneWhen.
	DoneWhen []string
	// Segment is stamped verbatim onto the new strand's Segment.
	Segment string
	// Provider is stamped verbatim onto the new strand's Provider.
	Provider string
	// CreelHooks is stamped verbatim onto the new strand's CreelHooks.
	CreelHooks bool
}

// Removed reports every strand a removal deleted: the target plus its whole descendant subtree.
type Removed struct {
	Strands []struct{ GUID, Name string }
	// ConfigWarning names the invalid window config key that skipped the shared-window layout; empty when none.
	ConfigWarning string
}

// validateDisplay rejects an invalid Display at the op boundary, before any pane is launched or state persisted.
// The anchor must be hidden, own-window or overview, and render must accept the whole display.
// The retired below-parent and top anchors are refused with the way forward.
// An overview display takes no window group, since it always gets the window of its own.
func validateDisplay(display render.Display) error {
	switch display.Anchor {
	case render.AnchorHidden, render.AnchorOwnWindow:
		return render.ValidateDisplay(display)
	case "below-parent", "top":
		return fmt.Errorf("anchor %q is retired; way forward: use %s", display.Anchor, render.AnchorOwnWindow)
	case render.AnchorOverview:
		if display.Window != "" || display.WindowSize != 0 {
			return fmt.Errorf("anchor %q takes no window group; way forward: drop the group, or use %s", display.Anchor, render.AnchorOwnWindow)
		}
		return render.ValidateDisplay(display)
	default:
		return fmt.Errorf("invalid anchor %q; want own-window|hidden|overview", display.Anchor)
	}
}

// refuseWindowMove refuses an update that moves a strand into, out of, or between the own-window and overview anchors.
// It also refuses an update that changes an overview strand's order or an own-window strand's window group.
// A strand's pane lives in a window of its own from launch on.
// Such a move would strand the pane where the new display does not expect it.
// The error names the way forward: remove the strand and add it again.
func refuseWindowMove(name string, current, next render.Display) error {
	if isOwnWindow(current) != isOwnWindow(next) || isOverview(current) != isOverview(next) {
		return fmt.Errorf("strand %q cannot move between own-window, overview and another anchor; way forward: lym reed remove --name %s, then add it again with the new anchor", name, name)
	}
	if isOverview(current) && current.Order != next.Order {
		return fmt.Errorf("strand %q cannot change its overview order; way forward: lym reed remove --name %s, then have its spawner add it again with the new order", name, name)
	}
	if isOwnWindow(current) && (current.Window != next.Window || current.WindowSize != next.WindowSize) {
		return fmt.Errorf("strand %q cannot change its window group; way forward: lym reed remove --name %s, then add it again with the new group", name, name)
	}
	return nil
}

// validateIfAbsent rejects spec when IfAbsent is set without NameOverride.
// It runs at the op boundary, before any state is loaded.
//
// The name is required because an auto-numbered role never matches the strand it would duplicate:
// the number moves past the existing strand, so each reopen would add a duplicate instead of finding one to match.
func validateIfAbsent(spec AddSpec) error {
	if spec.IfAbsent && spec.NameOverride == "" {
		return fmt.Errorf("--if-absent requires --name")
	}
	return nil
}

// strandIndex returns the index of the strand with the given guid, or -1.
func strandIndex(strands []Strand, guid string) int {
	for i, s := range strands {
		if s.GUID == guid {
			return i
		}
	}
	return -1
}

// strandByGUID returns the strand with the given guid and true, or a zero Strand and false.
func strandByGUID(strands []Strand, guid string) (Strand, bool) {
	if i := strandIndex(strands, guid); i != -1 {
		return strands[i], true
	}
	return Strand{}, false
}

// wouldFormCycle reports whether making guid a child of parent would create a cycle.
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

// directChildren returns the GUIDs of strands whose Parent equals guid.
func directChildren(strands []Strand, guid string) []string {
	var out []string
	for _, s := range strands {
		if s.Parent == guid {
			out = append(out, s.GUID)
		}
	}
	return out
}

// descendantSubtree returns guid and every descendant beneath it (breadth-first).
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

// defaultRole is the role a spec takes when it names neither Role nor NameOverride.
// It lets a bare `lym reed add --cmd …` run without naming a role.
const defaultRole = "strand"

// validateNaming refuses an add or replace whose name cannot be formed from the hub's shortname and slug.
// It returns the full name NameOverride resolves to, or the empty string when NameOverride is empty.
// It runs before any tmux command.
// A hub with no recorded shortname refuses every name.
// A shortname or slug that fails its grammar is a wiring guard.
func (e *Engine) validateNaming(spec AddSpec) (string, error) {
	shortname, slug := e.geom.NameShortname, e.geom.NameSlug
	if shortname == "" {
		return "", fmt.Errorf("no strand name can be formed: this hub records no shortname; way forward: lym fabric shortname <shortname> records it, then retry; a session lym refuses the verb from reports status: FAILED and the orch runs it")
	}
	if err := agentname.ValidateShortname(shortname); err != nil {
		return "", fmt.Errorf("no strand name can be formed: the told shortname is invalid: %w", err)
	}
	if slug != "" {
		if err := agentname.ValidateSlug(slug); err != nil {
			return "", fmt.Errorf("no strand name can be formed: worktree %q does not fit the name grammar [a-z][a-z0-9-]*; way forward: lym fabric add <slug> creates the task under a slug that fits; a session lym refuses the verb from reports status: FAILED and the orch runs it: %w",
				e.geom.WorktreeName, err)
		}
	}
	if spec.NameOverride == "" {
		return "", nil
	}
	n, err := agentname.Resolve(shortname, slug, spec.NameOverride)
	if err != nil {
		return "", err
	}
	return n.String(), nil
}

// strandNameLocked forms the full name of the strand spec describes against st's strands.
// An explicit NameOverride is used as given;
// otherwise Role, defaulting to defaultRole, is numbered past every role segment in state, live or dormant.
// The geometry's shortname and slug were validated by validateNaming.
func (e *Engine) strandNameLocked(st *ReedState, spec AddSpec) (string, error) {
	shortname, slug := e.geom.NameShortname, e.geom.NameSlug
	if spec.NameOverride != "" {
		n, err := agentname.Resolve(shortname, slug, spec.NameOverride)
		if err != nil {
			return "", err
		}
		full := n.String()
		for _, s := range st.Strands {
			if s.Name == full {
				return "", fmt.Errorf("strand name %q is already held by strand %s; way forward: lym reed remove --name %s frees it, or pass another role",
					full, s.GUID, n.Role)
			}
		}
		return full, nil
	}

	role := spec.Role
	if role == "" {
		role = defaultRole
	}
	held := make([]string, 0, len(st.Strands))
	for _, s := range st.Strands {
		if n, err := agentname.Parse(s.Name); err == nil {
			held = append(held, n.Role)
		}
	}
	return agentname.Format(shortname, slug, agentname.NumberRole(role, held))
}

// reclaimDeadNameLocked removes the strand holding the name spec asks for when its engine is dead.
// This lets the add take that name instead of a numbered one or a refusal.
// The asked-for name is explicitName when spec sets NameOverride, else the un-numbered role name;
// a numbered form is never reclaimed.
// The holder keeps the name, unchanged, when it is hidden, when the pair is paused, or when it has children.
// It also keeps the name when its engine reads live, a launch grace included.
// Otherwise it is removed as the ordinary removal does, its pane killed only when this session lists it,
// and the removal reaches the state file with the add's own save.
// A failed pane listing is returned, since the add lists panes again to launch and would fail the same way.
func (e *Engine) reclaimDeadNameLocked(st *ReedState, spec AddSpec, explicitName string) error {
	name := explicitName
	if spec.NameOverride == "" {
		role := spec.Role
		if role == "" {
			role = defaultRole
		}
		var err error
		if name, err = agentname.Format(e.geom.NameShortname, e.geom.NameSlug, role); err != nil {
			return err
		}
	}
	idx := strandNamed(st.Strands, name)
	if idx == -1 {
		return nil
	}
	holder := st.Strands[idx]
	if holder.Display.Anchor == render.AnchorHidden || e.pairPaused() || len(directChildren(st.Strands, holder.GUID)) > 0 {
		return nil
	}
	live, windows, err := e.listStrandAndWindowPanes(st)
	if err != nil {
		return fmt.Errorf("list panes: %w", err)
	}
	if e.engineAliveIDSet(presentPanes(st.Strands, live, windows))[holder.PaneID] {
		return nil
	}
	_, paneIDs, err := e.removeStrandLocked(st, holder.GUID, false)
	if err != nil {
		return err
	}
	reapPIDs, _, _ := e.killStrandPanes(st, paneIDs, false)
	reapPaneChildren(reapPIDs, reapExitTimeout)
	logger.Info("reed: reclaimed a dead strand's name", "socket", e.Socket(), "session", e.SessionName(), "name", name, "guid", holder.GUID)
	return nil
}

// ifAbsentDecision is the closed set of outcomes the --if-absent path takes for a name against the persisted strands.
type ifAbsentDecision int

const (
	// ifAbsentAdd means no strand carries the name: the caller falls through to its ordinary add path.
	ifAbsentAdd ifAbsentDecision = iota
	// ifAbsentNoOpAlive means a matched, visible candidate is alive: the caller returns it unchanged.
	ifAbsentNoOpAlive
	// ifAbsentRelaunch means a matched, visible candidate exists but none is alive:
	// the caller relaunches the first one in persisted order.
	ifAbsentRelaunch
	// ifAbsentNoOpHidden means every matched strand is hidden (anchor:hidden):
	// the caller returns the first matched strand unchanged.
	ifAbsentNoOpHidden
)

// classifyIfAbsent decides which branch the --if-absent path takes for name against strands, given aliveIDs.
//
// aliveIDs maps pane IDs to engine liveness, not pane presence: a dead-but-present pane,
// or a shell-only pane whose engine exited, must read as not alive.
// A candidate is alive only when it has a pane ID and aliveIDs reports that ID alive;
// the pane-ID check matters because a server reboot clears every binding and leaves strands with no pane ID.
//
// It returns the decision and the index into strands that the decision names.
// The index is -1 only for ifAbsentAdd, which names no existing strand.
//
// matched is every strand whose Name equals name, hidden ones included.
// candidates is matched without the hidden strands, in persisted order.
// No matched strand gives ifAbsentAdd.
// Matched strands that are all hidden give ifAbsentNoOpHidden, with the first matched strand.
// An alive candidate gives ifAbsentNoOpAlive, with the first alive candidate in persisted order.
// Otherwise the result is ifAbsentRelaunch, with the first candidate.
//
// The four decisions are mutually exclusive and exhaustive.
func classifyIfAbsent(strands []Strand, name string, aliveIDs map[string]bool) (ifAbsentDecision, int) {
	var matched []int
	for i, s := range strands {
		if s.Name == name {
			matched = append(matched, i)
		}
	}
	if len(matched) == 0 {
		return ifAbsentAdd, -1
	}

	var candidates []int
	for _, i := range matched {
		if strands[i].Display.Anchor != render.AnchorHidden {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return ifAbsentNoOpHidden, matched[0]
	}

	for _, i := range candidates {
		s := strands[i]
		if s.PaneID != "" && aliveIDs[s.PaneID] {
			return ifAbsentNoOpAlive, i
		}
	}
	return ifAbsentRelaunch, candidates[0]
}

// strandNamed returns the index of the first strand named name, live, dormant or hidden, or -1.
func strandNamed(strands []Strand, name string) int {
	for i, s := range strands {
		if s.Name == name {
			return i
		}
	}
	return -1
}

// needsLaunchOnAdd reports whether a strand added with display must be realized into a live pane:
// every anchor except hidden.
func needsLaunchOnAdd(display render.Display) bool {
	return display.Anchor != render.AnchorHidden
}

// needsLaunchOnSurface reports whether a hidden strand that becomes visible under display must realize a pane.
func needsLaunchOnSurface(wasHidden bool, display render.Display) bool {
	return wasHidden && display.Anchor != render.AnchorHidden
}

// addStrandLocked builds and registers a new Strand from spec.
// It validates the parent chain and realizes each non-hidden strand into a live pane.
func (e *Engine) addStrandLocked(st *ReedState, spec AddSpec) (Strand, error) {
	if err := validateDisplay(spec.Display); err != nil {
		return Strand{}, err
	}

	guid, err := newGUID()
	if err != nil {
		return Strand{}, fmt.Errorf("generate guid: %w", err)
	}

	name, err := e.strandNameLocked(st, spec)
	if err != nil {
		return Strand{}, err
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
		GUID:       guid,
		Name:       name,
		Worktree:   e.geom.WorktreeRoot,
		Parent:     spec.Parent,
		Cmd:        spec.Cmd,
		ResumeCmd:  spec.ResumeCmd,
		SessionID:  spec.SessionID,
		Display:    spec.Display,
		DoneWhen:   spec.DoneWhen,
		Segment:    spec.Segment,
		Provider:   spec.Provider,
		CreelHooks: spec.CreelHooks,
	})
	strand := &st.Strands[len(st.Strands)-1]

	if needsLaunchOnAdd(spec.Display) {
		if err := e.launchStrandLocked(st, strand, strand.Cmd); err != nil {
			// The record is never persisted, so a script the launch already wrote has no owner.
			removeLaunchScripts(shell.ForGOOS(), e.stateDir(), []string{guid})
			return Strand{}, fmt.Errorf("launch strand: %w", err)
		}
	}

	return *strand, nil
}

// updateStrandLocked mutates guid's Display.
// It rejects a visible-to-hidden transition and realizes a hidden-to-visible one into a pane.
func (e *Engine) updateStrandLocked(st *ReedState, guid string, display render.Display) (Strand, error) {
	if err := validateDisplay(display); err != nil {
		return Strand{}, err
	}

	idx := strandIndex(st.Strands, guid)
	if idx == -1 {
		return Strand{}, fmt.Errorf("unknown strand %q", guid)
	}
	strand := &st.Strands[idx]

	if err := refuseWindowMove(strand.Name, strand.Display, display); err != nil {
		return Strand{}, err
	}

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

// removeStrandLocked removes guid, and with recursive set its descendants too.
// Without recursive it rejects a strand that has children.
// It returns the pane IDs of every removed strand that held a live binding.
// It also deletes the launch script of every removed strand, so surviving strands keep theirs.
func (e *Engine) removeStrandLocked(st *ReedState, guid string, recursive bool) (Removed, []string, error) {
	if _, ok := strandByGUID(st.Strands, guid); !ok {
		return Removed{}, nil, fmt.Errorf("%w %q", ErrUnknownStrand, guid)
	}
	if len(directChildren(st.Strands, guid)) > 0 && !recursive {
		return Removed{}, nil, fmt.Errorf("strand has children, use --recursive")
	}

	toRemove := descendantSubtree(st.Strands, guid)
	removeSet := make(map[string]bool, len(toRemove))
	for _, g := range toRemove {
		removeSet[g] = true
	}

	var removed Removed
	var paneIDs []string
	remaining := make([]Strand, 0, len(st.Strands))
	for _, s := range st.Strands {
		if removeSet[s.GUID] {
			removed.Strands = append(removed.Strands, struct{ GUID, Name string }{s.GUID, s.Name})
			if s.PaneID != "" {
				paneIDs = append(paneIDs, s.PaneID)
			}
			continue
		}
		remaining = append(remaining, s)
	}
	st.Strands = remaining
	pruneWindowGroups(st)
	removeLaunchScripts(shell.ForGOOS(), e.stateDir(), toRemove)

	return removed, paneIDs, nil
}

// removalEmptiedSession reports whether a removal left no visible strand in a session that is confirmed gone.
// A visible strand is one whose anchor is not hidden, so it should own a live pane.
// A confirmed-gone session with no visible strand left is an expected terminal state, not a failure.
func removalEmptiedSession(remaining []Strand, sessionGone bool) bool {
	if !sessionGone {
		return false
	}
	for _, s := range remaining {
		if s.Display.Anchor != render.AnchorHidden {
			return false
		}
	}
	return true
}

// AddStrand registers a new strand from spec.
// Unless the strand is added anchor:hidden, it realizes the strand into a live pane and runs its cmd.
// It then reconciles and re-applies the layout.
// The engine, not the caller, stamps Worktree and generates the GUID.
// It forms the full name under the state lock, from the prefix the geometry gives it.
//
// AddStrand self-heals a cold worktree.
// When there is nothing usable to add into, its pre-flight boots a session with the semantics of `up`:
// a bare substrate, with no persisted strand relaunched.
// The boot path refuses a recorded foreign session before anything creates a session.
//
// After a self-heal boot every pane binding is cleared.
// A --if-absent add of a persisted name therefore relaunches that strand rather than finding it alive.
// That is the intended behavior.
func (e *Engine) AddStrand(spec AddSpec) (Strand, error) {
	strand, _, err := e.AddStrandUnless(spec, "")
	return strand, err
}

// AddStrandUnless adds a strand unless this worktree's state holds a strand named unlessName.
// The match counts live, dormant and hidden strands alike.
// A non-empty unlessName resolves to a full name by the rule an explicit name follows, before the session pre-flight,
// so an unformable name refuses without booting a server.
// On a match it returns that strand and true.
// It then saves no state, reconciles nothing, launches nothing and moves no focus.
// A worktree whose state holds no such strand, a cold one included, gets the add and false.
// An empty unlessName makes the call an unconditional add.
func (e *Engine) AddStrandUnless(spec AddSpec, unlessName string) (Strand, bool, error) {
	var result Strand
	var skipped bool
	err := e.withRevivalFirst(e.withOpLock, func() error {
		// The --if-absent name requirement is a config error, unrelated to session or state,
		// so it surfaces before the session pre-flight below.
		// A rejected call must never leave a spawned tmux server behind as residue over what is actually a missing --name.
		if err := validateIfAbsent(spec); err != nil {
			return err
		}
		// A display naming a retired or unknown anchor is refused here too, so a refused add boots no server.
		if err := validateDisplay(spec.Display); err != nil {
			return err
		}
		// Likewise a name the geometry's prefix cannot form is refused before the pre-flight, so it never boots a tmux server.
		explicitName, err := e.validateNaming(spec)
		if err != nil {
			return err
		}
		if err := validateWindowConfig(e.cfg); err != nil {
			return err
		}
		unlessFull := ""
		if unlessName != "" {
			if unlessFull, err = e.validateNaming(AddSpec{NameOverride: unlessName}); err != nil {
				return err
			}
		}

		// Everything past the naming refusals reads tmux state, so a refused call reads none.
		booted, err := e.ensureSessionLocked()
		if err != nil {
			return err
		}
		if booted {
			// The log entry must name the verb behind a session that appears out of a bare `lym reed add`.
			// The entry also carries this engine's socket and session name.
			logger.Info("reed: add self-healed a cold worktree's session", "socket", e.Socket(), "session", e.SessionName())
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		if unlessFull != "" {
			if idx := strandNamed(st.Strands, unlessFull); idx != -1 {
				result = st.Strands[idx]
				skipped = true
				return nil
			}
		}

		if spec.IfAbsent {
			live, windows, err := e.listStrandAndWindowPanes(st)
			if err != nil {
				return fmt.Errorf("list panes: %w", err)
			}
			// Only panes whose strand's engine still runs count as alive, so a pane left holding just a shell does not.
			decision, idx := classifyIfAbsent(st.Strands, explicitName, e.engineAliveIDSet(presentPanes(st.Strands, live, windows)))
			switch decision {
			case ifAbsentNoOpAlive, ifAbsentNoOpHidden:
				// Neither no-op branch mutates anything: it saves no state, reconciles nothing,
				// and it never rewrites Cmd, ResumeCmd, Parent or Display from this invocation's flags.
				// An add that silently rewrote a matched strand's recorded command would be an update verb under the add name.
				result = st.Strands[idx]
				return nil
			case ifAbsentRelaunch:
				strand := &st.Strands[idx]
				// Launch with the stored ResumeCmd, falling back to Cmd: the same fallback a resume applies.
				// A relaunch matched by name therefore does what resuming that strand does.
				launchCmd := strand.ResumeCmd
				if launchCmd == "" {
					launchCmd = strand.Cmd
				}
				if err := e.launchStrandLocked(st, strand, launchCmd); err != nil {
					return fmt.Errorf("launch strand: %w", err)
				}
				// Persist after the launch succeeds and before the layout apply.
				// If apply then fails, the strand is already tracked with its new PaneID,
				// so the next reconcile repairs the layout instead of treating the launched pane as an untracked orphan.
				if err := SaveState(e.stateDir(), st); err != nil {
					return fmt.Errorf("persist strand: %w", err)
				}
				// This branch alone logs at Info.
				// The sibling paths through launchStrandLocked log no per-strand Info line.
				// They rely on the generic tmux Debug trace instead.
				logger.Info("reed: relaunched strand for --if-absent reopen",
					"socket", e.Socket(), "session", e.SessionName(), "guid", strand.GUID, "name", strand.Name)
				if _, err := e.reconcileApplyPersistLocked(st); err != nil {
					return err
				}
				result, _ = strandByGUID(st.Strands, strand.GUID)
				return nil
			}
			// ifAbsentAdd: no matching strand at all — fall through to the ordinary add path below.
		}

		if err := e.reclaimDeadNameLocked(st, spec, explicitName); err != nil {
			return err
		}

		strand, err := e.addStrandLocked(st, spec)
		if err != nil {
			return err
		}

		// Persist after the launch succeeds and before the layout apply.
		// If apply then fails, the strand is already tracked with its new PaneID,
		// so the next reconcile repairs the layout.
		// The launched pane thereby never becomes an untracked orphan that the next select-layout would silently reap.
		if err := SaveState(e.stateDir(), st); err != nil {
			removeLaunchScripts(shell.ForGOOS(), e.stateDir(), []string{strand.GUID})
			return fmt.Errorf("persist strand: %w", err)
		}

		if _, err := e.reconcileApplyPersistLocked(st); err != nil {
			return err
		}

		result, _ = strandByGUID(st.Strands, strand.GUID)
		return nil
	})
	return e.withColor(result), skipped, err
}

// UpdateStrand mutates guid's display settings, then reconciles and re-applies the layout.
// It refuses a visible-to-hidden transition.
// A hidden-to-visible transition surfaces the strand, which creates its pane and runs its cmd.
// It checks the session first.
// Surfacing a hidden strand before `up` fails with the no-session error, not a raw tmux error from the launch.
// It does not self-heal a cold worktree.
// UpdateStrand is reached through the engine API only; there is no CLI verb for it.
func (e *Engine) UpdateStrand(guid string, display render.Display) (Strand, error) {
	var result Strand
	err := e.withOpLock(func() error {
		if err := e.requireSessionLocked(); err != nil {
			return err
		}

		st, err := e.loadOrInitStateLocked()
		if err != nil {
			return err
		}

		if _, err := e.updateStrandLocked(st, guid, display); err != nil {
			return err
		}

		// Persist after a possible surface launch and before the layout apply,
		// so a launched pane is tracked even if the apply then fails.
		if err := SaveState(e.stateDir(), st); err != nil {
			return fmt.Errorf("persist strand: %w", err)
		}

		if _, err := e.reconcileApplyPersistLocked(st); err != nil {
			return err
		}

		result, _ = strandByGUID(st.Strands, guid)
		return nil
	})
	return result, err
}

// safeReapRoot reports whether p's #{pane_pid} may be used as a descendant-closure root by a
// pane-destroying op.
// Only a still-running pane qualifies.
// tmux keeps reporting a dead pane's recorded #{pane_pid} long after that process exited:
// remain-on-exit is on for every reed session, so a corpse can sit in its window for hours.
// Once the OS recycles that pid, the closure walk expands it into an unrelated process's whole subtree.
// The caller then waits on that subtree and finally SIGKILLs it.
// It is the single declaration of that rule, so the snapshots that take reap roots share it.
func safeReapRoot(p LivePane) bool {
	return !p.Dead && p.PID > 0
}

// alivePanePIDs returns the pids of the panes in paneIDs that are not dead and have a pid.
// Panes of live that paneIDs does not name, and ids absent from live, contribute nothing.
func alivePanePIDs(paneIDs []string, live []LivePane) []int {
	wanted := make(map[string]bool, len(paneIDs))
	for _, id := range paneIDs {
		wanted[id] = true
	}
	var pids []int
	for _, p := range live {
		if wanted[p.ID] && safeReapRoot(p) {
			pids = append(pids, p.PID)
		}
	}
	return pids
}

// paneIDsInSession returns the ids in paneIDs that are panes of the session live was enumerated from.
//
// A persisted pane id is not self-validating as a tmux target.
// The -L socket is per hub and shared by every worktree in it, and tmux pane ids are server-global,
// so a stale or copied reed.json can carry the pane id of a sibling worktree's live session.
// Membership, not aliveness, is the filter: a dead pane that is still present is this session's pane.
func paneIDsInSession(paneIDs []string, live []LivePane) []string {
	present := liveIDSet(live)
	inSession := make([]string, 0, len(paneIDs))
	for _, id := range paneIDs {
		if present[id] {
			inSession = append(inSession, id)
		}
	}
	return inSession
}

// sessionReapRoots returns the pids of every pane in live that is not dead and has a pid.
// It is the whole-session form, for teardown that removes every pane rather than a named subset.
// The targeted and whole-session forms share one root test, so they never disagree about which pids are safe to kill.
func sessionReapRoots(live []LivePane) []int {
	var pids []int
	for _, p := range live {
		if safeReapRoot(p) {
			pids = append(pids, p.PID)
		}
	}
	return pids
}

// killStrandPanes kills the removed strands' panes.
// It returns the process ids the caller must reap once the layout has been repaired.
//
// It snapshots the doomed panes' process subtrees before kill-pane.
// The panes still exist to be listed, and the processes still run, so their pids are not yet reused.
// The same enumeration narrows the kill list to panes that belong to this session.
// A stale or copied reed.json cannot make remove kill a sibling worktree's pane on the shared server.
// The enumeration is free: this call site already lists panes for the reap snapshot.
// The listing spans every window of the session, so an own-window strand's pane is killed too,
// and its window ends with its last pane.
// A failed enumeration kills nothing rather than falling back to the unchecked list.
// list-panes exits non-zero exactly when the session is gone.
// Its panes are gone with the session, so this remove has nothing left to destroy.
//
// The removed strands' panes are killed explicitly, not left to select-layout.
// Neither backend reaps deterministically enough to rely on select-layout for it.
// psmux reaps the extra panes as a side effect.
// tmux accepts a layout whose cell count differs from the pane count, assigning cells by position.
// The kills are best-effort: a pane may already be dead or gone.
// What killing a session's last pane does next depends on the backend.
// On psmux, remain-on-exit leaves the pane as a dead corpse (pane_dead=1, exit 0) and the session stays alive.
// On tmux, killing the true last pane destroys the session, and the server exits too if it was the only session.
// The reconcile tail after the kill then fails its listPanes call against the gone session.
// RemoveStrand handles both outcomes by re-probing hasSession.
// It treats the reconcile failure as success only when the session is confirmed gone.
// On psmux the reconcile tail simply re-enumerates and re-applies; a strand's pane is always a fresh split,
// so a corpse is never reused.
//
// Before any kill, an own window whose every listed pane is being killed is about to end.
// The session's current window is read once, and only when a window ends;
// the endedCurrent result reports whether it is among the ending windows.
// With selectOnEnd set, a viewer on an ending window is first moved to a window that stays:
// the newest live agent, else the lowest-index overview, never the shell.
// When no such window is left, the viewer is left to tmux.
// A viewer on any other window is never moved.
// A strand leaving a shared window that stays open ends no window and switches nothing.
// leftWindows names the own windows that lose a pane but keep at least one, for the caller to lay out again.
func (e *Engine) killStrandPanes(st *ReedState, paneIDs []string, selectOnEnd bool) (reapPIDs []int, leftWindows []string, endedCurrent bool) {
	var killPaneIDs []string
	if len(paneIDs) > 0 {
		windows, err := e.tmux.listWindowPanes(e.SessionName())
		if err != nil {
			logger.Warn("reed: could not enumerate panes before removing a strand, killing none",
				"socket", e.Socket(), "session", e.SessionName(), "err", err)
		} else {
			live := make([]LivePane, len(windows))
			for i, w := range windows {
				live[i] = w.livePane()
			}
			killPaneIDs = paneIDsInSession(paneIDs, live)
			reapPIDs = e.descendantClosurePIDs(alivePanePIDs(killPaneIDs, live))
			var endingWindows []string
			endingWindows, leftWindows = ownWindowsLosingPanes(windows, killPaneIDs)
			if len(endingWindows) > 0 {
				endedCurrent = e.currentWindowIsAmongLocked(endingWindows)
				if endedCurrent && selectOnEnd {
					e.selectRelevantWindowLocked(st, endingWindows)
				}
			}
		}
	}
	for _, id := range killPaneIDs {
		_ = e.tmux.run("kill-pane", "-t", id)
	}
	return reapPIDs, leftWindows, endedCurrent
}

// MarkRetiring sets or clears guid's Retiring flag under the op lock and persists the state.
// It touches only the strand table, so it needs no live session.
// An unknown guid wraps ErrUnknownStrand.
func (e *Engine) MarkRetiring(guid string, retiring bool) error {
	return e.withOpLock(func() error {
		st, err := LoadState(e.stateDir())
		if err != nil {
			return err
		}
		if st == nil {
			return fmt.Errorf("%w %q", ErrUnknownStrand, guid)
		}
		idx := strandIndex(st.Strands, guid)
		if idx == -1 {
			return fmt.Errorf("%w %q", ErrUnknownStrand, guid)
		}
		st.Strands[idx].Retiring = retiring
		return SaveState(e.stateDir(), st)
	})
}

// RemoveStrand removes guid and, when it has descendants, cascades the removal through its whole subtree.
// A non-leaf requires recursive to be true; otherwise the call errors instead of silently deleting descendants.
// It then reconciles and re-applies the layout, and returns every strand actually removed.
// It requires a live session, and a remove before up fails with the no-session error.
// Without that check, a raw tmux error would surface later, from the reconcile.
// It does not self-heal a cold worktree.
// It waits for the destroyed panes' process subtrees to exit before returning.
// tmux terminates a pane's children asynchronously.
// On Windows the process holding the worktree directory is a deep descendant of #{pane_pid}.
// Returning before that reap could leave a removed strand's grandchild alive and the worktree directory busy.
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

		removed, paneIDs, err := e.removeStrandLocked(st, guid, recursive)
		if err != nil {
			return err
		}

		reapPIDs, leftWindows, _ := e.killStrandPanes(st, paneIDs, true)
		removed.ConfigWarning = e.relayoutSharedWindowsLocked(leftWindows)

		// Reap after the layout repair, so the surviving panes re-tile immediately;
		// only the return waits for the async pane teardown.
		// Always reap, even when the repair failed: the panes were already killed above,
		// so their subtrees are dying either way.
		// Skipping the reap on a transient apply error would leave a grandchild holding the worktree directory.
		// The reap confirms each pid is gone within reapExitTimeout rather than trusting a fixed timer.
		_, applyErr := e.reconcileApplyPersistLocked(st)
		reapPaneChildren(reapPIDs, reapExitTimeout)
		if applyErr != nil {
			// applyErr alone cannot tell a removal that legitimately emptied the session from a genuine failure.
			// So the session is re-probed directly rather than string-matching applyErr.
			// hasSession maps a "no server running" exit (1) to (false, nil).
			// The session pre-flight relies on the same classification.
			up, herr := e.tmux.hasSession(e.SessionName())
			sessionGone := herr == nil && !up
			if removalEmptiedSession(st.Strands, sessionGone) {
				// removeStrandLocked pruned st.Strands in memory only, and reconcileApplyPersistLocked failed before its own save.
				// Persist the pruned state here, so a later "lym reed resume" does not resurrect the strand this call removed.
				if err := SaveState(e.stateDir(), st); err != nil {
					return fmt.Errorf("save state after emptying session: %w", err)
				}
				result = removed
				return nil
			}
			return applyErr
		}

		result = removed
		return nil
	})
	return result, err
}
