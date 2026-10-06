package main

import (
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// spawnVerdict is the static classification of one test for the redundancy mode.
// A test whose verdict has either field set is never a redundancy candidate:
// its in-process coverage does not see the module code another process ran.
type spawnVerdict struct {
	outOfProcess bool // may run this module's code in another process, directly or through a followed call, or sits in a tmux-tier file
	unresolved   bool // calls something the scan cannot resolve to source
}

func (v *spawnVerdict) merge(o spawnVerdict) {
	v.outOfProcess = v.outOfProcess || o.outOfProcess
	v.unresolved = v.unresolved || o.unresolved
}

// funcInfo is one declared function or method with the imports and build tier of its file.
type funcInfo struct {
	decl     *ast.FuncDecl
	imports  map[string]string // file-level import name -> import path
	isTest   bool              // declared in a _test.go file
	tmuxTier bool              // the file's //go:build line mentions the tmux tag
	tagged   bool              // the file's //go:build line mentions the integration, tmux or llm tag
}

// pkgSource indexes the declarations of one directory's Go sources.
type pkgSource struct {
	funcs        map[string][]*funcInfo // package-level functions by name
	methods      map[string][]*funcInfo // methods by name, whatever the receiver
	varNames     map[string]bool        // package-level vars
	typeNames    map[string]bool        // package-level types
	ifaceMethods map[string]bool        // method names of package-declared interfaces
	funcFields   map[string]bool        // struct fields of function type
}

var builtinNames = map[string]bool{
	"append": true, "cap": true, "clear": true, "close": true, "complex": true, "copy": true,
	"delete": true, "imag": true, "len": true, "make": true, "max": true, "min": true,
	"new": true, "panic": true, "print": true, "println": true, "real": true, "recover": true,
	"bool": true, "byte": true, "complex64": true, "complex128": true, "error": true,
	"float32": true, "float64": true, "int": true, "int8": true, "int16": true, "int32": true,
	"int64": true, "rune": true, "string": true, "uint": true, "uint8": true, "uint16": true,
	"uint32": true, "uint64": true, "uintptr": true, "any": true, "comparable": true,
}

// loadPkgSource parses every .go file in dir regardless of build constraints, so a helper declared in a platform-specific file is still found.
// Test files are skipped unless withTests is set.
func loadPkgSource(dir string, withTests bool) (*pkgSource, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	src := &pkgSource{
		funcs: map[string][]*funcInfo{}, methods: map[string][]*funcInfo{},
		varNames: map[string]bool{}, typeNames: map[string]bool{},
		ifaceMethods: map[string]bool{}, funcFields: map[string]bool{},
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		isTest := strings.HasSuffix(name, "_test.go")
		if e.IsDir() || !strings.HasSuffix(name, ".go") || (isTest && !withTests) {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution|parser.ParseComments)
		if err != nil {
			return nil, err
		}
		tagged, tmuxTier := fileTier(file)
		imports := map[string]string{}
		for _, imp := range file.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			local := path.Base(p)
			if imp.Name != nil {
				local = imp.Name.Name
			}
			if local != "_" && local != "." {
				imports[local] = p
			}
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				fi := &funcInfo{decl: d, imports: imports, isTest: isTest, tmuxTier: tmuxTier, tagged: tagged}
				if d.Recv != nil {
					src.methods[d.Name.Name] = append(src.methods[d.Name.Name], fi)
				} else {
					src.funcs[d.Name.Name] = append(src.funcs[d.Name.Name], fi)
				}
			case *ast.GenDecl:
				src.indexGenDecl(d)
			}
		}
	}
	return src, nil
}

// fileTier reads the file's //go:build line and reports whether it mentions a tier tag at all and the tmux tag in particular.
// A constraint such as `tmux && linux` or `tmux && !windows` mentions tmux; a file without a //go:build line mentions nothing.
func fileTier(file *ast.File) (tagged, tmuxTier bool) {
	for _, group := range file.Comments {
		if group.Pos() >= file.Package {
			break
		}
		for _, c := range group.List {
			if !constraint.IsGoBuild(c.Text) {
				continue
			}
			expr, err := constraint.Parse(c.Text)
			if err != nil {
				continue
			}
			for _, tag := range mentionedTags(expr) {
				switch tag {
				case "tmux":
					tmuxTier, tagged = true, true
				case "integration", "llm":
					tagged = true
				}
			}
		}
	}
	return tagged, tmuxTier
}

// mentionedTags returns every build tag the expression names, whatever its polarity.
func mentionedTags(expr constraint.Expr) []string {
	switch x := expr.(type) {
	case *constraint.TagExpr:
		return []string{x.Tag}
	case *constraint.NotExpr:
		return mentionedTags(x.X)
	case *constraint.AndExpr:
		return append(mentionedTags(x.X), mentionedTags(x.Y)...)
	case *constraint.OrExpr:
		return append(mentionedTags(x.X), mentionedTags(x.Y)...)
	}
	return nil
}

func (src *pkgSource) indexGenDecl(d *ast.GenDecl) {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.ValueSpec:
			if d.Tok == token.VAR {
				for _, n := range s.Names {
					src.varNames[n.Name] = true
				}
			}
		case *ast.TypeSpec:
			src.typeNames[s.Name.Name] = true
			switch t := s.Type.(type) {
			case *ast.StructType:
				for _, f := range t.Fields.List {
					if _, ok := f.Type.(*ast.FuncType); ok {
						for _, n := range f.Names {
							src.funcFields[n.Name] = true
						}
					}
				}
			case *ast.InterfaceType:
				for _, m := range t.Methods.List {
					for _, n := range m.Names {
						src.ifaceMethods[n.Name] = true
					}
				}
			}
		}
	}
}

// spawnScanner classifies functions against the module's spawn primitives.
type spawnScanner struct {
	module  string // module import path
	kitRoot string // directory holding the testkits, internal/testkit
	kits    map[string]map[string]bool
}

// scanSpawns returns the verdict of every Test, Example and Fuzz function declared in pkgDir's _test.go files.
// A test in a file without a tier tag is always judged, because an untagged test spawns nothing;
// a test in a tmux-tier file is always out of process, because reed panes run lyx.
func scanSpawns(module, kitRoot, pkgDir string) (map[string]spawnVerdict, error) {
	src, err := loadPkgSource(pkgDir, true)
	if err != nil {
		return nil, err
	}
	s := &spawnScanner{module: module, kitRoot: kitRoot, kits: map[string]map[string]bool{}}
	memo := map[*funcInfo]*spawnVerdict{}
	out := map[string]spawnVerdict{}
	for name, fns := range src.funcs {
		for _, fn := range fns {
			if fn.isTest && isTestEntry(name) {
				found := s.analyze(src, fn, memo)
				switch {
				case !fn.tagged:
					found = spawnVerdict{}
				case fn.tmuxTier:
					found.outOfProcess = true
				}
				v := out[name]
				v.merge(found)
				out[name] = v
			}
		}
	}
	return out, nil
}

func isTestEntry(name string) bool {
	return strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Example") || strings.HasPrefix(name, "Fuzz")
}

// analyze follows fn's body, merging the verdicts of the same-package functions it calls.
// A call cycle contributes its partial verdict once, then stops.
func (s *spawnScanner) analyze(src *pkgSource, fn *funcInfo, memo map[*funcInfo]*spawnVerdict) spawnVerdict {
	if v, ok := memo[fn]; ok {
		return *v
	}
	v := &spawnVerdict{}
	memo[fn] = v
	if fn.decl.Body == nil {
		return *v
	}
	locals, localTypes := collectLocals(fn.decl)
	ast.Inspect(fn.decl.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); ok {
				if p, isImport := fn.imports[id.Name]; isImport && !locals[id.Name] && s.spawnRef(p, x.Sel.Name) {
					v.outOfProcess = true
				}
			}
		case *ast.IndexExpr:
			if readsOSArgsProgram(fn, x, locals) {
				v.outOfProcess = true
			}
		case *ast.CallExpr:
			if runsGoCommand(fn, x, locals) {
				v.outOfProcess = true
			}
			v.merge(s.analyzeCall(src, fn, x, locals, localTypes, memo))
		}
		return true
	})
	return *v
}

func (s *spawnScanner) analyzeCall(src *pkgSource, fn *funcInfo, call *ast.CallExpr, locals, localTypes map[string]bool, memo map[*funcInfo]*spawnVerdict) spawnVerdict {
	var out spawnVerdict
	switch f := unwrapCallee(call.Fun).(type) {
	case *ast.Ident:
		switch {
		case builtinNames[f.Name] || localTypes[f.Name]:
		case locals[f.Name], src.varNames[f.Name]:
			out.unresolved = true
		case len(src.funcs[f.Name]) > 0:
			for _, callee := range src.funcs[f.Name] {
				out.merge(s.analyze(src, callee, memo))
			}
		case src.typeNames[f.Name]:
		default:
			out.unresolved = true
		}
	case *ast.SelectorExpr:
		if id, ok := f.X.(*ast.Ident); ok {
			if _, isImport := fn.imports[id.Name]; isImport && !locals[id.Name] {
				return out
			}
		}
		for _, callee := range src.methods[f.Sel.Name] {
			out.merge(s.analyze(src, callee, memo))
		}
		if src.ifaceMethods[f.Sel.Name] || src.funcFields[f.Sel.Name] {
			out.unresolved = true
		}
	case *ast.CallExpr:
		out.unresolved = true
	}
	return out
}

// unwrapCallee strips parentheses and generic instantiation from a callee.
func unwrapCallee(e ast.Expr) ast.Expr {
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.IndexListExpr:
			e = x.X
		default:
			return e
		}
	}
}

// collectLocals returns the names a function declares for itself (parameters, results, receiver, := and var declarations, range variables) and the types it declares locally.
// A call to a local name is a call of a function value.
func collectLocals(decl *ast.FuncDecl) (locals, types map[string]bool) {
	locals, types = map[string]bool{}, map[string]bool{}
	addFields := func(fl *ast.FieldList) {
		if fl == nil {
			return
		}
		for _, f := range fl.List {
			for _, n := range f.Names {
				locals[n.Name] = true
			}
		}
	}
	addFields(decl.Recv)
	addFields(decl.Type.Params)
	addFields(decl.Type.Results)
	if decl.Body == nil {
		return locals, types
	}
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncType:
			addFields(x.Params)
			addFields(x.Results)
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				for _, l := range x.Lhs {
					if id, ok := l.(*ast.Ident); ok {
						locals[id.Name] = true
					}
				}
			}
		case *ast.RangeStmt:
			if x.Tok == token.DEFINE {
				for _, e := range []ast.Expr{x.Key, x.Value} {
					if id, ok := e.(*ast.Ident); ok {
						locals[id.Name] = true
					}
				}
			}
		case *ast.ValueSpec:
			for _, id := range x.Names {
				locals[id.Name] = true
			}
		case *ast.TypeSpec:
			types[x.Name.Name] = true
		}
		return true
	})
	return locals, types
}

// spawnRef reports whether importPath.name may run this module's code in another process:
// the lyxbin kit, os.Executable, or a testkit function that reaches one of those.
func (s *spawnScanner) spawnRef(importPath, name string) bool {
	if importPath == "os" {
		return name == "Executable"
	}
	rel, ok := strings.CutPrefix(importPath, s.module+"/")
	if !ok {
		return false
	}
	if rel == "internal/testkit/lyxbin" {
		return true
	}
	if kit, ok := strings.CutPrefix(rel, "internal/testkit/"); ok && !strings.Contains(kit, "/") {
		return s.kitSpawns(importPath, kit)[name]
	}
	return false
}

// readsOSArgsProgram reports whether x is os.Args[0], the running binary's own path.
// A read of any other index or a slice of os.Args does not count.
func readsOSArgsProgram(fn *funcInfo, x *ast.IndexExpr, locals map[string]bool) bool {
	sel, ok := x.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Args" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || fn.imports[id.Name] != "os" || locals[id.Name] {
		return false
	}
	index, ok := x.Index.(*ast.BasicLit)
	return ok && index.Kind == token.INT && index.Value == "0"
}

// runsGoCommand reports whether call is exec.Command or exec.CommandContext with the program "go", which builds and runs module code.
func runsGoCommand(fn *funcInfo, call *ast.CallExpr, locals map[string]bool) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || fn.imports[id.Name] != "os/exec" || locals[id.Name] {
		return false
	}
	programIndex := 0
	switch sel.Sel.Name {
	case "Command":
	case "CommandContext":
		programIndex = 1
	default:
		return false
	}
	if len(call.Args) <= programIndex {
		return false
	}
	program, ok := call.Args[programIndex].(*ast.BasicLit)
	if !ok || program.Kind != token.STRING {
		return false
	}
	value, err := strconv.Unquote(program.Value)
	return err == nil && value == "go"
}

// kitSpawns returns, per exported function of a testkit, whether the same scan marks it as out of process or unresolvable.
func (s *spawnScanner) kitSpawns(importPath, kit string) map[string]bool {
	if funcs, ok := s.kits[importPath]; ok {
		return funcs
	}
	funcs := map[string]bool{}
	s.kits[importPath] = funcs // a kit calling itself or a cycle sees the partial map
	src, err := loadPkgSource(filepath.Join(s.kitRoot, kit), false)
	if err != nil {
		return funcs
	}
	memo := map[*funcInfo]*spawnVerdict{}
	for name, fns := range src.funcs {
		for _, fn := range fns {
			if v := s.analyze(src, fn, memo); v.outOfProcess || v.unresolved {
				funcs[name] = true
			}
		}
	}
	return funcs
}
