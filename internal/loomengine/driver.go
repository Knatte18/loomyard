// driver.go implements ResolveDriver, the driver segment's config-to-settings resolver: a pure
// composer that parses and resolves the driver role's model-spec, modelled on ResolveReview in
// internal/loomengine/review.go.
// Unlike ReviewSettings, DriverSettings carries no Timeout: shuttleengine.Spec.Timeout feeds
// Run.deadline, which only Wait reads, and the driver path deliberately never calls Wait, so a
// timeout key would configure nothing.

package loomengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/modelspec"
)

// LoomDriverStrandName is the loom driver session's own strand's stable identity, the role the hub addresses as "<slug>:driver".
// It is pinned because reed's add has no upsert semantics and refuses an add whose explicit role another strand already holds.
// Every add and every lookup must use this exact constant, or a re-entrant bootstrap misses the driver already running and its add is refused.
// It lives here rather than in loomcli so batten can look the driver up without importing another CLI package.
const LoomDriverStrandName = agentname.RoleDriver

// LegacyLoomDriverStrandName names a driver strand recorded before agent names existed.
// No add ever uses it.
// It exists because agentname.Matches reaches a legacy name only by equality, so a "driver" query alone never sees it,
// and a run in flight across the deploy would otherwise miss its own live driver and spawn a second one.
const LegacyLoomDriverStrandName = "loom-driver"

// IsDriverStrand reports whether a strand recorded under name is the run's driver:
// it matches the driver role through agentname.Matches, or is exactly the legacy literal.
func IsDriverStrand(name string) bool {
	return agentname.Matches(name, LoomDriverStrandName) || name == LegacyLoomDriverStrandName
}

// DriverSettings is the driver role's resolved model settings, threaded onto the loom driver session's
// launch spec.
type DriverSettings struct {
	// Model is the resolved driver role's provider-side model string, empty when cfg.Driver is
	// empty (defer to the provider default).
	Model string
	// Effort is the resolved driver role's "effort" parameter, empty when unset.
	Effort string
	// Version is the resolved driver role's "version" parameter, empty when unset.
	Version string
}

// ResolveDriver parses and resolves the driver role's model-spec from cfg, returning the
// DriverSettings the caller threads onto the driver session's launch spec.
// An empty cfg.Driver resolves to a zero DriverSettings with a nil error, since an empty value
// means "defer to the engine default" rather than a mistake to reject.
func ResolveDriver(cfg Config, reg modelspec.Registry) (DriverSettings, error) {
	if cfg.Driver == "" {
		return DriverSettings{}, nil
	}

	spec, err := modelspec.Parse(cfg.Driver)
	if err != nil {
		return DriverSettings{}, fmt.Errorf("loom: ResolveDriver: driver role model-spec: %w", err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return DriverSettings{}, fmt.Errorf("loom: ResolveDriver: driver role model-spec: %w", err)
	}

	return DriverSettings{
		Model:   resolved.Model,
		Effort:  resolved.Params["effort"],
		Version: resolved.Params["version"],
	}, nil
}
