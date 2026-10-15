// engineset.go defines EngineSet, the named engines a shuttle consumer holds in place of one Engine,
// and the lookup that resolves a spec's or a run record's engine name to its engine.

package shuttleengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ErrEngineNotRegistered is wrapped by the error EngineSet.Lookup returns for a name the set does not hold.
var ErrEngineNotRegistered = errors.New("shuttle: the engine is not registered")

// EngineSet is the engines a shuttle consumer can resolve by name, with the name of the default engine.
// The zero value is an empty set that resolves nothing.
// It is safe for concurrent use, since nothing mutates it after NewEngineSet.
type EngineSet struct {
	defaultName string
	engines     map[string]Engine
}

// NewEngineSet returns a set over a copy of engines whose default is defaultName.
// It does not check that defaultName is registered; Lookup reports that.
func NewEngineSet(defaultName string, engines map[string]Engine) EngineSet {
	copied := make(map[string]Engine, len(engines))
	for name, engine := range engines {
		copied[name] = engine
	}
	return EngineSet{defaultName: defaultName, engines: copied}
}

// Lookup returns the engine registered under name; an empty name resolves the default engine.
// A name the set does not hold returns an error wrapping ErrEngineNotRegistered that names it and the registered names;
// a default name that is not registered fails the same way.
func (s EngineSet) Lookup(name string) (Engine, error) {
	if name == "" {
		name = s.defaultName
	}
	engine, ok := s.engines[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q; registered engines: %s", ErrEngineNotRegistered, name, strings.Join(s.Names(), ", "))
	}
	return engine, nil
}

// engineOfRun resolves the engine rs was started with, the default engine for a record naming none.
// A record naming an engine the set does not hold reads as not resolved: the skip is logged once per episode under a key of its own in runDir, and recovered once the engine resolves again.
func (s EngineSet) engineOfRun(runDir string, rs RunState) (Engine, bool) {
	key := filepath.Join(runDir, "engine")
	engine, err := s.Lookup(rs.Engine)
	if err != nil {
		unreadableFiles.Fail(key, "shuttle: skipping a run whose engine is not registered", "runDir", runDir, "cause", err)
		return nil, false
	}
	unreadableFiles.Recover(key, "shuttle: run engine registered again", "runDir", runDir)
	return engine, true
}

// Names returns the registered engine names, sorted.
func (s EngineSet) Names() []string {
	names := make([]string, 0, len(s.engines))
	for name := range s.engines {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DefaultName returns the name of the engine an empty name resolves to, as NewEngineSet was told.
func (s EngineSet) DefaultName() string {
	return s.defaultName
}
