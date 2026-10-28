//go:build tier0

package envkit

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

//lym:seam internal/battenshed#InnerRunDeps
//lym:seam internal/battenshed#LandDeps
//lym:seam internal/battenshed#PrimeLock
//lym:seam internal/battenshed#RowNoticeDeps
//lym:seam internal/battenshed#SeedChildDeps
//lym:seam internal/battenshed#TeardownDeps
//lym:seam internal/configengine#Dirs
//lym:seam internal/driftengine#Deps
//lym:seam internal/driftengine#Paths
//lym:seam internal/landingshed#BeamDescribeDeps
//lym:seam internal/landingshed#Config
//lym:seam internal/landingshed#Deps
//lym:seam internal/landingshed#ParentLockDeps
//lym:seam internal/loomshed#DarnDeps
//lym:seam internal/loomshed#PRReworkDeps
//lym:seam internal/pawlengine#Deps
//lym:seam internal/pawlengine#Paths
//lym:seam internal/shedadapters#CommandDeps
//lym:seam internal/shedrecipe#Env
//lym:seam internal/shuttleengine#Config
//lym:seam internal/websterengine#Config
//lym:seam internal/websterengine#Geometry
//lym:seam internal/websterengine#RunDeps
func TestFullEnv_FillsEverySeam(t *testing.T) {
	if got := NilSeams(FullEnv(t)); len(got) != 0 {
		t.Errorf("NilSeams(FullEnv) = %v; want none", got)
	}

	t.Run("TemplateSetting_AnswersATemplateKey", func(t *testing.T) {
		got, err := TemplateSetting("loom", "plan_auto_accept")
		if err != nil {
			t.Fatalf("TemplateSetting(loom, plan_auto_accept): %v", err)
		}
		if got != false {
			t.Errorf("TemplateSetting(loom, plan_auto_accept) = %v; want false", got)
		}
	})

	t.Run("TemplateSetting_RefusesUnknownModuleAndKey", func(t *testing.T) {
		if _, err := TemplateSetting("no-such-module", "plan_auto_accept"); err == nil || !strings.Contains(err.Error(), "no-such-module") {
			t.Errorf("TemplateSetting(no-such-module, ...) error = %v; want one naming the module", err)
		}
		if _, err := TemplateSetting("loom", "no_such_key"); err == nil || !strings.Contains(err.Error(), "no_such_key") {
			t.Errorf("TemplateSetting(loom, no_such_key) error = %v; want one naming the key", err)
		}
	})

	t.Run("Segments_HoldEveryReviewSegmentOfTheEmbeddedRecipes", func(t *testing.T) {
		segments := FullEnv(t).Segments
		for name, data := range map[string][]byte{"loom": recipes.LoomRecipe, "darn": recipes.DarnRecipe} {
			recipe, err := shedbuild.Parse(data)
			if err != nil {
				t.Fatalf("parse the %s recipe: %v", name, err)
			}
			for _, row := range recipe.Producers {
				if row.Engine != "Bouncer" {
					continue
				}
				if _, ok := segments[row.Segment]; !ok {
					t.Errorf("FullEnv(t).Segments lacks the %s recipe's review segment %q", name, row.Segment)
				}
			}
		}
	})
}

//lym:seam internal/battenshed#InnerRunDeps
//lym:seam internal/battenshed#LandDeps
//lym:seam internal/battenshed#PrimeLock
//lym:seam internal/battenshed#RowNoticeDeps
//lym:seam internal/battenshed#SeedChildDeps
//lym:seam internal/battenshed#TeardownDeps
//lym:seam internal/configengine#Dirs
//lym:seam internal/driftengine#Deps
//lym:seam internal/driftengine#Paths
//lym:seam internal/landingshed#BeamDescribeDeps
//lym:seam internal/landingshed#Config
//lym:seam internal/landingshed#Deps
//lym:seam internal/landingshed#ParentLockDeps
//lym:seam internal/loomshed#DarnDeps
//lym:seam internal/loomshed#PRReworkDeps
//lym:seam internal/pawlengine#Deps
//lym:seam internal/pawlengine#Paths
//lym:seam internal/shedadapters#CommandDeps
//lym:seam internal/shedrecipe#Env
//lym:seam internal/shuttleengine#Config
//lym:seam internal/websterengine#Config
//lym:seam internal/websterengine#Geometry
//lym:seam internal/websterengine#RunDeps
func TestNilSeams_ReportsExactPaths(t *testing.T) {
	env := FullEnv(t)
	env.Shuttle = nil
	env.CommitPlan = nil
	env.Landing.OpenFabric = nil
	env.Teardown.Remove = nil
	env.SeedChild.WriteSeed = nil

	got := NilSeams(env)
	slices.Sort(got)
	want := []string{"CommitPlan", "Landing.OpenFabric", "SeedChild.WriteSeed", "Shuttle", "Teardown.Remove"}
	if !slices.Equal(got, want) {
		t.Errorf("NilSeams = %v; want %v", got, want)
	}
}

//lym:seam internal/battenshed#InnerRunDeps
//lym:seam internal/battenshed#LandDeps
//lym:seam internal/battenshed#PrimeLock
//lym:seam internal/battenshed#RowNoticeDeps
//lym:seam internal/battenshed#SeedChildDeps
//lym:seam internal/battenshed#TeardownDeps
//lym:seam internal/configengine#Dirs
//lym:seam internal/driftengine#Deps
//lym:seam internal/driftengine#Paths
//lym:seam internal/landingshed#BeamDescribeDeps
//lym:seam internal/landingshed#Config
//lym:seam internal/landingshed#Deps
//lym:seam internal/landingshed#ParentLockDeps
//lym:seam internal/loomshed#DarnDeps
//lym:seam internal/loomshed#PRReworkDeps
//lym:seam internal/pawlengine#Deps
//lym:seam internal/pawlengine#Paths
//lym:seam internal/shedadapters#CommandDeps
//lym:seam internal/shedrecipe#Env
//lym:seam internal/shuttleengine#Config
//lym:seam internal/websterengine#Config
//lym:seam internal/websterengine#Geometry
//lym:seam internal/websterengine#RunDeps
func TestNilSeams_SkipsNilLegalPaths(t *testing.T) {
	env := FullEnv(t)
	env.Now = nil
	env.PrimeLock.Sleep = nil
	if got := NilSeams(env); len(got) != 0 {
		t.Errorf("NilSeams = %v; want none for nil-legal seams", got)
	}
}

//lym:seam internal/shedrecipe#Env
func TestFullEnv_PathFieldsAbsoluteUnderOneRoot(t *testing.T) {
	env := FullEnv(t)
	root := filepath.Dir(env.Cwd)

	v := reflect.ValueOf(env)
	checked := 0
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		if v.Field(i).Kind() != reflect.String {
			continue
		}
		if name != "Cwd" && !strings.HasSuffix(name, "Path") && !strings.HasSuffix(name, "Dir") && !strings.HasSuffix(name, "Root") {
			continue
		}
		checked++
		p := v.Field(i).String()
		if !filepath.IsAbs(p) {
			t.Errorf("%s = %q; want an absolute path", name, p)
		}
		if !strings.HasPrefix(p, root+string(filepath.Separator)) {
			t.Errorf("%s = %q; want a path under %q", name, p, root)
		}
	}
	if checked == 0 {
		t.Fatal("no path field checked")
	}
}

const (
	moduleGlyphPrefix = "github.com/Knatte18/loomyard/"
	envkitImportPath  = moduleGlyphPrefix + "internal/testkit/envkit"
)

// envSeamStructs returns, sorted, the glyph of shedrecipe.Env and of each struct type it nests by value, recursively, walking the fields NilSeams walks.
// A struct from outside the module is walked into but not listed, since it has no worktree glyph.
func envSeamStructs() []string {
	seen := map[string]bool{}
	collectSeamStructs(reflect.TypeFor[shedrecipe.Env](), "", seen)
	return slices.Sorted(maps.Keys(seen))
}

func collectSeamStructs(typ reflect.Type, prefix string, seen map[string]bool) {
	if unit, ok := strings.CutPrefix(typ.PkgPath(), moduleGlyphPrefix); ok && typ.Name() != "" {
		seen[unit+"#"+typ.Name()] = true
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		fieldPath := field.Name
		if prefix != "" {
			fieldPath = prefix + "." + field.Name
		}
		if nilLegal[fieldPath] || field.Type.Kind() != reflect.Struct {
			continue
		}
		collectSeamStructs(field.Type, fieldPath, seen)
	}
}

// testFuncInfo is what the seam scan knows of one top-level function of a package directory's test files.
type testFuncInfo struct {
	file     string
	reflects bool
	nilSeams bool
	calls    []string
}

// seamViolations checks the _test.go files of each package directory under root, root-relative and slash-separated, and returns one line per violation:
// a top-level test that reflects over a struct's fields, directly or through a test-file function of its directory, without a `//lym:seam` marker;
// a test reaching NilSeams whose markers miss a glyph of envStructs;
// and a marker whose glyph names no struct declared in its unit's directory.
// A marker whose unit's first path segment names no directory under root is taken as a struct outside the module and is not checked against a declaration.
func seamViolations(root string, dirs []string, envStructs []string) ([]string, error) {
	markers, err := impactset.FindSeamTests(root, dirs)
	if err != nil {
		return nil, err
	}
	var violations []string
	declared := map[string]map[string]bool{}
	for _, dir := range dirs {
		funcs, tests, err := parseTestFuncs(root, dir)
		if err != nil {
			return nil, err
		}
		structsByTest := map[string][]string{}
		for _, seam := range markers[dir] {
			structsByTest[seam.File+"#"+seam.Name] = seam.Structs
		}
		for _, test := range tests {
			info := funcs[test]
			structs := structsByTest[info.file+"#"+test]
			if reaches(info, funcs, func(f *testFuncInfo) bool { return f.reflects }) && len(structs) == 0 {
				violations = append(violations, fmt.Sprintf("%s: %s reflects over a struct's fields but carries no //lym:seam marker; way forward: add a `//lym:seam <unit>#<Type>` line for each struct it reflects over", info.file, test))
			}
			if reaches(info, funcs, func(f *testFuncInfo) bool { return f.nilSeams }) {
				var missing []string
				for _, glyph := range envStructs {
					if !slices.Contains(structs, glyph) {
						missing = append(missing, glyph)
					}
				}
				if len(missing) > 0 {
					violations = append(violations, fmt.Sprintf("%s: %s reaches envkit.NilSeams but its //lym:seam markers miss %s; way forward: add a `//lym:seam <unit>#<Type>` line for each struct it reflects over", info.file, test, strings.Join(missing, ", ")))
				}
			}
			for _, glyph := range structs {
				ok, err := markerNamesStruct(root, glyph, declared)
				if err != nil {
					return nil, err
				}
				if !ok {
					violations = append(violations, fmt.Sprintf("%s: %s marker %s names no struct; way forward: correct the marker's glyph to name the struct the test reflects over", info.file, test, glyph))
				}
			}
		}
	}
	return violations, nil
}

// parseTestFuncs returns the top-level functions of dir's _test.go files by name, and the names of those that are tests in source order.
func parseTestFuncs(root, dir string) (map[string]*testFuncInfo, []string, error) {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
	if err != nil {
		return nil, nil, err
	}
	funcs := map[string]*testFuncInfo{}
	var tests []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		rel := path.Join(dir, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, filepath.FromSlash(rel)), nil, 0)
		if err != nil {
			return nil, nil, err
		}
		envkitName := ""
		for _, spec := range file.Imports {
			if spec.Path.Value != `"`+envkitImportPath+`"` {
				continue
			}
			envkitName = "envkit"
			if spec.Name != nil {
				envkitName = spec.Name.Name
			}
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Body == nil {
				continue
			}
			info := &testFuncInfo{file: rel}
			scanBody(fn.Body, envkitName, file.Name.Name == "envkit", info)
			funcs[fn.Name.Name] = info
			if strings.HasPrefix(fn.Name.Name, "Test") {
				tests = append(tests, fn.Name.Name)
			}
		}
	}
	return funcs, tests, nil
}

// scanBody records the reflection, NilSeams and bare function calls in a function body.
func scanBody(body *ast.BlockStmt, envkitName string, inEnvkit bool, info *testFuncInfo) {
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			receiver, _ := fun.X.(*ast.Ident)
			switch {
			case fun.Sel.Name == "NumField":
				info.reflects = true
			case fun.Sel.Name == "VisibleFields" && receiver != nil && receiver.Name == "reflect":
				info.reflects = true
			case fun.Sel.Name == "NilSeams" && receiver != nil && envkitName != "" && receiver.Name == envkitName:
				info.nilSeams = true
			}
		case *ast.Ident:
			if fun.Name == "NilSeams" && inEnvkit {
				info.nilSeams = true
			} else {
				info.calls = append(info.calls, fun.Name)
			}
		}
		return true
	})
}

// reaches reports whether start, or a function it calls transitively, satisfies pick.
func reaches(start *testFuncInfo, funcs map[string]*testFuncInfo, pick func(*testFuncInfo) bool) bool {
	visited := map[*testFuncInfo]bool{}
	var visit func(info *testFuncInfo) bool
	visit = func(info *testFuncInfo) bool {
		if visited[info] {
			return false
		}
		visited[info] = true
		if pick(info) {
			return true
		}
		for _, name := range info.calls {
			if callee, ok := funcs[name]; ok && visit(callee) {
				return true
			}
		}
		return false
	}
	return visit(start)
}

// markerNamesStruct reports whether a marker's glyph names a struct declared in its unit's directory.
// A glyph whose unit's first path segment names no directory under root names a struct outside the module and passes.
// declared caches the struct names per directory.
func markerNamesStruct(root, glyph string, declared map[string]map[string]bool) (bool, error) {
	unit, name, _ := strings.Cut(glyph, "#")
	dir := strings.TrimSuffix(unit, "_test")
	first, _, _ := strings.Cut(dir, "/")
	if _, err := os.Stat(filepath.Join(root, first)); os.IsNotExist(err) {
		return true, nil
	}
	if _, ok := declared[dir]; !ok {
		names, err := structNamesIn(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil {
			return false, err
		}
		declared[dir] = names
	}
	return declared[dir][name], nil
}

// structNamesIn returns the names of the struct types declared in the .go files of dir, test files included; a missing dir declares none.
func structNamesIn(dir string) (map[string]bool, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			return nil, err
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					if _, isStruct := typeSpec.Type.(*ast.StructType); isStruct {
						names[typeSpec.Name.Name] = true
					}
				}
			}
		}
	}
	return names, nil
}

// TestSeamMarkers_ReflectiveTestsAreMarked proves every test that reflects over a struct's fields in the module carries a `//lym:seam` marker naming a struct.
//
//lym:seam internal/shedrecipe#Env
//lym:guard
func TestSeamMarkers_ReflectiveTestsAreMarked(t *testing.T) {
	t.Parallel()

	t.Run("RealTree_EveryReflectiveTestIsMarked", func(t *testing.T) {
		t.Parallel()
		root := scankit.Root(t)
		seen := map[string]bool{}
		var dirs []string
		scankit.Walk(t, scankit.Options{Filter: scankit.Test}, func(f *scankit.File) {
			if dir := path.Dir(f.Rel); !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
		})
		violations, err := seamViolations(root, dirs, envSeamStructs())
		if err != nil {
			t.Fatalf("seamViolations: %v", err)
		}
		for _, violation := range violations {
			t.Error(violation)
		}
	})

	t.Run("EnvSeamStructs_ListsEnvAndItsNestedModuleStructs", func(t *testing.T) {
		t.Parallel()
		got := envSeamStructs()
		for _, want := range []string{"internal/shedrecipe#Env", "internal/landingshed#Deps"} {
			if !slices.Contains(got, want) {
				t.Errorf("envSeamStructs() = %v; want it to hold %q", got, want)
			}
		}
		for _, glyph := range got {
			if strings.HasPrefix(glyph, "time#") || !strings.Contains(glyph, "/") {
				t.Errorf("envSeamStructs() holds %q; want module glyphs only", glyph)
			}
		}
	})

	const declarations = "package p\n\ntype T struct{}\n\ntype Env struct{}\n\ntype Inner struct{}\n\nfunc F() {}\n"
	const envkitImport = "import \"github.com/Knatte18/loomyard/internal/testkit/envkit\"\n\n"
	fixtures := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name:  "UnmarkedNumField_IsAViolation",
			files: map[string]string{"a_test.go": "package p\n\nfunc TestA(t *testing.T) { typ.NumField() }\n"},
			want:  []string{"TestA"},
		},
		{
			name:  "MarkedNumField_Passes",
			files: map[string]string{"a_test.go": "package p\n\n//lym:seam p#T\nfunc TestA(t *testing.T) { typ.NumField() }\n"},
		},
		{
			name:  "UnmarkedVisibleFields_IsAViolation",
			files: map[string]string{"a_test.go": "package p\n\nfunc TestA(t *testing.T) { reflect.VisibleFields(typ) }\n"},
			want:  []string{"TestA"},
		},
		{
			name: "UnmarkedCallToAReflectingHelper_IsAViolation",
			files: map[string]string{
				"a_test.go":      "package p\n\nfunc TestA(t *testing.T) { check(t) }\n",
				"helper_test.go": "package p\n\nfunc check(t *testing.T) { typ.NumField() }\n",
			},
			want: []string{"TestA"},
		},
		{
			name:  "MarkerNamingAFunc_IsAViolation",
			files: map[string]string{"a_test.go": "package p\n\n//lym:seam p#F\nfunc TestA(t *testing.T) { typ.NumField() }\n"},
			want:  []string{"TestA", "p#F"},
		},
		{
			name:  "MarkerOutsideTheModule_IsNotCheckedAgainstADeclaration",
			files: map[string]string{"a_test.go": "package p\n\n//lym:seam time#Time\nfunc TestA(t *testing.T) { typ.NumField() }\n"},
		},
		{
			name:  "MarkerUnderAModuleDirectoryWithNoDeclaration_IsAViolation",
			files: map[string]string{"a_test.go": "package p\n\n//lym:seam p/missing#T\nfunc TestA(t *testing.T) { typ.NumField() }\n"},
			want:  []string{"TestA", "p/missing#T"},
		},
		{
			name:  "UnmarkedNilSeams_NamesEveryEnvStruct",
			files: map[string]string{"a_test.go": "package p\n\n" + envkitImport + "func TestA(t *testing.T) { envkit.NilSeams(env) }\n"},
			want:  []string{"TestA", "p#Env, p#Inner"},
		},
		{
			name:  "NilSeamsMarkedWithOneEnvStruct_NamesTheMissingOne",
			files: map[string]string{"a_test.go": "package p\n\n" + envkitImport + "//lym:seam p#Env\nfunc TestA(t *testing.T) { envkit.NilSeams(env) }\n"},
			want:  []string{"TestA", "miss p#Inner"},
		},
		{
			name:  "NilSeamsMarkedWithEveryEnvStruct_Passes",
			files: map[string]string{"a_test.go": "package p\n\n" + envkitImport + "//lym:seam p#Env\n//lym:seam p#Inner\nfunc TestA(t *testing.T) { envkit.NilSeams(env) }\n"},
		},
		{
			name:  "BareNilSeamsInPackageEnvkit_NamesTheMissingStruct",
			files: map[string]string{"a_test.go": "package envkit\n\n//lym:seam p#Env\nfunc TestA(t *testing.T) { NilSeams(env) }\n"},
			want:  []string{"TestA", "miss p#Inner"},
		},
		{
			name: "NilSeamsThroughAHelper_NamesTheMissingStruct",
			files: map[string]string{
				"a_test.go":      "package p\n\n//lym:seam p#Env\nfunc TestA(t *testing.T) { fill(t) }\n",
				"helper_test.go": "package p\n\n" + envkitImport + "func fill(t *testing.T) { envkit.NilSeams(env) }\n",
			},
			want: []string{"TestA", "miss p#Inner"},
		},
	}
	for _, tt := range fixtures {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			dir := filepath.Join(root, "p")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("MkdirAll(%s): %v", dir, err)
			}
			files := map[string]string{"decl.go": declarations}
			maps.Copy(files, tt.files)
			for name, content := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile(%s): %v", name, err)
				}
			}
			got, err := seamViolations(root, []string{"p"}, []string{"p#Env", "p#Inner"})
			if err != nil {
				t.Fatalf("seamViolations() error = %v; want nil", err)
			}
			if len(tt.want) == 0 {
				if len(got) != 0 {
					t.Errorf("seamViolations() = %v; want none", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("seamViolations() = %v; want one violation", got)
			}
			for _, fragment := range append(tt.want, "way forward:") {
				if !strings.Contains(got[0], fragment) {
					t.Errorf("seamViolations()[0] = %q; want it to hold %q", got[0], fragment)
				}
			}
		})
	}
}
