// config.go declares the beam config module, beam.yaml: its template, its layered-load spec and LoadConfig.

package dobbyengine

import (
	_ "embed"
	"fmt"
	"math"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/beamsetting"
	"github.com/Knatte18/loomyard/internal/configengine"
)

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns beam.yaml's default YAML template.
func ConfigTemplate() string {
	return configTemplate
}

// The keys of beam.yaml's per-task settings, which only the per-task lookup reads.
// They are declared once, in beamsetting.
const (
	// PullRequestKey decides whether a task's landing goes through a GitHub pull request.
	PullRequestKey = beamsetting.PullRequestKey
	// LandNoticeKey decides whether dobby posts a line to the orch once a task's run lands or ends without landing.
	// A landed motif run's line goes whatever it reads.
	LandNoticeKey = beamsetting.LandNoticeKey
	// AutonomyKey decides how far the orch settles the halts of a task's runs without the operator.
	AutonomyKey = beamsetting.AutonomyKey
)

// AutonomyLevel is a beam.autonomy level: how far the orch settles the halts of a task's runs without the operator.
// Its text is the value a layer sets, and a halt notice's autonomy stamp carries it.
type AutonomyLevel string

// The levels of beam.autonomy, declared once in beamsetting.
const (
	// AutonomyOff relays to the operator every halt the orch does not settle.
	AutonomyOff AutonomyLevel = beamsetting.AutonomyOff
	// AutonomyOn has the orch settle every halt.
	// It reaches the operator only for a severe halt, a merge-ready beam or a raddle split's approval.
	AutonomyOn AutonomyLevel = beamsetting.AutonomyOn
	// AutonomyUnattended has the orch settle every halt and never ask the operator; only a severe halt reaches them.
	AutonomyUnattended AutonomyLevel = beamsetting.AutonomyUnattended
)

// On reports whether the level is true or unattended, the two under which the orch settles halts itself.
// A reader that asks only whether autonomy is on reads it through On.
func (level AutonomyLevel) On() bool {
	return level == AutonomyOn || level == AutonomyUnattended
}

// The values of beam.pull_request, declared once in beamsetting.
const (
	// PullRequestReview lands through a pull request whose review ends on the operator's word.
	// The word is given on GitHub or through `lym loom pr-continue`, with a PR-Review session in the task's window.
	PullRequestReview = beamsetting.PullRequestReview
	// PullRequestOff lands with no pull request unless GitHub's rules for the parent branch require one.
	PullRequestOff = beamsetting.PullRequestOff
	// PullRequestTrail lands through a pull request squash-merged on GitHub at once, with no review.
	PullRequestTrail = beamsetting.PullRequestTrail
	// PullRequestMergeIn lands a task that merges its parent's parent branch into its parent with no pull request.
	PullRequestMergeIn = beamsetting.PullRequestMergeIn
)

// The values of beam.land_notice, declared once in beamsetting.
const (
	// LandNoticeOn sends the landing notice.
	LandNoticeOn = beamsetting.LandNoticeOn
	// LandNoticeOff sends none.
	LandNoticeOff = beamsetting.LandNoticeOff
)

// RunWatchKey is the per-task key deciding which changes of a task's run dobby mails the orch, and its values.
// They are declared once, in beamsetting.
const (
	// RunWatchKey names the key.
	RunWatchKey = beamsetting.RunWatchKey
	// RunWatchStops mails a line for each stop of the run.
	RunWatchStops = beamsetting.RunWatchStops
	// RunWatchRows mails a line for every change of the run.
	RunWatchRows = beamsetting.RunWatchRows
	// RunWatchOff mails none.
	RunWatchOff = beamsetting.RunWatchOff
)

// WindowPlacementKey is the per-task key deciding which desktop a task's terminal window is placed on.
const WindowPlacementKey = beamsetting.WindowPlacementKey

// Config is the resolved beam.yaml's worktree-wide keys.
// The per-task keys are beam.pull_request, beam.land_notice, beam.autonomy, beam.run_watch and beam.window_placement.
// LookupPullRequest, LookupLandNotice, LookupAutonomy, LookupRunWatch and LookupWindowPlacement each read one of them.
// The only other reader of them is LookupWithSource.
type Config struct {
	// MaxRuns is the most runs dobby drives at once; 0 starts none.
	MaxRuns int `yaml:"max_runs"`
	// BoardTidyEvery is the landings between board tidies; 0 never tidies.
	BoardTidyEvery int `yaml:"board_tidy_every"`
	// BoardTidyTimeoutMin is the minutes the board-tidy agent may run.
	BoardTidyTimeoutMin int `yaml:"board_tidy_timeout_min"`
	// TaskHeadroom is the fewest free tasks under the cgroup task limit for dobby to start a run; 0 never holds a start.
	TaskHeadroom int `yaml:"task_headroom"`
	// AdoptFixTries is the most fix tasks a failed adopt check files for the worktree's subject between two hand adopts; 0 files none.
	AdoptFixTries int `yaml:"adopt_fix_tries"`
	// GateHoldFactor times a tier's gate.tiers.<tier>.timeout_min is how long a run may hold a gate slot unnoticed.
	// 0 sends no gate-hold notice.
	GateHoldFactor float64 `yaml:"gate_hold_factor"`
	// GateHoldMin is the minutes the gate-hold bound is for a holder with no tier or a tier with no timeout.
	GateHoldMin int `yaml:"gate_hold_min"`
}

// ConfigSpec returns beam's layered-load spec: a per-worktree module whose keys are all open to every layer.
// beam.pull_request decides how a task lands, never what lands,
// so a task's run settings may set it like beam.land_notice.
// test_map_upkeep, test_map_every and test_map_batch are retired: a layer that still holds one loads as if it did not,
// and a reconcile prunes it.
func ConfigSpec() configengine.Spec {
	return configengine.Spec{
		Name:     beamsetting.Module,
		Template: []byte(ConfigTemplate()),
		Categories: map[string]configengine.Category{
			"max_runs":               configengine.CategoryNone,
			"board_tidy_every":       configengine.CategoryNone,
			"board_tidy_timeout_min": configengine.CategoryNone,
			"task_headroom":          configengine.CategoryNone,
			PullRequestKey:           configengine.CategoryNone,
			LandNoticeKey:            configengine.CategoryNone,
			AutonomyKey:              configengine.CategoryNone,
			RunWatchKey:              configengine.CategoryNone,
			WindowPlacementKey:       configengine.CategoryNone,
			"adopt_fix_tries":        configengine.CategoryNone,
			gateHoldFactorKey:        configengine.CategoryNone,
			gateHoldMinKey:           configengine.CategoryNone,
		},
		RetiredKeys: map[string]configengine.Category{
			"test_map_upkeep": configengine.CategoryNone,
			"test_map_every":  configengine.CategoryNone,
			"test_map_batch":  configengine.CategoryNone,
		},
	}
}

// LoadConfig resolves beam.yaml strictly from the layer stack under dirs, since dobby only ever runs inside a hub.
// A negative value is refused, naming its key.
// So is a beam.gate_hold_factor that is negative or not finite, and a beam.gate_hold_min below 1.
// A resolved per-task key holding a value it does not take is refused too.
// beam.pull_request takes review, off, trail or merge-in, and beam.land_notice takes on or off.
// beam.run_watch takes stops, rows or off, and beam.autonomy takes false, true or unattended.
// beam.window_placement takes "", off, same or next.
// A write the per-task lookup would refuse is therefore refused when it is made.
func LoadConfig(dirs configengine.Dirs) (Config, error) {
	resolved, err := configengine.Load(dirs, ConfigSpec())
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal beam config: %w", err))
	}
	for _, key := range []struct {
		name  string
		value int
	}{
		{"max_runs", cfg.MaxRuns},
		{"board_tidy_every", cfg.BoardTidyEvery},
		{"board_tidy_timeout_min", cfg.BoardTidyTimeoutMin},
		{"task_headroom", cfg.TaskHeadroom},
		{"adopt_fix_tries", cfg.AdoptFixTries},
	} {
		if key.value < 0 {
			return Config{}, configengine.MarkInvalid(fmt.Errorf("config key %q is %d: must not be negative; way forward: fix it with `lym config beam --set %s=<value>`", keyName(key.name), key.value, key.name))
		}
	}
	// A NaN or an infinity passes a sign check and makes a bound every hold is past.
	if factor := cfg.GateHoldFactor; math.IsNaN(factor) || math.IsInf(factor, 0) || factor < 0 {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("config key %q is %v: must be a finite number that is not negative; way forward: fix it with `lym config beam --set %s=<value>`", keyName(gateHoldFactorKey), factor, gateHoldFactorKey))
	}
	if cfg.GateHoldMin < 1 {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("config key %q is %d: must be at least 1; way forward: fix it with `lym config beam --set %s=<value>`", keyName(gateHoldMinKey), cfg.GateHoldMin, gateHoldMinKey))
	}
	var perTask map[string]any
	if err := yaml.Unmarshal(resolved, &perTask); err != nil {
		return Config{}, configengine.MarkInvalid(fmt.Errorf("unmarshal beam config: %w", err))
	}
	for _, key := range []string{PullRequestKey, LandNoticeKey, AutonomyKey, RunWatchKey, WindowPlacementKey} {
		if _, err := checkTaskSetting(key, perTask[key], "the resolved beam.yaml"); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

// CheckLocalValues refuses a local-layer beam document holding a value its key does not take.
// It checks only beam.pull_request, beam.land_notice, beam.autonomy, beam.run_watch and beam.window_placement.
// The refusal is marked [configengine.ErrInvalid] and names the key, the values it takes and the way forward.
func CheckLocalValues(document []byte) error {
	var keys map[string]any
	if err := yaml.Unmarshal(document, &keys); err != nil {
		return configengine.MarkInvalid(fmt.Errorf("unmarshal beam run settings: %w", err))
	}
	for _, key := range []string{PullRequestKey, LandNoticeKey, AutonomyKey, RunWatchKey, WindowPlacementKey} {
		value, set := keys[key]
		if !set {
			continue
		}
		if _, err := checkTaskSetting(key, value, "the task's run_settings"); err != nil {
			return err
		}
	}
	return nil
}

// checkTaskSetting returns the per-task key's value as text, or refuses a value its key does not take.
// where names the file or layer the value was read from, for the refusal.
// A refusal is marked [configengine.ErrInvalid] and names the key, the values it takes and the way forward.
// beam.autonomy takes a YAML boolean, answered "true" or "false", or the text unattended.
func checkTaskSetting(key string, value any, where string) (string, error) {
	if key == AutonomyKey {
		if flag, isBool := value.(bool); isBool {
			return strconv.FormatBool(flag), nil
		}
		if text, isText := value.(string); isText && text == beamsetting.AutonomyUnattended {
			return text, nil
		}
		levels := beamsetting.AutonomyValues()
		return "", configengine.MarkInvalid(fmt.Errorf("config key %q in %s is %v: must be one of %v; way forward: fix it with `lym config beam --set %s=<%s>` in that worktree, or in the task's run_settings for its next run", keyName(key), where, value, levels, key, strings.Join(levels, "|")))
	}
	allowed := beamsetting.LandNoticeValues()
	switch key {
	case PullRequestKey:
		allowed = beamsetting.PullRequestValues()
	case RunWatchKey:
		allowed = beamsetting.RunWatchValues()
	case WindowPlacementKey:
		allowed = beamsetting.WindowPlacementValues()
	}
	text, isText := value.(string)
	if isText {
		for _, candidate := range allowed {
			if text == candidate {
				return text, nil
			}
		}
	}
	choices := fmt.Sprint(allowed)
	return "", configengine.MarkInvalid(fmt.Errorf("config key %q in %s is %v: must be one of %s; way forward: fix it with `lym config beam --set %s=<value>` in that worktree, or in the task's run_settings for its next run", keyName(key), where, value, choices, key))
}
