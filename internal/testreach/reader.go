// reader.go opens the shared and the worktree's coverage maps and answers which tests reach a function.

package testreach

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/testcover"
)

// Reader answers lookups over the coverage maps of a module, from the shared maps and the worktree's own.
// It is read-only and safe for concurrent use once opened.
type Reader struct {
	module Module
	// packages holds the winning map of each package, by module-relative path.
	packages map[string]packageInfo
	// shared and worktree are the reverse tables of the two directories; an entry counts only while its map wins.
	shared, worktree map[testcover.FuncKey][]reachEntry
}

// Reach is one test that reaches a function.
type Reach struct {
	// Package is the import path of the test's package.
	Package string
	// Test is the test's name.
	Test string
	// Own is set when the test is of the function's own package.
	Own bool
	// Funcs is how many functions the test's entry reaches, own and linked together.
	Funcs int
	// MapCommit is the commit of the map the entry came from.
	MapCommit string
}

// UnseenTest is a test of a package's map that may run a function the map does not show it reaching.
type UnseenTest struct {
	Test string `json:"test"`
	// Spawn is the spawn scan's verdict on the test.
	Spawn testcover.SpawnVerdict `json:"spawn"`
	// Outcome is why the test's run alone recorded no coverage, empty when it recorded some.
	Outcome testcover.TestOutcome `json:"outcome,omitempty"`
}

// MapSource describes one map a Reader read.
type MapSource struct {
	// Package is the module-relative slash path of the map's package.
	Package string
	Commit  string
	BuiltAt time.Time
	// Worktree is set for a map read from the worktree's own maps.
	Worktree bool
}

// Open reads the coverage maps of module from the hub's shared maps directory sharedDir.
// It also reads the worktree's own maps directory worktreeDir, which is "" where the worktree keeps none.
// An absent directory reads as one with no maps.
// For a package both hold, the worktree's map wins when it has the current format and Fresh holds for it;
// otherwise the map built later wins.
// A map that holds only a build failure, names a block or function outside its tables or does not parse is passed over.
// The reverse table of the shared maps comes from the index beside them when the index's stamp is current.
// Otherwise it is built in memory; Open never writes the index.
func Open(module Module, sharedDir, worktreeDir string) (*Reader, error) {
	shared, err := readSharedTable(module, sharedDir)
	if err != nil {
		return nil, err
	}
	r := &Reader{module: module, packages: shared.Packages, shared: shared.Reaches}
	if worktreeDir == "" {
		return r, nil
	}
	worktreeMaps, err := readMapDir(worktreeDir)
	if err != nil {
		return nil, err
	}
	table := buildTable(module, worktreeMaps, true)
	r.worktree = table.Reaches
	for pkg, m := range worktreeMaps {
		own, sharedHas := shared.Packages[pkg]
		wins := !sharedHas
		if sharedHas {
			if wins, err = worktreeMapWins(module, pkg, m, own); err != nil {
				return nil, err
			}
		}
		if wins {
			r.packages[pkg] = table.Packages[pkg]
		}
	}
	return r, nil
}

// worktreeMapWins reports whether the worktree's map m of pkg is read instead of the shared map described by shared.
func worktreeMapWins(module Module, pkg string, m testcover.Map, shared packageInfo) (bool, error) {
	fresh, err := Fresh(module, pkg, m)
	if err != nil {
		return false, err
	}
	return fresh && m.HasCurrentFormat() || !shared.BuiltAt.After(m.BuiltAt), nil
}

// readSharedTable returns the reverse table of the shared maps in dir, from a current index or else from the maps.
func readSharedTable(module Module, dir string) (mapTable, error) {
	stamp, err := directoryStamp(dir)
	if err != nil {
		return mapTable{}, err
	}
	if index, usable := readIndex(dir); usable && index.Stamp == stamp {
		table := mapTable{Packages: index.Packages, Reaches: map[testcover.FuncKey][]reachEntry{}}
		if table.Packages == nil {
			table.Packages = map[string]packageInfo{}
		}
		for _, reaches := range index.Reaches {
			table.Reaches[reaches.Func] = reaches.Tests
		}
		return table, nil
	}
	maps, err := readMapDir(dir)
	if err != nil {
		return mapTable{}, err
	}
	return buildTable(module, maps, false), nil
}

// buildTable returns the reverse table of maps, by module-relative package.
// worktree marks the packages as read from the worktree's own maps.
func buildTable(module Module, maps map[string]testcover.Map, worktree bool) mapTable {
	table := mapTable{Packages: map[string]packageInfo{}, Reaches: map[testcover.FuncKey][]reachEntry{}}
	for pkg, m := range maps {
		importPath := importPathOf(module, pkg)
		info := packageInfo{Commit: m.Commit, BuiltAt: m.BuiltAt, Worktree: worktree}
		for linked, hashes := range m.Linked {
			if info.LinkedDigests == nil {
				info.LinkedDigests = map[string]string{}
			}
			info.LinkedDigests[linked] = digestHashes(hashes)
		}
		for _, test := range m.Tests {
			if mayRunUnseen(test) {
				info.Unseen = append(info.Unseen, UnseenTest{Test: test.Name, Spawn: test.Spawn, Outcome: test.Outcome})
			}
			if !test.HoldsCoverage() {
				continue
			}
			reached := map[testcover.FuncKey]struct{}{}
			for _, i := range test.Blocks {
				reached[testcover.FuncKey{Package: importPath, File: m.Blocks[i].File, Func: m.Blocks[i].Func}] = struct{}{}
			}
			for _, i := range test.Funcs {
				reached[m.Funcs[i]] = struct{}{}
			}
			for fn := range reached {
				table.Reaches[fn] = append(table.Reaches[fn], reachEntry{MapPackage: pkg, Package: importPath, Test: test.Name, Funcs: len(reached)})
			}
		}
		slices.SortFunc(info.Unseen, func(a, b UnseenTest) int { return strings.Compare(a.Test, b.Test) })
		table.Packages[pkg] = info
	}
	return table
}

// mayRunUnseen reports whether the test of entry may run a function its map does not show it reaching.
// A spawn-marked test may run one in another process; a run alone that failed, timed out or was skipped recorded none.
func mayRunUnseen(entry testcover.MapTest) bool {
	return entry.Spawn.OutOfProcess || entry.Spawn.Unresolved || (entry.Outcome != "" && entry.Outcome != testcover.OutcomeNoCoverage)
}

// TestsReaching returns every test that reaches fn.
// A test of fn's own package reaches it through a block of the function;
// a test of another package reaches it through the functions its map lists.
// The tests of fn's own package come first, then those of other packages;
// within each group the test reaching the fewest functions comes first, then by package and name.
func (r *Reader) TestsReaching(fn testcover.FuncKey) []Reach {
	var reaches []Reach
	for _, source := range []struct {
		table    map[testcover.FuncKey][]reachEntry
		worktree bool
	}{{r.shared, false}, {r.worktree, true}} {
		for _, entry := range source.table[fn] {
			info, wins := r.packages[entry.MapPackage]
			if !wins || info.Worktree != source.worktree {
				continue
			}
			reaches = append(reaches, Reach{
				Package: entry.Package, Test: entry.Test, Own: entry.Package == fn.Package, Funcs: entry.Funcs, MapCommit: info.Commit,
			})
		}
	}
	slices.SortFunc(reaches, func(a, b Reach) int {
		return cmp.Or(
			cmp.Compare(boolRank(b.Own), boolRank(a.Own)),
			cmp.Compare(a.Funcs, b.Funcs),
			strings.Compare(a.Package, b.Package),
			strings.Compare(a.Test, b.Test),
		)
	})
	return reaches
}

// FuncsReachedBy returns the functions of among that the test named test of the package pkg reaches, in among's order.
// pkg is the module-relative slash path of the package whose map holds the test.
// A test reaches a function by the rules TestsReaching states.
// It also returns the map that holds the test and whether a map holds a measured entry of that name.
// A test whose entry reaches no function counts as held by no map.
func (r *Reader) FuncsReachedBy(pkg, test string, among []testcover.FuncKey) ([]testcover.FuncKey, MapSource, bool) {
	info, wins := r.packages[pkg]
	if !wins {
		return nil, MapSource{}, false
	}
	table := r.shared
	if info.Worktree {
		table = r.worktree
	}
	holdsTest := func(entries []reachEntry) bool {
		return slices.ContainsFunc(entries, func(entry reachEntry) bool { return entry.MapPackage == pkg && entry.Test == test })
	}
	var reached []testcover.FuncKey
	for _, fn := range among {
		if holdsTest(table[fn]) {
			reached = append(reached, fn)
		}
	}
	held := len(reached) > 0
	for _, entries := range table {
		if held {
			break
		}
		held = holdsTest(entries)
	}
	if !held {
		return nil, MapSource{}, false
	}
	return reached, MapSource{Package: pkg, Commit: info.Commit, BuiltAt: info.BuiltAt, Worktree: info.Worktree}, true
}

// boolRank orders false before true.
func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Maps lists the map of each package the reader read, sorted by package.
func (r *Reader) Maps() []MapSource {
	sources := make([]MapSource, 0, len(r.packages))
	for pkg, info := range r.packages {
		sources = append(sources, MapSource{Package: pkg, Commit: info.Commit, BuiltAt: info.BuiltAt, Worktree: info.Worktree})
	}
	slices.SortFunc(sources, func(a, b MapSource) int { return strings.Compare(a.Package, b.Package) })
	return sources
}

// UnseenTests returns, sorted by name, the tests that may run a function unseen.
// They are tests of the map of the module-relative package pkg.
// One kind is a test the spawn scan marks, which may run module code in another process.
// The other is a test whose run alone failed, timed out or was skipped and so recorded no coverage.
func (r *Reader) UnseenTests(pkg string) []UnseenTest {
	return r.packages[pkg].Unseen
}

// UnseenLinkerTests returns the tests that may run a function unseen from the maps recording a package as linked.
// pkg is the linked package's import path.
// The result is keyed by the module-relative package of each such map, its tests sorted by name;
// a map holding no such test has no key.
func (r *Reader) UnseenLinkerTests(pkg string) map[string][]UnseenTest {
	unseen := map[string][]UnseenTest{}
	for mapPkg, info := range r.packages {
		if _, linked := info.LinkedDigests[pkg]; linked && len(info.Unseen) > 0 {
			unseen[mapPkg] = info.Unseen
		}
	}
	return unseen
}

// StaleLinkers names, sorted, the module-relative packages whose maps record the package at import path pkg as linked.
// Only packages whose recorded file hashes differ from the worktree's files of pkg count.
// A test of such a package may reach a function new to pkg without the map showing it.
func (r *Reader) StaleLinkers(pkg string) ([]string, error) {
	rel, inModule := relativePackage(r.module, pkg)
	if !inModule {
		return nil, fmt.Errorf("package %s lies outside the module %s", pkg, r.module.Path)
	}
	onDisk, err := diskPackageHashes(r.module, rel)
	if err != nil {
		return nil, err
	}
	current := digestHashes(onDisk)
	var stale []string
	for mapPkg, info := range r.packages {
		if digest, linked := info.LinkedDigests[pkg]; linked && digest != current {
			stale = append(stale, mapPkg)
		}
	}
	slices.Sort(stale)
	return stale, nil
}
