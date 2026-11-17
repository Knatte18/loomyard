// active.go is the one seam every loom reader of implementer state goes through.
// It tells which implementer, Webster or Weaver, the run belongs to by the begun batches in their state files,
// and it refuses with the way forward when the two disagree or a state file cannot be read.

package weaverengine

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// Implementer names one of tapestry's implementers, by the value of `tapestry.implementer`.
type Implementer string

const (
	// ImplementerWebster is the in-session implementer, and the setting's default.
	ImplementerWebster Implementer = "webster"
	// ImplementerWeaver is the cold-start implementer with model escalation.
	ImplementerWeaver Implementer = "weaver"
)

// BatchStrandRole is the reed role of Weaver's batch strands.
const BatchStrandRole = "weaver"

// settingRefusal is the way forward of every refusal about the `tapestry.implementer` setting.
const settingRefusal = "way forward: set the implementer with `lym config tapestry --set implementer=<webster|weaver>`"

var (
	// ErrImplementerConflict is wrapped by the refusal for a run whose two implementers both hold begun batches.
	ErrImplementerConflict = errors.New("weaver: the run's implementers disagree")
	// ErrActiveRefused is wrapped by every refusal to name the run's active implementer.
	ErrActiveRefused = errors.New("weaver: the run's active implementer cannot be read")
)

// StateVersion is one committed version of Weaver's state file.
type StateVersion struct {
	// Commit is the records-repository commit the version was read from.
	Commit string
	// Data is the version's bytes.
	Data []byte
}

// ActiveDeps carries what Active and ActiveOrSetting read.
type ActiveDeps struct {
	// AnchorRoot is the anchor both implementers' run directories hang off.
	AnchorRoot string
	// Setting reads `tapestry.implementer`.
	// Nil answers webster, the setting's default.
	Setting func() (string, error)
	// StateHistory returns Weaver's state file's versions on the records branch's first-parent line, newest first.
	StateHistory func() ([]StateVersion, error)
	// TaskHead returns the task branch's HEAD.
	TaskHead func() (string, error)
	// LiveStrands returns the names of the strands `lym reed status` lists live.
	// A nil StateHistory, TaskHead or LiveStrands makes the repair advice name `lym weaver reset` alone.
	LiveStrands func() ([]string, error)
}

// RunDirs returns impl's run and scratch directories under anchorRoot,
// so a reader names the active implementer's without a switch of its own.
func RunDirs(impl Implementer, anchorRoot string) (runDir, scratchDir string) {
	if impl == ImplementerWeaver {
		return Dir(anchorRoot), ScratchDir(anchorRoot)
	}
	return websterengine.Dir(anchorRoot), websterengine.ScratchDir(anchorRoot)
}

// Active returns the run's active implementer and its run-plan state, nil when its state file is missing.
// A state file counts as begun when it holds a batch record; a missing file counts as no begun batches.
// With neither begun, the active implementer is the setting's value.
//
// It refuses with an error wrapping ErrActiveRefused when a state file fails to load.
// It refuses the same way when both implementers hold begun batches, and that error also wraps ErrImplementerConflict.
// It refuses the same way when the setting cannot be read or holds another value.
// Each refusal ends in its way forward.
// A corrupt file never counts as missing.
func Active(deps ActiveDeps) (Implementer, *runplan.State, error) {
	websterState, err := websterengine.LoadState(websterengine.Dir(deps.AnchorRoot), websterengine.ScratchDir(deps.AnchorRoot))
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w; way forward: repair %s, or restore its committed version with git",
			ErrActiveRefused, err, websterStateRel())
	}
	weaverState, err := LoadState(Dir(deps.AnchorRoot), ScratchDir(deps.AnchorRoot))
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w; way forward: %s", ErrActiveRefused, err, weaverRepairAdvice(deps))
	}

	websterBegun := websterState != nil && len(websterState.Batches) > 0
	weaverBegun := BegunBatches(weaverState)
	switch {
	case websterBegun && weaverBegun:
		return "", nil, fmt.Errorf("%w: %w: both %s and %s hold begun batches; way forward: "+
			"end the run you do not mean to keep, with `lym webster reset --to start` for Webster's run or `lym weaver reset` for Weaver's",
			ErrActiveRefused, ErrImplementerConflict, websterStateRel(), StateRel())
	case websterBegun:
		return ImplementerWebster, &websterState.State, nil
	case weaverBegun:
		return ImplementerWeaver, &weaverState.State, nil
	}

	impl, err := settingImplementer(deps)
	if err != nil {
		return "", nil, err
	}
	switch {
	case impl == ImplementerWeaver && weaverState != nil:
		return impl, &weaverState.State, nil
	case impl == ImplementerWebster && websterState != nil:
		return impl, &websterState.State, nil
	}
	return impl, nil, nil
}

// ActiveOrSetting is the advisory readers' form of Active.
// On a refusal it falls back to the setting's implementer, and to webster when the setting cannot be read.
// It loads that implementer's state when it loads, and returns the refusal's text as the reason the caller logs.
// The reason is empty when Active answered.
func ActiveOrSetting(deps ActiveDeps) (Implementer, *runplan.State, string) {
	impl, st, err := Active(deps)
	if err == nil {
		return impl, st, ""
	}

	impl, settingErr := settingImplementer(deps)
	if settingErr != nil {
		impl = ImplementerWebster
	}
	runDir, scratchDir := RunDirs(impl, deps.AnchorRoot)
	if impl == ImplementerWeaver {
		if weaverState, loadErr := LoadState(runDir, scratchDir); loadErr == nil && weaverState != nil {
			return impl, &weaverState.State, err.Error()
		}
		return impl, nil, err.Error()
	}
	if websterState, loadErr := websterengine.LoadState(runDir, scratchDir); loadErr == nil && websterState != nil {
		return impl, &websterState.State, err.Error()
	}
	return impl, nil, err.Error()
}

// settingImplementer reads the setting and refuses a failed read and a value that is not an implementer.
func settingImplementer(deps ActiveDeps) (Implementer, error) {
	if deps.Setting == nil {
		return ImplementerWebster, nil
	}
	value, err := deps.Setting()
	if err != nil {
		return "", fmt.Errorf("%w: read the tapestry.implementer setting: %w; %s", ErrActiveRefused, err, settingRefusal)
	}
	switch Implementer(value) {
	case ImplementerWebster, ImplementerWeaver:
		return Implementer(value), nil
	}
	return "", fmt.Errorf("%w: tapestry.implementer is %q, neither webster nor weaver; %s", ErrActiveRefused, value, settingRefusal)
}

// websterStateRel returns Webster's state file's anchor-relative path.
func websterStateRel() string {
	return websterengine.DirRel() + "/" + stateFileName
}

// weaverRepairAdvice names how to repair a Weaver state file that fails to load.
// It names the committed version restorableVersion picks, and `lym weaver reset` when it picks none.
func weaverRepairAdvice(deps ActiveDeps) string {
	version, ok := restorableVersion(deps)
	if !ok {
		return "start the run over with `lym weaver reset`"
	}
	return fmt.Sprintf("restore %s from records commit %s with git;"+
		" that version holds because the next begin captures HEAD as the start, so no attempt may have committed since and none may be in flight,"+
		" and neither has; or start the run over with `lym weaver reset`", StateRel(), version.Commit)
}

// restorableVersion picks the newest committed version of Weaver's state file that parses.
// It picks it only when its LastEnd equals the task branch's HEAD and no strand of Weaver's roles is live.
// A restored state then holds, because the next begin captures HEAD as the start.
// So no attempt may have committed since, and none may be in flight.
// A read that is not wired or that fails picks none.
func restorableVersion(deps ActiveDeps) (StateVersion, bool) {
	if deps.StateHistory == nil || deps.TaskHead == nil || deps.LiveStrands == nil {
		return StateVersion{}, false
	}
	versions, err := deps.StateHistory()
	if err != nil {
		return StateVersion{}, false
	}
	head, err := deps.TaskHead()
	if err != nil {
		return StateVersion{}, false
	}
	live, err := deps.LiveStrands()
	if err != nil {
		return StateVersion{}, false
	}

	for _, version := range versions {
		var parsed State
		if json.Unmarshal(version.Data, &parsed) != nil {
			continue
		}
		if parsed.LastEnd != head || weaverStrandLive(live) {
			return StateVersion{}, false
		}
		return version, true
	}
	return StateVersion{}, false
}

// weaverStrandLive reports whether any of the named strands holds a batch strand or verify fixer role.
func weaverStrandLive(names []string) bool {
	for _, name := range names {
		parsed, err := agentname.Parse(name)
		if err != nil {
			continue
		}
		if agentname.MatchesRole(parsed.Role, BatchStrandRole) || agentname.MatchesRole(parsed.Role, gatefixer.RoleVerifyFixer) {
			return true
		}
	}
	return false
}
