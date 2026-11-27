// reinterpret.go carries a hub across the move of pull_request and land_notice onto the per-task lookup.
// It also carries a hub across the renaming of pull_request's on to review and merge to trail.
// Each rewrites the hub's beam.yaml and flags each beam.yaml in the hub that still holds a retired value.
// The move also carries the beam settings the board's entries hold into the live worktrees that need them.
// The renaming also renames the values the board's entries and the hub's run profiles hold in their run settings.

package dobbyengine

import (
	"bytes"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/beamsetting"
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

// requiredLoss says what the review that replaces a retired required no longer does.
const requiredLoss = "review does not keep what required meant: `lym beam ready` no longer refuses a beam with no pull request" +
	" recorded, and a task missing an origin fact now lands locally instead of halting"

// requiredWayForward says how to make every landing go through a pull request again.
const requiredWayForward = "to require a pull request for every landing, require pull requests with GitHub branch rules" +
	" on the parent branch, which review follows; `lym config beam --layer default --set pull_request=trail` from the prime" +
	" also demands one for every landing, but it opens and squash-merges that pull request on GitHub at once, with no review"

// retiredReplacements maps each key's value the per-task lookup retired to the value that stands in for it.
func retiredReplacements() map[string]map[string]string {
	return map[string]map[string]string{
		PullRequestKey: {retiredPullRequestRequired: PullRequestReview},
		LandNoticeKey:  {retiredLandNoticeAll: LandNoticeOn, retiredLandNoticePerTask: LandNoticeOff},
	}
}

// PullRequestModesReinterpretationID names the change that renamed pull_request's on to review and merge to trail.
const PullRequestModesReinterpretationID = "beam-pull-request-modes"

// The pull_request values the mode names replaced.
const (
	renamedPullRequestOn    = "on"
	renamedPullRequestMerge = "merge"
)

// renamedModes maps each renamed pull_request value to the value that now means the same.
func renamedModes() map[string]map[string]string {
	return map[string]map[string]string{
		PullRequestKey: {renamedPullRequestOn: PullRequestReview, renamedPullRequestMerge: PullRequestTrail},
	}
}

// ReinterpretPullRequestModes carries a hub's beam config across the renamed values of pull_request.
// An on in the hub file becomes review and a merge becomes trail.
// A merge keeps what it meant.
// An on does too, except that its review now waits for the operator's own ending whatever the autonomy.
// The own beam.yaml files of each worktree it is told are read too.
// A renamed value in one is flagged with the way forward, since only the hub file is rewritten.
// A read that fails is flagged and never fails the reinterpretation.
// A file with no renamed value is returned unchanged.
func ReinterpretPullRequestModes(existing []byte, worktrees []configengine.HubWorktree) (rewritten []byte, rewrote, flagged []string, err error) {
	rewritten, rewrote, flagged, err = rewriteHubFile(existing, renamedModes())
	if err != nil {
		return nil, nil, nil, err
	}
	worktreeFlags, _ := flagRetiredValues(worktrees, renamedModes())
	return rewritten, rewrote, append(flagged, worktreeFlags...), nil
}

// ProfilePullRequestModesReinterpretationID names the rename of pull_request's on and merge in the hub's run profiles.
const ProfilePullRequestModesReinterpretationID = "profiles-pull-request-modes"

// ReinterpretProfilePullRequestModes renames pull_request's on and merge in the bytes of a hub profiles.yaml.
// The values sit in the beam settings of each `<profile>.<recipe>` block, and no seed admits the renamed ones.
// It keeps comments and every other key, and returns one rewrote line per replaced value, naming its block.
// A file with no renamed value is returned unchanged.
// The worktrees argument is unused, as the rewrite judges the file alone.
func ReinterpretProfilePullRequestModes(existing []byte, _ []configengine.HubWorktree) (rewritten []byte, rewrote, flagged []string, err error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, nil, fmt.Errorf("parse the profiles config: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return existing, nil, nil, nil
	}
	for profile, recipes := range mappingEntries(document.Content[0]) {
		for recipe, block := range mappingEntries(recipes) {
			for module, settings := range mappingEntries(block) {
				if module != beamsetting.Module {
					continue
				}
				for key, value := range mappingEntries(settings) {
					replacement, renamed := renamedModes()[key][value.Value]
					if value.Kind != yaml.ScalarNode || !renamed {
						continue
					}
					rewrote = append(rewrote, profile+"."+recipe+" "+rewriteLine(key, value.Value, replacement))
					setString(value, replacement)
				}
			}
		}
	}
	if len(rewrote) == 0 {
		return existing, nil, nil, nil
	}
	rewritten, err = encodeDocument(&document)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("write the profiles config: %w", err)
	}
	return rewritten, rewrote, nil, nil
}

// mappingEntries yields each key and value node of node in order, and nothing when node is no mapping.
func mappingEntries(node *yaml.Node) iter.Seq2[string, *yaml.Node] {
	return func(yield func(string, *yaml.Node) bool) {
		if node.Kind != yaml.MappingNode {
			return
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			if !yield(node.Content[i].Value, node.Content[i+1]) {
				return
			}
		}
	}
}

// setString makes the scalar node hold text as a quoted string.
func setString(node *yaml.Node, text string) {
	node.Value = text
	node.Tag = "!!str"
	node.Style = yaml.DoubleQuotedStyle
}

// encodeDocument returns the YAML bytes of document, indented by two spaces.
func encodeDocument(document *yaml.Node) ([]byte, error) {
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// RenameRunSettingsModes replaces a renamed pull_request value in the beam settings of a board entry's run_settings.
// The replacement is the value that now means the same, and no layer admits the renamed one.
// It returns the settings and one line per replaced value; settings with none come back as they were, with no line.
func RenameRunSettingsModes(settings map[string]any) (map[string]any, []string) {
	beam, isMapping := settings[beamsetting.Module].(map[string]any)
	if !isMapping {
		return settings, nil
	}
	value, _ := beam[PullRequestKey].(string)
	replacement, renamed := renamedModes()[PullRequestKey][value]
	if !renamed {
		return settings, nil
	}
	beam[PullRequestKey] = replacement
	return settings, []string{rewriteLine(PullRequestKey, value, replacement)}
}

// ReinterpretPerTaskSettings carries a hub's beam config across the retired values of pull_request and land_notice.
// Those are pull_request's required and land_notice's all and per-task.
// A required pull_request in the hub file becomes review only so that beam.yaml loads.
// It is flagged, since review does not keep what required meant.
// An all land_notice becomes on, and a per-task one becomes off.
// The own beam.yaml files of each worktree it is told are read too.
// A retired value in one is flagged with the way forward, since only the hub file is rewritten.
// A worktree file's pull_request now decides that worktree's own landing, and a last line says so when any sets one.
// A read that fails is flagged and never fails the reinterpretation.
// A file with no retired value is returned unchanged.
func ReinterpretPerTaskSettings(existing []byte, worktrees []configengine.HubWorktree) (rewritten []byte, rewrote, flagged []string, err error) {
	rewritten, rewrote, flagged, err = rewriteHubFile(existing, retiredReplacements())
	if err != nil {
		return nil, nil, nil, err
	}
	worktreeFlags, pullRequestFiles := flagRetiredValues(worktrees, retiredReplacements())
	if len(pullRequestFiles) > 0 {
		worktreeFlags = append(worktreeFlags, fmt.Sprintf("a worktree file's %s now decides that worktree's own landing, and every task below it inherits it, "+
			"where it once decided the landings into that branch; it is set in %s", keyName(PullRequestKey), strings.Join(pullRequestFiles, ", ")))
	}
	return rewritten, rewrote, append(flagged, worktreeFlags...), nil
}

// rewriteHubFile replaces each value of the hub's beam.yaml bytes that replacements maps.
// It keeps comments and every other key.
// replacements maps a key to its replaced values, each to the value that stands in for it.
// rewrote has one line per replaced value, and flagged one line for each required that review cannot stand in for.
// A file with no replaced value comes back as it was.
func rewriteHubFile(existing []byte, replacements map[string]map[string]string) (rewritten []byte, rewrote, flagged []string, err error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, nil, fmt.Errorf("parse the beam config: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return existing, nil, nil, nil
	}
	mapping := document.Content[0]
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key, value := mapping.Content[i].Value, mapping.Content[i+1]
		replacement, retired := replacements[key][value.Value]
		if value.Kind != yaml.ScalarNode || !retired {
			continue
		}
		rewrote = append(rewrote, rewriteLine(key, value.Value, replacement))
		if key == PullRequestKey && value.Value == retiredPullRequestRequired {
			flagged = append(flagged, fmt.Sprintf("%s: %s is rewritten to %s only so beam.yaml loads, and %s; way forward: %s", keyName(key), value.Value, replacement, requiredLoss, requiredWayForward))
		}
		setString(value, replacement)
	}
	if len(rewrote) == 0 {
		return existing, nil, nil, nil
	}
	rewritten, err = encodeDocument(&document)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("write the beam config: %w", err)
	}
	return rewritten, rewrote, flagged, nil
}

// worktreeLabel names a worktree in a way forward: the prime, or the worktree called by its name.
func worktreeLabel(worktree configengine.HubWorktree) string {
	if worktree.Main {
		return "the prime"
	}
	return "the worktree " + worktree.Name
}

// flagRetiredValues returns one line for each value replacements maps that the own beam.yaml files of the worktrees set.
// A worktree whose files cannot be read gives one line naming the failure, and the rest are still read.
// It also returns the path of each worktree file that sets pull_request.
func flagRetiredValues(worktrees []configengine.HubWorktree, replacements map[string]map[string]string) (flagged, pullRequestFiles []string) {
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
	return flagged, pullRequestFiles
}

// retiredFlag returns the line that flags a retired value at path, with the way forward in the worktree called label.
func retiredFlag(label, path, key, retired, replacement string) string {
	opening := fmt.Sprintf("%s sets %s: %s, which is retired, so beam.yaml stops loading there; way forward: run `lym config beam --set %s=%s` in %s",
		path, keyName(key), retired, key, replacement, label)
	if key == PullRequestKey && retired == retiredPullRequestRequired {
		return fmt.Sprintf("%s to load it again, though %s; %s", opening, requiredLoss, requiredWayForward)
	}
	if key == PullRequestKey && retired == renamedPullRequestOn {
		return opening + ", " + retiredMeaning(key, retired)
	}
	return opening + ", which keeps what it meant"
}

// rewriteLine returns the line that reports a retired value of key as replaced.
func rewriteLine(key, retired, replacement string) string {
	return fmt.Sprintf("%s: %s is retired; it now reads %s, %s", keyName(key), retired, replacement, retiredMeaning(key, retired))
}

// retiredMeaning says what the replacement of the retired value of key does for the hub.
func retiredMeaning(key, retired string) string {
	switch {
	case key == PullRequestKey && retired == retiredPullRequestRequired:
		return "which only lets beam.yaml load, and the flag on required names what it no longer does"
	case key == PullRequestKey && retired == renamedPullRequestOn:
		return "which keeps what on meant, except that the review now waits for the operator's own ending whatever " +
			keyName(AutonomyKey) + " says, since no parent review ends it"
	case key == PullRequestKey:
		return "which keeps what it meant"
	case retired == retiredLandNoticeAll:
		return "so every run still gets the landing notice unless a task or worktree below sets off"
	default:
		return "and each board entry that carried notify_on_land carries " + keyName(LandNoticeKey) + " on, so the same runs still get the notice"
	}
}

// CarryBeamTaskSettings writes the beam settings that the board's entries hold into the local layer of their worktrees.
// An entry's settings are the pull_request and land_notice of its beam run settings,
// and its retired pull_request and notify_on_land keys while a save has not dropped them.
// A renamed pull_request value is carried as the value that now means the same.
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
		if replacement, renamed := renamedModes()[PullRequestKey][settings[PullRequestKey]]; renamed {
			settings[PullRequestKey] = replacement
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
