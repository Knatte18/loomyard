// gopackages.go implements the harness's primary measurement arm: an
// in-process reference finder built directly on golang.org/x/tools/go/packages
// and go/types. It owns loadPackages (the shared package-load step every
// in-process mode reuses) and resolveSymbol (the shared symbol-spec resolver),
// plus the "refs" mode's own findReferences and reporting.

package main

import (
	"encoding/json"
	"fmt"
	"go/token"
	"go/types"
	"sort"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
)

// packagesLoadMode is the set of packages.NeedX bits the harness requires:
// names and files for reporting, syntax and full type info to resolve
// symbols and walk identifier uses, deps/imports so a symbol defined in one
// package is still visible when referenced from another, and module info so
// resolveSymbol can match a spec's import path against the loaded packages.
const packagesLoadMode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports | packages.NeedModule

// loadPackages loads every package under dir (module root) with full syntax
// and type information, returning the loaded packages plus how long the load
// took. Every mode that resolves symbols or inspects syntax reuses this
// function rather than calling packages.Load directly, so the harness has one
// place that pins the Mode bits and one warm-up timing measurement per run.
func loadPackages(dir string) ([]*packages.Package, time.Duration, error) {
	cfg := &packages.Config{
		Dir:   dir,
		Tests: false,
		Mode:  packagesLoadMode,
	}

	start := time.Now()
	pkgs, err := packages.Load(cfg, "./...")
	elapsed := time.Since(start)
	if err != nil {
		return nil, elapsed, fmt.Errorf("load packages: %w", err)
	}

	// packages.Load can return a partial, non-error result for packages with
	// compile errors (e.g. a broken import); PrintErrors surfaces those to
	// stderr and reports whether any were found so the caller can fail loudly
	// rather than silently querying an incomplete type-checked graph.
	if packages.PrintErrors(pkgs) > 0 {
		return nil, elapsed, fmt.Errorf("load packages: one or more packages failed to type-check")
	}

	return pkgs, elapsed, nil
}

// resolveSymbol finds the types.Object a symbol spec identifies among the
// loaded packages. A spec is either "<import-path>.<Name>" (a package-level
// func, type, or var) or "<import-path>.<Type>.<Method>" (a method on a named
// type), per the spec format documented in main.go's usage text.
func resolveSymbol(pkgs []*packages.Package, spec string) (types.Object, error) {
	importPath, rest, err := splitSpec(spec)
	if err != nil {
		return nil, err
	}

	pkg := findPackage(pkgs, importPath)
	if pkg == nil {
		return nil, fmt.Errorf("resolve symbol %q: package %q not found among loaded packages", spec, importPath)
	}

	parts := strings.Split(rest, ".")
	switch len(parts) {
	case 1:
		// <import-path>.<Name>: a package-level func, type, or var, found
		// directly in the package scope.
		obj := pkg.Types.Scope().Lookup(parts[0])
		if obj == nil {
			return nil, fmt.Errorf("resolve symbol %q: %q not found in package %q", spec, parts[0], importPath)
		}
		return obj, nil

	case 2:
		// <import-path>.<Type>.<Method>: look up the named type first, then
		// its method set (value receiver set covers both value and pointer
		// receiver methods for reference-finding purposes since Uses records
		// the concrete resolved method object either way).
		typeName, methodName := parts[0], parts[1]
		typeObj := pkg.Types.Scope().Lookup(typeName)
		if typeObj == nil {
			return nil, fmt.Errorf("resolve symbol %q: type %q not found in package %q", spec, typeName, importPath)
		}
		named, ok := typeObj.Type().(*types.Named)
		if !ok {
			return nil, fmt.Errorf("resolve symbol %q: %q is not a named type", spec, typeName)
		}

		// LookupFieldOrMethod on the pointer type surfaces both pointer- and
		// value-receiver methods; NewMethodSet on the pointer type is the
		// superset method set for the same reason.
		obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(named), true, pkg.Types, methodName)
		if obj == nil {
			return nil, fmt.Errorf("resolve symbol %q: method %q not found on type %q", spec, methodName, typeName)
		}
		return obj, nil

	default:
		return nil, fmt.Errorf("resolve symbol %q: expected <Name> or <Type>.<Method> after the import path, got %q", spec, rest)
	}
}

// splitSpec splits a symbol spec into its import path and the trailing
// dotted name (Name, or Type.Method). It splits on the last "/"-delimited
// path segment's own final "." boundary set: since Go import paths never
// contain the symbol name, the split point is the first "." that occurs
// after the last "/" in spec.
func splitSpec(spec string) (importPath, rest string, err error) {
	slash := strings.LastIndex(spec, "/")
	dot := strings.Index(spec[slash+1:], ".")
	if dot < 0 {
		return "", "", fmt.Errorf("invalid symbol spec %q: expected <import-path>.<Name> or <import-path>.<Type>.<Method>", spec)
	}
	dot += slash + 1
	return spec[:dot], spec[dot+1:], nil
}

// findPackage returns the loaded package whose PkgPath matches importPath, or
// nil if no loaded package matches. pkgs includes every package reachable
// from the module root's "./..." pattern (per loadPackages), so this is a
// linear scan over that set rather than a second load.
func findPackage(pkgs []*packages.Package, importPath string) *packages.Package {
	for _, pkg := range pkgs {
		if pkg.PkgPath == importPath {
			return pkg
		}
	}
	return nil
}

// findReferences scans every loaded package's TypesInfo.Uses (identifier
// references) and TypesInfo.Defs (the definition site itself) for
// identifiers whose resolved object is obj, converting each match's position
// to a token.Position via the owning package's Fset. Positions are returned
// sorted by file:line:col for stable, diffable output across runs.
func findReferences(pkgs []*packages.Package, obj types.Object) []token.Position {
	var positions []token.Position
	for _, pkg := range pkgs {
		for ident, use := range pkg.TypesInfo.Uses {
			if use == obj {
				positions = append(positions, pkg.Fset.Position(ident.Pos()))
			}
		}
		for ident, def := range pkg.TypesInfo.Defs {
			if def == obj {
				positions = append(positions, pkg.Fset.Position(ident.Pos()))
			}
		}
	}
	sort.Slice(positions, func(i, j int) bool {
		return positions[i].String() < positions[j].String()
	})
	return positions
}

// refsReport is the JSON shape emitted by -json for the refs mode: warm-up
// and steady-state timings alongside the reference count and positions.
type refsReport struct {
	WarmUpMS       float64   `json:"warm_up_ms"`
	SteadyStateMS  []float64 `json:"steady_state_ms"`
	MinMS          float64   `json:"min_ms"`
	MedianMS       float64   `json:"median_ms"`
	ReferenceCount int       `json:"reference_count"`
	Positions      []string  `json:"positions"`
}

// runRefs implements the "refs" mode: load the target module once (recording
// the warm-up load duration), then run findReferences cfg.n times against the
// already-loaded packages to measure steady-state (subsequent same-process
// query) latency separate from the one-time load cost.
func runRefs(cfg config) error {
	pkgs, warmUp, err := loadPackages(cfg.dir)
	if err != nil {
		return err
	}

	obj, err := resolveSymbol(pkgs, cfg.symbol)
	if err != nil {
		return err
	}

	repeats := cfg.n
	if repeats < 1 {
		repeats = 1
	}

	var positions []token.Position
	durations := make([]time.Duration, repeats)
	for i := 0; i < repeats; i++ {
		start := time.Now()
		positions = findReferences(pkgs, obj)
		durations[i] = time.Since(start)
	}

	printTimedReport(cfg, "refs", warmUp, durations, len(positions), formatPositions(positions))
	return nil
}

// formatPositions renders a slice of token.Position as "file:line:col"
// strings, the shared textual/JSON output shape for both refs and callers.
func formatPositions(positions []token.Position) []string {
	out := make([]string, len(positions))
	for i, p := range positions {
		out[i] = p.String()
	}
	return out
}

// printTimedReport prints the shared warm-up + steady-state timing shape
// every in-process mode reports: warm-up load duration, per-query
// steady-state durations with min/median, a result count, and the result
// list itself (as JSON when cfg.json, otherwise as plain text lines).
func printTimedReport(cfg config, mode string, warmUp time.Duration, durations []time.Duration, count int, items []string) {
	msValues := make([]float64, len(durations))
	for i, d := range durations {
		msValues[i] = float64(d.Microseconds()) / 1000.0
	}
	minMS, medianMS := minMedian(msValues)

	if cfg.json {
		report := refsReport{
			WarmUpMS:       float64(warmUp.Microseconds()) / 1000.0,
			SteadyStateMS:  msValues,
			MinMS:          minMS,
			MedianMS:       medianMS,
			ReferenceCount: count,
			Positions:      items,
		}
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			// MarshalIndent on this fixed, JSON-safe shape cannot fail in
			// practice; fall back to a text report rather than losing output.
			fmt.Printf("marshal report: %v\n", err)
			return
		}
		fmt.Println(string(data))
		return
	}

	fmt.Printf("mode: %s\n", mode)
	fmt.Printf("warm-up load: %.2fms\n", float64(warmUp.Microseconds())/1000.0)
	fmt.Printf("steady-state (n=%d): min=%.3fms median=%.3fms\n", len(durations), minMS, medianMS)
	fmt.Printf("count: %d\n", count)
	for _, item := range items {
		fmt.Println(item)
	}
}

// minMedian returns the minimum and median of values. It sorts a copy so the
// caller's slice ordering (steady-state run order) is preserved for any
// later per-run inspection.
func minMedian(values []float64) (min, median float64) {
	if len(values) == 0 {
		return 0, 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	min = sorted[0]
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		median = (sorted[mid-1] + sorted[mid]) / 2
	} else {
		median = sorted[mid]
	}
	return min, median
}
