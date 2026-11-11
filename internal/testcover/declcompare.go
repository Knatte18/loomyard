// declcompare.go compares one Go file's top-level declarations as a map's commit stores them with the working tree's.
// It names the functions whose declaration changed, and the change a coverage map cannot speak for.

package testcover

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// directivePrefix opens a compiler directive line.
const directivePrefix = "//go:"

// goSource is one side of a Go file's comparison: its text, or its absence.
type goSource struct {
	text    []byte
	present bool
}

// goFileComparison is what one Go file's comparison found.
type goFileComparison struct {
	// miss is the first change the map cannot speak for, nil for none.
	miss *Miss
	// changed names each function whose declaration differs or that only the commit holds.
	// A name is the one the coverage map's function keys use, Type.Method for a method.
	changed []string
	// addedImports are the import paths the working tree's file imports and the commit's does not.
	// Whether an added import is a miss depends on the map's linked set, so the caller judges them.
	addedImports []string
	// removedImports are the import paths a deleted file imported.
	// A file that remains and drops an import is a miss, so this is empty for it.
	// A deleted file's imports reach no other file, but a module package it imported may leave the build,
	// so the caller judges them.
	removedImports []string
	// methodsBefore and methodsAfter hold the file's methods by key name, at the commit and in the working tree.
	methodsBefore, methodsAfter map[string][]*ast.FuncDecl
}

// fileDecls indexes one Go file's top-level declarations.
type fileDecls struct {
	packageName string
	// directives are the file's `//go:` lines, in file order.
	directives []string
	// imports maps each import, keyed by its name and path, to its path.
	imports map[string]string
	// values holds the const, var and type declarations by a key of their kind and names.
	values map[string][]valueDecl
	// funcs holds the function and method declarations by the name a block key gives them.
	funcs map[string][]*ast.FuncDecl
}

// valueDecl is one const, var or type declaration, or a whole const group that uses iota or an implicit value.
type valueDecl struct {
	node  ast.Node
	names []string
	// addable is set for a declaration that, once added, no unchanged code can reach:
	// a const outside such a group, a var with no initializer or a type.
	addable bool
}

// compareGoSources compares the Go file rel as the map's commit stores it, before, with the working tree's, after.
// Either side may be absent.
func compareGoSources(rel string, before, after goSource) goFileComparison {
	old, err := parseFileDecls(before)
	if err != nil {
		return missed(MissParse, fmt.Sprintf("%s at the map's commit: %v", rel, err))
	}
	cur, err := parseFileDecls(after)
	if err != nil {
		return missed(MissParse, fmt.Sprintf("%s in the working tree: %v", rel, err))
	}
	if before.present && after.present && old.packageName != cur.packageName {
		return missed(MissNonBody, rel+" changes its package clause")
	}
	if !slices.Equal(old.directives, cur.directives) {
		return missed(MissDirective, rel+" adds, removes or changes a "+directivePrefix+" line")
	}
	var removedImports []string
	for _, key := range slices.Sorted(maps.Keys(old.imports)) {
		if _, kept := cur.imports[key]; kept {
			continue
		}
		if after.present {
			return missed(MissNonBody, fmt.Sprintf("%s removes the import %s", rel, strings.TrimSpace(key)))
		}
		removedImports = append(removedImports, old.imports[key])
	}
	for _, key := range slices.Sorted(maps.Keys(old.values)) {
		if !valueDeclsEqual(old.values[key], cur.values[key]) {
			return missed(MissNonBody, fmt.Sprintf("%s changes or removes %s", rel, key))
		}
	}
	for _, key := range slices.Sorted(maps.Keys(cur.values)) {
		if _, existed := old.values[key]; existed {
			continue
		}
		for _, value := range cur.values[key] {
			if name := firstPredeclared(value.names); name != "" {
				return missed(MissPredeclared, fmt.Sprintf("%s declares %s", rel, name))
			}
			if !value.addable {
				return missed(MissNonBody, fmt.Sprintf("%s adds %s, which unchanged code can reach", rel, key))
			}
		}
	}
	for _, key := range slices.Sorted(maps.Keys(cur.funcs)) {
		if _, existed := old.funcs[key]; existed {
			continue
		}
		for _, fn := range cur.funcs[key] {
			if fn.Recv == nil && predeclared(fn.Name.Name) {
				return missed(MissPredeclared, fmt.Sprintf("%s declares %s", rel, fn.Name.Name))
			}
		}
	}
	if added, held := len(cur.funcs["init"]), len(old.funcs["init"]); added > held {
		return missed(MissInit, fmt.Sprintf("%s holds %d init functions, %d at the map's commit", rel, added, held))
	}

	comparison := goFileComparison{
		removedImports: removedImports, methodsBefore: methodsOf(old.funcs), methodsAfter: methodsOf(cur.funcs),
	}
	for _, key := range slices.Sorted(maps.Keys(old.funcs)) {
		if !syntaxListsEqual(old.funcs[key], cur.funcs[key]) {
			comparison.changed = append(comparison.changed, key)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(cur.imports)) {
		if _, existed := old.imports[key]; !existed {
			comparison.addedImports = append(comparison.addedImports, cur.imports[key])
		}
	}
	return comparison
}

// missed returns a comparison that found the miss reason with detail.
func missed(reason MissReason, detail string) goFileComparison {
	return goFileComparison{miss: &Miss{Reason: reason, Detail: detail}}
}

// parseFileDecls parses src and indexes its top-level declarations; an absent src has none.
func parseFileDecls(src goSource) (fileDecls, error) {
	decls := fileDecls{imports: map[string]string{}, values: map[string][]valueDecl{}, funcs: map[string][]*ast.FuncDecl{}}
	if !src.present {
		return decls, nil
	}
	file, err := parser.ParseFile(token.NewFileSet(), "", src.text, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return fileDecls{}, err
	}
	decls.packageName = file.Name.Name
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, directivePrefix) {
				decls.directives = append(decls.directives, strings.TrimSpace(comment.Text))
			}
		}
	}
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			key := funcKeyName(decl)
			decls.funcs[key] = append(decls.funcs[key], decl)
		case *ast.GenDecl:
			decls.addGenDecl(decl)
		}
	}
	return decls, nil
}

// addGenDecl indexes the imports, consts, vars or types of decl.
// A const group that uses iota or an implicit value is one declaration, so a spec added to it changes it.
func (d fileDecls) addGenDecl(decl *ast.GenDecl) {
	if decl.Tok == token.CONST && implicitConstGroup(decl) {
		var names []string
		for _, spec := range decl.Specs {
			names = append(names, identNames(spec.(*ast.ValueSpec).Names)...)
		}
		d.addValue("const group "+strings.Join(names, ","), valueDecl{node: decl, names: names})
		return
	}
	for _, spec := range decl.Specs {
		switch spec := spec.(type) {
		case *ast.ImportSpec:
			// The parser accepted the file, so the path is a valid string literal.
			importPath, _ := strconv.Unquote(spec.Path.Value)
			name := ""
			if spec.Name != nil {
				name = spec.Name.Name
			}
			d.imports[name+" "+importPath] = importPath
		case *ast.ValueSpec:
			names := identNames(spec.Names)
			addable := decl.Tok == token.CONST || len(spec.Values) == 0
			d.addValue(decl.Tok.String()+" "+strings.Join(names, ","), valueDecl{node: spec, names: names, addable: addable})
		case *ast.TypeSpec:
			d.addValue("type "+spec.Name.Name, valueDecl{node: spec, names: []string{spec.Name.Name}, addable: true})
		}
	}
}

func (d fileDecls) addValue(key string, value valueDecl) {
	d.values[key] = append(d.values[key], value)
}

// implicitConstGroup reports whether a const declaration has a spec with no value or a value that uses iota.
func implicitConstGroup(decl *ast.GenDecl) bool {
	for _, spec := range decl.Specs {
		values := spec.(*ast.ValueSpec).Values
		if len(values) == 0 {
			return true
		}
		for _, value := range values {
			usesIota := false
			ast.Inspect(value, func(node ast.Node) bool {
				if ident, ok := node.(*ast.Ident); ok && ident.Name == "iota" {
					usesIota = true
				}
				return !usesIota
			})
			if usesIota {
				return true
			}
		}
	}
	return false
}

func identNames(idents []*ast.Ident) []string {
	names := make([]string, len(idents))
	for i, ident := range idents {
		names[i] = ident.Name
	}
	return names
}

// predeclared reports whether name is an identifier of the universe scope, such as max, len or error.
func predeclared(name string) bool {
	return types.Universe.Lookup(name) != nil
}

// firstPredeclared returns the first of names that predeclared reports, or "".
func firstPredeclared(names []string) string {
	for _, name := range names {
		if predeclared(name) {
			return name
		}
	}
	return ""
}

// methodsOf returns the methods among funcs, by key name.
func methodsOf(funcs map[string][]*ast.FuncDecl) map[string][]*ast.FuncDecl {
	methods := map[string][]*ast.FuncDecl{}
	for key, decls := range funcs {
		if decls[0].Recv != nil {
			methods[key] = decls
		}
	}
	return methods
}

// methodSetMiss returns the miss of a package whose changed files change its method set, or nil for none.
// A method added, removed, or with a changed receiver or signature changes the set.
// before and after hold the methods of the package's changed files, by key name.
// before is at the map's commit and after is in the working tree.
func methodSetMiss(dir string, before, after map[string][]*ast.FuncDecl) *Miss {
	keys := map[string]struct{}{}
	for key := range before {
		keys[key] = struct{}{}
	}
	for key := range after {
		keys[key] = struct{}{}
	}
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		old, cur := before[key], after[key]
		switch {
		case len(old) == 0:
			return &Miss{Reason: MissMethodSet, Detail: fmt.Sprintf("%s adds the method %s", dir, key)}
		case len(cur) == 0:
			return &Miss{Reason: MissMethodSet, Detail: fmt.Sprintf("%s removes the method %s", dir, key)}
		case len(old) != len(cur) || !signaturesEqual(old[0], cur[0]):
			return &Miss{Reason: MissMethodSet, Detail: fmt.Sprintf("%s changes the receiver or signature of %s", dir, key)}
		}
	}
	return nil
}

// signaturesEqual reports whether two methods have the same receiver type, parameter types and result types.
// Parameter and receiver names do not count.
func signaturesEqual(a, b *ast.FuncDecl) bool {
	return syntaxEqual(a.Recv.List[0].Type, b.Recv.List[0].Type) &&
		syntaxListsEqual(fieldTypes(a.Type.Params), fieldTypes(b.Type.Params)) &&
		syntaxListsEqual(fieldTypes(a.Type.Results), fieldTypes(b.Type.Results))
}

// fieldTypes returns the type of each entry of list, once per name it declares.
func fieldTypes(list *ast.FieldList) []ast.Expr {
	if list == nil {
		return nil
	}
	var expressions []ast.Expr
	for _, field := range list.List {
		for range max(1, len(field.Names)) {
			expressions = append(expressions, field.Type)
		}
	}
	return expressions
}

func valueDeclsEqual(a, b []valueDecl) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !syntaxEqual(a[i].node, b[i].node) {
			return false
		}
	}
	return true
}

// syntaxListsEqual reports whether a and b hold pairwise equal syntax, in order.
func syntaxListsEqual[T any](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !syntaxEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// syntaxEqual reports whether two syntax trees are equal, ignoring comments and formatting.
func syntaxEqual(a, b any) bool {
	return equalSyntax(reflect.ValueOf(a), reflect.ValueOf(b))
}

// The syntax types equalSyntax treats specially.
var (
	posType          = reflect.TypeFor[token.Pos]()
	commentGroupType = reflect.TypeFor[*ast.CommentGroup]()
	objectType       = reflect.TypeFor[*ast.Object]()
	scopeType        = reflect.TypeFor[*ast.Scope]()
)

// equalSyntax compares two values of a syntax tree field by field.
// It ignores comments and resolved objects.
// It compares a position only by whether it is valid, which tells `type A = B` from `type A B`.
func equalSyntax(a, b reflect.Value) bool {
	if !a.IsValid() || !b.IsValid() {
		return a.IsValid() == b.IsValid()
	}
	if a.Type() != b.Type() {
		return false
	}
	switch a.Type() {
	case posType:
		return token.Pos(a.Int()).IsValid() == token.Pos(b.Int()).IsValid()
	case commentGroupType, objectType, scopeType:
		return true
	}
	switch a.Kind() {
	case reflect.Pointer, reflect.Interface:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() == b.IsNil()
		}
		return equalSyntax(a.Elem(), b.Elem())
	case reflect.Struct:
		for i := range a.NumField() {
			if !equalSyntax(a.Field(i), b.Field(i)) {
				return false
			}
		}
		return true
	case reflect.Slice:
		if a.Len() != b.Len() {
			return false
		}
		for i := range a.Len() {
			if !equalSyntax(a.Index(i), b.Index(i)) {
				return false
			}
		}
		return true
	default:
		return a.Equal(b)
	}
}
