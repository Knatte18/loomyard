// keynames_enforcement_test.go guards production string literals against config keys named without their module.

package configreg_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// minScannedKeyNameFiles is the fewest production Go files a key-name scan must read to be other than vacuous.
const minScannedKeyNameFiles = 500

// dottedName matches one dotted name in prose, such as `code.max_bounces` or `<module>.<dotted`.
var dottedName = regexp.MustCompile(`[A-Za-z0-9_<>]+(?:\.[A-Za-z0-9_<>]+)*`)

// keyNameIndex maps each bare form a template key may be written in to the full names it stands for.
// A bare form is the key's dotted path, or any trailing part of it, with no module in front.
type keyNameIndex struct {
	fullNames map[string][]string
}

// buildKeyNameIndex reads every registered module's template.
// A seed-only module is left out: its keys are names the operator chooses, not template keys.
// An open or per-name map is indexed as a key, and its entries are not.
func buildKeyNameIndex(t *testing.T) keyNameIndex {
	t.Helper()
	index := keyNameIndex{fullNames: map[string][]string{}}
	for _, module := range configreg.Modules() {
		if module.SeedOnly {
			continue
		}
		var document yaml.Node
		if err := yaml.Unmarshal([]byte(module.Template()), &document); err != nil {
			t.Fatalf("module %s: template does not parse: %v", module.Name, err)
		}
		if len(document.Content) == 0 {
			continue
		}
		leafMaps := append(slices.Clone(module.OpenMaps), module.PerNameMaps...)
		for _, path := range templatePaths(document.Content[0], "", leafMaps) {
			segments := strings.Split(path, ".")
			for start := range segments {
				bare := strings.Join(segments[start:], ".")
				index.fullNames[bare] = append(index.fullNames[bare], configengine.KeyName(module.Name, path))
			}
		}
	}
	return index
}

// templatePaths returns the dotted path of every key under node, a map's own path included.
func templatePaths(node *yaml.Node, prefix string, leafMaps []string) []string {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	var paths []string
	for i := 0; i+1 < len(node.Content); i += 2 {
		path := node.Content[i].Value
		if prefix != "" {
			path = prefix + "." + path
		}
		paths = append(paths, path)
		if !slices.Contains(leafMaps, path) {
			paths = append(paths, templatePaths(node.Content[i+1], path, leafMaps)...)
		}
	}
	return paths
}

// bareKeyName is one config key named in prose without its module.
type bareKeyName struct {
	name      string
	fullNames []string
}

// qualifiedPrefixes end the text before a name the rendered text qualifies with its module,
// or a path argument of a command that names the module itself.
var qualifiedPrefixes = []string{".", "%", "--set ", "--set '", `--set "`, "--set=", "--unset ", "--unset '", `--unset "`}

// bareKeyNameAllowlist holds the literals that write a template key's name for something other than the key.
// Each entry is `<file>: <name>: <literal text>`.
var bareKeyNameAllowlist = []scankit.Entry{
	{Key: "internal/battencli/keynames.go: driver_kind: the child recipe's driver_kind, then ", Why: "names the recipe's own driver_kind field, which comes before the shed key the literal names in full"},
	{Key: "internal/shedbuild/parse.go: max_bounces: shedbuild: producer %d %q: max_bounces must not be negative, got %d", Why: "names a recipe row's max_bounces field"},
	{Key: "internal/shedbuild/loomcontract.go: tree_scope: the recipe sets no verify_source; way forward: set verify_source to plan or tree_scope", Why: "names tree_scope as a value of a recipe's verify_source field"},
	{Key: "internal/gatecli/tierrun.go: timeout_min: the timeout_min bound passed", Why: "names the minute bound of a gate tier or of the gate build step, a field of an open map's entry"},
	{Key: "internal/gateslot/runlog.go: timeout_min:  exceeded timeout_min ", Why: "part of the run-log line format that verifytree parses back"},
	{Key: "internal/verifytree/verify.go: timeout_min:  exceeded timeout_min ", Why: "parses the run-log line format gateslot writes"},
	{Key: "internal/shuttleengine/claudeengine/command.go: prompt_cache_ttl_roles: prompt_cache_ttl_roles[%s]", Why: "a path the callee renders in full through the package's key-name helper"},
	{Key: "internal/shuttleengine/claudeengine/command.go: keep_warm_cap_roles: keep_warm_cap_roles[%s]", Why: "a path the callee renders in full through the package's key-name helper"},
	{Key: "internal/shuttleengine/codexengine/sandbox.go: sandbox_mode: sandbox_mode=", Why: "the Codex CLI's own -c option, which the codex.sandbox_mode key mirrors"},
}

// bareKeyNames returns each template key text names without its module.
// A name with an underscore or a dot counts wherever it stands.
// A one-word name counts only in backticks before the word key, as in "the `scoper` key".
// A word such as engine is also plain prose.
// A name written as a field of a document example is not a key named in prose, so it does not count.
func (index keyNameIndex) bareKeyNames(text string) []bareKeyName {
	var found []bareKeyName
	for _, span := range dottedName.FindAllStringIndex(text, -1) {
		start, end := span[0], span[1]
		name := text[start:end]
		fullNames, isKey := index.fullNames[name]
		if !isKey {
			continue
		}
		before, after := text[:start], text[end:]
		if !strings.ContainsAny(name, "_.") && !(strings.HasSuffix(before, "`") && strings.HasPrefix(after, "` key")) {
			continue
		}
		if slices.ContainsFunc(qualifiedPrefixes, func(prefix string) bool { return strings.HasSuffix(before, prefix) }) {
			continue
		}
		if isDocumentField(before, after) {
			continue
		}
		found = append(found, bareKeyName{name: name, fullNames: fullNames})
	}
	return found
}

// isDocumentField reports whether the text around a name makes it a field of a document example.
// That takes a colon behind the name and then a value of at most one word on the same line.
// Inside an open brace, a comma or a closing brace ends the value too.
// A colon followed by a space and nothing else introduces text joined on later, so it is no field.
// A colon followed by several words introduces prose.
func isDocumentField(before, after string) bool {
	value, hasColon := strings.CutPrefix(strings.TrimLeft(after, "`\"\\"), ":")
	if !hasColon {
		return false
	}
	ends := "\n"
	if strings.LastIndex(before, "{") > strings.LastIndex(before, "}") {
		ends = "\n,}"
	}
	if end := strings.IndexAny(value, ends); end >= 0 {
		value = value[:end]
	}
	return value == "" || len(strings.Fields(value)) == 1
}

// isKeyNameCall reports whether call renders a key's full name.
// configengine.KeyName does, and so does each package's own wrapper of it.
func isKeyNameCall(call *ast.CallExpr) bool {
	var name string
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		name = fun.Name
	case *ast.SelectorExpr:
		name = fun.Sel.Name
	}
	return strings.HasSuffix(name, "KeyName") || strings.HasSuffix(name, "keyName")
}

// stringLiterals returns every string literal of file's declarations, struct tags and import paths left out.
// The arguments of a call that renders a key's full name are left out, since the call adds the module.
func stringLiterals(file *ast.File) []*ast.BasicLit {
	var literals []*ast.BasicLit
	for _, declaration := range file.Decls {
		if general, ok := declaration.(*ast.GenDecl); ok && general.Tok == token.IMPORT {
			continue
		}
		ast.Inspect(declaration, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.Field:
				return false
			case *ast.CallExpr:
				return !isKeyNameCall(node)
			case *ast.BasicLit:
				if node.Kind == token.STRING {
					literals = append(literals, node)
				}
			}
			return true
		})
	}
	return literals
}

// TestKeyNames_DetectorReportsOnlyBareNames pins what the key-name scan reports and what it skips.
func TestKeyNames_DetectorReportsOnlyBareNames(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	index := buildKeyNameIndex(t)

	tests := []struct {
		name string
		text string
		want []string
	}{
		{"OneWordKeyInBackticks", "the `scoper` key", []string{"scoper"}},
		{"OneWordPlain", "the scoper key", nil},
		{"DottedPath", "set code.max_bounces now", []string{"code.max_bounces"}},
		{"AfterFileName", "gate.yaml's refuse_raw", []string{"refuse_raw"}},
		{"FullName", "loom.code.max_bounces", nil},
		{"SetArgument", "--set max_bounces=1", nil},
		{"QuotedUnsetArgument", `--unset "max_bounces"`, nil},
		{"DocumentLine", "max_bounces: 3", nil},
		{"DocumentFlowMap", "{command: x, timeout_min: 30}", nil},
		{"MessageAfterColon", "max_bounces: must be at least 0", []string{"max_bounces"}},
		{"TextJoinedAfterColon", "max_bounces: ", []string{"max_bounces"}},
		{"MessageWithCommaAfterColon", "max_bounces: negative, got %d", []string{"max_bounces"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			for _, bare := range index.bareKeyNames(tt.text) {
				got = append(got, bare.name)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("bareKeyNames(%q) = %q; want %q", tt.text, got, tt.want)
			}
		})
	}

	t.Run("SourceLiterals", func(t *testing.T) {
		t.Parallel()
		const source = `package p

import "example.com/max_bounces"

type row struct {
	Bounces int ` + "`yaml:\"max_bounces\"`" + `
}

var full = configengine.KeyName("loom", "code.max_bounces")

var message = "set it"
`
		file, err := parser.ParseFile(token.NewFileSet(), "p.go", source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse the source: %v", err)
		}
		var got []string
		for _, literal := range stringLiterals(file) {
			got = append(got, literal.Value)
		}
		if want := []string{`"set it"`}; !slices.Equal(got, want) {
			t.Errorf("stringLiterals() = %q; want %q", got, want)
		}
	})
}

// TestKeyNames_StringLiteralsNameEveryConfigKeyWithItsModule fails on a production literal naming a key bare.
// A bare name is one without its module, such as `code.max_bounces` for `loom.code.max_bounces`.
// A message renders a key's full name through configengine.KeyName, and help text spells it in full.
//
//lym:guard
func TestKeyNames_StringLiteralsNameEveryConfigKeyWithItsModule(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	index := buildKeyNameIndex(t)
	allowlist := scankit.NewAllowlist(bareKeyNameAllowlist)
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"cmd", "internal"}}, func(f *scankit.File) {
		file := f.AST(t, parser.SkipObjectResolution)
		for _, literal := range stringLiterals(file) {
			value, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatalf("%s: literal %s does not unquote: %v", f.Rel, literal.Value, err)
			}
			// A literal that is one key name whole is data, such as a map key or a field name.
			if _, whole := index.fullNames[value]; whole {
				continue
			}
			for _, bare := range index.bareKeyNames(value) {
				if allowlist.Allowed(f.Rel + ": " + bare.name + ": " + value) {
					continue
				}
				t.Errorf("%s:%d: string literal names config key %s without its module; name it as %s",
					f.Rel, f.FileSet().Position(literal.Pos()).Line, bare.name, strings.Join(bare.fullNames, " or "))
			}
		}
	})
	scankit.RequireFloor(t, scanned, minScannedKeyNameFiles, "production Go files under cmd/ and internal/")
	allowlist.RequireNoStale(t)
}
