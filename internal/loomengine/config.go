// config.go — configuration for the loom module.
//
// Defines the Config type mirroring loom.yaml's keys and LoadConfig, which
// uses internal/configengine.Load with ConfigTemplate() to strictly
// validate and resolve loom's config file, then validates the discussion
// and plan role model-specs' grammar via modelspec.Parse so a typo'd spec
// fails loud at load time rather than hours into a run when the
// discussion or plan producer first spawns.

package loomengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"gopkg.in/yaml.v3"
)

// discussionDirName is the relative-path segment loomengine joins onto
// configengine.LyxDirName to form the discussion phase's output directory.
// loomengine is this segment's sole declarer.
const discussionDirName = "discussion"

// PlanDir returns the path to the Plan phase's output directory for this
// worktree. It is AnchorPath-anchored, matching DiscussionDir's rationale.
// Per the Hub Geometry Invariant, no other package may construct this path.
func PlanDir(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), configengine.LyxDirName, planparser.PlanDirName)
}

// PlanOverview returns the path to the plan's overview file: the Plan
// phase's done-sentinel and the Planner producer's sole Spec.OutputFiles
// entry. It shares PlanDir's AnchorPath anchoring. Per the Hub Geometry
// Invariant, no other package may construct this path.
func PlanOverview(l *lyxcwd.Location) string {
	return filepath.Join(PlanDir(l), "00-overview.md")
}

// DiscussionDir returns the path to the Discussion phase's output directory
// for this worktree (the decision-record.md/support-log.md pair). It is
// AnchorPath-anchored. Per the Hub Geometry Invariant, no other package may
// construct this path.
func DiscussionDir(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), configengine.LyxDirName, discussionDirName)
}

// DiscussionDecisionRecord returns the path to the distilled decision
// record that is the Plan producer's sole input from `_lyx/discussion/`. It
// shares DiscussionDir's AnchorPath anchoring. Per the Hub Geometry
// Invariant, no other package may construct this path.
func DiscussionDecisionRecord(l *lyxcwd.Location) string {
	return filepath.Join(DiscussionDir(l), "decision-record.md")
}

// DiscussionSupportLog returns the path to the raw support log read by the
// Discussion-review gate only. It shares DiscussionDir's AnchorPath
// anchoring. Per the Hub Geometry Invariant, no other package may construct
// this path.
func DiscussionSupportLog(l *lyxcwd.Location) string {
	return filepath.Join(DiscussionDir(l), "support-log.md")
}

// LoomStatusFile returns the path to the loom phase-machine's status.json
// sidecar for this worktree. It is AnchorPath-anchored so a caller invoked
// from anywhere else within the worktree still resolves the one true
// status.json at the anchored subpath.
func LoomStatusFile(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), configengine.LyxDirName, "status.json")
}

// LoomStatusLock returns the path to the advisory lock file guarding
// concurrent access to LoomStatusFile(l). It shares LoomStatusFile's
// AnchorPath anchoring.
func LoomStatusLock(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), configengine.LyxDirName, "status.json.lock")
}

// Config represents the resolved loom.yaml configuration: role model-specs and timeout knobs.
type Config struct {
	Discussion           string `yaml:"discussion"`
	DiscussionTimeoutMin int    `yaml:"discussion_timeout_min"`
	Plan                 string `yaml:"plan"`
	PlanTimeoutMin       int    `yaml:"plan_timeout_min"`
}

// LoadConfig loads and unmarshals configuration for the loom module.
// It validates model-spec grammar at load time.
func LoadConfig(baseDir, module string) (Config, error) {
	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
	if err != nil {
		if strings.Contains(err.Error(), "not initialized") {
			return Config{}, fmt.Errorf("not initialized here; run \"lyx fabric reconcile\"")
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal loom config: %w", err)
	}

	if _, err := modelspec.Parse(cfg.Discussion); err != nil {
		return Config{}, fmt.Errorf("loom config key %q: %w", "discussion", err)
	}

	if _, err := modelspec.Parse(cfg.Plan); err != nil {
		return Config{}, fmt.Errorf("loom config key %q: %w", "plan", err)
	}

	return cfg, nil
}
