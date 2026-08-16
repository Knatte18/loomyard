// enforcement_test.go is a repo-wide guard: it walks every package and fails the build if any file
// outside internal/lyxcwd reaches for raw cwd or top-level git geometry, keeps internal/lyxcwd
// the sole geometry owner, and (via TestEnforcement_FabricVocabulary) keeps the fabric-vocabulary
// leak fabric-weft-visibility-cleanup closed. All three enforcement tests share one
// filepath.WalkDir-based helper, walkEnforcementRoots, so the walk semantics (skip .git/testdata,
// suffix-filter files, hand each match to a per-file callback) live in exactly one place.

package lyxcwd

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
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

// repoRootForEnforcement resolves the repository root relative to this test file's own location,
// so every enforcement walk shares one runtime.Caller(0) resolution instead of repeating it.
func repoRootForEnforcement(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	// Two levels up from internal/lyxcwd/enforcement_test.go -> repo root.
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

// walkEnforcementRoots walks every root in roots (each a repoRoot-relative path, "." for the
// whole tree) and invokes fn once per file whose name ends with one of suffixes, passing the
// file's repoRoot-relative, slash-normalized path and its raw bytes. Directories named ".git" or
// containing "testdata" are skipped entirely; any further filtering (e.g. excluding *_test.go, or
// applying an owner allowlist) is the caller's responsibility, since that rule differs across
// TestEnforcement, TestEnforcement_GeometryLiterals, and TestEnforcement_FabricVocabulary.
func walkEnforcementRoots(t *testing.T, repoRoot string, roots []string, suffixes []string, fn func(relPath string, data []byte)) {
	t.Helper()

	hasSuffix := func(name string) bool {
		for _, suffix := range suffixes {
			if strings.HasSuffix(name, suffix) {
				return true
			}
		}
		return false
	}

	for _, root := range roots {
		walkRoot := filepath.Join(repoRoot, filepath.FromSlash(root))
		err := filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == ".git" || strings.Contains(d.Name(), "testdata")) {
				return filepath.SkipDir
			}
			if d.IsDir() || !hasSuffix(d.Name()) {
				return nil
			}

			relPath, relErr := filepath.Rel(repoRoot, path)
			if relErr != nil {
				return relErr
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			fn(filepath.ToSlash(relPath), data)
			return nil
		})
		if err != nil {
			t.Fatalf("failed to walk %s: %v", root, err)
		}
	}
}

// TestEnforcement walks the repo source tree and verifies that no source file outside
// internal/lyxcwd and cmd/lyx contains the raw cwd/root primitives os.Getwd or git rev-parse
// --show-toplevel.
func TestEnforcement(t *testing.T) {
	t.Run("tree-scan", func(t *testing.T) {
		repoRoot := repoRootForEnforcement(t)

		// Predicate: returns true if the bytes contain a banned token.
		isBanned := func(data []byte) bool {
			content := string(data)
			return strings.Contains(content, "os.Getwd") ||
				strings.Contains(content, "--show-toplevel")
		}

		var failures []string

		walkEnforcementRoots(t, repoRoot, []string{"."}, []string{".go"}, func(relPath string, data []byte) {
			// Skip _test.go files.
			if strings.HasSuffix(relPath, "_test.go") {
				return
			}

			pkgDir := filepath.ToSlash(filepath.Dir(relPath))

			// Check allowlist: internal/lyxcwd, cmd/lyx/main.go
			isAllowed := pkgDir == "internal/lyxcwd" ||
				(pkgDir == "cmd/lyx" && filepath.Base(relPath) == "main.go")

			// Skip files in the allowlist (they are allowed to contain banned tokens).
			if isAllowed {
				return
			}

			// Check the file for banned tokens. Comments are stripped first so
			// that a file which merely *names* a banned token in an explanatory
			// comment (e.g. scoutcli/cli.go documenting why lyxcwd.Getwd
			// is the only permitted os.Getwd caller) is not falsely flagged; the
			// guard is about real code usage, not prose.
			if isBanned(stripGoComments(data)) {
				failures = append(failures, relPath)
			}
		})

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
		case "_board", "-weft", "-HUB", "_portals", "_launchers", "_lyx", ".lyx":
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
		// "_board" and "-HUB" are dual-owned: internal/lyxcwd keeps a private
		// boardDir/boardDirName pair (readRecordedAnchor's sole remaining
		// reason to know the name) and a private hubSuffix const
		// (Location.RepoName derives from it), while internal/fabricengine
		// owns the exported BoardDir/HubPath constructors every other
		// caller uses. The duplication is sanctioned by this map, not a leak.
		"_board": {"internal/lyxcwd", "internal/fabricengine"},
		"-weft":  {"internal/weftname"},
		"-HUB":   {"internal/lyxcwd", "internal/fabricengine"},
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
				name: "const_HUB",
				src:  `package p; const s = "-HUB"`,
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
		repoRoot := repoRootForEnforcement(t)

		var scanned int
		var failures []string

		walkEnforcementRoots(t, repoRoot, []string{"."}, []string{".go"}, func(relPath string, data []byte) {
			// Only scan production Go files; test files (*_test.go) are excluded because
			// test geometry is a review-only rule, not a machine-enforced invariant.
			if strings.HasSuffix(relPath, "_test.go") {
				return
			}

			relDir := filepath.ToSlash(filepath.Dir(relPath))

			fset := token.NewFileSet()
			f, parseErr := parser.ParseFile(fset, relPath, data, parser.SkipObjectResolution)
			if parseErr != nil {
				// Skip files that cannot be parsed (e.g. build-tag-guarded platform files).
				return
			}
			scanned++
			for _, tok := range geometryLiteralTokensInConstructionContext(f) {
				if !tokenOwnedByDir(tok, relDir) {
					failures = append(failures, relPath)
					break
				}
			}
		})

		// Sanity check: at least one production file outside internal/lyxcwd must have
		// been scanned so a misconfigured walk (wrong root, all files skipped) cannot
		// silently produce a vacuous all-pass result.
		t.Run("scanned_non_empty", func(t *testing.T) {
			if scanned == 0 {
				t.Error("geometry-literal guard: no production Go files scanned outside internal/lyxcwd; the AST walk may be misconfigured")
			}
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
	// internal/hubforge is in the narrower weftname-import subset CONSTRAINTS.md's Fabric
	// Vocabulary Invariant already names, alongside internal/fabricengine, internal/fabriccli
	// and internal/gitkit -- this map is an allowlist of what may import weftname, not an
	// assertion of what does, so this entry is correct even though hub.go imports no weftname
	// identifier today.
	"internal/hubforge": true,
}

// weftnameImportPath is the fully-qualified import path TestEnforcement_FabricVocabulary's
// import rule polices.
const weftnameImportPath = "github.com/Knatte18/loomyard/internal/weftname"

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
func bareVocabularyToken(s string) bool {
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
	for _, cg := range f.Comments {
		text := cg.Text()
		if bareVocabularyToken(text) {
			bareLiteralOrComment = true
		}
		if fabricSenseHostPhrase(text) {
			hostHit = true
		}
	}

	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Ident:
			if bareVocabularyToken(node.Name) {
				bareIdent = true
			}
			if hostGeometryIdentifiers[strings.ToLower(node.Name)] {
				hostHit = true
			}
		case *ast.BasicLit:
			if node.Kind == token.STRING {
				if bareVocabularyToken(node.Value) {
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

// TestEnforcement_FabricVocabulary is the machine check that keeps the fabric-weft-visibility
// leak this task closes from reopening. Per decisions enforcement-test and
// fabric-vocabulary-rule, it fails any production .go file under internal/ or cmd/, outside the
// owner set, that contains the bare token "weft" or "warp" (in an identifier, a string literal,
// or a comment); it fails any such file, owner set or not, that contains a fabric-sense "host"
// phrase -- host is retired, not merely scoped, so the owner set never carves out a host hit. It
// also fails any file outside {fabricengine, fabriccli, gitkit, hubforge} that imports
// internal/weftname.
// It additionally walks every internal/**/*.md file (a plain walk, not a //go:embed parse, so a
// future non-embedded template is policed rather than silently skipped) for the same bare-token
// and host-phrase rules. *_test.go files are excluded from all three rules -- rule (3) included,
// since internal/lyxcwd/geometry_test.go legitimately imports internal/weftname to test
// weftname.SiblingPath.
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
			if !bareVocabularyToken("This template mentions the weft sibling directory.") {
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
		repoRoot := repoRootForEnforcement(t)

		var failures []string
		fail := func(relPath, reason string) {
			failures = append(failures, relPath+": "+reason)
		}

		// Rules (1)-(3): production .go files under internal/ and cmd/.
		walkEnforcementRoots(t, repoRoot, []string{"internal", "cmd"}, []string{".go"}, func(relPath string, data []byte) {
			if strings.HasSuffix(relPath, "_test.go") {
				return
			}

			dir := filepath.ToSlash(filepath.Dir(relPath))

			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, relPath, data, parser.ParseComments|parser.SkipObjectResolution)
			if err != nil {
				// Build-tag-guarded platform files may not fully parse; skip rather than
				// falsely flag or crash, matching the sibling enforcement tests' stance.
				return
			}

			bareIdent, bareLiteralOrComment, hostHit := fabricVocabularyHits(f)
			if !shouldSkipBareVocabularyCheck(dir) && failsBareVocabularyCheck(dir, bareIdent, bareLiteralOrComment) {
				fail(relPath, "bare weft/warp token outside the owner set")
			}
			if hostHit {
				fail(relPath, "fabric-sense host phrase")
			}
			if !weftnameImportOwners[dir] && importsWeftname(f) {
				fail(relPath, "imports internal/weftname outside its owner set")
			}
		})

		// Coverage additionally includes a plain internal/**/*.md and contracts/stencils/**/*.md walk --
		// not a //go:embed parse, so a future non-embedded template is policed rather than
		// silently skipped. contracts/stencils/ is a walked root alongside internal/ so a prompt
		// relocated out of internal/ (see contracts/stencils/stencils.go) does not silently leave
		// Fabric Vocabulary coverage.
		mdVisitCount := 0
		walkEnforcementRoots(t, repoRoot, []string{"internal", "contracts/stencils"}, []string{".md"}, func(relPath string, data []byte) {
			mdVisitCount++
			dir := filepath.ToSlash(filepath.Dir(relPath))
			text := string(data)

			if !fabricVocabularyOwners[dir] && bareVocabularyToken(text) {
				fail(relPath, "bare weft/warp token outside the owner set")
			}
			if fabricSenseHostPhrase(text) {
				fail(relPath, "fabric-sense host phrase")
			}
		})
		if mdVisitCount == 0 {
			t.Fatal("the internal/stencils .md walk visited zero files; want a mistyped or missing root to fail loudly rather than pass vacuously")
		}

		if len(failures) > 0 {
			t.Errorf("fabric-vocabulary leak found:\n%s", strings.Join(failures, "\n"))
		}
	})
}
