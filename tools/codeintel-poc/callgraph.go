// callgraph.go implements the harness's "callgraph" mode: a *transitive*
// caller finder built on golang.org/x/tools/go/callgraph's CHA, RTA, and VTA
// algorithms, selected by -algo. Unlike callers.go's one-hop syntactic scan,
// this walks the whole-program call graph to answer "every function that
// can, directly or indirectly, reach the target" — exactly where the three
// algorithms diverge once an interface is involved, which is why this mode
// reports the caller set (not just its size) for cross-algorithm diffing.

package main

import (
	"encoding/json"
	"fmt"
	"go/types"
	"sort"
	"strings"
	"time"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// callgraphResult carries one algorithm's output: the constructed call
// graph, the seed root set it used (empty for cha, which is whole-program
// with no roots concept), and how long the algorithm itself took (excluding
// the shared SSA-build step, timed separately by the caller).
type callgraphResult struct {
	Graph       *callgraph.Graph
	Roots       []string
	AnalysisDur time.Duration
}

// runCallgraph implements the "callgraph" mode: build an SSA program from
// the batch-1-loaded packages, run the -algo-selected transitive-caller
// algorithm, and report the target symbol's transitive caller set alongside
// the algorithm, the root set used, the caller-set size, and the SSA-build +
// analysis durations measured separately (per this batch's requirements) so
// the per-algorithm cost and the per-algorithm result set are both
// independently inspectable.
func runCallgraph(cfg config) error {
	pkgs, _, err := loadPackages(cfg.dir)
	if err != nil {
		return err
	}

	obj, err := resolveSymbol(pkgs, cfg.symbol)
	if err != nil {
		return err
	}
	funcObj, ok := obj.(*types.Func)
	if !ok {
		return fmt.Errorf("callgraph mode requires a func or method symbol, got %T for %q", obj, cfg.symbol)
	}

	// ssautil.InstantiateGenerics mirrors AllPackages' own mode flag
	// requirement (vta.CallGraph's doc comment demands it of its input
	// functions), and AllPackages (rather than Packages) pulls in every
	// dependency so the graph can represent calls that cross package
	// boundaries, not just the target's own package.
	buildStart := time.Now()
	prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()
	ssaBuildDuration := time.Since(buildStart)

	target := prog.FuncValue(funcObj)
	if target == nil {
		return fmt.Errorf("callgraph mode: %q has no SSA function (an interface method has none; see go/ssa Program.FuncValue)", cfg.symbol)
	}

	roots, rootNames := seedRoots(prog)

	var result callgraphResult
	switch cfg.algo {
	case "cha":
		result = runCHA(prog)
	case "rta":
		result, err = runRTA(roots, rootNames)
	case "vta":
		result, err = runVTA(prog, roots, rootNames)
	default:
		err = fmt.Errorf("unknown -algo %q: expected cha, rta, or vta", cfg.algo)
	}
	if err != nil {
		return err
	}

	callers := transitiveCallers(result.Graph, target)
	printCallgraphReport(cfg, cfg.algo, result.Roots, ssaBuildDuration, result.AnalysisDur, callers)
	return nil
}

// runCHA runs the Class Hierarchy Analysis algorithm, which builds a
// whole-program call graph directly from the static type hierarchy with no
// seed-root concept (every function in the program is a graph member,
// reachable or not) — the fastest and least precise of the three, since an
// interface call site resolves to every method with a matching signature
// anywhere in the program.
func runCHA(prog *ssa.Program) callgraphResult {
	start := time.Now()
	g := cha.CallGraph(prog)
	return callgraphResult{Graph: g, AnalysisDur: time.Since(start)}
}

// runRTA runs Rapid Type Analysis seeded from roots: a reachability
// algorithm that only considers functions and concrete types provably
// reachable from those roots, giving a more precise (smaller) call graph
// than CHA at the cost of requiring a correct root set — miss a root and RTA
// silently under-reports reachable callers.
func runRTA(roots []*ssa.Function, rootNames []string) (callgraphResult, error) {
	if len(roots) == 0 {
		return callgraphResult{}, fmt.Errorf("rta: no seed roots found (need cmd/lyx's main.main, a package init, or TestMain)")
	}
	start := time.Now()
	res := rta.Analyze(roots, true)
	return callgraphResult{Graph: res.CallGraph, Roots: rootNames, AnalysisDur: time.Since(start)}, nil
}

// runVTA runs Variable Type Analysis: build an initial CHA graph, narrow it
// to the functions reachable from the same seed roots RTA uses (rather than
// every function in the program, which would include dead code VTA has no
// need to propagate types through), then refine that reachable set's call
// edges via VTA's type-propagation graph — per the go/callgraph/vta package
// doc, VTA needs an initial call graph as its starting point and produces a
// call graph typically more precise than CHA but not as precise as RTA for
// programs where RTA's reachability alone is already tight.
func runVTA(prog *ssa.Program, roots []*ssa.Function, rootNames []string) (callgraphResult, error) {
	if len(roots) == 0 {
		return callgraphResult{}, fmt.Errorf("vta: no seed roots found (need cmd/lyx's main.main, a package init, or TestMain)")
	}

	chaGraph := cha.CallGraph(prog)
	reachable := reachableFromCHA(chaGraph, roots)

	start := time.Now()
	g := vta.CallGraph(reachable, chaGraph)
	return callgraphResult{Graph: g, Roots: rootNames, AnalysisDur: time.Since(start)}, nil
}

// seedRoots collects the RTA-style seed root set this harness uses for both
// "rta" and "vta": cmd/lyx's main.main (the harness's own eventual consumer
// program), every loaded package's synthetic init function (which itself
// calls each package's explicit init#N functions, so seeding just "init"
// transitively covers them), and TestMain wherever present. loadPackages
// loads with Tests: false, so no test package ever contributes a TestMain
// today; it is still checked defensively so the root set stays correct if
// that ever changes, per this batch's requirement to seed "every reachable
// TestMain/exported test entry if present".
func seedRoots(prog *ssa.Program) (roots []*ssa.Function, names []string) {
	add := func(fn *ssa.Function) {
		if fn == nil {
			return
		}
		roots = append(roots, fn)
		names = append(names, fn.String())
	}

	for _, pkg := range prog.AllPackages() {
		add(pkg.Func("init"))
		add(pkg.Func("TestMain"))
		if pkg.Pkg.Name() == "main" && strings.HasSuffix(pkg.Pkg.Path(), "/cmd/lyx") {
			add(pkg.Func("main"))
		}
	}

	sort.Strings(names)
	return roots, names
}

// reachableFromCHA returns every function reachable from roots by following
// chaGraph's outgoing call edges — VTA's own required "initial" call graph
// argument establishes which edges exist, so this reuses that same graph to
// derive VTA's function-set argument rather than falling back to "every
// function in the program" (which would waste analysis time propagating
// types through dead code the seed roots can never reach).
func reachableFromCHA(chaGraph *callgraph.Graph, roots []*ssa.Function) map[*ssa.Function]bool {
	reached := make(map[*ssa.Function]bool)

	var visit func(n *callgraph.Node)
	visit = func(n *callgraph.Node) {
		if n == nil || n.Func == nil || reached[n.Func] {
			return
		}
		reached[n.Func] = true
		for _, edge := range n.Out {
			visit(edge.Callee)
		}
	}

	for _, root := range roots {
		visit(chaGraph.CreateNode(root))
	}
	return reached
}

// transitiveCallers walks g's incoming edges outward from every node that
// represents target, recursing into each newly discovered caller in turn,
// to produce the deduplicated set of functions that can reach target
// directly or indirectly. A generic target has no single graph node: per
// ssa.InstantiateGenerics, each call site targets a distinct monomorphized
// instantiation of the generic origin rather than the origin itself, so
// this seeds the walk from every node sharing target's Origin() (which for
// a non-generic target is just target itself) rather than only
// g.Nodes[target] — otherwise a generic function's real callers would be
// silently missed. It returns nil (not an error) when no node shares
// target's origin — an honest "target is never called in this graph"
// result, not a failure, since cha.CallGraph and rta/vta results only
// create nodes for functions actually involved in a call edge.
func transitiveCallers(g *callgraph.Graph, target *ssa.Function) []string {
	origin := target.Origin()

	visited := make(map[*ssa.Function]bool)
	var order []string

	var visit func(n *callgraph.Node)
	visit = func(n *callgraph.Node) {
		for _, edge := range n.In {
			caller := edge.Caller
			if caller.Func == nil || visited[caller.Func] {
				continue
			}
			visited[caller.Func] = true
			order = append(order, caller.Func.String())
			visit(caller)
		}
	}

	for fn, node := range g.Nodes {
		if fn != nil && fn.Origin() == origin {
			visit(node)
		}
	}

	sort.Strings(order)
	return order
}

// callgraphReport is the JSON shape emitted by -json for the callgraph
// mode: the algorithm used, the root set that drove it, SSA-build and
// analysis timings measured separately, and the full transitive caller set
// (not just its count) so batch 3 can diff the set across algorithms.
type callgraphReport struct {
	Algo        string   `json:"algo"`
	Roots       []string `json:"roots"`
	SSABuildMS  float64  `json:"ssa_build_ms"`
	AnalysisMS  float64  `json:"analysis_ms"`
	CallerCount int      `json:"transitive_caller_count"`
	CallerSet   []string `json:"transitive_callers"`
}

// printCallgraphReport prints the callgraph-mode report: the algorithm and
// root set used, the SSA-build and analysis durations, and the transitive
// caller set (as JSON when cfg.json, otherwise as plain text lines).
func printCallgraphReport(cfg config, algo string, roots []string, ssaBuild, analysis time.Duration, callers []string) {
	if cfg.json {
		report := callgraphReport{
			Algo:        algo,
			Roots:       roots,
			SSABuildMS:  float64(ssaBuild.Microseconds()) / 1000.0,
			AnalysisMS:  float64(analysis.Microseconds()) / 1000.0,
			CallerCount: len(callers),
			CallerSet:   callers,
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

	fmt.Printf("mode: callgraph (algo=%s)\n", algo)
	fmt.Printf("roots (%d): %s\n", len(roots), strings.Join(roots, ", "))
	fmt.Printf("ssa build: %.2fms, analysis: %.2fms\n", float64(ssaBuild.Microseconds())/1000.0, float64(analysis.Microseconds())/1000.0)
	fmt.Printf("transitive callers: %d\n", len(callers))
	for _, c := range callers {
		fmt.Println(c)
	}
}
