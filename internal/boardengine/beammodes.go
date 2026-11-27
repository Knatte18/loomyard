// beammodes.go holds the beam modes the board files show:
// the types a reader answers in, the process-wide reader the lym root installs, and the mode lines a render writes.

package boardengine

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/Knatte18/loomyard/internal/logger"
)

// BeamModes is how the root and each open beam run their members, as their runs resolve it on this machine.
type BeamModes struct {
	// HubWide holds the hub's default of each per-task setting, which applies where no worktree sets one.
	// Its landing facts stay empty.
	HubWide BeamMode
	// Root holds the root's settings, read from the prime's worktree up; its landing facts stay empty.
	Root BeamMode
	// Beams holds each beam's settings and landing facts by slug.
	// A beam whose read failed, or whose worktree is not on this machine, is absent.
	// The mode line of an open absent beam says its settings were not read.
	Beams map[string]BeamMode
}

// BeamMode is how one beam, or the root, runs its members.
type BeamMode struct {
	// Autonomy is on when the orch settles every halt of the members' runs, and off otherwise.
	Autonomy SettingValue
	// PullRequest is beam.yaml's pull_request as it resolves for the members.
	PullRequest SettingValue
	// ParentBranch is the branch the beam lands on, empty when its origin record is not on this machine.
	ParentBranch string
	// PullRequestNumber and PullRequestURL name the beam's own recorded pull request.
	// The number is 0 when none is recorded.
	PullRequestNumber int
	PullRequestURL    string
}

// SettingValue is one resolved per-task setting and the place whose own files set it.
type SettingValue struct {
	Value string
	// SetBy is the slug of the worktree whose own files set Value, and RootSubject stands for the prime.
	// It is empty when no worktree sets it, so the hub-wide default decides.
	SetBy string
}

// BeamModeReader reads the BeamModes of the root and of the beams named, for the hub whose board directory is boardDir.
type BeamModeReader func(boardDir string, beams []string) (BeamModes, error)

// installedBeamModeReader holds the reader Config.Outputs hands every render, nil while none is installed.
var installedBeamModeReader atomic.Pointer[BeamModeReader]

// InstallBeamModeReader makes reader the one every board render of this process reads the beam modes through.
// With none installed, a render shows no mode line.
func InstallBeamModeReader(reader BeamModeReader) {
	installedBeamModeReader.Store(&reader)
}

// installedReader returns the installed reader, or nil when none is.
func installedReader() BeamModeReader {
	if reader := installedBeamModeReader.Load(); reader != nil {
		return *reader
	}
	return nil
}

// readBeamModes reads the modes of the root and of the open beams among tasks through out's reader.
// It returns nil when out has no reader or the read fails, which it logs, so the render shows no mode line.
func readBeamModes(boardPath string, tasks []Task, out Outputs) *BeamModes {
	if out.BeamModes == nil {
		return nil
	}
	var beams []string
	for _, t := range tasks {
		if t.Kind == KindProject && t.Beam && !isDone(t) {
			beams = append(beams, t.Slug)
		}
	}
	modes, err := out.BeamModes(boardPath, beams)
	if err != nil {
		logger.Warn("boardengine: the beam modes did not read, rendering the board without them", "path", boardPath, "error", err)
		return nil
	}
	return &modes
}

// The labels of a mode line's parts.
const (
	autonomyLabel     = "Autonomy"
	pullRequestLabel  = "PR setting"
	parentBranchLabel = "Parent branch"
	beamPRLabel       = "Beam PR"
)

// hubWideSourceText names the hub-wide default as the place an inherited value comes from.
const hubWideSourceText = "hub-wide"

// unreadModesText is the mode line of an open beam whose settings the render could not read.
// A beam whose worktree is on another machine is one such beam.
const unreadModesText = autonomyLabel + " and " + pullRequestLabel + " not read on this machine"

// modeHeader is the README's statement of the hub-wide defaults and the blank line after it.
// It is empty when the render has no modes.
func (b boardView) modeHeader() []string {
	if b.modes == nil {
		return nil
	}
	hubWide := b.modes.HubWide
	return []string{
		fmt.Sprintf("Hub-wide default: %s %s · %s %s.",
			strings.ToLower(autonomyLabel), codeSpan(hubWide.Autonomy.Value), pullRequestLabel, codeSpan(hubWide.PullRequest.Value)),
		"",
	}
}

// modeLines are project's mode line and the blank line after it.
// The root and every open beam have one, which always shows autonomy and the PR setting;
// an open beam the modes do not hold gets a line saying its settings were not read.
// They are empty for any other project and for a render with no modes.
func (b boardView) modeLines(project Task) []string {
	if b.modes == nil {
		return nil
	}
	mode, own := b.modes.Root, RootSubject
	if project.Slug != "" {
		if !project.Beam {
			return nil
		}
		beamMode, found := b.modes.Beams[project.Slug]
		if !found {
			if isDone(project) {
				return nil
			}
			return []string{unreadModesText, ""}
		}
		mode, own = beamMode, project.Slug
	}
	parts := []string{
		autonomyLabel + " " + b.settingText(mode.Autonomy, own),
		pullRequestLabel + " " + b.settingText(mode.PullRequest, own),
	}
	if mode.ParentBranch != "" {
		parts = append(parts, parentBranchLabel+" "+codeSpan(mode.ParentBranch))
	}
	if mode.PullRequestNumber > 0 {
		parts = append(parts, fmt.Sprintf("%s [#%d](%s)", beamPRLabel, mode.PullRequestNumber, mode.PullRequestURL))
	}
	return []string{strings.Join(parts, " · "), ""}
}

// settingText is setting's value as a mode line shows it:
// bold when own set it, else italic with the place it is inherited from in brackets.
func (b boardView) settingText(setting SettingValue, own string) string {
	if setting.SetBy == own {
		return "**" + cellText(setting.Value) + "**"
	}
	return "_" + cellText(setting.Value) + "_ (" + b.sourceText(setting.SetBy) + ")"
}

// sourceText names the place setBy as an inherited value's source.
// That is a project's handle, the root's P0, or the slug of a worktree with no handle on the board.
// An empty setBy names the hub-wide default.
func (b boardView) sourceText(setBy string) string {
	switch setBy {
	case "":
		return hubWideSourceText
	case RootSubject:
		return rootProject.Handle
	}
	if entry, found := b.bySlug[setBy]; found && entry.Handle != "" {
		return cellText(entry.Handle)
	}
	return codeSpan(setBy)
}
