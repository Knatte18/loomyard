// junctionnames.go implements the fabric-config-to-junction-name-set bridge:
// loading a pair's fabric.yaml pathspec and turning it into the wired
// name-set the junction primitives operate on, with the hub-reserved-name
// wiring guard applied. Every wiring caller sources names through
// junctionNames/WiredNames — never applies filterHubReserved itself — so the
// guard cannot be forgotten at a call site.

package fabricengine

import "github.com/Knatte18/loomyard/internal/hubgeometry"

// filterHubReserved drops every name in names that is also present in
// hubgeometry.HubReservedNames(), preserving the input order of the
// remaining names. This is the wiring guard: a hub-structural name
// (_board, _portals, _launchers, _raddle) mis-added to fabric.yaml's
// pathspec must never wire a per-worktree junction that would collide with
// the hub-level path of the same name.
func filterHubReserved(names []string) []string {
	reserved := make(map[string]bool)
	for _, r := range hubgeometry.HubReservedNames() {
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

// junctionNames loads the fabric config at baseDir and returns its
// pathspec's directory names with the wiring guard applied (see
// filterHubReserved). It is the in-package name-sourcing helper for sites
// that already hold a *hubgeometry.Layout and can compute their own weft
// base: the read-only health checks (PairInSync, checkJunctionHealth,
// junctionRepointedDetail) and checkout.go/reconcile.go's re-wire call
// sites.
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
	return filterHubReserved(cfg.Dirs()), nil
}

// WiredNames loads the fabric config at baseDir and returns its wired
// name-set — the pathspec directory names with hub-reserved names filtered
// out — for callers outside this package. It is a thin wrapper over
// junctionNames so out-of-package callers (internal/initengine's init and
// undo) obtain the same filtered name-set the in-package sites use, without
// duplicating the filterHubReserved guard themselves.
//
// The raw, unfiltered Config.Dirs() is used only by Topology.Add's
// reserved-name union (which must include every pathspec name, filtered or
// not, in the set a new slug cannot claim) — never by WiredNames.
func WiredNames(baseDir string) ([]string, error) {
	return junctionNames(baseDir)
}
