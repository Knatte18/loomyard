// only.go reads the `//lym:only` tests of a `--gate` call.
// These are the tests the call's key holds back, and the tests a marked scope runs.

package gatecli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/logger"
)

// scopeMarked labels the scope of a key whose scope is gateslot.ScopeMarked.
const scopeMarked = "marked"

// withOnlyTests returns request with held and marked filled from the `//lym:only` tests of the module the call scans.
// That module is the -C directory of explicit targets, else tier0's dir.
// A call without --gate, a call under another scoper than go and a directory with no go.mod hold nothing back.
// Neither does a call whose words after `--` carry a filter flag.
// A marker naming a key the config does not hold refuses the call, as does a marker the scan rejects.
// A module whose packages cannot be listed holds nothing back, naming why on stderr, so the tiers report the break;
// a key of scope marked, which would then run nothing, is refused instead.
// ctx bounds the package listing.
func withOnlyTests(ctx context.Context, cfg gateslot.Config, request testRequest) (testRequest, error) {
	if request.gate == "" || cfg.Scoper != "go" {
		return request, nil
	}
	dir, err := guardScanDir(cfg, request, scope{})
	if err != nil {
		return request, err
	}
	moduleDir := resolveRunDir(request.root, dir)
	if _, err := os.Stat(filepath.Join(moduleDir, "go.mod")); err != nil {
		return request, nil
	}
	only, err := impactset.ModuleOnlyTests(ctx, moduleDir)
	if err != nil {
		if impactset.IsGuardScanError(err) || isMarkedCall(cfg, request) {
			return request, errors.New(guardScanRefusal(moduleDir, err))
		}
		logger.Warn("gate test: could not list the //lym:only tests; no test is held back", "module", moduleDir, "cause", err)
		printLine(request.stderr, fmt.Sprintf("gate test: cannot list the //lym:only tests of %s, so --gate %q holds no test back: %v", moduleDir, request.gate, err))
		return request, nil
	}
	request.marked = map[int]map[string][]string{}
	for _, packageDir := range slices.Sorted(maps.Keys(only)) {
		for _, test := range only[packageDir] {
			for _, key := range test.Keys {
				if _, held := cfg.Gates[key]; !held {
					return request, fmt.Errorf("gate test: the //lym:only marker of %s in %s names the gate key %q, which %s does not hold; way forward: name a key %s holds in the marker, then re-run the same command", test.Name, packageDir, key, keyName(gatesKey), keyName(gatesKey))
				}
			}
			if !slices.Contains(test.Keys, request.gate) {
				request.held = append(request.held, test.Name)
				continue
			}
			if request.marked[test.Tier] == nil {
				request.marked[test.Tier] = map[string][]string{}
			}
			request.marked[test.Tier][packageDir] = append(request.marked[test.Tier][packageDir], test.Name)
		}
	}
	sort.Strings(request.held)
	request.held = slices.Compact(request.held)
	// A call with a filter of its own holds no test back, in the listed runs and the guard step alike.
	if slices.ContainsFunc(request.args, isFilterFlag) {
		request.held = nil
	}
	return request, nil
}

// markedScope returns the scope of a key whose scope is gateslot.ScopeMarked: no listed tier runs a package,
// and the guard step runs the tests marked for the key, each under its file's tier.
// Explicit targets narrow those tests to the packages the targets name.
// A narrowed call names on request.stderr the tests it keeps and the targets that name no package holding one.
// It also says there that a `-run` among the words after `--` is replaced.
// The key's listed tiers limit only the marked tests of a manual tier:
// every other marked test runs whatever the key lists, so a hub never sets them to reach a higher tier.
// It refuses a marked test of a manual tier that the key neither lists nor covers through a listed tier.
// It also refuses a module with no test marked for the key, and targets that name no package holding one.
// Any scoper but go is refused, since the scan is Go's.
func markedScope(cfg gateslot.Config, request testRequest) (scope, error) {
	if cfg.Scoper != "go" {
		return scope{}, configRefusal("--gate %q has scope %s, which needs the go scoper to find the //lym:only tests, but %s is %q; set the key's scope to diff or all, or set the scoper to go", request.gate, gateslot.ScopeMarked, keyName(scoperKey), cfg.Scoper)
	}
	dir, err := guardScanDir(cfg, request, scope{})
	if err != nil {
		return scope{}, err
	}
	if len(request.marked) == 0 {
		moduleDir := resolveRunDir(request.root, dir)
		if _, err := os.Stat(filepath.Join(moduleDir, "go.mod")); err != nil {
			return scope{}, configRefusal("--gate %q has scope %s and would run no test, since %s holds no go.mod to scan for marked tests; set %s to the module that holds them",
				request.gate, gateslot.ScopeMarked, moduleDir, tierKeyName(tierName(0), tierDirLeaf))
		}
		return scope{}, fmt.Errorf("gate test: --gate %q has scope %s and would run no test: no test of the module at %s is marked //lym:only %s; way forward: mark a test of the module `//lym:only %s`, then re-run the same command",
			request.gate, gateslot.ScopeMarked, moduleDir, request.gate, request.gate)
	}
	if len(request.targets) > 0 {
		request.marked = markedInTargets(request.marked, request.targets)
		if len(request.marked) == 0 {
			return scope{}, fmt.Errorf("gate test: --gate %q has scope %s and would run no test: no package that %s names in the module at %s holds a test marked //lym:only %s; "+
				"way forward: name as targets the packages that hold such a test, or drop the targets to run all of them, then re-run the same command; "+
				"a target names a package only by its path relative to the module, such as ./internal/x or ./internal/x/..., so name the package that way; "+
				"a test not marked for the key runs through %s, which holds no test back",
				request.gate, gateslot.ScopeMarked, strings.Join(request.targets, " "), resolveRunDir(request.root, dir), request.gate, oneTestCall)
		}
	}
	// A manual tier runs only when named, such as one that spends tokens, so a marker alone never starts it.
	for _, number := range slices.Sorted(maps.Keys(request.marked)) {
		named := slices.ContainsFunc(cfg.Gates[request.gate].Tiers, func(listed int) bool {
			return listed == number || slices.Contains(cfg.Tiers[tierName(listed)].Covers, number)
		})
		if named || !cfg.Tiers[tierName(number)].Manual {
			continue
		}
		var names []string
		for _, inDir := range request.marked[number] {
			names = append(names, inDir...)
		}
		slices.Sort(names)
		return scope{}, configRefusal("--gate %q marks %s in %s, which is manual and runs only when the key lists it or a tier that covers it; list %s in %s",
			request.gate, strings.Join(names, ", "), tierName(number), tierName(number), keyName(gatesKey+"."+request.gate))
	}
	if len(request.targets) > 0 {
		printLine(request.stderr, narrowedLine(request))
	}
	return scope{form: scopeMarked, guardStep: true, guardDir: dir, guards: request.marked}, nil
}

// oneTestCall is the form of the call that runs one test whatever --gate key marks it.
const oneTestCall = "lym gate test --tier <its tier> <its package> -- -run '^<its name>$'"

// narrowedLine returns the line of a marked scope that explicit targets narrowed to request.marked.
// It names the tests kept, the targets that name no package holding one and a replaced `-run` of the caller.
func narrowedLine(request testRequest) string {
	var names, dirs []string
	for _, byDir := range request.marked {
		for dir, inDir := range byDir {
			dirs = append(dirs, dir)
			names = append(names, inDir...)
		}
	}
	slices.Sort(names)
	line := fmt.Sprintf("gate test: --gate %q has scope %s, so these targets narrow it to %s", request.gate, gateslot.ScopeMarked, strings.Join(slices.Compact(names), ", "))
	unmatched := slices.DeleteFunc(slices.Clone(request.targets), func(target string) bool {
		return slices.ContainsFunc(dirs, func(dir string) bool { return targetNamesDir(target, dir) })
	})
	if len(unmatched) > 0 {
		line += fmt.Sprintf("; no package that %s names holds a test marked //lym:only %s", strings.Join(unmatched, " "), request.gate)
	}
	if slices.ContainsFunc(request.args, func(arg string) bool { return testFlagName(arg) == "run" }) {
		line += "; the guard step's -run over those names replaces the caller's -run; way forward for any other test: " + oneTestCall
	}
	return line
}

// markedInTargets returns the tests of marked whose package directory one of targets names.
// A tier left with none is dropped.
// marked groups tests by tier, then by module-relative, slash-separated package directory.
// A target names a directory by its path relative to the module, such as `./internal/x` or `.`;
// one ending in `/...` names that directory and every directory below it.
// A target that is no relative path, such as an import path, names none.
func markedInTargets(marked map[int]map[string][]string, targets []string) map[int]map[string][]string {
	kept := map[int]map[string][]string{}
	for number, byDir := range marked {
		for dir, names := range byDir {
			if !slices.ContainsFunc(targets, func(target string) bool { return targetNamesDir(target, dir) }) {
				continue
			}
			if kept[number] == nil {
				kept[number] = map[string][]string{}
			}
			kept[number][dir] = names
		}
	}
	return kept
}

// targetNamesDir reports whether target, a package argument relative to a module, names the package directory dir.
func targetNamesDir(target, dir string) bool {
	target = filepath.ToSlash(target)
	if !strings.HasPrefix(target, ".") {
		return false
	}
	base, below := strings.CutSuffix(target, "/...")
	base = path.Clean(base)
	if !below {
		return base == dir
	}
	return base == "." || base == dir || strings.HasPrefix(dir, base+"/")
}

// withoutHeld returns guards with the held test names removed, dropping a directory or tier left with none.
// A nil guards stays nil, so a scope that has not scanned still reads as unscanned.
func withoutHeld(guards map[int]map[string][]string, held []string) map[int]map[string][]string {
	if guards == nil || len(held) == 0 {
		return guards
	}
	kept := map[int]map[string][]string{}
	for number, byDir := range guards {
		for dir, names := range byDir {
			rest := slices.DeleteFunc(slices.Clone(names), func(name string) bool { return slices.Contains(held, name) })
			if len(rest) == 0 {
				continue
			}
			if kept[number] == nil {
				kept[number] = map[string][]string{}
			}
			kept[number][dir] = rest
		}
	}
	return kept
}

// printHeldLine names on stderr the held tests a tier run skips, since the //lym:only markers do not name the call's key.
func printHeldLine(stderr io.Writer, number int, gate string, held []string) {
	printLine(stderr, fmt.Sprintf("gate test: %s skips %s, since their //lym:only markers do not name --gate %q", tierName(number), strings.Join(held, ", "), gate))
}
