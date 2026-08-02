// Package hubgeometry is the single owner of Loomyard worktree and container geometry.
// It resolves the active Layout from a working directory and exposes typed
// accessors for every derived path, so no other package recomputes geometry
// from raw os.Getwd or git --show-toplevel calls.
package hubgeometry

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// Layout and geometry constants define directory and file names used by lyx
// configuration and weft/board/hub geometry. All path construction must use these
// constants, never inline string literals.
const (
	// LyxDirName is the directory name for the lyx system directory within a worktree.
	LyxDirName = "_lyx"

	// dotLyxDirName is the directory name for ephemeral, machine-bound lyx state (e.g. reed runtime
	// state), distinct from LyxDirName ("_lyx") which is durable and weft-synced.
	dotLyxDirName = ".lyx"

	// configDirName is the subdirectory name within LyxDirName that holds configuration files.
	configDirName = "config"

	// dotEnvName is the filename for environment variable overrides.
	dotEnvName = ".env"

	// WeftSuffix is the suffix appended to a host-worktree slug to form the weft sibling
	// directory name (e.g. "feat" → "feat-weft"). Use WeftSiblingPath/WeftRepoRoot/WeftWorktreePath
	// rather than this constant directly.
	WeftSuffix = "-weft"

	// BoardDirName is the name of the board data directory inside the hub (i.e. <hub>/_board).
	// It is the single source of this literal; use BoardDir(hub) to obtain the full path.
	BoardDirName = "_board"

	// HubSuffix is the suffix appended to a repo name to form the hub container directory
	// (e.g. "loomyard" → "loomyard-HUB"). Use HubPath(parent, name) to obtain the full path.
	HubSuffix = "-HUB"

	// PatternDirName is the directory name for the PATTERN constraint-injection surface
	// within a worktree (i.e. <worktree>/_pattern). Use PatternDir/PatternFile to obtain paths.
	PatternDirName = "_pattern"
)

// ErrNotAGitRepo is returned when a directory is not within a git repository.
var ErrNotAGitRepo = errors.New("not a git repository")

// Layout represents the geometry of a worktree and container within a git repository.
type Layout struct {
	Cwd          string
	WorktreeRoot string
	Hub          string
	RelPath      string
	Prime        string
	Repo         string
}

// Getwd returns the current working directory.
//
// It wraps os.Getwd and is the ONLY permitted os.Getwd call outside cmd/lyx/main.go.
// Returns an error if the current directory cannot be determined.
func Getwd() (string, error) {
	return os.Getwd()
}

// Resolve builds a Layout from the given cwd by running git rev-parse --show-toplevel
// and reading the recorded .fabric-anchor marker for RelPath.
//
// When an anchor is recorded, it validates that cwd is at or below the anchored subpath,
// returning ErrCwdOutsideAnchor if not. When absent, RelPath defaults to cwd-relative path.
// Resolve does NOT check for _lyx/ (that stays in internal/configengine).
//
// Returns the Layout on success, ErrNotAGitRepo when git fails or cwd is outside a git repo,
// or ErrCwdOutsideAnchor when cwd violates a recorded anchor's subtree.
func Resolve(cwd string) (*Layout, error) {
	return resolveCore(cwd, true)
}

// ResolveWorktree builds a Layout like Resolve but applies NO cwd at-or-below gate.
//
// It exists for callers holding a worktree root (not an acting cwd) where the gate would
// spuriously fire. The gate applies only to Resolve(cwd), not internal sibling-layout
// construction above a subpath anchor.
func ResolveWorktree(worktreeRoot string) (*Layout, error) {
	return resolveCore(worktreeRoot, false)
}

// resolveCore is the shared body behind Resolve and ResolveWorktree: it runs
// git rev-parse --show-toplevel, reads the recorded anchor for RelPath, and
// optionally applies the cwd at-or-below gate. applyGate is true only for
// Resolve's entry-point cwd; ResolveWorktree passes false because its input is
// a worktree root, not an acting cwd, and must never be gated against itself.
func resolveCore(cwd string, applyGate bool) (*Layout, error) {
	stdout, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--show-toplevel"}, cwd)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotAGitRepo, err)
	}
	if exitCode != 0 {
		return nil, ErrNotAGitRepo
	}

	workTreeRoot := filepath.FromSlash(strings.TrimSpace(stdout))
	workTreeRoot = filepath.Clean(workTreeRoot)

	cleanCwd := filepath.Clean(cwd)
	hub := filepath.Dir(workTreeRoot)
	relPath, _ := filepath.Rel(workTreeRoot, cleanCwd)

	if anchor, found := readRecordedAnchor(hub); found {
		relPath = anchor

		if applyGate {
			anchorAbs := filepath.Join(workTreeRoot, anchor)
			rel, err := filepath.Rel(anchorAbs, cleanCwd)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("%w: cwd %s is not at or below %s", ErrCwdOutsideAnchor, cleanCwd, anchorAbs)
			}
		}
	}

	entries, err := List(cwd)
	if err != nil {
		return nil, fmt.Errorf("failed to get main worktree: %w", err)
	}

	prime := ""
	for _, entry := range entries {
		if entry.Main {
			// Normalize prime path (git may emit forward slashes)
			prime = filepath.FromSlash(entry.Path)
			prime = filepath.Clean(prime)
			break
		}
	}

	return &Layout{
		Cwd:          cleanCwd,
		WorktreeRoot: workTreeRoot,
		Hub:          hub,
		RelPath:      relPath,
		Prime:        prime,
		Repo:         deriveRepo(prime, workTreeRoot),
	}, nil
}

// deriveRepo derives the repository name from Prime without spawning git.
// It returns filepath.Base(prime) when prime is non-empty; otherwise filepath.Base(worktreeRoot).
func deriveRepo(prime, worktreeRoot string) string {
	if prime != "" {
		return filepath.Base(prime)
	}
	return filepath.Base(worktreeRoot)
}

// SiblingLayout derives the Layout for a hub-sibling worktree without spawning git.
//
// Precondition: worktreeRoot must be a direct child of l.Hub. Callers must guard the
// non-sibling case (filepath.Dir(worktreeRoot) != l.Hub) themselves; SiblingLayout performs
// no check. Like ResolveWorktree, it applies no cwd-legitimacy gate.
func (l *Layout) SiblingLayout(worktreeRoot string) *Layout {
	c := filepath.Clean(worktreeRoot)

	relPath := "."
	if anchor, found := readRecordedAnchor(l.Hub); found {
		relPath = anchor
	}

	return &Layout{
		Cwd:          c,
		WorktreeRoot: c,
		Hub:          l.Hub,
		RelPath:      relPath,
		Prime:        l.Prime,
		Repo:         l.Repo,
	}
}

// ConfigDir returns the path to the config directory within a baseDir.
func ConfigDir(baseDir string) string {
	return filepath.Join(baseDir, LyxDirName, configDirName)
}

// ConfigFile returns the path to a module-specific configuration YAML file within a baseDir.
func ConfigFile(baseDir, module string) string {
	return filepath.Join(ConfigDir(baseDir), module+".yaml")
}

// PerchRunsDir returns the path to the base directory for perch run artifacts.
// It lives under _lyx so artifacts are weft-synced. Per the Hub Geometry Invariant, no other
// package may construct this path.
func PerchRunsDir(baseDir string) string {
	return filepath.Join(baseDir, LyxDirName, "perch")
}

// PlanDir returns the path to the plan's artifact directory within a baseDir.
// It holds 00-overview.md and per-plan-unit files. Both v1/v2 and v3 formats share this directory.
// It lives under _lyx so the plan is weft-synced. Per the Hub Geometry Invariant, no other
// package may construct this path.
func PlanDir(baseDir string) string {
	return filepath.Join(baseDir, LyxDirName, "plan")
}

// PlanDirRel returns the worktree-relative plan-directory token, `_lyx/plan`.
// Callers use this for relative plan-file pointers (e.g. planparser's Card.SourcePath token).
// It uses the stdlib path package so the token is always forward-slash, never OS-dependent.
func PlanDirRel() string {
	return path.Join(LyxDirName, "plan")
}

// PlanDir returns the path to the Plan phase's output directory for this worktree.
// It is WorktreeRoot-anchored, not Cwd-anchored, matching DiscussionDir's rationale.
// Per the Hub Geometry Invariant, no other package may construct this path.
func (l *Layout) PlanDir() string {
	return PlanDir(l.WorktreeRoot)
}

// PlanOverview returns the path to the plan's overview file: the Plan phase's done-sentinel
// and the Planner producer's sole Spec.OutputFiles entry. It shares PlanDir's WorktreeRoot
// anchoring. Per the Hub Geometry Invariant, no other package may construct this path.
func (l *Layout) PlanOverview() string {
	return filepath.Join(l.PlanDir(), "00-overview.md")
}

// BuilderDir returns the path to the builder's durable run state directory (state.json,
// pause flag, outcome.yaml). It lives under _lyx so it is weft-synced. Per the Hub Geometry
// Invariant, no other package may construct this path.
func BuilderDir(baseDir string) string {
	return filepath.Join(baseDir, LyxDirName, "builder")
}

// BuilderReportsDir returns the path to the directory holding builder's per-batch report files.
// It lives under _lyx so reports are weft-synced. Per the Hub Geometry Invariant, no
// other package may construct this path.
func BuilderReportsDir(baseDir string) string {
	return filepath.Join(BuilderDir(baseDir), "reports")
}

// WebsterDir returns the path to the webster's durable run state directory (state.json,
// pause flag, outcome.yaml). It lives under _lyx so it is weft-synced. Per the Hub Geometry
// Invariant, no other package may construct this path.
func WebsterDir(baseDir string) string {
	return filepath.Join(baseDir, LyxDirName, "webster")
}

// WebsterReportsDir returns the path to the directory holding webster's per-batch report files.
// It lives under _lyx so reports are weft-synced. Per the Hub Geometry Invariant, no other
// package may construct this path.
func WebsterReportsDir(baseDir string) string {
	return filepath.Join(WebsterDir(baseDir), "reports")
}

// WebsterPromptsDir returns the path to the directory holding webster's rendered fork prompts.
// Prompts are machine-local, re-renderable artifacts excluded from weft commits.
// Per the Hub Geometry Invariant, no other package may construct this path.
func WebsterPromptsDir(baseDir string) string {
	return filepath.Join(WebsterDir(baseDir), "prompts")
}

// DotEnv returns the path to the .env file within a baseDir.
func DotEnv(baseDir string) string {
	return filepath.Join(baseDir, dotEnvName)
}

// PatternDir returns the path to the _pattern directory within a baseDir.
// Per the Hub Geometry Invariant, no other package may construct this path.
func PatternDir(baseDir string) string {
	return filepath.Join(baseDir, PatternDirName)
}

// PatternFile returns the path to the PATTERN.md file within a baseDir.
func PatternFile(baseDir string) string {
	return filepath.Join(PatternDir(baseDir), "PATTERN.md")
}

// WeftSiblingPath returns the absolute path to the weft sibling worktree for the given slug inside hub.
func WeftSiblingPath(hub, slug string) string {
	return filepath.Join(hub, slug+WeftSuffix)
}

// BoardDir returns the absolute path to the board data directory inside hub.
func BoardDir(hub string) string {
	return filepath.Join(hub, BoardDirName)
}

// HubPath returns the absolute path to the hub container directory for the given repo name inside parent.
func HubPath(parent, name string) string {
	return filepath.Join(parent, name+HubSuffix)
}

// WeftHostSlug parses a weft sibling directory name and returns the host slug it corresponds to.
// It reports whether name ends with WeftSuffix AND the stripped prefix is non-empty.
func WeftHostSlug(name string) (slug string, ok bool) {
	if !strings.HasSuffix(name, WeftSuffix) {
		return "", false
	}
	s := strings.TrimSuffix(name, WeftSuffix)
	if s == "" {
		return "", false
	}
	return s, true
}

// HubReservedNames returns the hub-structural reserved name-set that hubgeometry owns:
// _raddle, _board, _portals, _launchers. It deliberately excludes LyxDirName and PatternDirName,
// which are config-migrated junction names folded into the reserved set by IsReservedHubName's
// junctionNames parameter instead.
func HubReservedNames() []string {
	return []string{BoardDirName, "_portals", "_launchers", "_raddle"}
}

// IsReservedHubName reports whether name is one of the hub-level entry names a worktree slug
// must never claim: HubReservedNames UNION the caller-supplied junctionNames (the weft-backed
// junction name-set injected from fabric config). hubgeometry stays config-blind; the junction
// portion is passed in by the caller.
func IsReservedHubName(name string, junctionNames []string) bool {
	for _, reserved := range HubReservedNames() {
		if name == reserved {
			return true
		}
	}
	for _, junctionName := range junctionNames {
		if name == junctionName {
			return true
		}
	}
	return false
}

// LyxDir returns the path to the _lyx directory in the current working directory.
func (l *Layout) LyxDir() string {
	return filepath.Join(l.Cwd, LyxDirName)
}

// DotLyxDir returns the path to the ephemeral .lyx directory (machine-bound runtime state),
// distinct from the durable, weft-synced LyxDir().
func (l *Layout) DotLyxDir() string {
	return filepath.Join(l.Cwd, dotLyxDirName)
}

// LoomStatusFile returns the path to the loom phase-machine's status.json sidecar for this worktree.
// It is WorktreeRoot-anchored, not Cwd-anchored, to ensure a caller invoked from a subdirectory
// resolves the one true status.json at the worktree root.
func (l *Layout) LoomStatusFile() string {
	return filepath.Join(l.WorktreeRoot, LyxDirName, "status.json")
}

// LoomStatusLock returns the path to the advisory lock file guarding concurrent access to LoomStatusFile().
// It shares LoomStatusFile's WorktreeRoot anchoring.
func (l *Layout) LoomStatusLock() string {
	return filepath.Join(l.WorktreeRoot, LyxDirName, "status.json.lock")
}

// ScoutDaemonStateFile returns the path to the scout daemon's runtime state file for the given
// language. It is WorktreeRoot-anchored so the daemon is a worktree-wide singleton per language.
// It lives under .lyx (ephemeral) not _lyx (durable) so PIDs/sockets don't get committed.
func (l *Layout) ScoutDaemonStateFile(lang string) string {
	return filepath.Join(l.WorktreeRoot, dotLyxDirName, "scout", lang, "daemon.json")
}

// ScoutDaemonLock returns the path to the advisory lock file guarding concurrent access to
// ScoutDaemonStateFile(lang). It shares that method's WorktreeRoot anchoring and per-lang scoping.
func (l *Layout) ScoutDaemonLock(lang string) string {
	return filepath.Join(l.WorktreeRoot, dotLyxDirName, "scout", lang, "daemon.lock")
}

// DiscussionDir returns the path to the Discussion phase's output directory for this worktree
// (the decision-record.md/support-log.md pair). It is WorktreeRoot-anchored, not Cwd-anchored.
// Per the Hub Geometry Invariant, no other package may construct this path.
func (l *Layout) DiscussionDir() string {
	return filepath.Join(l.WorktreeRoot, LyxDirName, "discussion")
}

// DiscussionDecisionRecord returns the path to the distilled decision record that is the Plan
// producer's sole input from `_lyx/discussion/`. It shares DiscussionDir's WorktreeRoot anchoring.
// Per the Hub Geometry Invariant, no other package may construct this path.
func (l *Layout) DiscussionDecisionRecord() string {
	return filepath.Join(l.DiscussionDir(), "decision-record.md")
}

// DiscussionSupportLog returns the path to the raw support log read by the Discussion-review gate only.
// It shares DiscussionDir's WorktreeRoot anchoring. Per the Hub Geometry Invariant, no other
// package may construct this path.
func (l *Layout) DiscussionSupportLog() string {
	return filepath.Join(l.DiscussionDir(), "support-log.md")
}

// HubLogsDir returns the path to the hub-level directory where the shared per-hub reed server
// writes its runtime log. It is hub-anchored so one server per hub resolves to one deterministic place.
// It lives under the ephemeral .lyx directory; server logs are runtime artifacts, never weft-synced.
func (l *Layout) HubLogsDir() string {
	return filepath.Join(l.Hub, dotLyxDirName, "logs")
}

// WorktreeLogsDir returns the path to the worktree-level directory where internal/logger's
// durable trace sink writes one file per process. It is WorktreeRoot-anchored, not Cwd-anchored,
// matching LoomStatusFile's anchoring. It lives under the ephemeral .lyx directory.
func (l *Layout) WorktreeLogsDir() string {
	return filepath.Join(l.WorktreeRoot, dotLyxDirName, "logs")
}

// WorktreePath returns the path to a sibling worktree with the given slug.
func (l *Layout) WorktreePath(slug string) string {
	return filepath.Join(l.Hub, slug)
}

// PortalsDir returns the path to the _portals directory in the hub.
func (l *Layout) PortalsDir() string {
	return filepath.Join(l.Hub, "_portals")
}

// PortalLink returns the path to the mirrored portal junction link for the given slug.
// It is mirrored into the repo subpath structure, including RelPath segments.
func (l *Layout) PortalLink(slug string) string {
	return filepath.Join(l.Hub, "_portals", l.RelPath, slug)
}

// PortalTarget returns the path to the _lyx directory within a portal for the given slug.
func (l *Layout) PortalTarget(slug string) string {
	return filepath.Join(l.Hub, slug, l.RelPath, LyxDirName)
}

// LaunchersDir returns the path to the _launchers directory in the hub.
func (l *Layout) LaunchersDir() string {
	return filepath.Join(l.Hub, "_launchers")
}

// LauncherDir returns the path to the mirrored launcher directory for the given slug.
// It is mirrored into the repo subpath structure, including RelPath segments.
func (l *Layout) LauncherDir(slug string) string {
	return filepath.Join(l.Hub, "_launchers", l.RelPath, slug)
}

// MenuLauncherPath returns the path to the per-subpath menu launcher script.
// It is mirrored into the repo subpath structure. The extension is GOOS-selected: ".cmd" on Windows, ".sh" elsewhere.
func (l *Layout) MenuLauncherPath() string {
	return filepath.Join(l.Hub, "_launchers", l.RelPath, menuLauncherName())
}

// menuLauncherName returns the OS-appropriate filename for the menu launcher script.
func menuLauncherName() string {
	if runtime.GOOS == "windows" {
		return "ide-menu.cmd"
	}
	return "ide-menu.sh"
}

// LauncherSpawnRel returns the relative path from a launcher directory to the target worktree's
// subpath for spawning.
func (l *Layout) LauncherSpawnRel(slug string) string {
	rel, _ := filepath.Rel(l.LauncherDir(slug), filepath.Join(l.WorktreePath(slug), l.RelPath))
	return rel
}

// MenuLauncherRel returns the relative path from the menu launcher directory to the main
// worktree's subpath for menu spawning.
func (l *Layout) MenuLauncherRel() string {
	rel, _ := filepath.Rel(filepath.Dir(l.MenuLauncherPath()), filepath.Join(l.Prime, l.RelPath))
	return rel
}

// PrimeName returns the base name of the main worktree.
func (l *Layout) PrimeName() string {
	return filepath.Base(l.Prime)
}

// WeftRepoRoot returns the path to the weft Prime worktree (the git -C target for weft worktree add/remove).
func (l *Layout) WeftRepoRoot() string {
	return WeftSiblingPath(l.Hub, l.PrimeName())
}

// WeftWorktreePath returns the path to a sibling weft worktree with the given slug.
func (l *Layout) WeftWorktreePath(slug string) string {
	return WeftSiblingPath(l.Hub, slug)
}

// WeftWorktree returns the path to the weft worktree paired with the current host worktree.
func (l *Layout) WeftWorktree() string {
	return WeftSiblingPath(l.Hub, filepath.Base(l.WorktreeRoot))
}

// WeftLyxDir returns the path to the _lyx directory in the current worktree's weft sibling.
// It is the junction target for lyx weft and the pathspec base for weft operations.
func (l *Layout) WeftLyxDir() string {
	return filepath.Join(l.WeftWorktree(), l.RelPath, LyxDirName)
}

// WeftLyxDirFor returns the path to the _lyx directory within a named slug's weft worktree.
// It is the junction target paired by spawn seeds and pairs with HostLyxLink(slug).
func (l *Layout) WeftLyxDirFor(slug string) string {
	return filepath.Join(l.WeftWorktreePath(slug), l.RelPath, LyxDirName)
}

// WeftPatternDir returns the path to the _pattern directory in the current worktree's weft sibling.
// It mirrors WeftLyxDir exactly and is the junction target for pattern weft.
func (l *Layout) WeftPatternDir() string {
	return filepath.Join(l.WeftWorktree(), l.RelPath, PatternDirName)
}

// WeftPatternDirFor returns the path to the _pattern directory within a named slug's weft worktree.
// It mirrors WeftLyxDirFor exactly and pairs with HostPatternLink(slug) as junction endpoints.
func (l *Layout) WeftPatternDirFor(slug string) string {
	return filepath.Join(l.WeftWorktreePath(slug), l.RelPath, PatternDirName)
}

// WeftRaddleDir returns the path to the _raddle directory in the current worktree's weft sibling.
//
// Returns filepath.Join(WeftWorktree(), RelPath, "_raddle").
func (l *Layout) WeftRaddleDir() string {
	return filepath.Join(l.WeftWorktree(), l.RelPath, "_raddle")
}

// HostLyxLink returns the path to the _lyx junction link in a named slug's host worktree.
// It is the host-side junction endpoint that points into the paired weft worktree via WeftLyxDirFor(slug).
func (l *Layout) HostLyxLink(slug string) string {
	return filepath.Join(l.WorktreePath(slug), l.RelPath, LyxDirName)
}

// HostLyxLinkHere returns the path to the _lyx junction link in the current host worktree.
// Derived from WorktreeRoot+RelPath, not from Cwd. It serves as the host-side junction endpoint
// paired with WeftLyxDir().
func (l *Layout) HostLyxLinkHere() string {
	return filepath.Join(l.WorktreeRoot, l.RelPath, LyxDirName)
}

// HostPatternLink returns the path to the _pattern junction link in a named slug's host worktree.
// It mirrors HostLyxLink exactly and points into the paired weft worktree via WeftPatternDirFor(slug).
func (l *Layout) HostPatternLink(slug string) string {
	return filepath.Join(l.WorktreePath(slug), l.RelPath, PatternDirName)
}

// HostPatternLinkHere returns the path to the _pattern junction link in the current host worktree.
// Derived from WorktreeRoot+RelPath, not from Cwd. It mirrors HostLyxLinkHere exactly and serves
// as the host-side junction endpoint paired with WeftPatternDir().
func (l *Layout) HostPatternLinkHere() string {
	return filepath.Join(l.WorktreeRoot, l.RelPath, PatternDirName)
}

// PatternFileHere returns the path to the PATTERN.md file for the current worktree.
// It is anchored at WorktreeRoot+RelPath to correctly handle nested-hub geometry and to stay
// consistent with junction endpoints above, which are all WorktreeRoot+RelPath-anchored.
func (l *Layout) PatternFileHere() string {
	return PatternFile(filepath.Join(l.WorktreeRoot, l.RelPath))
}

// HostJunction represents a directory junction in the host worktree that links to a weft directory.
type HostJunction struct {
	Name   string // Name is the directory name (e.g., "_lyx")
	Link   string // Link is the host-side path to the junction
	Target string // Target is the weft-side path the junction points to
}

// HostJunctions returns the list of host junctions for a given slug, one record per name in names,
// in names's own order (no forced sort). For each name, the record is {Name, Link, Target} where
// Link and Target are computed from WorktreePath/WeftWorktreePath and RelPath.
// HostJunctions is Hub/slug-anchored; HostJunctionsHere below is the Here-anchored counterpart.
func (l *Layout) HostJunctions(slug string, names []string) []HostJunction {
	junctions := make([]HostJunction, 0, len(names))
	for _, name := range names {
		junctions = append(junctions, HostJunction{
			Name:   name,
			Link:   filepath.Join(l.WorktreePath(slug), l.RelPath, name),
			Target: filepath.Join(l.WeftWorktreePath(slug), l.RelPath, name),
		})
	}
	return junctions
}

// HostJunctionsHere returns the same HostJunction records as HostJunctions(slug, names),
// but resolved against the current worktree rather than a named slug: Link is built from
// WorktreeRoot and each Target from WeftWorktree(). This mirrors HostLyxLinkHere()/HostLyxLink(slug).
// It exists for health-check sites that are Here-anchored and have no slug available.
func (l *Layout) HostJunctionsHere(names []string) []HostJunction {
	junctions := make([]HostJunction, 0, len(names))
	for _, name := range names {
		junctions = append(junctions, HostJunction{
			Name:   name,
			Link:   filepath.Join(l.WorktreeRoot, l.RelPath, name),
			Target: filepath.Join(l.WeftWorktree(), l.RelPath, name),
		})
	}
	return junctions
}
