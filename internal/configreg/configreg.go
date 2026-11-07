// configreg.go declares the registry of config modules and their templates, which the config CLI and its callers read.
// The config modules of the registered shuttle engines come from shuttleengines.ConfigModules.
// It also marks which modules are hub-wide, so callers branch on the registry rather than on a module name,
// and it declares the cross-module key-move table, which carries a key's value from one module's file to another's.

package configreg

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"strconv"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelconfig"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/desktophook"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/envconfig"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/hibernateconfig"
	"github.com/Knatte18/loomyard/internal/hubregistry"
	"github.com/Knatte18/loomyard/internal/ideconfig"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/loggerconfig"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reportconfig"
	"github.com/Knatte18/loomyard/internal/runprofile"
	"github.com/Knatte18/loomyard/internal/samplerengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/standalonestate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// Module describes one config module: its name, default template, and the hooks that migrate, check and load its file.
type Module struct {
	// Name is the module identifier (e.g., "board", "fabric").
	Name string
	// Template returns the module's default YAML template.
	Template func() string
	// OpenMaps names the template's keys whose entries belong to the repository, as dotted paths.
	// configsync carries them whole through reconcile.
	OpenMaps []string
	// PerNameMaps names the template's keys whose entries the operator names, as dotted paths.
	// configsync keeps the names the file changed and every entry the template lacks, and never copies a template name in.
	PerNameMaps []string
	// SeedOnly marks a module whose key set is open-ended and owned by the operator (e.g. models.yaml aliases).
	// configsync materializes a seed-only module's template when its file is absent,
	// and it never prunes a present file down to the template or strips it sparse.
	// The board dir's reconcile runs Migrate on a present file, then merges each template change into it against its base;
	// it keeps every value the operator set, then runs Unsettled. A worktree's copy is never rewritten.
	SeedOnly bool
	// HubWide marks a module that describes a hub-level fact.
	// Its one file lives at configengine.ConfigFile(<BoardDir>, name)
	// and is never read or written in a worktree's _lym/config/.
	HubWide bool
	// SeedsFromPrime marks a hub-wide module whose absent hub file is seeded from the prime's worktree copy.
	// The seed is sparse.
	SeedsFromPrime bool
	// Migrate, when non-nil, rewrites a present config file's bytes before reconcile compares them to the template.
	// It returns the rewritten bytes and one line per rewrite, changing only retired keys and keys raised to a floor.
	Migrate func(existing []byte) ([]byte, []string, error)
	// Unsettled, when non-nil, lists the pairs a seed-only module's present file holds for the operator to settle.
	// It runs after the migration; reconcile reports these pairs and changes none, since only the operator can settle them.
	Unsettled func(existing []byte) ([]string, error)
	// MigrateHubBase, when non-nil, runs only over the module's hub base file:
	// over a present file after the module's own migration, and over the prime or legacy seed on an absent one.
	// It adds what only that file may gain, returns one line per addition, and leaves the registry fingerprint unchanged.
	MigrateHubBase func(existing []byte) ([]byte, []string, error)
	// CheckValues, when non-nil, checks the values of a local-layer document after its keys pass configengine.ValidateLocal.
	// It checks only the keys its module names, and returns an error marked configengine.ErrInvalid that names the key.
	CheckValues func(document []byte) error
	// CheckLoad runs the module's loader over the layer stack under dirs and returns its error, discarding the config.
	// Every module sets it, so a write is checked against what a later reader refuses.
	CheckLoad func(dirs configengine.Dirs) error
	// CheckDirs, when non-nil, loads the module under dirs and refuses a command dir the worktree at worktreeRoot lacks.
	// A module with command keys sets it, so a hub-wide write refuses a dir that would fail every run in the prime.
	CheckDirs func(dirs configengine.Dirs, worktreeRoot, anchorDir string) error
	// Reinterpretations lists the changes a binary made to what a value in the hub base file means, in order.
	// A binary that changes a meaning adds one, so reconcile carries each hub's file across it before any command runs.
	Reinterpretations []Reinterpretation
	// Spec returns the module's layered-load spec: its name, template, open maps, hub-wide flag and per-key classification.
	Spec func() configengine.Spec
	// LocalKeys are the keys of a hub-wide module that a local override may set, read from the module's spec.
	// It is nil for a per-worktree module.
	LocalKeys []string
	// LocalDenied are the dotted key paths a local override may not set in a per-worktree module.
	// It is nil for a hub-wide module.
	LocalDenied []string
}

// Reinterpretation records one change a binary made to what a value in a module's hub base file means.
// It carries the rewrite that keeps the meaning each hub gave the value.
type Reinterpretation struct {
	// ID names the change; once applied, the hub-wide reconcile records it as a board marker, so each hub applies it once.
	ID string
	// Rewrite receives the hub base file's bytes and the prime's worktree root and anchored directory.
	// It returns the bytes, rewritten only if a value changed; one line per change or unsettled value, naming a way forward.
	Rewrite func(existing []byte, worktreeRoot, anchorDir string) (rewritten []byte, rewrote, flagged []string, err error)
}

// repositoryOfShortname answers the repository name of the hub that shortname names in this machine's record of hubs.
func repositoryOfShortname(shortname string) (string, error) {
	registryDir, err := standalonestate.UserStateDir()
	if err != nil {
		return "", err
	}
	return hubregistry.LookupRepository(registryDir, shortname)
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

// Modules returns every config module in alphabetical order, each with its name and template function.
// Help text, errors and menu numbering follow this order, so a misordered entry is user-visible.
// Keep new entries in sort order.
func Modules() []Module {
	modules := []Module{
		{Name: agentconfig.ModuleName, Template: agentconfig.ConfigTemplate, OpenMaps: agentconfig.ConfigOpenMaps(), HubWide: true, CheckLoad: checkLoad(agentconfig.Load), Spec: agentconfig.ConfigSpec},
		{Name: "batcher", Template: batcher.ConfigTemplate, PerNameMaps: batcher.ConfigPerNameMaps(), Migrate: batcher.MigrateConfig, CheckLoad: checkLoad(batcher.Active), Spec: batcher.ConfigSpec},
		{Name: "batten", Template: battenshed.ConfigTemplate, CheckValues: battenshed.ValidateLocalValues, CheckLoad: checkLoad(battenshed.LoadConfig), Spec: battenshed.ConfigSpec},
		{Name: "beam", Template: dobbyengine.ConfigTemplate, CheckLoad: checkLoad(dobbyengine.LoadConfig), Spec: dobbyengine.ConfigSpec},
		{Name: "board", Template: boardengine.ConfigTemplate, OpenMaps: boardengine.ConfigOpenMaps(), HubWide: true, CheckLoad: checkLoadNamed(boardengine.LoadConfig, "board"), Spec: boardengine.ConfigSpec},
		{Name: "creel", Template: creelconfig.ConfigTemplate, HubWide: true, CheckLoad: checkLoad(creelconfig.LoadConfig), Reinterpretations: []Reinterpretation{{ID: creelconfig.TrustedHubsRepositoryReinterpretation, Rewrite: creelconfig.RewriteShortnamesToRepositories(repositoryOfShortname)}}, Spec: creelconfig.ConfigSpec},
		{Name: "darn", Template: darnengine.ConfigTemplate, OpenMaps: darnengine.ConfigOpenMaps(), HubWide: true, Migrate: darnengine.MigrateConfig, CheckLoad: checkLoadNamed(darnengine.LoadConfig, "darn"), Spec: darnengine.ConfigSpec},
		{Name: "desktop", Template: desktophook.ConfigTemplate, HubWide: true, CheckLoad: checkLoad(desktophook.LoadConfig), Spec: desktophook.ConfigSpec},
		{Name: "drift", Template: driftengine.ConfigTemplate, OpenMaps: driftengine.ConfigOpenMaps(), HubWide: true, Migrate: driftengine.MigrateConfig, CheckLoad: checkLoad(driftengine.LoadConfig), CheckDirs: driftengine.CheckDirs, Reinterpretations: []Reinterpretation{{ID: driftengine.DirBaseReinterpretation, Rewrite: driftengine.RebaseDirsOntoRoot}, {ID: driftengine.DiffModeReinterpretation, Rewrite: driftengine.FlagDiffMode}}, Spec: driftengine.ConfigSpec},
		{Name: "env", Template: envconfig.ConfigTemplate, PerNameMaps: envconfig.ConfigPerNameMaps(), HubWide: true, CheckLoad: checkLoad(envconfig.LoadConfig), Spec: envconfig.ConfigSpec},
		{Name: "fabric", Template: fabricengine.ConfigTemplate, HubWide: true, CheckLoad: checkLoad(fabricengine.LoadConfig), Reinterpretations: []Reinterpretation{{ID: heddleengine.SettingsExcludeReinterpretation, Rewrite: fabricengine.AppendExcludePaths(heddleengine.ClaudeSettingsFile)}}, Spec: fabricengine.ConfigSpec},
		{Name: "gate", Template: gateslot.ConfigTemplate, OpenMaps: gateslot.ConfigOpenMaps(), PerNameMaps: gateslot.ConfigPerNameMaps(), HubWide: true, Migrate: gateslot.MigrateConfig, MigrateHubBase: gateslot.SeedAdoptGate, CheckLoad: checkLoad(gateslot.LoadConfig), CheckDirs: gateslot.CheckDirs, Spec: gateslot.ConfigSpec},
		{Name: "heddle", Template: heddleengine.ConfigTemplate, HubWide: true, CheckLoad: checkLoad(heddleengine.LoadConfig), Spec: heddleengine.ConfigSpec},
		{Name: "hibernate", Template: hibernateconfig.ConfigTemplate, HubWide: true, CheckLoad: checkLoad(hibernateconfig.LoadConfig), Spec: hibernateconfig.ConfigSpec},
		{Name: "ide", Template: ideconfig.ConfigTemplate, HubWide: true, CheckLoad: checkLoad(ideconfig.LoadConfig), Spec: ideconfig.ConfigSpec},
		{Name: "landing", Template: landingshed.ConfigTemplate, OpenMaps: landingshed.ConfigOpenMaps(), HubWide: true, SeedsFromPrime: true, Migrate: landingshed.MigrateConfig, CheckLoad: checkLoadNamed(landingshed.LoadConfig, "landing"), Spec: landingshed.ConfigSpec},
		{Name: "language", Template: languageconfig.ConfigTemplate, HubWide: true, CheckLoad: checkLoad(languageconfig.Load), Spec: languageconfig.ConfigSpec},
		{Name: "lenses", Template: lenses.ConfigTemplate, PerNameMaps: lenses.ConfigPerNameMaps(), CheckLoad: lenses.CheckLoad, Spec: lenses.ConfigSpec},
		{Name: "logger", Template: loggerconfig.ConfigTemplate, CheckLoad: checkLoad(loggerconfig.Load), Spec: loggerconfig.ConfigSpec},
		{Name: "loom", Template: loomengine.ConfigTemplate, OpenMaps: loomengine.ConfigOpenMaps(), Migrate: loomengine.MigrateConfig, CheckValues: loomengine.ValidateLocalValues, CheckLoad: checkLoadNamed(loomengine.LoadConfig, "loom"), Spec: loomengine.ConfigSpec},
		{Name: "models", Template: modelspec.ConfigTemplate, SeedOnly: true, CheckLoad: checkLoad(modelspec.LoadRegistry), Spec: modelspec.ConfigSpec},
		{Name: "orch", Template: orchengine.ConfigTemplate, Migrate: orchengine.MigrateConfig, CheckLoad: checkLoadNamed(orchengine.LoadConfig, "orch"), Spec: orchengine.ConfigSpec},
		{Name: runprofile.ModuleName, Template: runprofile.ConfigTemplate, HubWide: true, SeedOnly: true, Migrate: runprofile.MigrateConfig, Unsettled: runprofile.UnsettledFormerBlocks, CheckLoad: checkProfiles, Spec: runprofile.ConfigSpec},
		{Name: "reed", Template: reedengine.ConfigTemplate, OpenMaps: reedengine.ConfigOpenMaps(), CheckLoad: checkLoadNamed(reedengine.LoadConfig, "reed"), Spec: reedengine.ConfigSpec},
		{Name: "report", Template: reportconfig.ConfigTemplate, HubWide: true, CheckLoad: checkLoad(reportconfig.LoadConfig), Spec: reportconfig.ConfigSpec},
		{Name: "sampler", Template: samplerengine.ConfigTemplate, HubWide: true, CheckLoad: checkLoadNamed(samplerengine.LoadConfig, "sampler"), Spec: samplerengine.ConfigSpec},
		{Name: "shed", Template: sheddriver.ConfigTemplate, CheckValues: sheddriver.ValidateLocalValues, CheckLoad: checkLoad(sheddriver.LoadConfig), Spec: sheddriver.ConfigSpec},
		{Name: "shuttle", Template: shuttleengine.ConfigTemplate, Migrate: shuttleengine.MigrateConfig, CheckLoad: checkLoadNamed(shuttleengine.LoadConfig, "shuttle"), Spec: shuttleengine.ConfigSpec},
		{Name: "slack", Template: slackengine.ConfigTemplate, HubWide: true, CheckLoad: checkLoad(slackengine.LoadConfig), Spec: slackengine.ConfigSpec},
		{Name: "tapestry", Template: loomengine.TapestryConfigTemplate, OpenMaps: loomengine.TapestryConfigOpenMaps(), CheckValues: loomengine.ValidateTapestryLocalValues, CheckLoad: checkLoadNamed(loomengine.LoadTapestryConfig, "tapestry"), Spec: loomengine.TapestryConfigSpec},
		{Name: "webster", Template: websterengine.ConfigTemplate, CheckLoad: checkLoadNamed(websterengine.LoadConfig, "webster"), Spec: websterengine.ConfigSpec},
	}
	for _, engine := range shuttleengines.ConfigModules() {
		modules = append(modules, Module{Name: engine.Name, Template: engine.Template, OpenMaps: engine.OpenMaps, Migrate: engine.Migrate, CheckLoad: engine.CheckLoad, Spec: engine.Spec})
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Name < modules[j].Name })
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

// Fingerprint returns a hex digest of the registry.
// It changes whenever a registered module's name, flags, open maps, per-name maps, template text, or overlay changes.
// A change that only swaps a module's Migrate hook leaves it unchanged, since a function value has no stable bytes.
func Fingerprint() string {
	return fingerprintOf(Modules())
}

// fingerprintOf returns the lowercase hex SHA-256 of modules in their given order, with length-prefixed fields.
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
		writeField(strconv.Itoa(len(m.PerNameMaps)))
		for _, perNameMap := range m.PerNameMaps {
			writeField(perNameMap)
		}
		writeField(m.Template())
		if m.Spec == nil {
			continue
		}
		// A layer's prose is part of what a hub reconciles against, so a binary changing only a layer changes the stamp.
		overlay := m.Spec().Overlay
		if overlay.Selector == "" && len(overlay.Documents) == 0 {
			continue
		}
		writeField(overlay.Selector)
		names := make([]string, 0, len(overlay.Documents))
		for name := range overlay.Documents {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			writeField(name)
			writeField(string(overlay.Documents[name]))
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}
