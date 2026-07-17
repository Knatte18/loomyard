// callers.go implements the harness's "callers" mode: an in-process
// *direct*-caller (call-hierarchy) finder — the "who calls this function"
// slice, resolved syntactically from the current call sites' type
// information. This is deliberately shallow: it finds every *ast.CallExpr
// whose callee resolves to the target symbol and reports the enclosing
// function, one hop only. *Transitive* callers (who calls the callers,
// recursively) are a different problem — the callgraph arm (Card 5,
// go/callgraph CHA/RTA/VTA) — not this file.

package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"time"

	"golang.org/x/tools/go/packages"
)

// CallerInfo records one resolved call site: the qualified name of the
// function or method that lexically encloses the call, and the call
// expression's own source position. A target called from multiple sites
// inside the same enclosing function produces one CallerInfo per call site;
// findDirectCallers' caller reports the deduplicated caller-*function* set
// separately from this raw per-site list.
type CallerInfo struct {
	// Caller is the qualified name of the enclosing *ast.FuncDecl or
	// *ast.FuncLit: "<package>.<Func>" for a top-level func,
	// "<package>.<Type>.<Method>" for a method, or "<package>.<enclosing
	// func>.func literal" for a closure.
	Caller string
	// Position is the call expression's own source location, not the
	// enclosing function's.
	Position token.Position
}

// findDirectCallers walks every loaded package's syntax with ast.Inspect,
// and for every *ast.CallExpr whose callee identifier resolves (via
// TypesInfo.Uses) to obj, records the enclosing function and the call site's
// position. It intentionally does not descend into or follow the callees it
// finds — that one-hop syntactic scan is exactly what makes this "direct"
// rather than transitive.
func findDirectCallers(pkgs []*packages.Package, obj types.Object) []CallerInfo {
	var callers []CallerInfo
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			// ancestors tracks every node on the path from the file root to
			// the node currently being visited, pushed on entry and popped
			// on exit. ast.Inspect's nil callback fires once per visited
			// node's exit (not just FuncDecl/FuncLit's), so the stack must
			// track every node, not just the func ones — filtering by type
			// happens only when a *ast.CallExpr is found, by scanning back
			// through ancestors for the nearest FuncDecl/FuncLit.
			var ancestors []ast.Node

			var visit func(n ast.Node) bool
			visit = func(n ast.Node) bool {
				if n == nil {
					ancestors = ancestors[:len(ancestors)-1]
					return false
				}

				if call, ok := n.(*ast.CallExpr); ok {
					if callee := resolveCallee(pkg, call); callee == obj {
						callers = append(callers, CallerInfo{
							Caller:   enclosingName(pkg, ancestors),
							Position: pkg.Fset.Position(call.Pos()),
						})
					}
				}

				ancestors = append(ancestors, n)
				return true
			}
			ast.Inspect(file, visit)
		}
	}

	sort.Slice(callers, func(i, j int) bool {
		if callers[i].Caller != callers[j].Caller {
			return callers[i].Caller < callers[j].Caller
		}
		return callers[i].Position.String() < callers[j].Position.String()
	})
	return callers
}

// resolveCallee returns the types.Object a call expression's function
// operand resolves to, or nil if the callee is not a simple resolvable
// identifier (e.g. a call through an interface value or a function-valued
// expression with no static target) or is not found in TypesInfo.Uses.
// Method calls arrive as *ast.SelectorExpr; resolveCallee reads the
// selector's own Sel identifier, which TypesInfo.Uses maps to the concrete
// method types.Object exactly like a plain call's Ident.
func resolveCallee(pkg *packages.Package, call *ast.CallExpr) types.Object {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return pkg.TypesInfo.Uses[fn]
	case *ast.SelectorExpr:
		return pkg.TypesInfo.Uses[fn.Sel]
	default:
		return nil
	}
}

// enclosingName renders the qualified name of the nearest *ast.FuncDecl or
// *ast.FuncLit found scanning backward through ancestors (the path from the
// file root to the call site, built by findDirectCallers' walk), using the
// package path plus (for a *ast.FuncDecl) its receiver type when present, or
// a "func literal" suffix appended recursively for a closure with no
// declared name of its own — so a doubly-nested closure reports its full
// enclosing chain rather than just the innermost literal. A scan that finds
// no enclosing func (a call at file scope, which cannot occur in valid Go
// outside initializer expressions ast.Inspect still visits) reports
// "<package>.<init>" as a safe fallback.
func enclosingName(pkg *packages.Package, ancestors []ast.Node) string {
	for i := len(ancestors) - 1; i >= 0; i-- {
		switch fn := ancestors[i].(type) {
		case *ast.FuncDecl:
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				recvType := types.ExprString(fn.Recv.List[0].Type)
				return fmt.Sprintf("%s.%s.%s", pkg.PkgPath, recvType, fn.Name.Name)
			}
			return fmt.Sprintf("%s.%s", pkg.PkgPath, fn.Name.Name)
		case *ast.FuncLit:
			return enclosingName(pkg, ancestors[:i]) + " func literal"
		}
	}
	return pkg.PkgPath + ".<init>"
}

// callersReport is the JSON shape emitted by -json for the callers mode:
// warm-up and steady-state timings, the deduplicated caller-function set
// with per-function call-site counts, and the raw per-site list.
type callersReport struct {
	WarmUpMS      float64        `json:"warm_up_ms"`
	SteadyStateMS []float64      `json:"steady_state_ms"`
	MinMS         float64        `json:"min_ms"`
	MedianMS      float64        `json:"median_ms"`
	CallerCount   int            `json:"caller_function_count"`
	CallSiteCount int            `json:"call_site_count"`
	Callers       map[string]int `json:"callers"`
	CallSites     []string       `json:"call_sites"`
}

// runCallers implements the "callers" mode: load the target module once
// (recording the warm-up load duration), then run findDirectCallers cfg.n
// times against the already-loaded packages to measure steady-state
// latency, reporting the same warm-up + steady-state shape as "refs" plus
// the deduplicated caller-function set with per-function call counts.
func runCallers(cfg config) error {
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

	var callers []CallerInfo
	durations := make([]time.Duration, repeats)
	for i := 0; i < repeats; i++ {
		start := time.Now()
		callers = findDirectCallers(pkgs, obj)
		durations[i] = time.Since(start)
	}

	byFunction := make(map[string]int, len(callers))
	callSites := make([]string, len(callers))
	for i, c := range callers {
		byFunction[c.Caller]++
		callSites[i] = fmt.Sprintf("%s @ %s", c.Caller, c.Position.String())
	}

	printCallersReport(cfg, warmUp, durations, byFunction, callSites)
	return nil
}

// printCallersReport prints the callers-mode report: the shared warm-up +
// steady-state timing shape plus the deduplicated caller-function set (as
// JSON when cfg.json, otherwise as plain text lines).
func printCallersReport(cfg config, warmUp time.Duration, durations []time.Duration, byFunction map[string]int, callSites []string) {
	msValues := make([]float64, len(durations))
	for i, d := range durations {
		msValues[i] = float64(d.Microseconds()) / 1000.0
	}
	minMS, medianMS := minMedian(msValues)

	if cfg.json {
		report := callersReport{
			WarmUpMS:      float64(warmUp.Microseconds()) / 1000.0,
			SteadyStateMS: msValues,
			MinMS:         minMS,
			MedianMS:      medianMS,
			CallerCount:   len(byFunction),
			CallSiteCount: len(callSites),
			Callers:       byFunction,
			CallSites:     callSites,
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

	fmt.Println("mode: callers (direct, syntactic call sites only — see callers.go)")
	fmt.Printf("warm-up load: %.2fms\n", float64(warmUp.Microseconds())/1000.0)
	fmt.Printf("steady-state (n=%d): min=%.3fms median=%.3fms\n", len(durations), minMS, medianMS)
	fmt.Printf("caller functions: %d, call sites: %d\n", len(byFunction), len(callSites))

	functionNames := make([]string, 0, len(byFunction))
	for name := range byFunction {
		functionNames = append(functionNames, name)
	}
	sort.Strings(functionNames)
	for _, name := range functionNames {
		fmt.Printf("  %s (%d call site(s))\n", name, byFunction[name])
	}
	for _, site := range callSites {
		fmt.Println(site)
	}
}
