// citations.go implements the loom discussion check's citation test: the paths and symbols a decision record cites must exist in the worktree.

package loomshed

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/quarry/glyph"
)

const (
	checkCitationPathMissing     = "citation-path-missing"
	checkCitationSymbolMissing   = "citation-symbol-missing"
	checkCitationSymbolAmbiguous = "citation-symbol-ambiguous"
)

// markWayForward ends the way forward of a finding about something the task may create or remove.
const markWayForward = "mark it ` (new)` or ` (removed)` right after the closing backtick"

// citationItem is one citation under test: its finding when a local check settled it, or the glyph to resolve.
type citationItem struct {
	citation discussionparser.Citation
	finding  *discussionparser.Finding
	glyph    string
}

// citationFindings tests the citations of a loom decision record against the tree at worktreeRoot and returns its findings in record order, with an empty Path the caller stamps.
// A marked citation and a token holding whitespace or `<` are not tested.
// Each remaining token is a path, a glyph or a qualified symbol, or none of them and then untested; all glyphs and qualified symbols are resolved in one index call, whose error is returned as is.
func citationFindings(worktreeRoot string, index planindex.Index, citations []discussionparser.Citation) ([]discussionparser.Finding, error) {
	var trackedRoots map[string]bool
	var packageDirs []string
	packageDirsRead := false

	var items []citationItem
	for _, citation := range citations {
		token := citation.Token
		if citation.Marked || strings.Contains(token, "<") || strings.ContainsFunc(token, unicode.IsSpace) {
			continue
		}
		switch {
		case isPathShaped(token):
			if trackedRoots == nil {
				roots, err := trackedRootSet(worktreeRoot)
				if err != nil {
					return nil, err
				}
				trackedRoots = roots
			}
			root, _, _ := strings.Cut(token, "/")
			if !trackedRoots[root] || pathExists(worktreeRoot, token) {
				continue
			}
			items = append(items, citationItem{citation: citation, finding: &discussionparser.Finding{
				Check:  checkCitationPathMissing,
				Detail: fmt.Sprintf("line %d: cited path `%s` does not exist; way forward: correct the path, or mark one the task creates or removes with ` (new)` or ` (removed)` right after the closing backtick", citation.Line, token),
			}})
		case isMemberGlyph(worktreeRoot, token):
			items = append(items, citationItem{citation: citation, glyph: token})
		default:
			pkg, member, ok := qualifiedSymbol(token)
			if !ok {
				continue
			}
			if !packageDirsRead {
				dirs, err := goPackageDirs(worktreeRoot)
				if err != nil {
					return nil, err
				}
				packageDirs, packageDirsRead = dirs, true
			}
			var matches []string
			for _, dir := range packageDirs {
				if path.Base(dir) == pkg {
					matches = append(matches, dir)
				}
			}
			switch len(matches) {
			case 0:
			case 1:
				items = append(items, citationItem{citation: citation, glyph: matches[0] + "#" + member})
			default:
				items = append(items, citationItem{citation: citation, finding: &discussionparser.Finding{
					Check:  checkCitationSymbolAmbiguous,
					Detail: fmt.Sprintf("line %d: cited symbol `%s` matches packages in %s; way forward: spell the symbol as a glyph", citation.Line, token, strings.Join(matches, ", ")),
				}})
			}
		}
	}

	var glyphs []string
	for _, item := range items {
		if item.glyph != "" {
			glyphs = append(glyphs, item.glyph)
		}
	}
	var answers []planindex.GlyphAnswer
	if len(glyphs) > 0 {
		var err error
		if answers, err = index.ResolveGlyphs(worktreeRoot, glyphs); err != nil {
			return nil, err
		}
	}

	var findings []discussionparser.Finding
	next := 0
	for _, item := range items {
		if item.finding != nil {
			findings = append(findings, *item.finding)
			continue
		}
		answer := answers[next]
		next++
		switch answer.Verdict {
		case planindex.GlyphMissing:
			findings = append(findings, discussionparser.Finding{
				Check:  checkCitationSymbolMissing,
				Detail: fmt.Sprintf("line %d: cited symbol `%s` does not resolve; way forward: correct it as `lym quarry glyphs <dir>` lists the members, or %s", item.citation.Line, item.citation.Token, markWayForward),
			})
		case planindex.GlyphAmbiguous:
			findings = append(findings, discussionparser.Finding{
				Check:  checkCitationSymbolAmbiguous,
				Detail: fmt.Sprintf("line %d: cited symbol `%s` is declared in several places: %s; way forward: cite the declaring file meant by its path in place of the symbol", item.citation.Line, item.citation.Token, strings.Join(answer.Candidates, ", ")),
			})
		}
	}
	return findings, nil
}

// isPathShaped reports whether token reads as a repository path: it holds a slash, none of the characters of a glyph, pattern or placeholder, and no `...` segment.
func isPathShaped(token string) bool {
	if !strings.Contains(token, "/") || strings.ContainsAny(token, "#>*{}$:") {
		return false
	}
	for _, segment := range strings.Split(token, "/") {
		if segment == "..." {
			return false
		}
	}
	return true
}

// trackedRootSet returns the root entries git tracks in the worktree, without the two lym directories.
func trackedRootSet(worktreeRoot string) (map[string]bool, error) {
	entries, err := gitrepo.New(worktreeRoot).TrackedRootEntries()
	if err != nil {
		return nil, err
	}
	roots := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry != lymdirs.LymDirName && entry != lymdirs.DotLymDirName {
			roots[entry] = true
		}
	}
	return roots, nil
}

// pathExists reports whether token names an existing entry under worktreeRoot, a directory when it ends in a slash.
func pathExists(worktreeRoot, token string) bool {
	info, err := os.Stat(filepath.Join(worktreeRoot, filepath.FromSlash(strings.TrimSuffix(token, "/"))))
	if err != nil {
		return false
	}
	return info.IsDir() || !strings.HasSuffix(token, "/")
}

// isMemberGlyph reports whether token is a Go member glyph `<unit>#<member>` whose unit holds a slash or a dot and does not name an existing worktree file other than a `.go` or `.cs` one.
func isMemberGlyph(worktreeRoot, token string) bool {
	if strings.HasPrefix(token, "plan:") {
		return false
	}
	parsed, err := glyph.Parse(glyph.Go, token)
	if err != nil || parsed.IsSelf() {
		return false
	}
	unitPath, _ := parsed.UnitPath()
	if !strings.ContainsAny(unitPath, "/.") {
		return false
	}
	if info, err := os.Stat(filepath.Join(worktreeRoot, filepath.FromSlash(unitPath))); err == nil && !info.IsDir() {
		extension := path.Ext(unitPath)
		return extension == ".go" || extension == ".cs"
	}
	return true
}

// qualifiedSymbol splits a `pkg.Member` or `pkg.Member.Sub…` token, ignoring a trailing parenthesised argument list, into its package name and first member.
// The member must hold an upper-case letter, and every part must be an identifier.
func qualifiedSymbol(token string) (pkg, member string, ok bool) {
	if strings.HasSuffix(token, ")") {
		if open := strings.Index(token, "("); open >= 0 {
			token = token[:open]
		}
	}
	if strings.Contains(token, "/") {
		return "", "", false
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return "", "", false
	}
	for _, part := range parts {
		if !isIdentifier(part) {
			return "", "", false
		}
	}
	if !strings.ContainsFunc(parts[1], unicode.IsUpper) {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// isIdentifier reports whether s is a non-empty run of letters, digits and underscores that does not start with a digit.
func isIdentifier(s string) bool {
	for i, r := range s {
		if !unicode.IsLetter(r) && r != '_' && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return s != ""
}

// goPackageDirs walks worktreeRoot and returns, sorted and slash-separated, every directory holding a `.go` file.
// It skips a directory named `testdata` and any whose name starts with `_` or `.`, and the root itself.
func goPackageDirs(worktreeRoot string) ([]string, error) {
	seen := map[string]bool{}
	err := filepath.WalkDir(worktreeRoot, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if current != worktreeRoot && (name == "testdata" || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") {
			relative, err := filepath.Rel(worktreeRoot, filepath.Dir(current))
			if err != nil {
				return err
			}
			if relative != "." {
				seen[filepath.ToSlash(relative)] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(seen))
	for dir := range seen {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs, nil
}
