// driver.go implements ResolveDriver, the driver segment's config-to-settings resolver: a pure
// composer that parses and resolves the driver role's model-spec, modelled on ResolveReview in
// internal/loomengine/review.go.
// Unlike ReviewSettings, DriverSettings carries no Timeout: shuttleengine.Spec.Timeout feeds
// Run.deadline, which only Wait reads, and the driver path deliberately never calls Wait, so a
// timeout key would configure nothing.

package loomengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/modelspec"
)

// LoomDriverStrandName is the ly-drive session's own strand's stable identity.
// It is pinned because reed's add has no upsert semantics, so a second add under this same display name would append a second pane rather than replace the first.
// Every add and every lookup must use this exact constant, or a re-entrant bootstrap stacks a second driver pane instead of matching the one already running.
// It lives here rather than in loomcli so batten can look the driver up without importing another CLI package.
const LoomDriverStrandName = "loom-driver"

// DriverSettings is the driver role's resolved model settings, threaded onto the ly-drive session's
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
