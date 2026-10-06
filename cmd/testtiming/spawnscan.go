package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// spawnVerdict is the static spawn classification of one test.
// A test whose verdict has either field set is never a redundancy candidate:
// its in-process coverage does not see what the subprocess ran (D2).
type spawnVerdict struct {
	spawns     bool // references a spawn primitive, directly or through a followed call
	unresolved bool // calls something the scan cannot resolve to source
}

func (v *spawnVerdict) merge(o spawnVerdict) {
	v.spawns = v.spawns || o.spawns
	v.unresolved = v.unresolved || o.unresolved
}

// funcInfo is one declared function or method with the imports of its file.
type funcInfo struct {
	decl    *ast.FuncDecl
	imports map[string]string // file-level import name -> import path
	isTest  bool              // declared in a _test.go file
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
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
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
				fi := &funcInfo{decl: d, imports: imports, isTest: isTest}
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

// scanSpawns returns the spawn verdict of every Test, Example and Fuzz function declared in pkgDir's _test.go files.
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
				v := out[name]
				v.merge(s.analyze(src, fn, memo))
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
					v.spawns = true
				}
			}
		case *ast.CallExpr:
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

// spawnRef reports whether importPath.name is a spawn primitive.
func (s *spawnScanner) spawnRef(importPath, name string) bool {
	if importPath == "os/exec" {
		return true
	}
	rel, ok := strings.CutPrefix(importPath, s.module+"/")
	if !ok {
		return false
	}
	switch rel {
	case "internal/testkit/lyxbin", "internal/hubforge":
		return true
	case "internal/testkit/tmuxkit":
		return name != "Main"
	case "internal/gitkit":
		return name != "HermeticGitEnv"
	}
	if kit, ok := strings.CutPrefix(rel, "internal/testkit/"); ok && !strings.Contains(kit, "/") {
		return s.kitSpawns(importPath, kit)[name]
	}
	return false
}

// kitSpawns returns, per exported function of a testkit, whether the same scan marks it as spawning or unresolvable.
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
			if v := s.analyze(src, fn, memo); v.spawns || v.unresolved {
				funcs[name] = true
			}
		}
	}
	return funcs
}
