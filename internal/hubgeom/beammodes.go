// beammodes.go reads the beam modes the board files show.
// They are the per-task settings of the root and each beam, with the place that set each.
// Each beam also has its parent branch and pull request.

package hubgeom

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pawlengine"
)

// The autonomy values the board files show, for beam.yaml's true and false.
const (
	autonomyOnText  = "on"
	autonomyOffText = "off"
)

// errBeamWorktreeAbsent marks a beam whose worktree is not on this machine, which a hub that spans machines expects.
var errBeamWorktreeAbsent = errors.New("the beam's worktree is not on this machine")

// BeamModes reads how the root and each beam named run their members, for the hub whose board directory is boardDir.
// Each setting is read as the runs read it, through TaskSettingChain and the per-task lookup from the beam's worktree up.
// A beam whose worktree is absent on this machine is left out, and a beam whose read fails is left out with a warning.
// A prime that does not resolve, or a failed read of the root's or the hub-wide settings, is an error.
func BeamModes(boardDir string, beams []string) (boardengine.BeamModes, error) {
	prime, err := primeOfBoard(boardDir)
	if err != nil {
		return boardengine.BeamModes{}, err
	}
	hubWide, err := readBeamSettings(dobbyengine.TaskSettingChain{Board: fabriclayout.BoltDir(prime.HubPath)})
	if err != nil {
		return boardengine.BeamModes{}, fmt.Errorf("hubgeom: read the hub-wide beam settings: %w", err)
	}
	rootChain, err := TaskSettingChain(prime, "")
	if err != nil {
		return boardengine.BeamModes{}, err
	}
	root, err := readBeamSettings(rootChain)
	if err != nil {
		return boardengine.BeamModes{}, fmt.Errorf("hubgeom: read the root's beam settings: %w", err)
	}
	modes := boardengine.BeamModes{HubWide: hubWide, Root: root, Beams: make(map[string]boardengine.BeamMode, len(beams))}
	for _, slug := range beams {
		mode, err := readBeamMode(prime, slug)
		if errors.Is(err, errBeamWorktreeAbsent) {
			logger.Info("hubgeom: leaving a beam out of the board files' modes", "beam", slug, "reason", err)
			continue
		}
		if err != nil {
			logger.Warn("hubgeom: the beam's modes did not read, leaving them out of the board files", "beam", slug, "error", err)
			continue
		}
		modes.Beams[slug] = mode
	}
	return modes, nil
}

// primeOfBoard returns the Location of the prime of the hub whose board directory is boardDir.
// The Bolt checkout is a worktree of the records repository, whose main worktree is the prime's records sibling,
// so the prime is the worktree that sibling belongs to.
// It spawns git only for a records repository whose refs are not files.
func primeOfBoard(boardDir string) (*lymcwd.Location, error) {
	worktrees, err := fabricengine.List(boardDir)
	if err != nil {
		return nil, fmt.Errorf("hubgeom: list the records worktrees from the board at %s: %w", boardDir, err)
	}
	// The main worktree comes first; the porcelain listing spells its path with forward slashes.
	recordsMain := filepath.FromSlash(worktrees[0].Path)
	prime, err := lymcwd.ResolveWithin(recordsMain)
	if err != nil {
		return nil, fmt.Errorf("hubgeom: resolve the prime from its records worktree %s: %w", recordsMain, err)
	}
	if filepath.Clean(prime.WorktreePath()) == filepath.Clean(recordsMain) {
		return nil, fmt.Errorf("hubgeom: the records worktree %s belongs to no worktree beside it, so the prime does not resolve", recordsMain)
	}
	return prime, nil
}

// readBeamMode reads the beam slug's settings from its worktree up.
// It also reads the parent branch its origin record names and the pull request its landing record names.
// A beam whose worktree is not on this machine is errBeamWorktreeAbsent.
// The settings such a beam's runs read are not here to read.
func readBeamMode(prime *lymcwd.Location, slug string) (boardengine.BeamMode, error) {
	if _, err := os.Stat(fabricengine.WorktreePath(prime, slug)); errors.Is(err, fs.ErrNotExist) {
		return boardengine.BeamMode{}, errBeamWorktreeAbsent
	} else if err != nil {
		return boardengine.BeamMode{}, fmt.Errorf("hubgeom: stat the worktree of the beam %q: %w", slug, err)
	}
	chain, err := TaskSettingChain(prime, slug)
	if err != nil {
		return boardengine.BeamMode{}, err
	}
	mode, err := readBeamSettings(chain)
	if err != nil {
		return boardengine.BeamMode{}, err
	}
	origin, found, err := fabricengine.ReadOriginFor(prime, slug)
	if err != nil {
		return boardengine.BeamMode{}, fmt.Errorf("hubgeom: read the origin record of the beam %q: %w", slug, err)
	}
	if found {
		mode.ParentBranch = origin.ParentBranch
	}
	beam, err := lymcwd.NamedWorktree(prime.HubPath, slug)
	if err != nil {
		return boardengine.BeamMode{}, fmt.Errorf("hubgeom: resolve the worktree of the beam %q: %w", slug, err)
	}
	record, found, err := pawlengine.ReadLanding(loomengine.BeamLandingPath(beam), loomengine.BeamLandingLockPath(beam), slug)
	if err != nil {
		return boardengine.BeamMode{}, fmt.Errorf("hubgeom: read the landing record of the beam %q: %w", slug, err)
	}
	if found && record.PullRequest != nil {
		mode.PullRequestNumber = record.PullRequest.Number
		mode.PullRequestURL = record.PullRequest.URL
	}
	return mode, nil
}

// readBeamSettings reads autonomy and pull_request over chain, each with the worktree whose own files set it.
// Autonomy reads as on or off.
func readBeamSettings(chain dobbyengine.TaskSettingChain) (boardengine.BeamMode, error) {
	autonomy, err := readBeamSetting(chain, dobbyengine.AutonomyKey)
	if err != nil {
		return boardengine.BeamMode{}, err
	}
	text := autonomyOffText
	if autonomy.Value == strconv.FormatBool(true) {
		text = autonomyOnText
	}
	autonomy.Value = text
	pullRequest, err := readBeamSetting(chain, dobbyengine.PullRequestKey)
	if err != nil {
		return boardengine.BeamMode{}, err
	}
	return boardengine.BeamMode{Autonomy: autonomy, PullRequest: pullRequest}, nil
}

// readBeamSetting reads the per-task key over chain.
// The value's SetBy is the slug of the worktree that set it, and the root's subject stands for the prime.
// SetBy is empty when the hub-wide default decides.
func readBeamSetting(chain dobbyengine.TaskSettingChain, key string) (boardengine.SettingValue, error) {
	value, anchor, err := dobbyengine.LookupWithSource(chain, key)
	if err != nil {
		return boardengine.SettingValue{}, err
	}
	setting := boardengine.SettingValue{Value: value}
	if anchor != dobbyengine.HubWideSource {
		setting.SetBy = chain.Names[anchor]
		if setting.SetBy == "" {
			setting.SetBy = boardengine.RootSubject
		}
	}
	return setting, nil
}
