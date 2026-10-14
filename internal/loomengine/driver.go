// driver.go implements ResolveDriver, the driver segment's config-to-settings resolver: a pure
// composer that parses and resolves the driver role's model-spec, modelled on ResolveReview in
// internal/loomengine/review.go.
// Unlike ReviewSettings, DriverSettings carries no Timeout: shuttleengine.Spec.Timeout feeds
// Run.deadline, which only Wait reads, and the driver path deliberately never calls Wait, so a
// timeout key would configure nothing.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shedrun"
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

// arbiterDefaultModelSpec is the model-spec an empty arbiter key takes.
const arbiterDefaultModelSpec = "opus[high]"

// arbiterDefaultTimeout is the timeout a zero arbiter_timeout_min takes.
const arbiterDefaultTimeout = 30 * time.Minute

// ArbiterSettings is the arbiter role's resolved model settings, timeout and spawn budget, threaded onto the arbiter session's launch spec.
type ArbiterSettings struct {
	// Model is the resolved provider-side model string.
	Model string
	// Effort is the resolved "effort" parameter, empty when unset.
	Effort string
	// Version is the resolved "version" parameter, empty when unset.
	Version string
	// Timeout is how long one arbiter spawn may run.
	Timeout time.Duration
	// MaxSpawns is how many arbiter spawns a run may use; zero turns the arbiter off.
	MaxSpawns int
}

// ResolveArbiter parses and resolves the arbiter role's model-spec from cfg through reg.
// An empty cfg.Arbiter takes opus[high] and a zero cfg.ArbiterTimeoutMin takes 30 minutes;
// cfg.ArbiterMaxSpawns passes through.
// A spec that does not parse or resolve is an error naming the arbiter key.
func ResolveArbiter(cfg Config, reg modelspec.Registry) (ArbiterSettings, error) {
	specText := cfg.Arbiter
	if specText == "" {
		specText = arbiterDefaultModelSpec
	}
	spec, err := modelspec.Parse(specText)
	if err != nil {
		return ArbiterSettings{}, fmt.Errorf("loom: ResolveArbiter: arbiter role model-spec: %w", err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return ArbiterSettings{}, fmt.Errorf("loom: ResolveArbiter: arbiter role model-spec: %w", err)
	}

	return ArbiterSettings{
		Model:     resolved.Model,
		Effort:    resolved.Params["effort"],
		Version:   resolved.Params["version"],
		Timeout:   ArbiterTimeout(cfg),
		MaxSpawns: cfg.ArbiterMaxSpawns,
	}, nil
}

// ArbiterTimeout returns how long one arbiter spawn may run: arbiter_timeout_min in minutes, or 30 minutes when it is 0.
func ArbiterTimeout(cfg Config) time.Duration {
	if cfg.ArbiterTimeoutMin > 0 {
		return time.Duration(cfg.ArbiterTimeoutMin) * time.Minute
	}
	return arbiterDefaultTimeout
}

// tacklerDefaultModelSpec is the model-spec an empty tackler key takes.
const tacklerDefaultModelSpec = "haiku"

// tacklerDefaultTimeout is the timeout a zero tackler_timeout_min takes.
const tacklerDefaultTimeout = 10 * time.Minute

// TacklerSettings is the tackler role's resolved model settings, timeout and spawn budget, threaded onto the tackler session's launch spec.
type TacklerSettings struct {
	// Model is the resolved provider-side model string.
	Model string
	// Effort is the resolved "effort" parameter, empty when unset.
	Effort string
	// Version is the resolved "version" parameter, empty when unset.
	Version string
	// Timeout is how long one tackler spawn may run.
	Timeout time.Duration
	// MaxSpawns is how many tackler spawns a run may use; zero turns the tackler off.
	MaxSpawns int
}

// ResolveTackler parses and resolves the tackler role's model-spec from cfg through reg.
// An empty cfg.Tackler takes haiku and a zero cfg.TacklerTimeoutMin takes 10 minutes;
// cfg.TacklerMaxSpawns passes through.
// A spec that does not parse or resolve is an error naming the tackler key.
func ResolveTackler(cfg Config, reg modelspec.Registry) (TacklerSettings, error) {
	specText := cfg.Tackler
	if specText == "" {
		specText = tacklerDefaultModelSpec
	}
	spec, err := modelspec.Parse(specText)
	if err != nil {
		return TacklerSettings{}, fmt.Errorf("loom: ResolveTackler: tackler role model-spec: %w", err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return TacklerSettings{}, fmt.Errorf("loom: ResolveTackler: tackler role model-spec: %w", err)
	}

	timeout := tacklerDefaultTimeout
	if cfg.TacklerTimeoutMin > 0 {
		timeout = time.Duration(cfg.TacklerTimeoutMin) * time.Minute
	}
	return TacklerSettings{
		Model:     resolved.Model,
		Effort:    resolved.Params["effort"],
		Version:   resolved.Params["version"],
		Timeout:   timeout,
		MaxSpawns: cfg.TacklerMaxSpawns,
	}, nil
}

// ResolveDriverKind returns the driver a task's loom run is seeded with when nothing typed one: shedrun.DriverLLM for an empty cfg.DriverKind, and the configured value otherwise.
func ResolveDriverKind(cfg Config) string {
	if cfg.DriverKind == "" {
		return shedrun.DriverLLM
	}
	return cfg.DriverKind
}

// alignmentDefaultModelSpec is the model-spec an empty alignment key takes.
const alignmentDefaultModelSpec = "opus[high]"

// alignmentDefaultTimeoutMin is the minutes a zero alignment_timeout_min takes.
const alignmentDefaultTimeoutMin = 20

// AlignmentSettings is the alignment agent's resolved model settings and timeout, threaded onto its session's launch spec.
type AlignmentSettings struct {
	// Model is the resolved provider-side model string.
	Model string
	// Effort is the resolved "effort" parameter, empty when unset.
	Effort string
	// Version is the resolved "version" parameter, empty when unset.
	Version string
	// Timeout is how long one alignment spawn may run.
	Timeout time.Duration
}

// ResolveAlignment parses and resolves the alignment agent's model-spec from cfg through reg.
// An empty cfg.Alignment takes opus[high] and a zero cfg.AlignmentTimeoutMin takes 20 minutes.
// A spec that does not parse or resolve is an error naming the alignment key.
func ResolveAlignment(cfg Config, reg modelspec.Registry) (AlignmentSettings, error) {
	specText := cfg.Alignment
	if specText == "" {
		specText = alignmentDefaultModelSpec
	}
	spec, err := modelspec.Parse(specText)
	if err != nil {
		return AlignmentSettings{}, fmt.Errorf("loom: ResolveAlignment: alignment model-spec: %w", err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return AlignmentSettings{}, fmt.Errorf("loom: ResolveAlignment: alignment model-spec: %w", err)
	}

	timeoutMin := alignmentDefaultTimeoutMin
	if cfg.AlignmentTimeoutMin > 0 {
		timeoutMin = cfg.AlignmentTimeoutMin
	}
	return AlignmentSettings{
		Model:   resolved.Model,
		Effort:  resolved.Params["effort"],
		Version: resolved.Params["version"],
		Timeout: time.Duration(timeoutMin) * time.Minute,
	}, nil
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
