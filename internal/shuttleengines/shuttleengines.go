// shuttleengines.go declares the registry of named engines and builds an EngineSet from a shuttle config.

package shuttleengines

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
)

// constructor builds one engine from the shuttle config.
type constructor func(cfg shuttleengine.Config) shuttleengine.Engine

// registry maps each registered engine name to its constructor.
var registry = map[string]constructor{
	"claude": func(cfg shuttleengine.Config) shuttleengine.Engine { return claudeengine.NewFromConfig(cfg) },
}

// New builds the set of every registered engine from cfg, with cfg.Engine as the default.
// It refuses a cfg.Engine that names no registered engine.
func New(cfg shuttleengine.Config) (shuttleengine.EngineSet, error) {
	return build(cfg, registry)
}

// build constructs one engine per entry of constructors from cfg and returns them as a set whose default is cfg.Engine.
// It refuses, before returning a set, a cfg.Engine that is not one of the entries.
func build(cfg shuttleengine.Config, constructors map[string]constructor) (shuttleengine.EngineSet, error) {
	engines := make(map[string]shuttleengine.Engine, len(constructors))
	for name, construct := range constructors {
		engines[name] = construct(cfg)
	}
	if _, ok := engines[cfg.Engine]; !ok {
		names := make([]string, 0, len(engines))
		for name := range engines {
			names = append(names, name)
		}
		sort.Strings(names)
		return shuttleengine.EngineSet{}, fmt.Errorf(
			"shuttle: shuttle.yaml's engine: %q names no registered engine; registered engines: %s; way forward: set engine: in shuttle.yaml to one of them",
			cfg.Engine, strings.Join(names, ", "))
	}
	return shuttleengine.NewEngineSet(cfg.Engine, engines), nil
}

// Names returns the registered engine names, sorted.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SessionNamer returns the provider reed's watchdog uses to read and repair a session's own name.
// It is the one place a provider is chosen for reed, which stays provider-blind.
func SessionNamer() reedengine.SessionNamer {
	return claudeengine.New()
}
