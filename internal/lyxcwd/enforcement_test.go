// enforcement_test.go is a repo-wide guard: it walks every package and fails the build if any file
// outside internal/lyxcwd reaches for raw cwd or top-level git geometry, keeps internal/lyxcwd
// the sole geometry owner, and (via TestEnforcement_FabricVocabulary) keeps the fabric-vocabulary
// leak fabric-weft-visibility-cleanup closed.
// All three enforcement tests walk through scankit.

package lyxcwd

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// stripGoComments blanks every comment in the Go source data, replacing the
// comment bytes with spaces while preserving newlines and the exact byte offsets
// of all non-comment code. This lets TestEnforcement's substring guard match real
// code usages of banned tokens (os.Getwd, --show-toplevel) without tripping on the
// same tokens when they appear only inside explanatory comments. go/scanner (not
// go/parser) is used deliberately: scanning tolerates build-tag-guarded platform
// files that would not fully parse, so no production file is silently skipped.
func stripGoComments(data []byte) []byte {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(data))
	var s scanner.Scanner
	s.Init(file, data, nil, scanner.ScanComments)

	out := make([]byte, len(data))
	copy(out, data)
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok != token.COMMENT {
			continue
		}
		start := file.Offset(pos)
		for i := start; i < start+len(lit) && i < len(out); i++ {
			if out[i] != '\n' {
				out[i] = ' '
			}
		}
	}
	return out
}

// TestStripGoComments locks in the comment-stripping guard: banned tokens that appear only in
// comments must be removed, while identical tokens in real code (including string literals) must
// survive untouched.
func TestStripGoComments(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		present bool // whether "os.Getwd" survives stripping
	}{
		{
			name:    "line comment mentioning token is stripped",
			src:     "package p\n// lyxcwd.Getwd is the only permitted os.Getwd caller\nvar _ = 1\n",
			present: false,
		},
		{
			name:    "block comment mentioning token is stripped",
			src:     "package p\n/* avoid os.Getwd here */\nvar _ = 1\n",
			present: false,
		},
		{
			name:    "real code usage survives",
			src:     "package p\nimport \"os\"\nvar _, _ = os.Getwd()\n",
			present: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Contains(string(stripGoComments([]byte(tt.src))), "os.Getwd")
			if got != tt.present {
				t.Errorf("after stripGoComments, os.Getwd present = %v, want %v\nsrc:\n%s", got, tt.present, tt.src)
			}
		})
	}
}

// enforcementAllowlist is the set of paths TestEnforcement lets name the raw cwd/root primitives: the whole of internal/lyxcwd and cmd/lyx/main.go.
var enforcementAllowlist = []scankit.Entry{
	{
		Key: "internal/lyxcwd/",
		Why: "the sole cwd owner",
	},
	{
		Key: "cmd/lyx/main.go",
		Why: "the CLI entry point resolves the process cwd once",
	},
}

// TestEnforcement walks the repo source tree and verifies that no source file outside
// internal/lyxcwd and cmd/lyx contains the raw cwd/root primitives os.Getwd or git rev-parse
// --show-toplevel.
func TestEnforcement(t *testing.T) {
	t.Run("tree-scan", func(t *testing.T) {
		// Predicate: returns true if the bytes contain a banned token.
		isBanned := func(data []byte) bool {
			content := string(data)
			return strings.Contains(content, "os.Getwd") ||
				strings.Contains(content, "--show-toplevel")
		}

		var failures []string
		allow := scankit.NewAllowlist(enforcementAllowlist)

		scanned := scankit.Walk(t, scankit.Options{}, func(f *scankit.File) {
			relPath, data := f.Rel, f.Data

			// Skip files in the allowlist (they are allowed to contain banned tokens).
			if allow.Allowed(relPath) {
				return
			}

			// Check the file for banned tokens. Comments are stripped first so
			// that a file which merely *names* a banned token in an explanatory
			// comment (e.g. a doc comment documenting why lyxcwd.Getwd
			// is the only permitted os.Getwd caller) is not falsely flagged; the
			// guard is about real code usage, not prose.
			if isBanned(stripGoComments(data)) {
				failures = append(failures, relPath)
			}
		})

		scankit.RequireFloor(t, scanned, 1, "raw cwd primitive scan")
		allow.RequireNoStale(t)
		if len(failures) > 0 {
			t.Errorf("found banned tokens in files: %v", failures)
		}
	})

	// Sub-test: verify the predicate itself on synthetic snippets.
	t.Run("predicate", func(t *testing.T) {
		tests := []struct {
			name    string
			content string
			want    bool
		}{
			{
				name:    "os.Getwd",
				content: "x := os.Getwd()",
				want:    true,
			},
			{
				name:    "--show-toplevel",
				content: `git rev-parse --show-toplevel`,
				want:    true,
			},
			{
				name:    "clean",
				content: "fmt.Println(hello)",
				want:    false,
			},
		}

		isBanned := func(content string) bool {
			return strings.Contains(content, "os.Getwd") ||
				strings.Contains(content, "--show-toplevel")
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := isBanned(tt.content)
				if got != tt.want {
					t.Errorf("isBanned(%q) = %v, want %v", tt.content, got, tt.want)
				}
			})
		}
	})
}

// TestEnforcement_GeometryLiterals walks the repo source tree and verifies that no production file
// outside each token's registered owner directory (or directories, during a transitional
// co-ownership window) constructs a geometry path token as a string literal in a path-construction
// context: a filepath.Join argument, a binary + operand, or a string const declaration value.
// Whole-token matching (exact equality, not substring) avoids false positives on compound names
// such as "_boardroom" or "-weft-bare".
// Test files (*_test.go) are excluded because test geometry is a review rule, not a
// machine-enforced invariant.
func TestEnforcement_GeometryLiterals(t *testing.T) {
	// geometryToken reports whether s is exactly one of the policed geometry path
	// tokens. Only a token's registered owner directory (below) may use it in
	// path-construction context.
	geometryToken := func(s string) bool {
		switch s {
		case "_board", "-weft", "-LYXHUB", "_portals", "_launchers", "_lyx", ".lyx":
			return true
		}
		return false
	}

	// geometryTokenOwners maps each policed geometry token to the set of
	// directories permitted to declare or construct it in path-construction
	// context: the finished per-token ownership map, converged batch by batch
	// from a single allowlisted directory (internal/hubgeometry, then
	// internal/lyxcwd) to this map, each token's row landing in the same
	// batch that moved its declaration.
	//
	geometryTokenOwners := map[string][]string{
		// "_board" and "-LYXHUB" are dual-owned: internal/lyxcwd keeps a private
		// boardDir/boardDirName pair (readRecordedAnchor's sole remaining
		// reason to know the name) and a private hubSuffix const
		// (Location.RepoName derives from it), while internal/fabricengine
		// owns the exported BoardDir/HubPath constructors every other
		// caller uses. The duplication is sanctioned by this map, not a leak.
		"_board":  {"internal/lyxcwd", "internal/fabricengine"},
		"-weft":   {"internal/weftname"},
		"-LYXHUB": {"internal/lyxcwd", "internal/fabricengine"},
		// "_portals" and "_launchers" are fabric's own illusion-maintenance
		// plumbing: the portal/launcher path surface relocated to
		// internal/fabricengine in this batch.
		"_portals":   {"internal/fabricengine"},
		"_launchers": {"internal/fabricengine"},
		// "_lyx"'s declaration moved to internal/lyxdirs (lyxdirs.LyxDirName) in
		// this slice, the single leaf every module now names both directory
		// tokens through.
		"_lyx": {"internal/lyxdirs"},
		// ".lyx" (the machine-local, never-git-tracked sibling of "_lyx") is
		// lyxdirs.DotLyxDirName, declared in the same leaf as "_lyx"; the five
		// private dotLyxDirName declarers this slice retired never get a row
		// here.
		".lyx": {"internal/lyxdirs"},
	}

	// "_pattern" and "_raddle" are retired geometry tokens, deliberately
	// absent from both geometryToken and geometryTokenOwners above rather
	// than left as unused rows: "_pattern" because the PATTERN surface now
	// lives inside "_lyx" (internal/pattern builds its paths from
	// lyxdirs.LyxDirName instead of declaring its own directory token), and
	// "_raddle" because raddle converged on an anchor-level "_lyx/raddle/"
	// design with no hub-level presence to police. Do not re-add either row
	// on the assumption it was dropped by accident.

	// tokenOwnedByDir reports whether dir is one of tok's registered owners.
	tokenOwnedByDir := func(tok, dir string) bool {
		for _, owner := range geometryTokenOwners[tok] {
			if owner == dir {
				return true
			}
		}
		return false
	}

	// hasGeometryLiteralInConstructionContext reports whether the parsed AST file
	// contains a string literal whose unquoted value equals a geometry token and that
	// appears in a path-construction context:
	//   (a) an argument to filepath.Join(...)
	//   (b) an operand of a binary + expression (token.ADD)
	//   (c) the value of a string const declaration
	// Whole-token matching is enforced via exact equality after strconv.Unquote.
	hasGeometryLiteralInConstructionContext := func(f *ast.File) bool {
		found := false
		ast.Inspect(f, func(n ast.Node) bool {
			if found {
				return false
			}
			switch node := n.(type) {
			case *ast.CallExpr:
				// Context (a): filepath.Join(...) argument.
				sel, ok := node.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Join" {
					break
				}
				ident, ok2 := sel.X.(*ast.Ident)
				if !ok2 || ident.Name != "filepath" {
					break
				}
				for _, arg := range node.Args {
					lit, ok3 := arg.(*ast.BasicLit)
					if !ok3 || lit.Kind != token.STRING {
						continue
					}
					v, err := strconv.Unquote(lit.Value)
					if err == nil && geometryToken(v) {
						found = true
						return false
					}
				}
			case *ast.BinaryExpr:
				// Context (b): binary + operand.
				if node.Op != token.ADD {
					break
				}
				for _, operand := range []ast.Expr{node.X, node.Y} {
					lit, ok := operand.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					v, err := strconv.Unquote(lit.Value)
					if err == nil && geometryToken(v) {
						found = true
						return false
					}
				}
			case *ast.GenDecl:
				// Context (c): string const declaration.
				if node.Tok != token.CONST {
					break
				}
				for _, spec := range node.Specs {
					valSpec, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, val := range valSpec.Values {
						lit, ok2 := val.(*ast.BasicLit)
						if !ok2 || lit.Kind != token.STRING {
							continue
						}
						v, err := strconv.Unquote(lit.Value)
						if err == nil && geometryToken(v) {
							found = true
							return false
						}
					}
				}
			}
			return true
		})
		return found
	}

	// geometryLiteralTokensInConstructionContext returns every policed geometry
	// token found in path-construction context in f, in AST-visitation order
	// (a token may repeat if the file constructs it more than once). It shares
	// hasGeometryLiteralInConstructionContext's three contexts, but does not
	// stop at the first match: the tree-scan sub-test below needs every distinct
	// token a file constructs, so it can check each one's ownership separately
	// rather than only knowing that *some* token was found.
	geometryLiteralTokensInConstructionContext := func(f *ast.File) []string {
		var tokens []string
		ast.Inspect(f, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				sel, ok := node.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Join" {
					break
				}
				ident, ok2 := sel.X.(*ast.Ident)
				if !ok2 || ident.Name != "filepath" {
					break
				}
				for _, arg := range node.Args {
					lit, ok3 := arg.(*ast.BasicLit)
					if !ok3 || lit.Kind != token.STRING {
						continue
					}
					v, err := strconv.Unquote(lit.Value)
					if err == nil && geometryToken(v) {
						tokens = append(tokens, v)
					}
				}
			case *ast.BinaryExpr:
				if node.Op != token.ADD {
					break
				}
				for _, operand := range []ast.Expr{node.X, node.Y} {
					lit, ok := operand.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					v, err := strconv.Unquote(lit.Value)
					if err == nil && geometryToken(v) {
						tokens = append(tokens, v)
					}
				}
			case *ast.GenDecl:
				if node.Tok != token.CONST {
					break
				}
				for _, spec := range node.Specs {
					valSpec, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, val := range valSpec.Values {
						lit, ok2 := val.(*ast.BasicLit)
						if !ok2 || lit.Kind != token.STRING {
							continue
						}
						v, err := strconv.Unquote(lit.Value)
						if err == nil && geometryToken(v) {
							tokens = append(tokens, v)
						}
					}
				}
			}
			return true
		})
		return tokens
	}

	// predicate sub-test: validates the AST detector against synthetic Go snippets
	// parsed with go/parser. Positives must be detected; negatives must not.
	t.Run("predicate", func(t *testing.T) {
		positives := []struct {
			name string
			src  string
		}{
			{
				name: "filepath.Join_arg_board",
				src:  `package p; import "path/filepath"; var _ = filepath.Join(x, "_board")`,
			},
			{
				name: "add_operand_weft",
				src:  `package p; var _ = slug + "-weft"`,
			},
			{
				name: "const_LYXHUB",
				src:  `package p; const s = "-LYXHUB"`,
			},
		}
		for _, tt := range positives {
			t.Run(tt.name, func(t *testing.T) {
				fset := token.NewFileSet()
				f, err := parser.ParseFile(fset, "<fixture>", tt.src, parser.SkipObjectResolution)
				if err != nil {
					t.Fatalf("parse positive fixture: %v", err)
				}
				if !hasGeometryLiteralInConstructionContext(f) {
					t.Errorf("geometry literal was not detected in positive fixture:\n%s", tt.src)
				}
			})
		}

		negatives := []struct {
			name string
			src  string
		}{
			{
				name: "doc_comment_weft",
				// A comment is not an AST expression node and must never be flagged.
				src: "// Package p discusses the -weft sibling directory.\npackage p",
			},
			{
				name: "struct_field_long_weft",
				// A struct-literal field value is not a construction context.
				src: "package p\n\nvar _ = struct{ Long string }{Long: \"-weft\"}",
			},
			{
				name: "plain_non_token_string",
				// A string that does not equal any geometry token must not be flagged.
				src: `package p; var _ = "not-a-geometry-token"`,
			},
			{
				name: "add_near_token_weft_bare",
				// "-weft-bare" ≠ "-weft"; whole-token matching must reject the compound name.
				src: `package p; var _ = slug + "-weft-bare"`,
			},
			{
				name: "filepath.Join_near_token_boardroom",
				// "_boardroom" ≠ "_board"; whole-token matching must reject the compound name.
				src: `package p; import "path/filepath"; var _ = filepath.Join(x, "_boardroom")`,
			},
		}
		for _, tt := range negatives {
			t.Run(tt.name, func(t *testing.T) {
				fset := token.NewFileSet()
				f, err := parser.ParseFile(fset, "<fixture>", tt.src, parser.SkipObjectResolution)
				if err != nil {
					t.Fatalf("parse negative fixture: %v", err)
				}
				if hasGeometryLiteralInConstructionContext(f) {
					t.Errorf("geometry literal was falsely detected in negative fixture:\n%s", tt.src)
				}
			})
		}
	})

	// tree-scan sub-test: walks every production Go file in the repo and fails if
	// any file constructs a geometry token in a path context outside that
	// token's registered owner directory (or directories, per geometryTokenOwners).
	t.Run("tree-scan", func(t *testing.T) {
		var failures []string

		// Only production Go files are scanned (scankit's default filter);
		// test files are excluded because test geometry is a review-only rule, not a machine-enforced invariant.
		scanned := scankit.Walk(t, scankit.Options{}, func(file *scankit.File) {
			relDir := filepath.ToSlash(filepath.Dir(file.Rel))

			f := file.AST(t, parser.SkipObjectResolution)
			for _, tok := range geometryLiteralTokensInConstructionContext(f) {
				if !tokenOwnedByDir(tok, relDir) {
					failures = append(failures, file.Rel)
					break
				}
			}
		})

		// A misconfigured walk (wrong root, all files skipped) must not silently produce a vacuous all-pass result.
		t.Run("scanned_non_empty", func(t *testing.T) {
			scankit.RequireFloor(t, scanned, 1, "geometry-literal guard")
		})

		if len(failures) > 0 {
			t.Errorf("geometry-literal construction found outside its registered owner directory in:\n%v", failures)
		}
	})
}

// configsyncOwnerDir is fabricVocabularyOwners' one narrow row: internal/configsync may name
// "warp"/"weft" in string literals and comments (the on-disk legacy config filenames the
// migration must read by name), but not in identifiers.
const configsyncOwnerDir = "internal/configsync"

// fabricVocabularyOwners is the set of directories permitted to use the bare weft/warp tokens, in
// the same idiom as TestEnforcement_GeometryLiterals's geometryTokenOwners. It governs the bare
// weft/warp rule only -- host is retired and the fabric-sense host-phrase rule applies everywhere,
// including inside these owner dirs, so this set never carves out a host-phrase hit.
// configsyncOwnerDir's narrower literal-and-comment carve-out (weft/warp only) is applied
// separately by failsBareVocabularyCheck.
var fabricVocabularyOwners = map[string]bool{
	"internal/fabricengine": true,
	"internal/fabriccli":    true,
	"internal/weftname":     true,
	"internal/gitkit":       true,
	"internal/boardengine":  true,
	configsyncOwnerDir:      true,
	// internal/hubforge is a directory of non-test .go files (the hub factory must be non-test
	// to be importable across packages) that names fabric's own geometry, so it owns the bare
	// weft/warp tokens the same way internal/fabricengine does.
	"internal/hubforge": true,
}

// weftnameImportOwners is the set of directories permitted to import internal/weftname: the
// narrower owner set fabric-vocabulary-rule's rule (3) grants, excluding boardengine and
// configsync (owners for the bare-token rule but not for this import rule).
var weftnameImportOwners = map[string]bool{
	"internal/fabricengine": true,
	"internal/fabriccli":    true,
	"internal/gitkit":       true,
	// internal/weftname's own external-package test imports the package it tests.
	"internal/weftname": true,
	// internal/hubforge is in the narrower weftname-import subset PATTERN-fabric-vocabulary
	// already names, alongside internal/fabricengine, internal/fabriccli
	// and internal/gitkit -- this map is an allowlist of what may import weftname, not an
	// assertion of what does, so this entry is correct even though hub.go imports no weftname
	// identifier today.
	"internal/hubforge": true,
}

// weftnameImportPath is the fully-qualified import path TestEnforcement_FabricVocabulary's
// import rule polices.
const weftnameImportPath = "github.com/Knatte18/loomyard/internal/weftname"

// weftnameTestImporter is the one test file outside weftnameImportOwners that may import
// internal/weftname, to test weftname.SiblingPath.
const weftnameTestImporter = "internal/lyxcwd/geometry_test.go"

// vocabExemptEntry is one spelling a non-owner file may carry despite naming a side.
type vocabExemptEntry struct {
	token    string
	declared string
}

// vocabExempt is the closed list of spellings the bare weft/warp rule strips whole before matching.
// Each is spelled by its owner and cannot be reworded without renaming the owner's API or output,
// which is out of scope here (board note fabric-api-vocabulary).
// Adding an entry needs a declared field naming its owner-set declaration; any other spelling that
// carries a side, including a new owner-set export, still fails.
var vocabExempt = []vocabExemptEntry{
	{"PairWarpWorktree", "internal/hubforge, a *Hub method"},
	{"PairWeftSibling", "internal/hubforge, a *Hub method"},
	{"WeftBare", "internal/hubforge, a Hub field"},
	{"WarpBare", "internal/hubforge, a Hub field"},
	{"PrimeWeft", "internal/hubforge, a *Hub method"},
	{"WeftWorktree", "internal/fabricengine"},
	{"WeftWorktreePath", "internal/fabricengine"},
	{"WeftBranchName", "internal/fabricengine"},
	{"WeftRepoRoot", "internal/fabricengine"},
	{"CommitWeftPaths", "internal/fabricengine"},
	{"WarpLyxLink", "internal/fabricengine"},
	{"WEFT_SKIP_GIT", "fabric's env-var name"},
	{"WEFT_SKIP_PUSH", "fabric's env-var name"},
	{"ResetPairWarp", "internal/fabricengine, a *Fabric method"},
	{"WarpWorktree", "internal/fabricengine, a result field of Prune, Reconcile and Status"},
	{"side=warp", "internal/fabricengine, the mutation record's trace detail"},
	{"warp_branch_deleted", "internal/fabriccli, an envelope key"},
	{"warp_branch_kept_reason", "internal/fabriccli, an envelope key"},
	{"weft sibling", "internal/fabricengine, RequireWarpWorktree's refusal wording"},
	{".weft", "internal/fabricengine, the records worktree's lock directory name"},
	{"warp.ResetHard(", "internal/fabricengine, the code-side handle's raw call the destructive guard bans"},
	{"weft.ResetHard(", "internal/fabricengine, the records-side handle's raw call the destructive guard bans"},
	{"warpprobe.go", "internal/fabricengine, a file name"},
	{"warpbinding.go", "internal/fabricengine, a file name"},
	{"weftgit.go", "internal/fabricengine, a file name"},
	{"lyx weft sync", "internal/fabricengine, a retired spelling RefScanner still recognizes"},
	{"lyx warp checkout", "internal/fabricengine, a retired spelling RefScanner still recognizes"},
	{"lyx.exe weft push", "internal/fabricengine, a retired spelling RefScanner still recognizes"},
}

// vocabExemptPattern matches any vocabExempt token, bounded by a word edge wherever the token
// itself begins or ends with a word character.
var vocabExemptPattern = func() *regexp.Regexp {
	isWord := func(r byte) bool {
		return r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
	}
	alts := make([]string, len(vocabExempt))
	for i, e := range vocabExempt {
		alt := regexp.QuoteMeta(e.token)
		if isWord(e.token[0]) {
			alt = `\b` + alt
		}
		if isWord(e.token[len(e.token)-1]) {
			alt += `\b`
		}
		alts[i] = alt
	}
	return regexp.MustCompile(`(?:` + strings.Join(alts, "|") + `)`)
}()

// weftnamePackagePattern matches the weftname package name as a whole word.
var weftnamePackagePattern = regexp.MustCompile(`\bweftname\b`)

// vocabScanAllowlistEntry is one path the extended scan skips; a path ending in "/" is a prefix.
type vocabScanAllowlistEntry struct {
	path   string
	reason string
}

// vocabScanAllowlist is the closed list of paths the extended bare weft/warp scan skips.
var vocabScanAllowlist = []vocabScanAllowlistEntry{
	{"docs/benchmarks/", "measurement reports record one run and are not rewritten"},
	{"docs/research/", "measurement reports record one run and are not rewritten"},
	{"internal/lyxcwd/enforcement_test.go", "this scan's own test file spells the tokens it bans"},
	{"internal/lyxcwd/vocabscan_test.go", "this scan's own test file spells the tokens it bans"},
}

// vocabScanSkippedRoots are the first path segments the extended scan never enters.
// sandbox/ is the repo-root fixture tree; tools/sandbox/ is scanned.
var vocabScanSkippedRoots = map[string]bool{"_lyx": true, ".lyx": true, "sandbox": true}

// vocabScanDocRoots are the roots whose .md, .yaml and .go files the extended scan covers.
var vocabScanDocRoots = map[string]bool{"contracts": true, "docs": true, "plugins": true, "crucible": true, "cmd": true}

// vocabScanRootFiles are the repo-root files the extended scan covers.
var vocabScanRootFiles = map[string]bool{"CLAUDE.md": true, "README.md": true}

func vocabScanAllowlisted(rel string) bool {
	for _, e := range vocabScanAllowlist {
		if rel == e.path || (strings.HasSuffix(e.path, "/") && strings.HasPrefix(rel, e.path)) {
			return true
		}
	}
	return false
}

// fabricVocabularyFailures applies the fabric-vocabulary rules to the tree under base (the module
// root when empty) and returns one failure line per violation plus how many files it visited.
//
// Bare weft/warp rule, outside the owner set (configsync's literal-and-comment carve-out aside):
// every *_test.go file anywhere; every non-test .go file under internal/ and the doc roots; every
// .md and .yaml file under the doc roots; every .md under internal/; CLAUDE.md and README.md.
// Host-phrase rule: non-test .go under internal/ and cmd/, and .md under internal/ and contracts/stencils/.
// Weftname import rule: non-test .go under internal/ and cmd/, and every test file but weftnameTestImporter.
func fabricVocabularyFailures(t *testing.T, base string) ([]string, int) {
	t.Helper()
	var failures []string
	fail := func(relPath, reason string) {
		failures = append(failures, relPath+": "+reason)
	}

	scanned := scankit.Walk(t, scankit.Options{Base: base, Exts: []string{".go", ".md", ".yaml"}, Filter: scankit.All}, func(file *scankit.File) {
		rel := file.Rel
		first, _, _ := strings.Cut(rel, "/")
		if vocabScanSkippedRoots[first] || vocabScanAllowlisted(rel) {
			return
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		inInternal := first == "internal"
		inDocRoot := vocabScanDocRoots[first]

		switch filepath.Ext(rel) {
		case ".go":
			isTest := strings.HasSuffix(rel, "_test.go")
			bareScope := isTest || inInternal || inDocRoot
			hostScope := !isTest && (inInternal || first == "cmd")
			if !bareScope && !hostScope {
				return
			}
			f := file.AST(t, parser.ParseComments|parser.SkipObjectResolution)
			bareIdent, bareLiteralOrComment, hostHit := fabricVocabularyHits(f)
			if bareScope && !shouldSkipBareVocabularyCheck(dir) && failsBareVocabularyCheck(dir, bareIdent, bareLiteralOrComment) {
				fail(rel, "bare weft/warp token outside the owner set")
			}
			if hostScope && hostHit {
				fail(rel, "fabric-sense host phrase")
			}
			importRuleScope := hostScope || (isTest && rel != weftnameTestImporter)
			if importRuleScope && !weftnameImportOwners[dir] && importsWeftname(f) {
				fail(rel, "imports internal/weftname outside its owner set")
			}
		default:
			text := string(file.Data)
			ext := filepath.Ext(rel)
			bareScope := (ext == ".md" && (inInternal || vocabScanRootFiles[rel])) || inDocRoot
			hostScope := ext == ".md" && (inInternal || strings.HasPrefix(rel, "contracts/stencils/"))
			if bareScope && !fabricVocabularyOwners[dir] && bareVocabularyToken(text) {
				fail(rel, "bare weft/warp token outside the owner set")
			}
			if hostScope && fabricSenseHostPhrase(text) {
				fail(rel, "fabric-sense host phrase")
			}
		}
	})
	return failures, scanned
}

// hostGeometryIdentifiers are the fabric-geometry identifiers fabric-vocabulary-rule names as the
// identifier form of the host phrase predicate: host is never policed as a bare word, but these
// identifiers compound it with a repo/geometry noun exactly as the phrase form does. Matched
// case-insensitively against an *ast.Ident's Name.
var hostGeometryIdentifiers = map[string]bool{
	"hostbranch":    true,
	"hostlayoutfor": true,
	"hostreason":    true,
	"hostjunction":  true,
	"hostclean":     true,
}

// hostPhrases are the fabric-sense "host X" phrases fabric-vocabulary-rule polices, checked
// case-insensitively in both spaced and hyphenated form. host itself is never policed as a bare
// word -- see fabricSenseHostPhrase.
var hostPhrases = []string{
	"host repo", "host repository", "host worktree", "host working tree",
	"host checkout", "host branch", "host junction", "host path", "host side", "host head",
}

// bareVocabularyToken reports whether s contains, case-insensitively, the bare token "weft" or
// "warp" anywhere as a substring. Unlike host, weft/warp are banned wherever they occur -- fabric
// has no other meaning for them in this repo -- so substring matching (not whole-word matching)
// is deliberate: it is what catches the token inside a camelCase identifier such as
// WeftWorktree, not just a standalone word.
// The vocabExempt tokens are stripped whole first, so only a spelling outside that closed list hits.
func bareVocabularyToken(s string) bool {
	return bareVocabularyTokenIn(s, false)
}

// bareVocabularyTokenIn is bareVocabularyToken with the weftname package name additionally
// stripped when weftnameOK, for a file whose weftname import the import rule already governs.
// Both patterns only strip spellings that themselves contain weft or warp,
// so text lacking both tokens returns false before either regex runs.
func bareVocabularyTokenIn(s string, weftnameOK bool) bool {
	if lower := strings.ToLower(s); !strings.Contains(lower, "weft") && !strings.Contains(lower, "warp") {
		return false
	}
	s = vocabExemptPattern.ReplaceAllString(s, "")
	if weftnameOK {
		s = weftnamePackagePattern.ReplaceAllString(s, "")
	}
	lower := strings.ToLower(s)
	return strings.Contains(lower, "weft") || strings.Contains(lower, "warp")
}

// fabricSenseHostPhrase reports whether s contains a fabric-sense "host" phrase, case-
// insensitively, in either spaced or hyphenated form. The verb sense ("cannot host a strand"),
// the machine/OS sense ("a non-Windows test host"), and the PowerShell Write-Host cmdlet must all
// return false here -- that is the entire reason host is policed as a phrase and not a bare word.
func fabricSenseHostPhrase(s string) bool {
	lower := strings.ToLower(s)
	for _, phrase := range hostPhrases {
		if strings.Contains(lower, phrase) || strings.Contains(lower, strings.ReplaceAll(phrase, " ", "-")) {
			return true
		}
	}
	return false
}

// shouldSkipBareVocabularyCheck reports whether dir is exempt from the bare weft/warp check
// entirely: every fabricVocabularyOwners row except configsyncOwnerDir, whose narrower carve-out
// is applied by failsBareVocabularyCheck instead of a blanket skip.
func shouldSkipBareVocabularyCheck(dir string) bool {
	return fabricVocabularyOwners[dir] && dir != configsyncOwnerDir
}

// failsBareVocabularyCheck applies fabric-vocabulary-rule's owner-set carve-outs to a file's bare
// weft/warp hits. configsyncOwnerDir's row is narrower than a full skip: it passes on a
// literal-or-comment-only hit (the on-disk legacy config filenames it must name verbatim) but
// still fails on an identifier hit, since identifiers are never carved out there. Every other
// directory reaching this function is already known non-owner (shouldSkipBareVocabularyCheck
// exempts every other owner row before this is called), so it fails on either kind of hit.
func failsBareVocabularyCheck(dir string, bareIdent, bareLiteralOrComment bool) bool {
	if dir == configsyncOwnerDir {
		return bareIdent
	}
	return bareIdent || bareLiteralOrComment
}

// fabricVocabularyHits inspects a parsed, comment-carrying Go AST file and reports three
// independent hits: a bare weft/warp token inside an identifier, a bare weft/warp token inside a
// string literal or a comment, and a fabric-sense host phrase anywhere (identifiers, literals, or
// comments alike -- host's owner-set carve-out does not distinguish by kind, so the split does
// not matter for it). f must have been parsed with parser.ParseComments so f.Comments is
// populated.
func fabricVocabularyHits(f *ast.File) (bareIdent, bareLiteralOrComment, hostHit bool) {
	weftnameOK := importsWeftname(f)
	for _, cg := range f.Comments {
		text := cg.Text()
		if bareVocabularyTokenIn(text, weftnameOK) {
			bareLiteralOrComment = true
		}
		if fabricSenseHostPhrase(text) {
			hostHit = true
		}
	}

	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Ident:
			if bareVocabularyTokenIn(node.Name, weftnameOK) {
				bareIdent = true
			}
			if hostGeometryIdentifiers[strings.ToLower(node.Name)] {
				hostHit = true
			}
		case *ast.BasicLit:
			if node.Kind == token.STRING {
				if bareVocabularyTokenIn(node.Value, weftnameOK) {
					bareLiteralOrComment = true
				}
				if fabricSenseHostPhrase(node.Value) {
					hostHit = true
				}
			}
		}
		return true
	})
	return bareIdent, bareLiteralOrComment, hostHit
}

// importsWeftname reports whether f imports internal/weftname.
func importsWeftname(f *ast.File) bool {
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err == nil && path == weftnameImportPath {
			return true
		}
	}
	return false
}

// TestEnforcement_FabricVocabulary is the machine check that keeps the fabric-weft-visibility leak this task closes from reopening.
// The bare-token rule fails any file in fabricVocabularyFailures's scope, outside the owner set,
// that contains "weft" or "warp" (in an identifier, a string literal or a comment) beyond the vocabExempt spellings;
// the scope is every test file, the non-test Go and the doc files under the roots it names,
// and the root CLAUDE.md and README.md, minus vocabScanAllowlist.
// The host-phrase rule fails any production file or markdown body in its scope, owner set or not --
// host is retired, not merely scoped, so the owner set never carves out a host hit.
// The import rule fails any file outside {fabricengine, fabriccli, gitkit, hubforge} that imports internal/weftname,
// except weftnameTestImporter, which tests weftname.SiblingPath.
// The walk is plain, not a //go:embed parse, so a future non-embedded template is policed rather than silently skipped.
func TestEnforcement_FabricVocabulary(t *testing.T) {
	parseWithComments := func(t *testing.T, src string) *ast.File {
		t.Helper()
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "<fixture>", src, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse fixture: %v", err)
		}
		return f
	}

	t.Run("predicate", func(t *testing.T) {
		t.Run("weft_in_identifier_fails", func(t *testing.T) {
			f := parseWithComments(t, "package p\n\nvar weftPath string\n")
			bareIdent, _, _ := fabricVocabularyHits(f)
			if !bareIdent {
				t.Error("expected a bare weft identifier to be detected")
			}
		})

		t.Run("warp_in_string_literal_fails", func(t *testing.T) {
			f := parseWithComments(t, `package p; var _ = "warp"`)
			_, bareLiteralOrComment, _ := fabricVocabularyHits(f)
			if !bareLiteralOrComment {
				t.Error("expected a bare warp string literal to be detected")
			}
		})

		t.Run("weft_in_comment_fails", func(t *testing.T) {
			f := parseWithComments(t, "package p\n\n// discusses weft here\nvar _ = 1\n")
			_, bareLiteralOrComment, _ := fabricVocabularyHits(f)
			if !bareLiteralOrComment {
				t.Error("expected a bare weft comment to be detected")
			}
		})

		t.Run("embedded_md_style_body_with_weft_fails", func(t *testing.T) {
			if !bareVocabularyToken("This template mentions the weft side directory.") {
				t.Error("expected weft in a markdown-style body to be detected")
			}
		})

		t.Run("owner_set_file_skips_bare_token_rule_only", func(t *testing.T) {
			// An owner-set file skips only the bare weft/warp rule in the tree-scan; the
			// host-phrase rule reaches it exactly as it reaches every other file, since host
			// is retired everywhere rather than merely scoped away from the owner set.
			if !shouldSkipBareVocabularyCheck("internal/fabricengine") {
				t.Error("expected internal/fabricengine to skip the bare-token check entirely")
			}
			f := parseWithComments(t, "package fabricengine\n\nvar hostBranch string\n")
			_, _, hostHit := fabricVocabularyHits(f)
			if !hostHit {
				t.Error("expected the host-phrase rule to still fire for internal/fabricengine")
			}
		})

		t.Run("configsync_row_passes_on_literal_and_comment_but_fails_on_identifier", func(t *testing.T) {
			f := parseWithComments(t, "package configsync\n\n"+
				"// legacyFabricConfigModules names the pre-cutover warp.yaml/weft.yaml files.\n"+
				"var legacyFabricConfigModules = []string{\"warp\", \"weft\"}\n")
			bareIdent, bareLiteralOrComment, _ := fabricVocabularyHits(f)
			if bareIdent {
				t.Error("literal-and-comment-only fixture unexpectedly produced an identifier hit")
			}
			if !bareLiteralOrComment {
				t.Fatal("expected the literal/comment hit to be detected")
			}
			if failsBareVocabularyCheck(configsyncOwnerDir, bareIdent, bareLiteralOrComment) {
				t.Error("configsync row must pass on a literal/comment-only hit")
			}

			identFixture := parseWithComments(t, "package configsync\n\nvar weftModule = \"m\"\n")
			identHit, _, _ := fabricVocabularyHits(identFixture)
			if !identHit {
				t.Fatal("expected the identifier hit to be detected")
			}
			if !failsBareVocabularyCheck(configsyncOwnerDir, identHit, true) {
				t.Error("configsync row must still fail on an identifier hit")
			}
		})

		t.Run("host_repo_phrase_fails", func(t *testing.T) {
			if !fabricSenseHostPhrase("commit the card to the host repo") {
				t.Error(`expected "the host repo" to be detected as a fabric-sense host phrase`)
			}
		})

		t.Run("hostBranch_identifier_fails", func(t *testing.T) {
			f := parseWithComments(t, "package p\n\nvar hostBranch string\n")
			_, _, hostHit := fabricVocabularyHits(f)
			if !hostHit {
				t.Error("expected the hostBranch identifier to be detected")
			}
		})

		t.Run("host_verb_sense_passes", func(t *testing.T) {
			if fabricSenseHostPhrase("a downed reed session cannot host a strand") {
				t.Error("the verb sense of host must not be flagged")
			}
		})

		t.Run("host_machine_sense_passes", func(t *testing.T) {
			if fabricSenseHostPhrase("a non-Windows test host") {
				t.Error("the machine/OS sense of host must not be flagged")
			}
		})

		t.Run("write_host_cmdlet_passes", func(t *testing.T) {
			if fabricSenseHostPhrase(`Write-Host "done"`) {
				t.Error("the PowerShell Write-Host cmdlet must not be flagged")
			}
		})

		t.Run("host_phrase_in_owner_dir_now_fails", func(t *testing.T) {
			// Drive it the way the tree-scan does: get hostHit from fabricVocabularyHits on
			// a fixture carrying a policed phrase, then apply the tree-scan's own (now
			// unconditional) condition for "internal/fabricengine" -- an owner dir. Before
			// this card the condition also gated on !fabricVocabularyOwners[dir], so this
			// hit passed; now it fails, which is the whole point of the tightening.
			f := parseWithComments(t, "package fabricengine\n\n// commit the card to the host repo\nvar _ = 1\n")
			_, _, hostHit := fabricVocabularyHits(f)
			if !hostHit {
				t.Fatal("expected the fixture's host repo phrase to be detected")
			}
			const dir = "internal/fabricengine"
			if !fabricVocabularyOwners[dir] {
				t.Fatal("internal/fabricengine must still be an owner dir for the bare weft/warp rule")
			}
			if !hostHit {
				t.Error("expected the tree-scan's tightened condition (hostHit alone) to fail inside an owner dir")
			}
		})

		t.Run("bare_owner_skip_unchanged", func(t *testing.T) {
			if !shouldSkipBareVocabularyCheck("internal/fabricengine") {
				t.Error("expected internal/fabricengine to still skip the bare weft/warp check")
			}
		})
	})

	t.Run("tree-scan", func(t *testing.T) {
		// The walk is a plain file walk, not a //go:embed parse, so a future non-embedded template
		// is policed rather than silently skipped.
		failures, scanned := fabricVocabularyFailures(t, "")
		scankit.RequireFloor(t, scanned, 1, "fabric vocabulary walk")

		if len(failures) > 0 {
			t.Errorf("fabric-vocabulary leak found:\n%s", strings.Join(failures, "\n"))
		}
	})
}
