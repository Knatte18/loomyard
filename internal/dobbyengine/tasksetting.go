// tasksetting.go alone reads beam.pull_request, beam.land_notice, beam.autonomy and beam.run_watch per task.
// A task's value is the one its own worktree's files set.
// Failing that, it is the value of the nearest worktree above it that sets one, up to the prime.
// Failing all of those, it is the hub's default layer's, else the template's.

package dobbyengine

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/envsource"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// TaskSettingChain is the geometry a per-task beam setting is looked up through, told by the caller.
type TaskSettingChain struct {
	// Anchors are the anchor directories of the task's worktree and of each worktree above it, nearest first.
	// The last is the prime's.
	// A worktree absent on this machine is left out, so its parent's value applies.
	Anchors []string
	// Names are the worktree names of Anchors in the same order, "" for the prime's.
	// Only a caller that reads where a value was set needs them.
	Names []string
	// Board is the hub's board directory, whose default layer applies when no worktree of Anchors sets the key.
	// Empty means no default layer.
	Board string
}

// LookupPullRequest returns the beam.pull_request the task at the chain's head lands by:
// on, off, merge or merge-in.
// A value the key does not take is refused, naming the file that set it.
func LookupPullRequest(chain TaskSettingChain) (string, error) {
	return lookupTaskSetting(chain, PullRequestKey)
}

// LookupLandNotice reports whether beam.land_notice is on for the task at the chain's head.
// A task with it on asks dobby for a line to the orch once its run lands or ends without landing.
// A value the key does not take is refused, naming the file that set it.
func LookupLandNotice(chain TaskSettingChain) (bool, error) {
	value, err := lookupTaskSetting(chain, LandNoticeKey)
	return value == LandNoticeOn, err
}

// LookupAutonomy reports whether beam.autonomy is on for the task at the chain's head.
// Under it the orch settles every halt of the task's runs.
// It reaches the operator only for a severe halt, a merge-ready beam or a raddle split's approval.
// A value the key does not take is refused, naming the file that set it.
func LookupAutonomy(chain TaskSettingChain) (bool, error) {
	value, err := lookupTaskSetting(chain, AutonomyKey)
	return value == "true", err
}

// LookupRunWatch returns the beam.run_watch of the task at the chain's head: stops, rows or off.
// It is the only reader of the key.
// A value the key does not take is refused, naming the file that set it.
func LookupRunWatch(chain TaskSettingChain) (string, error) {
	return lookupTaskSetting(chain, RunWatchKey)
}

// ownTaskSetting returns the value the files of the one place dirs names set for the per-task key.
// It returns the setting file's path with it.
// It never checks the value, and set is false when none of that place's own files sets the key.
// A merged value from a place below never counts as this place's own.
func ownTaskSetting(dirs configengine.Dirs, key string) (value any, path string, set bool, err error) {
	return configengine.TopLevelValue(dirs, ConfigSpec(), key)
}

// lookupTaskSetting returns the value of the per-task key for the task at the chain's head.
func lookupTaskSetting(chain TaskSettingChain, key string) (string, error) {
	value, _, err := LookupWithSource(chain, key)
	return value, err
}

// HubWideSource is the anchor index LookupWithSource answers when no worktree of the chain sets the key.
// The hub's default layer, else the template, then decides.
const HubWideSource = -1

// LookupWithSource returns the value of the per-task key for the task at the chain's head, as its run reads it.
// It also returns the index into the chain's Anchors of the worktree whose own files set the value, or HubWideSource.
// Each anchor's own files, its worktree file and its local file, are read in chain order,
// and the first that sets the key decides.
// With none set, the board's default layer decides, and then the template.
// An envsource marker in the winning value is expanded the way a load of beam.yaml expands it.
// beam.autonomy answers "true" or "false".
// A value the key does not take is refused, naming the file that set it.
func LookupWithSource(chain TaskSettingChain, key string) (value string, anchor int, err error) {
	places := make([]configengine.Dirs, 0, len(chain.Anchors)+1)
	for _, dir := range chain.Anchors {
		places = append(places, configengine.Dirs{Anchor: dir})
	}
	if chain.Board != "" {
		places = append(places, configengine.Dirs{Board: chain.Board})
	}
	for index, dirs := range places {
		raw, path, set, err := ownTaskSetting(dirs, key)
		if err != nil {
			return "", 0, fmt.Errorf("look up %s: %w", keyName(key), err)
		}
		if !set {
			continue
		}
		expanded, err := expandEnvMarkers(dirs, key, raw)
		if err != nil {
			return "", 0, fmt.Errorf("expand %s in %s: %w", keyName(key), path, err)
		}
		if index >= len(chain.Anchors) {
			index = HubWideSource
		}
		value, err = checkTaskSetting(key, expanded, path)
		return value, index, err
	}
	var template map[string]any
	if err := yaml.Unmarshal([]byte(ConfigTemplate()), &template); err != nil {
		return "", 0, fmt.Errorf("decode beam.yaml's template: %w", err)
	}
	value, err = checkTaskSetting(key, template[key], "beam.yaml's template")
	return value, HubWideSource, err
}

// expandEnvMarkers expands the envsource markers of a text value, with the environment of the place dirs names.
// A value that is no text, or holds no marker, comes back as it is.
func expandEnvMarkers(dirs configengine.Dirs, key string, value any) (any, error) {
	text, isText := value.(string)
	if !isText || !strings.Contains(text, "${") {
		return value, nil
	}
	baseDir := dirs.Anchor
	if baseDir == "" {
		baseDir = dirs.Board
	}
	env, err := envsource.Build(baseDir)
	if err != nil {
		return nil, fmt.Errorf("build environment: %w", err)
	}
	document, err := yaml.Marshal(map[string]any{key: text})
	if err != nil {
		return nil, fmt.Errorf("encode value: %w", err)
	}
	resolved, err := yamlengine.Resolve(document, env)
	if err != nil {
		return nil, err
	}
	var expanded map[string]any
	if err := yaml.Unmarshal(resolved, &expanded); err != nil {
		return nil, fmt.Errorf("decode expanded value: %w", err)
	}
	return expanded[key], nil
}
