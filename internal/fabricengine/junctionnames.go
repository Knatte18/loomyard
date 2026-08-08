// junctionnames.go implements the fabric-config-to-junction-name-set bridge: loading the repo-wide
// fabric.yaml pathspec and turning it into the wired name-set the junction primitives operate on,
// with the hub-reserved-name wiring guard applied, plus the two structural directory sets
// (structuralCommittedDirs, structuralNeverCommittedDirs) that never come from that config at all.
// Every wiring caller sources names through junctionNames/WiredNames/RepoWiredNames — never applies
// filterHubReserved itself — so the guard cannot be forgotten at a call site.
// reconcile/status callers read the name-set from the repo-wide `weft:main` base
// (repoWideFabricBase), not each pair's own weft base.

package fabricengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// structuralCommittedDirs is the durable, git-tracked structural directory set: `_lyx` alone today.
// Geometry here is structural, never config/env-overridable (the Cwd Resolution Invariant) —
// every lyx module fails without `_lyx` existing, so its membership cannot be left to an
// operator-editable `fabric.yaml` `pathspec` value.
// A `fabric.yaml` that dropped `_lyx` from `pathspec` would, absent this set, tear away the durable
// tree entirely; injecting it in code instead means no config value can do that.
var structuralCommittedDirs = []string{lyxdirs.LyxDirName}

// structuralNeverCommittedDirs is the machine-local, never-git-tracked structural directory set:
// `.lyx` alone today.
// Geometry here is structural, never config/env-overridable (the Cwd Resolution Invariant) —
// every lyx module fails without `.lyx` existing, so its membership cannot be left to an
// operator-editable `fabric.yaml` `pathspec` value.
// A `fabric.yaml` that omitted `.lyx` from `pathspec` would, absent this set, leave machine-local
// scratch unwired; injecting it in code instead means no config value can do that.
var structuralNeverCommittedDirs = []string{lyxdirs.DotLyxDirName}

// dedupUnion concatenates every slice in groups, keeping only the first occurrence of each name and
// preserving first-occurrence order across the whole call.
// This is load-bearing, not tidy: a deployed `pathspec: _lyx _pattern` fabric.yaml means `_lyx`
// arrives from both structuralCommittedDirs and cfg.Dirs() in the same call, and without dedup the
// duplicate name would reach HostJunctions, ScopedPathspec, and status output.
func dedupUnion(groups ...[]string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, group := range groups {
		for _, name := range group {
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// pathspecNames returns the pathspec/commit-routing set for cfg: structuralCommittedDirs unioned
// with cfg's hub-reserved-filtered directory names.
// It never contains a structuralNeverCommittedDirs entry — this is the set fabric's own weft sync
// and Commit's classification build from, and either one committing a never-committed path would be
// a bug.
func pathspecNames(cfg Config) []string {
	return dedupUnion(structuralCommittedDirs, filterHubReserved(cfg.Dirs()))
}

// slugReservedNames returns the full slug-reservation set for cfg: both structural directory sets
// unioned with cfg.Dirs() taken raw (unfiltered by filterHubReserved) and hubSlugReservedNames().
// cfg.Dirs() is taken raw, not routing-filtered, because a worktree slug must be refused for every
// one of these names regardless of whether that name would actually wire a junction.
func slugReservedNames(cfg Config) []string {
	return dedupUnion(structuralCommittedDirs, structuralNeverCommittedDirs, cfg.Dirs(), hubSlugReservedNames())
}

// PathspecNames loads the fabric config at baseDir and returns its pathspec/commit-routing set (see
// pathspecNames) for callers outside this package — internal/fabriccli's weft sync pre-run in
// particular, which must never fall back to a raw, unfiltered cfg.Dirs() once `_lyx` leaves
// template.yaml's default.
func PathspecNames(baseDir string) ([]string, error) {
	cfg, err := LoadConfig(baseDir)
	if err != nil {
		return nil, err
	}
	return pathspecNames(cfg), nil
}

// BoardDirName is the name of the board data directory inside the hub (i.e. <hub>/_board).
// It is the single exported source of this literal;
// use BoardDir(hub) to obtain the full path.
// internal/lyxcwd retains a private second declarer (boardDirName in anchor.go) purely to find the
// recorded- anchor marker;
// see that const's comment for why the duplication is sanctioned rather than a leak.
const BoardDirName = "_board"

// HubSuffix is the suffix appended to a repo name to form the hub container directory (e.g.
// "loomyard" → "loomyard-HUB").
// Use HubPath(parent, name) to obtain the full path.
// internal/lyxcwd keeps its own private copy because Location.RepoName derives from it.
const HubSuffix = "-HUB"

// BoardDir returns the absolute path to the board data directory inside hub.
func BoardDir(hub string) string {
	return filepath.Join(hub, BoardDirName)
}

// HubPath returns the absolute path to the hub container directory for the given repo name inside
// parent.
func HubPath(parent, name string) string {
	return filepath.Join(parent, name+HubSuffix)
}

// HubReservedNames returns the junction-wiring block set: the hub-structural names that must never
// wire a per-worktree junction, _raddle, _board, _portals, _launchers.
// It is consumed by filterHubReserved and by scanOnDiskJunctionNames, and its exact current value
// and role are unchanged by this batch's structural-directory work — neither call site's behaviour
// changes.
// `.lyx` is deliberately NOT a member: adding it here would make filterHubReserved delete `.lyx` from
// the wired names so the per-worktree junction is never created, and would make
// scanOnDiskJunctionNames skip it so Unwire's sweep and applyStaleRemoval could never see it — wired
// forever, never torn down.
// It deliberately excludes lyxdirs.LyxDirName and pattern.DirName, which are config-migrated
// junction names folded into the reserved set by IsReservedHubName's junctionNames parameter
// instead.
func HubReservedNames() []string {
	return []string{BoardDirName, portalsDirName, launchersDirName, "_raddle"}
}

// hubSlugReservedNames returns the slug-reservation set: names a worktree slug may never claim.
// It is HubReservedNames() with lyxdirs.DotLyxDirName appended — `.lyx` is included because a
// worktree named `.lyx` would collide with the hub-level `<hub>/.lyx` batch 8 recognises.
// The returned slice is freshly allocated on every call, never a mutation of HubReservedNames()'s
// own backing array.
func hubSlugReservedNames() []string {
	base := HubReservedNames()
	names := make([]string, 0, len(base)+1)
	names = append(names, base...)
	names = append(names, lyxdirs.DotLyxDirName)
	return names
}

// IsReservedHubName reports whether name is one of the hub-level entry names a worktree slug must
// never claim: hubSlugReservedNames() (HubReservedNames() plus `.lyx`) UNION structuralCommittedDirs
// UNION structuralNeverCommittedDirs UNION the caller-supplied junctionNames (the weft-backed
// junction name-set injected from fabric config).
// Folding in both structural sets directly means Topology.Add's existing
// IsReservedHubName(slug, t.cfg.Dirs()) call site needs no change and still refuses `_lyx` and `.lyx`
// even for a config naming neither.
func IsReservedHubName(name string, junctionNames []string) bool {
	for _, reserved := range hubSlugReservedNames() {
		if name == reserved {
			return true
		}
	}
	for _, structuralName := range structuralCommittedDirs {
		if name == structuralName {
			return true
		}
	}
	for _, structuralName := range structuralNeverCommittedDirs {
		if name == structuralName {
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

// filterHubReserved drops every name in names that is also present in
// HubReservedNames(), preserving the input order of the
// remaining names. This is the wiring guard: a hub-structural name
// (_board, _portals, _launchers, _raddle) mis-added to fabric.yaml's
// pathspec must never wire a per-worktree junction that would collide with
// the hub-level path of the same name.
func filterHubReserved(names []string) []string {
	reserved := make(map[string]bool)
	for _, r := range HubReservedNames() {
		reserved[r] = true
	}

	kept := make([]string, 0, len(names))
	for _, name := range names {
		if reserved[name] {
			continue
		}
		kept = append(kept, name)
	}
	return kept
}

// junctionNames loads the fabric config at baseDir and returns its wired name-set: the pathspec
// directory names with the wiring guard applied (see filterHubReserved), unioned with
// structuralCommittedDirs so `_lyx` is wired structurally rather than by config.
// It deliberately does NOT include structuralNeverCommittedDirs — folding `.lyx` into the wired
// name-set is left to batch 8, where the content-adoption branch lands in the same commit range;
// doing it here would make the very next `lyx fabric reconcile` hard-error in every worktree that
// already holds a real `.lyx`.
// It is the in-package name-sourcing helper for sites that already hold a *lyxcwd.Location and can
// compute their own weft base: the read-only health checks (Healthy, checkJunctionHealth,
// junctionRepointedDetail) and checkout.go/reconcile.go's re-wire call sites.
//
// Returns (nil, err) on a config-load failure — deliberately no fallback
// default; see the callers of this function for how each one surfaces that
// failure (a hard error for wiring callers, a determinable reason string
// for the read-only health checks).
func junctionNames(baseDir string) ([]string, error) {
	cfg, err := LoadConfig(baseDir)
	if err != nil {
		return nil, err
	}
	return dedupUnion(structuralCommittedDirs, filterHubReserved(cfg.Dirs())), nil
}

// WiredNames loads the fabric config at baseDir and returns its wired name-set — structuralCommittedDirs
// unioned with the pathspec directory names, hub-reserved names filtered out — for callers outside
// this package.
// It is a thin wrapper over junctionNames so out-of-package callers (internal/fabriccli's clone and
// add handlers) obtain the same filtered name-set the in-package sites use, without duplicating the
// filterHubReserved guard themselves.
// `_lyx` is now always present here even for a Config naming neither structural directory;
// structuralNeverCommittedDirs (`.lyx`) is deliberately still absent — see junctionNames' doc comment
// for why that omission is this batch's boundary rather than an oversight.
//
// The raw, unfiltered Config.Dirs() is used only by Topology.Add's reserved-name union (which must
// include every pathspec name, filtered or not, in the set a new slug cannot claim) — never by
// WiredNames.
func WiredNames(baseDir string) ([]string, error) {
	return junctionNames(baseDir)
}

// repoWideFabricBase returns the single named source of the repo-wide fabric
// config base: the `weft:main` checkout at BoardDir(l.HubPath) that
// holds `_lyx/config/fabric.yaml`. It exists so every reconcile/status call
// site names the same base instead of re-deriving BoardDir(l.HubPath) inline.
func repoWideFabricBase(l *lyxcwd.Location) string {
	return BoardDir(l.HubPath)
}

// RepoWiredNames loads the repo-wide fabric config and returns its wired name-set.
// It is a Location-taking convenience for callers that want the repo-wide junction name-set without
// re-deriving the base.
// Callers use this so every worktree converges to the one repo-wide pathspec.
func RepoWiredNames(l *lyxcwd.Location) ([]string, error) {
	return WiredNames(repoWideFabricBase(l))
}
