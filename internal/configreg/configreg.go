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
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loggerconfig"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
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
	// Migrate, when non-nil, rewrites a present config file's bytes before reconcile compares them to the template.
	// It returns the rewritten bytes and one human-readable line per rewrite, and changes only the keys its module declares retired.
	Migrate func(existing []byte) ([]byte, []string, error)
}

// Modules returns the ordered list of all available config modules, each with its name and template
// function.
// The order is ALPHABETICAL and every caller-visible surface (help text, errors, menu numbering)
// renders it this way — a misordered entry is user-visible.
// Keep new entries in sort order.
func Modules() []Module {
	return []Module{
		{Name: "batcher", Template: batcher.ConfigTemplate, OpenMaps: batcher.ConfigOpenMaps(), Migrate: batcher.MigrateConfig},
		{Name: "board", Template: boardengine.ConfigTemplate, OpenMaps: boardengine.ConfigOpenMaps(), HubWide: true},
		{Name: "burler", Template: burlerengine.ConfigTemplate, SeedOnly: true},
		{Name: "fabric", Template: fabricengine.ConfigTemplate, HubWide: true},
		{Name: "landing", Template: landingshed.ConfigTemplate},
		{Name: "logger", Template: loggerconfig.ConfigTemplate},
		{Name: "loom", Template: loomengine.ConfigTemplate, OpenMaps: loomengine.ConfigOpenMaps()},
		{Name: "models", Template: modelspec.ConfigTemplate, SeedOnly: true},
		{Name: "orch", Template: orchengine.ConfigTemplate},
		{Name: "reed", Template: reedengine.ConfigTemplate},
		{Name: "shuttle", Template: shuttleengine.ConfigTemplate, OpenMaps: shuttleengine.ConfigOpenMaps()},
		{Name: "webster", Template: websterengine.ConfigTemplate},
	}
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
		writeField(strconv.Itoa(len(m.OpenMaps)))
		for _, openMap := range m.OpenMaps {
			writeField(openMap)
		}
		writeField(m.Template())
	}
	return hex.EncodeToString(hash.Sum(nil))
}
