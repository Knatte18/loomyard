// engineterms_test.go pins PATTERN-stencil-ownership's engine-wording rule:
// a term from the closed list of Claude Code tools and concepts appears only in the Claude engine's own places,
// and every engine block has a copy for every registered engine.
// The scan reads the stencils, the specs and the production Go string literals.
// It skips the orch's places, since the orch runs on Claude alone.
// It admits a Go literal on the allowlist, whose reason says why it never reaches a Codex session.
// The scan holds the reason's claim to no check; review does.

package stencils

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// claudeEngine is the engine whose own places may name the closed list's terms.
const claudeEngine = "claude"

// claudeFileTools matches the name of one of Claude Code's file, search and skill tools.
const claudeFileTools = `(Skill|Read|Edit|Write|Grep|Glob)`

// claudeTerm is one entry of the closed list: a Claude Code tool or concept no other engine has.
type claudeTerm struct {
	name    string
	pattern *regexp.Regexp
}

// claudeTerms is the closed list of Claude Code tools and concepts.
// A file tool counts when the text names it as a tool, such as "the Read tool" or "with Write",
// so an imperative such as "Write the report" and a plan card's Edit label do not.
var claudeTerms = []claudeTerm{
	{"the Agent tool", regexp.MustCompile(`\bAgent (tool|call)s?\b`)},
	{"subagent_type", regexp.MustCompile(`\bsubagent_type\b`)},
	{"fork subagents", regexp.MustCompile(`(?i)\bfork subagents?\b`)},
	{"a Claude file or skill tool", regexp.MustCompile(`\b` + claudeFileTools + `((, | and | or )` + claudeFileTools + `)* tools?\b|\b(with|via|using) ` + claudeFileTools + `\b`)},
	{"AskUserQuestion", regexp.MustCompile(`AskUserQuestion`)},
	{"system prompt", regexp.MustCompile(`(?i)\bsystem prompt\b`)},
	{"run_in_background", regexp.MustCompile(`\brun_in_background\b`)},
	{"the Bash tool's timeout", regexp.MustCompile("\\bBash (tool'?s? )?`?timeout")},
}

// skippedGoPrefixes are the module paths whose Go literals the scan skips:
// the Claude engine's own package, the orch packages and the test-support kits, whose text reaches no launched prompt.
var skippedGoPrefixes = []string{
	"internal/shuttleengine/claudeengine/",
	"internal/orch",
	"contracts/orchrole/",
	"internal/testkit/",
}

// claudeLiteralAllowlist admits a production Go literal that names a term and never reaches a Codex session.
// A key is the literal's file, the name of its enclosing declaration and the term's name, joined by "#".
// The declaration is a const, var, type or function, and a method is named Type.Name.
// The declaration is in the key because a reason is a claim about one literal, not about every literal of its file.
var claudeLiteralAllowlist = []scankit.Entry{
	{Key: "internal/shellguard/rows.go#editToolsRule#a Claude file or skill tool", Why: "editToolsRule is the Claude wording of the edit rule; a Codex session's rows carry codexEditToolsRule instead"},
	{Key: "internal/shellguard/rows.go#steerHeadlessClaudeDeny#the Agent tool", Why: "steerHeadlessClaudeDeny is the Claude wording of the headless-claude steer; a Codex session's row carries codexSteerHeadlessClaudeDeny instead"},
	{Key: "internal/heddlecli/cli.go#commandOf#system prompt", Why: "the heddle help text names where a prompt target lands, in a Claude session; it is the operator's reference, never a launched prompt"},
	{Key: "internal/heddleengine/overview.go#placeTitle#system prompt", Why: "HEDDLE.md's place titles name where a prompt target lands, in a Claude session; HEDDLE.md is the operator's overview, never a launched prompt"},
	{Key: "internal/heddleengine/overview.go#placeReach#system prompt", Why: "HEDDLE.md's place reach names where a prompt target lands, in a Claude session; HEDDLE.md is the operator's overview, never a launched prompt"},
}

// declarationUnit is one top-level const, var, type or function of a Go file, named for the allowlist key.
type declarationUnit struct {
	name string
	node ast.Node
}

// functionName returns the name of fn, prefixed by its receiver's type name for a method.
func functionName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	receiver := fn.Recv.List[0].Type
	if star, ok := receiver.(*ast.StarExpr); ok {
		receiver = star.X
	}
	if index, ok := receiver.(*ast.IndexExpr); ok {
		receiver = index.X
	}
	if ident, ok := receiver.(*ast.Ident); ok {
		return ident.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// declarationUnits returns the named units of decl.
// A unit is each spec of a const, var or type declaration, or the function itself.
// A spec naming several identifiers is named by all of them, joined by ",".
func declarationUnits(decl ast.Decl) []declarationUnit {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		return []declarationUnit{{functionName(d), d}}
	case *ast.GenDecl:
		var units []declarationUnit
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.ValueSpec:
				names := make([]string, len(s.Names))
				for i, name := range s.Names {
					names[i] = name.Name
				}
				units = append(units, declarationUnit{strings.Join(names, ","), s})
			case *ast.TypeSpec:
				units = append(units, declarationUnit{s.Name.Name, s})
			}
		}
		return units
	}
	return nil
}

// termsIn returns the name of every closed-list term text names, its whitespace runs read as one space.
func termsIn(text string) []string {
	flat := strings.Join(strings.Fields(text), " ")
	var names []string
	for _, term := range claudeTerms {
		if term.pattern.MatchString(flat) {
			names = append(names, term.name)
		}
	}
	return names
}

// stencilTermViolations returns one line per closed-list term a stencil in bodies names.
// bodies maps a stencil name to its agent-facing body.
// A stencil in the Claude engine's blocks or in the orch family is skipped.
func stencilTermViolations(bodies map[string]string) []string {
	claudeBlocks := stencilstore.EngineBlockName(claudeEngine, "") + "/"
	var violations []string
	for name, body := range bodies {
		if strings.HasPrefix(name, claudeBlocks) || strings.HasPrefix(name, "orch-") {
			continue
		}
		for _, term := range termsIn(body) {
			violations = append(violations, fmt.Sprintf("stencil %q names %s; move the passage into an engine block with a copy per engine", name, term))
		}
	}
	sort.Strings(violations)
	return violations
}

// specTermViolations returns one line per closed-list term a spec under base's contracts/specs names.
// A spec's leading comment is not read.
// It also returns the count of specs scanned.
func specTermViolations(t *testing.T, base string) ([]string, int) {
	t.Helper()
	var violations []string
	scanned := scankit.Walk(t, scankit.Options{Base: base, Roots: []string{"contracts/specs"}, Exts: []string{".md"}}, func(f *scankit.File) {
		for _, term := range termsIn(stencil.StripLeadingComment(string(f.Data))) {
			violations = append(violations, fmt.Sprintf("spec %s names %s; word it for any engine or move it into an engine's copy", f.Rel, term))
		}
	})
	sort.Strings(violations)
	return violations, scanned
}

// goLiteralTermViolations returns one line per closed-list term a production Go string literal under base names.
// A literal under skippedGoPrefixes is skipped, and one allow admits by its file, declaration and term is not reported.
// It also returns the count of files scanned.
func goLiteralTermViolations(t *testing.T, base string, allow *scankit.Allowlist) ([]string, int) {
	t.Helper()
	var violations []string
	scanned := scankit.Walk(t, scankit.Options{Base: base}, func(f *scankit.File) {
		for _, prefix := range skippedGoPrefixes {
			if strings.HasPrefix(f.Rel, prefix) {
				return
			}
		}
		file := f.AST(t, 0)
		for _, decl := range file.Decls {
			for _, unit := range declarationUnits(decl) {
				ast.Inspect(unit.node, func(node ast.Node) bool {
					literal, ok := node.(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						return true
					}
					text, err := strconv.Unquote(literal.Value)
					if err != nil {
						t.Fatalf("%s: unquote %s: %v", f.Rel, literal.Value, err)
					}
					for _, term := range termsIn(text) {
						if allow.Allowed(f.Rel + "#" + unit.name + "#" + term) {
							continue
						}
						line := f.FileSet().Position(literal.Pos()).Line
						violations = append(violations, fmt.Sprintf("%s:%d: a string literal in %s names %s; choose the text per engine or word it for any engine", f.Rel, line, unit.name, term))
					}
					return true
				})
			}
		}
	})
	sort.Strings(violations)
	return violations, scanned
}

// missingEngineBlockCopies returns one line per engine block that some engine in engines has no copy of.
// It also returns one line per stencil in bodies that fails to parse, since its includes cannot be checked.
// The blocks are those registered under any engine's directory and those a stencil in bodies includes.
// registered is every registered stencil name.
func missingEngineBlockCopies(registered []string, bodies map[string]string, engines []string) []string {
	have := make(map[string]bool, len(registered))
	blocks := map[string]bool{}
	enginesDir := stencilstore.EngineBlocksDir + "/"
	for _, name := range registered {
		have[name] = true
		if rest, isBlock := strings.CutPrefix(name, enginesDir); isBlock {
			if _, block, ok := strings.Cut(rest, "/"); ok {
				blocks[block] = true
			}
		}
	}
	var missing []string
	for name, body := range bodies {
		includes, err := stencil.IncludeNames([]byte(body))
		if err != nil {
			missing = append(missing, fmt.Sprintf("stencil %q does not parse, so its engine includes are unchecked: %v", name, err))
			continue
		}
		for _, include := range includes {
			if block, isBlock := strings.CutPrefix(include, stencilstore.EngineBlockPrefix); isBlock {
				blocks[block] = true
			}
		}
	}
	for block := range blocks {
		for _, engine := range engines {
			if !have[stencilstore.EngineBlockName(engine, block)] {
				missing = append(missing, fmt.Sprintf("engine block %q has no copy for engine %q; add %s.md", block, engine, stencilstore.EngineBlockName(engine, block)))
			}
		}
	}
	sort.Strings(missing)
	return missing
}

// TestStencils_ClaudeTermsOnlyInClaudePlaces fails on a stencil that names a closed-list term.
// A stencil in the Claude engine's blocks or in the orch family is skipped.
// A narrowed gate runs it only when a stencil or a package it reads changes; a whole gate always runs it.
//
//testtiming:keep pins PATTERN-stencil-ownership's rule that Claude-only wording lives in Claude engine blocks, a guard no other test asserts
//lym:guard
//lym:watch contracts/stencils/*.go contracts/stencils/*.md contracts/stencils/*/*.md contracts/stencils/*/*/*.md
//lym:watch contracts/stencils/*/*/*/*.md internal/stencil/*.go internal/stencilstore/*.go
func TestStencils_ClaudeTermsOnlyInClaudePlaces(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	for _, violation := range stencilTermViolations(registeredBodies(t)) {
		t.Error(violation)
	}
}

// TestStencils_EveryEngineBlockHasACopyPerEngine fails on an engine block that a registered engine has no copy of.
// A narrowed gate runs it only when a stencil or a package it reads changes; a whole gate always runs it.
//
//testtiming:keep pins that every engine block resolves on every registered engine, a guard no other test asserts
//lym:guard
//lym:watch contracts/stencils/*.go contracts/stencils/*.md contracts/stencils/*/*.md contracts/stencils/*/*/*.md
//lym:watch contracts/stencils/*/*/*/*.md internal/stencil/*.go internal/stencilstore/*.go internal/modelspec/*.go
func TestStencils_EveryEngineBlockHasACopyPerEngine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	for _, missing := range missingEngineBlockCopies(Registry().Names(), registeredBodies(t), modelspec.EngineNames()) {
		t.Error(missing)
	}
}

// TestClaudeTermsOnlyInClaudePlaces_SpecsAndGoLiterals scans the specs and the module's production Go string literals.
// It fails on a closed-list term outside the Claude engine's package, the orch packages and the allowlist.
// It also fails on an allowlist entry that admits nothing.
//
//testtiming:keep scans every spec and production Go literal for Claude-only wording, which no other test reads
//lym:guard
func TestClaudeTermsOnlyInClaudePlaces_SpecsAndGoLiterals(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	specViolations, specs := specTermViolations(t, "")
	scankit.RequireFloor(t, specs, 5, "spec term scan")
	for _, violation := range specViolations {
		t.Error(violation)
	}

	allow := scankit.NewAllowlist(claudeLiteralAllowlist)
	goViolations, files := goLiteralTermViolations(t, "", allow)
	scankit.RequireFloor(t, files, 500, "Go literal term scan")
	for _, violation := range goViolations {
		t.Error(violation)
	}
	allow.RequireNoStale(t)
}

// TestClaudeTermScans_SyntheticRows proves each scan fails on text that breaks the rule and passes text that keeps it.
//
//testtiming:keep proves each Claude-term scan fires on a break, which the scans over the shipped text cannot show while it complies
func TestClaudeTermScans_SyntheticRows(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("TermsInNamesEachToolForm", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			text string
			want []string
		}{
			{"Spawn it through the Agent\ntool.", []string{"the Agent tool"}},
			{"Pass `subagent_type: \"fork\"`.", []string{"subagent_type"}},
			{"It may spawn Fork subagents.", []string{"fork subagents"}},
			{"Paths you pass to the Read, Write and Edit tools stay absolute.", []string{"a Claude file or skill tool"}},
			{"Write your reply with Write.", []string{"a Claude file or skill tool"}},
			{"Never call `AskUserQuestion`.", []string{"AskUserQuestion"}},
			{"Follow the gate rule in your System Prompt.", []string{"system prompt"}},
			{"Never set run_in_background.", []string{"run_in_background"}},
			{"Run it with the Bash tool's `timeout` at 300000.", []string{"the Bash tool's timeout"}},
			{"Write the report, then Read the plan; a card's Edit target stays.", nil},
			{"Spawn it with `spawn_agent` and wait with `wait_agent`.", nil},
		}
		for _, tt := range tests {
			if got := termsIn(tt.text); strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("termsIn(%q) = %v; want %v", tt.text, got, tt.want)
			}
		}
	})

	t.Run("StencilScanSkipsTheClaudeBlocksAndTheOrch", func(t *testing.T) {
		t.Parallel()
		bodies := map[string]string{
			"darn-template-write":                       "Use the Agent tool.",
			"orch-template-start":                       "Use the Agent tool.",
			stencilstore.EngineBlockName("claude", "x"): "Use the Agent tool.",
			stencilstore.EngineBlockName("codex", "x"):  "Use the Agent tool.",
		}
		got := stencilTermViolations(bodies)
		if len(got) != 2 || !strings.Contains(got[0], "darn-template-write") || !strings.Contains(got[1], "engine/codex/x") {
			t.Errorf("stencilTermViolations = %v; want the darn stencil and the codex block alone", got)
		}
	})

	t.Run("SpecAndGoLiteralScans", func(t *testing.T) {
		t.Parallel()
		base := t.TempDir()
		files := map[string]string{
			"contracts/specs/x-spec.md":                     "<!-- Header names the system prompt. -->\nRead it with Read.\n",
			"contracts/specs/y-spec.md":                     "Read it whole.\n",
			"internal/x/x.go":                               "package x\n\n// The Agent tool is named in a comment only.\nvar a = \"fine\"\nvar b = \"use the system \" + \"prompt\"\nvar c = `the Agent\ntool`\n",
			"internal/shuttleengine/claudeengine/claude.go": "package claudeengine\n\nvar a = \"the Agent tool\"\n",
			"internal/orchcli/orch.go":                      "package orchcli\n\nvar a = \"the Agent tool\"\n",
			"internal/allowed/allowed.go":                   "package allowed\n\nvar a = \"your system prompt\"\nvar other = \"your system prompt\"\n\ntype holder struct{}\n\nfunc (holder) a() string { return \"your system prompt\" }\n",
		}
		for rel, body := range files {
			abs := filepath.Join(base, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		specs, _ := specTermViolations(t, base)
		if len(specs) != 1 || !strings.Contains(specs[0], "contracts/specs/x-spec.md names a Claude file or skill tool") {
			t.Errorf("specTermViolations = %v; want x-spec.md's tool alone, its header unread", specs)
		}

		allow := scankit.NewAllowlist([]scankit.Entry{
			{Key: "internal/allowed/allowed.go#a#system prompt", Why: "test"},
			{Key: "internal/unused.go#a#system prompt", Why: "test"},
		})
		literals, _ := goLiteralTermViolations(t, base, allow)
		want := []string{
			"internal/allowed/allowed.go:4: a string literal in other names system prompt; choose the text per engine or word it for any engine",
			"internal/allowed/allowed.go:8: a string literal in holder.a names system prompt; choose the text per engine or word it for any engine",
			"internal/x/x.go:6: a string literal in c names the Agent tool; choose the text per engine or word it for any engine",
		}
		if strings.Join(literals, "\n") != strings.Join(want, "\n") {
			t.Errorf("goLiteralTermViolations = %q; want %q", literals, want)
		}
		if stale := allow.Stale(); len(stale) != 1 || stale[0] != "internal/unused.go#a#system prompt" {
			t.Errorf("Allowlist.Stale() = %v; want the entry that admitted nothing", stale)
		}
	})

	t.Run("EngineBlockWithoutACopy", func(t *testing.T) {
		t.Parallel()
		registered := []string{"engine/claude/a", "engine/codex/a", "engine/claude/b", "plain"}
		bodies := map[string]string{"plain": "{{template \"engine-c\"}}"}
		got := missingEngineBlockCopies(registered, bodies, []string{"claude", "codex"})
		want := []string{
			`engine block "b" has no copy for engine "codex"; add engine/codex/b.md`,
			`engine block "c" has no copy for engine "claude"; add engine/claude/c.md`,
			`engine block "c" has no copy for engine "codex"; add engine/codex/c.md`,
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("missingEngineBlockCopies = %q; want %q", got, want)
		}
	})
}
