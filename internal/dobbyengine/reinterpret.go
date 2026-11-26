// reinterpret.go carries a hub across the move of pull_request and land_notice onto the per-task lookup.
// It rewrites the hub's beam.yaml and flags each beam.yaml in the hub that still holds a retired value.
// It also carries the beam settings the board's entries hold into the live worktrees that need them.

package dobbyengine

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// PerTaskSettingsReinterpretationID names the change that moved pull_request and land_notice onto the per-task lookup.
const PerTaskSettingsReinterpretationID = "beam-per-task-settings"

// The values the per-task lookup retired, which no layer may hold now.
const (
	retiredPullRequestRequired = "required"
	retiredLandNoticeAll       = "all"
	retiredLandNoticePerTask   = "per-task"
)

// requiredLoss says what the on that replaces a retired required no longer does.
const requiredLoss = "on does not keep what required meant: `lym beam ready` no longer refuses a beam with no pull request" +
	" recorded, and a task missing an origin fact now lands locally instead of halting"

// requiredWayForward says how to make every landing go through a pull request again.
const requiredWayForward = "to require a pull request for every landing, require pull requests with GitHub branch rules" +
	" on the parent branch, which on follows; `lym config beam --layer default --set pull_request=merge` from the prime" +
	" also demands one for every landing, but it opens and squash-merges that pull request on GitHub at once, with no review"

// retiredReplacements maps each key's retired value to the value that stands in for it.
func retiredReplacements() map[string]map[string]string {
	return map[string]map[string]string{
		PullRequestKey: {retiredPullRequestRequired: PullRequestOn},
		LandNoticeKey:  {retiredLandNoticeAll: LandNoticeOn, retiredLandNoticePerTask: LandNoticeOff},
	}
}

// ReinterpretPerTaskSettings carries a hub's beam config across the retired values of pull_request and land_notice.
// Those are pull_request's required and land_notice's all and per-task.
// A required pull_request in the hub file becomes on only so that beam.yaml loads.
// It is flagged, since on does not keep what required meant.
// An all land_notice becomes on, and a per-task one becomes off.
// The own beam.yaml files of each worktree it is told are read too.
// A retired value in one is flagged with the way forward, since only the hub file is rewritten.
// A read that fails is flagged and never fails the reinterpretation.
// A file with no retired value is returned unchanged.
func ReinterpretPerTaskSettings(existing []byte, worktrees []configengine.HubWorktree) (rewritten []byte, rewrote, flagged []string, err error) {
	rewritten, rewrote, flagged, err = rewriteHubFile(existing)
	if err != nil {
		return nil, nil, nil, err
	}
	return rewritten, rewrote, append(flagged, flagRetiredValues(worktrees)...), nil
}

// rewriteHubFile replaces each retired value of the hub's beam.yaml bytes, keeping comments and every other key.
// rewrote has one line per replaced value, and flagged one line for each required that on cannot stand in for.
// A file with no retired value comes back as it was.
func rewriteHubFile(existing []byte) (rewritten []byte, rewrote, flagged []string, err error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, nil, fmt.Errorf("parse the beam config: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return existing, nil, nil, nil
	}
	replacements := retiredReplacements()
	mapping := document.Content[0]
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key, value := mapping.Content[i].Value, mapping.Content[i+1]
		replacement, retired := replacements[key][value.Value]
		if value.Kind != yaml.ScalarNode || !retired {
			continue
		}
		rewrote = append(rewrote, fmt.Sprintf("%s: %s is retired; it now reads %s, %s", keyName(key), value.Value, replacement, retiredMeaning(key, value.Value)))
		if key == PullRequestKey && value.Value == retiredPullRequestRequired {
			flagged = append(flagged, fmt.Sprintf("%s: %s is rewritten to %s only so beam.yaml loads, and %s; way forward: %s", keyName(key), value.Value, replacement, requiredLoss, requiredWayForward))
		}
		value.Value = replacement
		value.Tag = "!!str"
		value.Style = yaml.DoubleQuotedStyle
	}
	if len(rewrote) == 0 {
		return existing, nil, nil, nil
	}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, nil, nil, fmt.Errorf("write the beam config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, nil, fmt.Errorf("write the beam config: %w", err)
	}
	return out.Bytes(), rewrote, flagged, nil
}

// worktreeLabel names a worktree in a way forward: the prime, or the worktree called by its name.
func worktreeLabel(worktree configengine.HubWorktree) string {
	if worktree.Main {
		return "the prime"
	}
	return "the worktree " + worktree.Name
}

// flagRetiredValues returns one line for each retired value that the own beam.yaml files of the worktrees set.
// A worktree whose files cannot be read gives one line naming the failure, and the rest are still read.
// A worktree file's pull_request now decides that worktree's own landing.
// A last line says so when any worktree sets one.
func flagRetiredValues(worktrees []configengine.HubWorktree) []string {
	replacements := retiredReplacements()
	var flagged, pullRequestFiles []string
	for _, worktree := range worktrees {
		label := worktreeLabel(worktree)
		for _, key := range []string{PullRequestKey, LandNoticeKey} {
			value, path, set, err := ownTaskSetting(configengine.Dirs{Anchor: worktree.Anchor}, key)
			if err != nil {
				flagged = append(flagged, fmt.Sprintf("the beam.yaml files of %s could not be read: %v; way forward: fix the file the error names", label, err))
				break
			}
			if !set {
				continue
			}
			if key == PullRequestKey {
				pullRequestFiles = append(pullRequestFiles, path)
			}
			text, isText := value.(string)
			if replacement, retired := replacements[key][text]; isText && retired {
				flagged = append(flagged, retiredFlag(label, path, key, text, replacement))
			}
		}
	}
	if len(pullRequestFiles) > 0 {
		flagged = append(flagged, fmt.Sprintf("a worktree file's %s now decides that worktree's own landing, and every task below it inherits it, "+
			"where it once decided the landings into that branch; it is set in %s", keyName(PullRequestKey), strings.Join(pullRequestFiles, ", ")))
	}
	return flagged
}

// retiredFlag returns the line that flags a retired value at path, with the way forward in the worktree called label.
func retiredFlag(label, path, key, retired, replacement string) string {
	opening := fmt.Sprintf("%s sets %s: %s, which is retired, so beam.yaml stops loading there; way forward: run `lym config beam --set %s=%s` in %s",
		path, keyName(key), retired, key, replacement, label)
	if key == PullRequestKey && retired == retiredPullRequestRequired {
		return fmt.Sprintf("%s to load it again, though %s; %s", opening, requiredLoss, requiredWayForward)
	}
	return opening + ", which keeps what it meant"
}

// retiredMeaning says what the replacement of the retired value of key does for the hub.
func retiredMeaning(key, retired string) string {
	switch {
	case key == PullRequestKey:
		return "which only lets beam.yaml load, and the flag on required names what it no longer does"
	case retired == retiredLandNoticeAll:
		return "so every run still gets the landing notice unless a task or worktree below sets off"
	default:
		return "and each board entry that carried notify_on_land carries " + keyName(LandNoticeKey) + " on, so the same runs still get the notice"
	}
}

// CarryBeamTaskSettings writes the beam settings that the board's entries hold into the local layer of their worktrees.
// An entry's settings are the pull_request and land_notice of its beam run settings,
// and its retired pull_request and notify_on_land keys while a save has not dropped them.
// They go into the worktree named after the entry, among worktrees, and a project's into its own worktree.
// A setting the worktree's own files already set is left as it is.
// It returns one line per setting written.
// A failed read of a worktree's files and a failed write are flagged lines.
// So is an entry whose retired pull_request is off, which is not carried.
// The error is set only when the board cannot be read, so a caller may try again.
func CarryBeamTaskSettings(boardDir string, worktrees []configengine.HubWorktree) (carried, flagged []string, err error) {
	entries, err := boardengine.BeamSettingsByEntry(boardDir)
	if err != nil {
		return nil, nil, fmt.Errorf("read the beam settings of the board in %s: %w", boardDir, err)
	}
	for _, worktree := range worktrees {
		entry, held := entries[worktree.Name]
		if worktree.Main || !held {
			continue
		}
		label := worktreeLabel(worktree)
		settings := maps.Clone(entry.Set)
		if entry.RetiredOff {
			settings[PullRequestKey] = PullRequestOff
		}
		dirs := configengine.Dirs{Anchor: worktree.Anchor}
		for _, key := range slices.Sorted(maps.Keys(settings)) {
			_, _, set, err := ownTaskSetting(dirs, key)
			if err != nil {
				flagged = append(flagged, fmt.Sprintf("the beam.yaml files of %s could not be read, so the board entry's %s: %s is not carried: %v; way forward: fix the file the error names, then run `lym config beam --set %s=%s` in %s",
					label, keyName(key), settings[key], err, key, settings[key], label))
				continue
			}
			if set {
				continue
			}
			if key == PullRequestKey && entry.RetiredOff {
				flagged = append(flagged, fmt.Sprintf("the board entry of %s asked for %s off, which is not carried: under a required or merge parent it still opened a pull request; way forward: run `lym config beam --set %s=%s` in %s to land it with no pull request",
					worktree.Name, keyName(key), key, settings[key], label))
				continue
			}
			pairs := []yamlengine.KV{{Key: key, Value: settings[key]}}
			if _, err := configengine.Set(dirs, ConfigSpec(), configengine.LayerLocal, pairs); err != nil {
				flagged = append(flagged, fmt.Sprintf("the board entry of %s carried %s: %s, which could not be written to its worktree: %v; way forward: run `lym config beam --set %s=%s` in %s",
					worktree.Name, keyName(key), settings[key], err, key, settings[key], label))
				continue
			}
			carried = append(carried, fmt.Sprintf("the board entry of %s carried %s: %s, now set in %s's local beam.yaml", worktree.Name, keyName(key), settings[key], worktree.Name))
		}
	}
	return carried, flagged, nil
}
