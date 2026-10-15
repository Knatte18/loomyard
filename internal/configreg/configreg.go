// configreg.go — module registry for configuration management.
//
// Provides a neutral registry of available config modules and their templates,
// used by the config CLI command and callers such as fabric clone.
// Its second role is marking which modules are hub-wide, so every caller branches on the registry rather than on a module name.

package configreg

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strconv"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/loggerconfig"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// Module represents a single config module with its name and template function.
type Module struct {
	// Name is the module identifier (e.g., "board", "fabric").
	Name string
	// Template is a function that returns the default YAML template for this module.
	Template func() string
	// OpenMaps names the template's keys whose entries belong to the repository, as dotted paths.
	// configsync carries them whole through reconcile.
	OpenMaps []string
	// SeedOnly marks a module whose key set is open-ended and owned by the
	// operator (e.g. models.yaml aliases, burler.yaml lenses/fans).
	// configsync materializes a seed-only module's template when its file
	// is absent, and never rewrites a present file — it neither adds nor
	// prunes keys, unlike the default reconcile behavior applied to every
	// other module.
	SeedOnly bool
	// HubWide marks a module that describes a hub-level fact.
	// Its one file lives at configengine.ConfigFile(<BoardDir>, name)
	// and is never read or written in a worktree's _lyx/config/.
	HubWide bool
	// SeedsFromPrime marks a hub-wide module whose per-worktree copy was in effect before it moved hub-wide.
	// A hub's first reconcile seeds an absent hub file from the prime's copy, like a module with open maps.
	SeedsFromPrime bool
	// Migrate, when non-nil, rewrites a present config file's bytes before reconcile compares them to the template.
	// It returns the rewritten bytes and one human-readable line per rewrite, and changes only the keys its module declares retired, rewrites a key's retired meaning into its replacement, or raises a key to its module's floor.
	Migrate func(existing []byte) ([]byte, []string, error)
	// CheckValues, when non-nil, checks the values of a local-layer document after its keys pass configengine.ValidateLocal.
	// It checks only the keys its module names, and returns an error marked configengine.ErrInvalid that names the key.
	CheckValues func(document []byte) error
	// CheckLoad runs the module's own loader over the layer stack under dirs and returns its error, discarding the loaded config.
	// Every module sets it, so a write can be checked against exactly what a later reader refuses.
	CheckLoad func(dirs configengine.Dirs) error
	// Spec returns the module's own layered-load spec: its name, template, open maps, hub-wide flag and per-key classification.
	Spec func() configengine.Spec
	// LocalKeys are the keys of a hub-wide module a local override may set, derived from Spec.
	// It is nil for a per-worktree module.
	LocalKeys []string
	// LocalDenied are the dotted key paths of a per-worktree module a local override may not set, derived from Spec.
	// It is nil for a hub-wide module.
	LocalDenied []string
}

// withLocalBounds fills the module's LocalKeys or LocalDenied from its Spec, so the registry never restates a key.
func withLocalBounds(m Module) Module {
	spec := m.Spec()
	if m.HubWide {
		m.LocalKeys = configengine.LocalKeys(spec)
		return m
	}
	m.LocalDenied = configengine.LocalDenied(spec)
	return m
}

// checkLoad adapts a loader to Module.CheckLoad.
func checkLoad[T any](load func(configengine.Dirs) (T, error)) func(configengine.Dirs) error {
	return func(dirs configengine.Dirs) error {
		_, err := load(dirs)
		return err
	}
}

// checkLoadNamed adapts a loader that takes its module name to Module.CheckLoad.
func checkLoadNamed[T any](load func(configengine.Dirs, string) (T, error), module string) func(configengine.Dirs) error {
	return func(dirs configengine.Dirs) error {
		_, err := load(dirs, module)
		return err
	}
}

// Modules returns the ordered list of all available config modules, each with its name and template
// function.
// The order is ALPHABETICAL and every caller-visible surface (help text, errors, menu numbering)
// renders it this way — a misordered entry is user-visible.
// Keep new entries in sort order.
func Modules() []Module {
	modules := []Module{
		{Name: "batcher", Template: batcher.ConfigTemplate, OpenMaps: batcher.ConfigOpenMaps(), Migrate: batcher.MigrateConfig, CheckLoad: checkLoad(batcher.Active), Spec: batcher.ConfigSpec},
		{Name: "batten", Template: battenshed.ConfigTemplate, CheckValues: battenshed.ValidateLocalValues, CheckLoad: checkLoad(battenshed.LoadConfig), Spec: battenshed.ConfigSpec},
		{Name: "board", Template: boardengine.ConfigTemplate, OpenMaps: boardengine.ConfigOpenMaps(), HubWide: true, CheckLoad: checkLoadNamed(boardengine.LoadConfig, "board"), Spec: boardengine.ConfigSpec},
		{Name: "burler", Template: burlerengine.ConfigTemplate, SeedOnly: true, CheckLoad: checkLoad(burlerengine.LoadConfig), Spec: burlerengine.ConfigSpec},
		{Name: "darn", Template: darnengine.ConfigTemplate, HubWide: true, CheckValues: darnengine.ValidateLocalValues, CheckLoad: checkLoadNamed(darnengine.LoadConfig, "darn"), Spec: darnengine.ConfigSpec},
		{Name: "fabric", Template: fabricengine.ConfigTemplate, HubWide: true, CheckLoad: checkLoad(fabricengine.LoadConfig), Spec: fabricengine.ConfigSpec},
		{Name: "gate", Template: gateslot.ConfigTemplate, HubWide: true, CheckLoad: checkLoad(gateslot.LoadConfig), Spec: gateslot.ConfigSpec},
		{Name: "landing", Template: landingshed.ConfigTemplate, HubWide: true, SeedsFromPrime: true, CheckLoad: checkLoadNamed(landingshed.LoadConfig, "landing"), Spec: landingshed.ConfigSpec},
		{Name: "language", Template: languageconfig.ConfigTemplate, HubWide: true, CheckLoad: checkLoad(languageconfig.Load), Spec: languageconfig.ConfigSpec},
		{Name: "logger", Template: loggerconfig.ConfigTemplate, CheckLoad: checkLoad(loggerconfig.Load), Spec: loggerconfig.ConfigSpec},
		{Name: "loom", Template: loomengine.ConfigTemplate, OpenMaps: loomengine.ConfigOpenMaps(), Migrate: loomengine.MigrateConfig, CheckValues: loomengine.ValidateLocalValues, CheckLoad: checkLoadNamed(loomengine.LoadConfig, "loom"), Spec: loomengine.ConfigSpec},
		{Name: "models", Template: modelspec.ConfigTemplate, SeedOnly: true, CheckLoad: checkLoad(modelspec.LoadRegistry), Spec: modelspec.ConfigSpec},
		{Name: "orch", Template: orchengine.ConfigTemplate, Migrate: orchengine.MigrateConfig, CheckLoad: checkLoadNamed(orchengine.LoadConfig, "orch"), Spec: orchengine.ConfigSpec},
		{Name: "reed", Template: reedengine.ConfigTemplate, CheckLoad: checkLoadNamed(reedengine.LoadConfig, "reed"), Spec: reedengine.ConfigSpec},
		{Name: "shed", Template: sheddriver.ConfigTemplate, CheckValues: sheddriver.ValidateLocalValues, CheckLoad: checkLoad(sheddriver.LoadConfig), Spec: sheddriver.ConfigSpec},
		{Name: "shuttle", Template: shuttleengine.ConfigTemplate, OpenMaps: shuttleengine.ConfigOpenMaps(), Migrate: shuttleengine.MigrateConfig, CheckLoad: checkLoadNamed(shuttleengine.LoadConfig, "shuttle"), Spec: shuttleengine.ConfigSpec},
		{Name: "webster", Template: websterengine.ConfigTemplate, CheckLoad: checkLoadNamed(websterengine.LoadConfig, "webster"), Spec: websterengine.ConfigSpec},
	}
	for i := range modules {
		modules[i] = withLocalBounds(modules[i])
	}
	return modules
}

// Template returns the template function for the named module.
// It returns (nil, false) if the module name is unknown.
func Template(name string) (func() string, bool) {
	for _, m := range Modules() {
		if m.Name == name {
			return m.Template, true
		}
	}
	return nil, false
}

// Lookup returns the registered module for the name, with its template and open maps.
// It returns (Module{}, false) if the module name is unknown.
func Lookup(name string) (Module, bool) {
	for _, m := range Modules() {
		if m.Name == name {
			return m, true
		}
	}
	return Module{}, false
}

// Names returns the ordered list of all available config module names.
func Names() []string {
	mods := Modules()
	names := make([]string, len(mods))
	for i, m := range mods {
		names[i] = m.Name
	}
	return names
}

// Fingerprint returns a hex digest of the registry that changes whenever a registered module's name, flags, open maps or template text changes.
// A change that only swaps a module's Migrate hook leaves it unchanged, since a function value has no stable bytes.
func Fingerprint() string {
	return fingerprintOf(Modules())
}

// fingerprintOf hashes a canonical, length-prefixed encoding of modules in their given order and returns the lowercase hex SHA-256.
func fingerprintOf(modules []Module) string {
	hash := sha256.New()
	writeField := func(field string) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(field)))
		hash.Write(length[:])
		hash.Write([]byte(field))
	}
	writeFlag := func(flag bool) {
		if flag {
			writeField("1")
			return
		}
		writeField("0")
	}
	for _, m := range modules {
		writeField(m.Name)
		writeFlag(m.HubWide)
		writeFlag(m.SeedOnly)
		writeFlag(m.SeedsFromPrime)
		writeField(strconv.Itoa(len(m.OpenMaps)))
		for _, openMap := range m.OpenMaps {
			writeField(openMap)
		}
		writeField(m.Template())
	}
	return hex.EncodeToString(hash.Sum(nil))
}
