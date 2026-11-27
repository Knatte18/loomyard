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
	// The caller's own filter decides what runs, in the listed runs and the guard step alike.
	if slices.ContainsFunc(request.args, isFilterFlag) {
		request.held = nil
	}
	return request, nil
}

// markedScope returns the scope of a key whose scope is gateslot.ScopeMarked: no listed tier runs a package,
// and the guard step runs the tests marked for the key, each under its file's tier.
// The key's listed tiers limit only the marked tests of a manual tier:
// every other marked test runs whatever the key lists, so a hub never sets them to reach a higher tier.
// It refuses a marked test of a manual tier that the key neither lists nor covers through a listed tier,
// and a module with no test marked for the key.
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
	return scope{form: scopeMarked, guardStep: true, guardDir: dir, guards: request.marked}, nil
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
