// config.go — configuration for the loom module.
//
// Defines the Config type mirroring loom.yaml's keys and LoadConfig, which uses
// internal/configengine.Load with ConfigTemplate() to strictly validate and resolve loom's config
// file, then validates the discussion and plan role model-specs' grammar via modelspec.Parse so a
// typo'd spec fails loud at load time rather than hours into a run when the discussion or plan
// producer first spawns.

package loomengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"gopkg.in/yaml.v3"
)

// discussionDirName is the relative-path segment loomengine joins onto
// lyxdirs.LyxDirName to form the discussion phase's output directory.
// loomengine is this segment's sole declarer.
const discussionDirName = "discussion"

// loomDirName is the relative-path segment loomengine joins onto lyxdirs.LyxDirName or
// lyxdirs.DotLyxDirName to scope every loom-owned path under its own subdirectory, distinct from
// the other products (e.g. Someday Hardener) that also configure Shed.
// loomengine is this segment's sole declarer.
const loomDirName = "loom"

// loomStatusFileName is the filename of loom's phase-machine status sidecar within loomDirName.
// loomengine is this segment's sole declarer.
const loomStatusFileName = "status.json"

// DiscussionDir returns the path to the Discussion phase's output directory for this worktree (the
// decision-record.md/support-log.md pair).
// It is AnchorPath-anchored.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func DiscussionDir(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, discussionDirName)
}

// DiscussionDecisionRecord returns the path to the distilled decision record that is the Plan
// producer's sole input from `_lyx/discussion/`.
// It shares DiscussionDir's AnchorPath anchoring.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func DiscussionDecisionRecord(l *lyxcwd.Location) string {
	return filepath.Join(DiscussionDir(l), "decision-record.md")
}

// DiscussionSupportLog returns the path to the raw support log read by the Discussion-review gate
// only.
// It shares DiscussionDir's AnchorPath anchoring.
// Per the Cwd Resolution Invariant, no other package may construct this path.
func DiscussionSupportLog(l *lyxcwd.Location) string {
	return filepath.Join(DiscussionDir(l), "support-log.md")
}

// LoomStatusRel returns the worktree-anchor-relative form of LoomStatusFile's path: the join of
// lyxdirs.LyxDirName, loomDirName, and loomStatusFileName.
// It exists so a caller building a fabric commit pathspec never has to name a directory segment loom
// owns.
func LoomStatusRel() string {
	return filepath.Join(lyxdirs.LyxDirName, loomDirName, loomStatusFileName)
}

// LoomStatusFile returns the path to the loom phase-machine's status.json sidecar for this
// worktree.
// It is AnchorPath-anchored so a caller invoked from anywhere else within the worktree still
// resolves the one true status.json at the anchored subpath.
// Scoped under a "loom" subdirectory, not bare _lyx, because Shed (see manifest/designs/shed.md) is
// a generic engine more than one product configures -- the Someday Hardener product will need its
// own status file too, and a bare _lyx/status.json could not serve both without colliding.
func LoomStatusFile(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), LoomStatusRel())
}

// LoomStatusLock returns the path to the advisory lock file guarding concurrent access to
// LoomStatusFile(l).
// It is AnchorPath-anchored like LoomStatusFile, but lives under lyxdirs.DotLyxDirName rather than
// LoomStatusFile's lyxdirs.LyxDirName: the lock is a never-tracked transient, not durable orchestration
// status, so it is stated outright at its mirrored .lyx subpath rather than derived by analogy --
// loomengine has no Dir(l) accessor for a ScratchDir(l) to mirror.
// Scoped under the same "loom" subdirectory as LoomStatusFile, for the same product-collision reason.
func LoomStatusLock(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName, "status.json.lock")
}

// LoomRunLock returns the path to the advisory lock file Shed holds for the whole duration of a
// run, distinct from LoomStatusLock's per-persist status lock.
// It is AnchorPath-anchored like LoomStatusFile and LoomStatusLock.
// It must never equal LoomStatusLock(l): internal/state acquires the status lock with the
// blocking form, so Shed.validate() rejects LockPath == StatusLockPath outright, and a shared file
// would hang on the first persist rather than fail.
// Scoped under the same "loom" subdirectory as LoomStatusFile and LoomStatusLock, for the same
// product-collision reason.
func LoomRunLock(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName, "run.lock")
}

// LoomDriverLog returns the path to the detached driver's captured stdout and stderr for this
// worktree's session bootstrap.
// It is AnchorPath-anchored, living under the ephemeral tree at the mirrored subpath of the durable
// status file per the Durable-vs-Ephemeral State Invariant.
// It exists as an accessor rather than an inline path because cmd/lyx's transient guard walks
// constructors, not call sites.
func LoomDriverLog(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName, "driver.log")
}

// LoomBootstrapLock returns the path to the advisory lock file serialising the session bootstrap's
// probe-and-spawn sequence.
// It is AnchorPath-anchored, living under the ephemeral tree at the mirrored subpath of the durable
// status file per the Durable-vs-Ephemeral State Invariant.
// It is a third lock distinct from both LoomStatusLock's per-persist status lock and LoomRunLock's
// whole-run lock, and it is released before the terminal handover because that handover blocks for
// the operator's entire session.
func LoomBootstrapLock(l *lyxcwd.Location) string {
	return filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName, "bootstrap.lock")
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
