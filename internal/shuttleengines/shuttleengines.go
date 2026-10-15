// shuttleengines.go declares the registry of named engines and builds an EngineSet from a shuttle config.

package shuttleengines

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
)

// constructor builds one engine from the shuttle config and the Dirs of the location the engine serves.
type constructor func(cfg shuttleengine.Config, dirs configengine.Dirs) shuttleengine.Engine

// ConfigModule is one engine's config module as plain data, so the config registry can list it without importing the engine.
type ConfigModule struct {
	// Name is the module identifier and its file's base name.
	Name string
	// Template returns the module's default YAML template.
	Template func() string
	// OpenMaps are the template's dotted paths whose entries belong to the operator.
	OpenMaps []string
	// Migrate, when non-nil, rewrites a present file's bytes before reconcile compares them to the template.
	Migrate func(existing []byte) ([]byte, []string, error)
	// Spec returns the module's layered-load spec.
	Spec func() configengine.Spec
	// CheckLoad runs the module's own loader under dirs and returns its error, discarding the loaded config.
	CheckLoad func(configengine.Dirs) error
}

// entry pairs a registered engine's constructor with its config module.
type entry struct {
	constructor constructor
	config      ConfigModule
}

// registry maps each registered engine name to its entry.
var registry = map[string]entry{
	"claude": {
		constructor: func(cfg shuttleengine.Config, dirs configengine.Dirs) shuttleengine.Engine {
			return claudeengine.NewFromConfig(cfg, dirs)
		},
		config: ConfigModule{
			Name:     "claude",
			Template: claudeengine.ConfigTemplate,
			OpenMaps: claudeengine.ConfigOpenMaps(),
			Spec:     claudeengine.ConfigSpec,
			CheckLoad: func(dirs configengine.Dirs) error {
				_, err := claudeengine.LoadConfig(dirs)
				return err
			},
		},
	},
}

// New builds the set of every registered engine from cfg, with cfg.Engine as the default.
// Every engine is handed dirs, the layers of the location it serves.
// It refuses a cfg.Engine that names no registered engine.
func New(cfg shuttleengine.Config, dirs configengine.Dirs) (shuttleengine.EngineSet, error) {
	constructors := make(map[string]constructor, len(registry))
	for name, registered := range registry {
		constructors[name] = registered.constructor
	}
	return build(cfg, dirs, constructors)
}

// ConfigModules returns every registered engine's config module, sorted by name.
func ConfigModules() []ConfigModule {
	modules := make([]ConfigModule, 0, len(registry))
	for _, registered := range registry {
		modules = append(modules, registered.config)
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Name < modules[j].Name })
	return modules
}

// build constructs one engine per entry of constructors from cfg and dirs and returns them as a set whose default is cfg.Engine.
// It refuses, before returning a set, a cfg.Engine that is not one of the entries,
// and an engine that declares fork support through shuttleengine.RequirementSupporter without implementing shuttleengine.ForkAuditor.
func build(cfg shuttleengine.Config, dirs configengine.Dirs, constructors map[string]constructor) (shuttleengine.EngineSet, error) {
	engines := make(map[string]shuttleengine.Engine, len(constructors))
	names := make([]string, 0, len(constructors))
	for name, construct := range constructors {
		engines[name] = construct(cfg, dirs)
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		supporter, declares := engines[name].(shuttleengine.RequirementSupporter)
		if !declares || !supporter.Supports(shuttleengine.RequirementForks) {
			continue
		}
		if _, audits := engines[name].(shuttleengine.ForkAuditor); !audits {
			return shuttleengine.EngineSet{}, fmt.Errorf("shuttle: engine %q declares forks but cannot audit them", name)
		}
	}
	if _, ok := engines[cfg.Engine]; !ok {
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
