// testselect.go selects an importing package's tests from its coverage map:
// the tests that covered a function a linked package changed since the map's commit.
// It names instead the miss that runs the package whole when the map cannot speak for a change.

package testcover

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// embedDirective opens a `//go:embed` line, whose arguments are the patterns of the files it embeds.
const embedDirective = "//go:embed"

// CommitReader reads a file as a commit stores it.
// rel is slash-separated and relative to the worktree root.
// found is false with a nil error when rel is absent at rev; an error means rev or its tree cannot be read.
type CommitReader interface {
	CodeFileAt(rev, rel string) (data []byte, found bool, err error)
}

// SelectorConfig is the working tree a Selector compares coverage maps with.
type SelectorConfig struct {
	// MapsDir is the directory holding the per-package coverage maps.
	MapsDir string
	// WorktreeRoot is the worktree's absolute path.
	WorktreeRoot string
	// ModuleRel is the module's slash directory relative to the worktree root, "." for the root.
	ModuleRel string
	// ModulePath is the module's path, as its go.mod declares it.
	ModulePath string
	// GoRoot is the Go toolchain's root directory, whose src directory holds the standard library.
	// It must be set: with it empty, no import counts as standard library.
	GoRoot string
	// WorkingFiles are the worktree-relative slash paths of every working-tree file that git does not ignore.
	// It holds tracked and untracked files alike.
	WorkingFiles []string
	// Commits reads files at a map's commit.
	Commits CommitReader
	// Inert reports whether a worktree-relative slash path matches the hub's inert_paths; nil matches none.
	Inert func(rel string) bool
}

// MissReason names why a coverage map cannot select an importing package's tests.
type MissReason string

// The reasons a Selector misses for.
const (
	MissNoMap            MissReason = "no coverage map"
	MissMapUnreadable    MissReason = "coverage map unreadable"
	MissBuildFailure     MissReason = "coverage map records a build failure"
	MissUnmarked         MissReason = "coverage map unmarked"
	MissCommitUnreadable MissReason = "map commit unreadable"
	MissOwnFiles         MissReason = "own Go files changed"
	MissNonGoFile        MissReason = "non-Go file changed"
	MissParse            MissReason = "Go file does not parse"
	MissNonBody          MissReason = "change outside function declarations"
	MissDirective        MissReason = "compiler directive changed"
	MissMethodSet        MissReason = "method set changed"
	MissInit             MissReason = "init function added"
	MissPredeclared      MissReason = "predeclared name declared"
)

// Miss is why a coverage map cannot select an importing package's tests, so the package runs whole.
type Miss struct {
	Reason MissReason
	// Detail names the file, declaration or failure the reason is about.
	Detail string
}

// String renders the miss as its reason and detail.
func (m Miss) String() string {
	return string(m.Reason) + ": " + m.Detail
}

// Selection is the outcome of selecting one importing package's tests.
type Selection struct {
	// Tests are the selected tests' names, sorted, each once.
	Tests []string
	// Miss is set when the package runs whole instead, and Tests is then empty.
	Miss *Miss
	// Recorded are the tests the map records, in its order; empty with a Miss.
	// A caller tells from them which of the package's tests the map has no coverage for.
	Recorded []MapTest
}

// Selector selects importing packages' tests from their coverage maps against one working tree.
// It caches the working tree's file hashes, each directory's owned files and each Go file's comparison.
// Selecting many importing packages then reads each file once.
// The working tree must not change while it is in use.
// It is not safe for concurrent use.
type Selector struct {
	config SelectorConfig
	// owned holds the working files each package directory owns, by slash path relative to it.
	owned map[string]map[string]struct{}
	// hashes holds each working file's hash, by worktree-relative slash path.
	hashes map[string]workingHash
	// compared holds each Go file's comparison, by the map's commit and the file's worktree-relative slash path.
	compared map[string]goFileComparison
	// embeds holds the go:embed patterns of each package directory, read with or without its test files.
	embeds map[embedScope][]string
}

// embedScope keys the go:embed patterns of one package directory.
// withTests is set when the package's test files count, which holds for the importing package alone.
type embedScope struct {
	dir       string
	withTests bool
}

// workingHash is a working file's content hash, or its absence.
type workingHash struct {
	hash  string
	found bool
}

// NewSelector returns a Selector over the working tree config describes.
func NewSelector(config SelectorConfig) *Selector {
	return &Selector{
		config:   config,
		owned:    map[string]map[string]struct{}{},
		hashes:   map[string]workingHash{},
		compared: map[string]goFileComparison{},
		embeds:   map[embedScope][]string{},
	}
}

// Select selects the tests of the package pkg, a module-relative slash path, from its coverage map.
// A test is selected when it covered a linked package's function that changed or was removed since the map's commit.
// A test the spawn scan marks is selected whenever any such function changed, since its map sees only in-process code.
// A change the map cannot speak for gives a Miss instead.
// The error reports a working file that cannot be read.
func (s *Selector) Select(pkg string) (Selection, error) {
	m, found, err := ReadMap(s.config.MapsDir, pkg)
	if miss := mapMiss(m, found, err); miss != nil {
		return Selection{Miss: miss}, nil
	}
	ownDir := path.Join(s.config.ModuleRel, pkg)
	if miss, err := s.ownFilesMiss(ownDir, m); miss != nil || err != nil {
		return Selection{Miss: miss}, err
	}
	if miss := s.commitMiss(ownDir, m); miss != nil {
		return Selection{Miss: miss}, nil
	}
	changed := map[FuncKey]bool{}
	for _, linked := range slices.Sorted(maps.Keys(m.Linked)) {
		funcs, miss, err := s.linkedChanges(m, linked)
		if miss != nil || err != nil {
			return Selection{Miss: miss}, err
		}
		for _, key := range funcs {
			changed[key] = true
		}
	}
	return Selection{Tests: selectTests(m, changed), Recorded: m.Tests}, nil
}

// mapMiss returns the miss of a map a selection cannot use, or nil for a usable map.
// Unusable maps are missing, unreadable, failed to build, unmarked, or rewritten by another writer.
// found and err are ReadMap's.
func mapMiss(m Map, found bool, err error) *Miss {
	switch {
	case err != nil:
		return &Miss{Reason: MissMapUnreadable, Detail: err.Error()}
	case !found:
		return &Miss{Reason: MissNoMap, Detail: "no map was built for the package"}
	case m.BuildFailure != "":
		return &Miss{Reason: MissBuildFailure, Detail: m.BuildFailure}
	case m.Format == nil:
		return &Miss{Reason: MissUnmarked, Detail: "the map has no format marker"}
	case m.Format.Version != MapFormatVersion:
		return &Miss{Reason: MissUnmarked, Detail: fmt.Sprintf("the marker names format %d, not %d", m.Format.Version, MapFormatVersion)}
	case m.Format.Commit != m.Commit || !m.Format.BuiltAt.Equal(m.BuiltAt):
		return &Miss{Reason: MissUnmarked, Detail: "the marker's commit and build time differ from the map's, so another writer rewrote it"}
	}
	for _, test := range m.Tests {
		for _, index := range test.Funcs {
			if index < 0 || index >= len(m.Funcs) {
				return &Miss{Reason: MissMapUnreadable, Detail: fmt.Sprintf("test %s names function %d of a table of %d", test.Name, index, len(m.Funcs))}
			}
		}
	}
	return nil
}

// ownFilesMiss returns the miss of a package whose own Go files differ in the working tree from m's record.
// dir is the package's worktree-relative slash directory.
// Its other files that change miss as a linked package's would.
func (s *Selector) ownFilesMiss(dir string, m Map) (*Miss, error) {
	recorded := map[string]string{}
	maps.Copy(recorded, m.OtherFiles)
	maps.Copy(recorded, m.Files)
	changed, err := s.changedFiles(dir, recorded)
	if err != nil {
		return nil, err
	}
	for _, rel := range changed {
		if isGoSourceFile(rel) || isGoTestFile(rel) {
			return &Miss{Reason: MissOwnFiles, Detail: path.Join(dir, rel)}, nil
		}
		if miss, err := s.otherFileMiss(dir, rel, true); miss != nil || err != nil {
			return miss, err
		}
	}
	return nil, nil
}

// commitMiss returns the miss of a map whose commit the object store cannot read.
// It probes with one of the package's own Go files; dir is the package's worktree-relative slash directory.
func (s *Selector) commitMiss(dir string, m Map) *Miss {
	names := slices.Sorted(maps.Keys(m.Files))
	if len(names) == 0 {
		return nil
	}
	probe := path.Join(dir, names[0])
	_, found, err := s.config.Commits.CodeFileAt(m.Commit, probe)
	switch {
	case err != nil:
		return &Miss{Reason: MissCommitUnreadable, Detail: err.Error()}
	case !found:
		return &Miss{Reason: MissCommitUnreadable, Detail: fmt.Sprintf("%s is absent at the map's commit %s", probe, m.Commit)}
	}
	return nil
}

// linkedChanges returns the changed functions of the linked package at importPath, or the miss its changes give.
func (s *Selector) linkedChanges(m Map, importPath string) ([]FuncKey, *Miss, error) {
	dir := s.importDir(importPath)
	recorded := m.Linked[importPath]
	changed, err := s.changedFiles(dir, recorded)
	if err != nil {
		return nil, nil, err
	}
	var funcs []FuncKey
	methodsBefore, methodsAfter := map[string][]*ast.FuncDecl{}, map[string][]*ast.FuncDecl{}
	for _, rel := range changed {
		switch {
		case isGoTestFile(rel):
			continue
		case !isGoSourceFile(rel):
			if miss, err := s.otherFileMiss(dir, rel, false); miss != nil || err != nil {
				return nil, miss, err
			}
			continue
		}
		_, wasRecorded := recorded[rel]
		comparison, err := s.compareGoFile(m.Commit, path.Join(dir, rel), wasRecorded)
		if err != nil || comparison.miss != nil {
			return nil, comparison.miss, err
		}
		for _, imported := range comparison.addedImports {
			if !s.importAllowed(m, imported) {
				return nil, &Miss{Reason: MissNonBody, Detail: fmt.Sprintf(
					"%s adds an import of %s, which is neither in the map's linked set nor in the standard library", path.Join(dir, rel), imported)}, nil
			}
		}
		for _, removed := range comparison.removedImports {
			if !s.standardLibrary(removed) {
				return nil, &Miss{Reason: MissNonBody, Detail: fmt.Sprintf(
					"%s is deleted and imported %s, which may leave the build with it", path.Join(dir, rel), removed)}, nil
			}
		}
		for _, name := range comparison.changed {
			funcs = append(funcs, FuncKey{Package: importPath, File: rel, Func: name})
		}
		for key, decls := range comparison.methodsBefore {
			methodsBefore[key] = append(methodsBefore[key], decls...)
		}
		for key, decls := range comparison.methodsAfter {
			methodsAfter[key] = append(methodsAfter[key], decls...)
		}
	}
	if miss := methodSetMiss(dir, methodsBefore, methodsAfter); miss != nil {
		return nil, miss, nil
	}
	return funcs, nil, nil
}

// changedFiles returns the sorted files that differ from recorded in the package at dir.
// dir is a worktree-relative slash directory.
// recorded maps each file, by slash path relative to dir, to its content hash at the map's commit.
// A file on one side alone counts.
func (s *Selector) changedFiles(dir string, recorded map[string]string) ([]string, error) {
	owned := s.ownedFiles(dir)
	candidates := map[string]struct{}{}
	maps.Copy(candidates, owned)
	for rel := range recorded {
		candidates[rel] = struct{}{}
	}
	var changed []string
	for _, rel := range slices.Sorted(maps.Keys(candidates)) {
		var current workingHash
		if _, inWorkingTree := owned[rel]; inWorkingTree {
			var err error
			if current, err = s.workingHash(path.Join(dir, rel)); err != nil {
				return nil, err
			}
		}
		hash, wasRecorded := recorded[rel]
		if wasRecorded != current.found || hash != current.hash {
			changed = append(changed, rel)
		}
	}
	return changed, nil
}

// ownedFiles returns the working files that the package at the worktree-relative slash directory dir owns.
// Paths are slash paths relative to dir.
// A package owns the files under dir and under no directory below it that holds another package or module.
func (s *Selector) ownedFiles(dir string) map[string]struct{} {
	if owned, cached := s.owned[dir]; cached {
		return owned
	}
	var below []string
	for _, file := range s.config.WorkingFiles {
		rel, under := file, true
		if dir != "." {
			rel, under = strings.CutPrefix(file, dir+"/")
		}
		if under {
			below = append(below, rel)
		}
	}
	nested := NestedPackageDirs(below)
	owned := map[string]struct{}{}
	for _, rel := range below {
		if !slices.ContainsFunc(nested, func(nestedDir string) bool { return strings.HasPrefix(rel, nestedDir+"/") }) {
			owned[rel] = struct{}{}
		}
	}
	s.owned[dir] = owned
	return owned
}

// workingHash returns the hash of the working file at the worktree-relative slash path rel.
func (s *Selector) workingHash(rel string) (workingHash, error) {
	if current, cached := s.hashes[rel]; cached {
		return current, nil
	}
	data, found, err := s.readWorkingFile(rel)
	if err != nil {
		return workingHash{}, err
	}
	current := workingHash{found: found}
	if found {
		current.hash = ContentHash(data)
	}
	s.hashes[rel] = current
	return current, nil
}

// readWorkingFile reads the working file at the worktree-relative slash path rel; found is false for an absent file.
func (s *Selector) readWorkingFile(rel string) (data []byte, found bool, err error) {
	data, err = os.ReadFile(filepath.Join(s.config.WorktreeRoot, filepath.FromSlash(rel)))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, false, nil
	case err != nil:
		return nil, false, err
	}
	return data, true, nil
}

// compareGoFile compares the Go file at the worktree-relative slash path rel as commit stores it with the working tree's.
// wasRecorded tells whether the map's record holds the file, and so whether the commit does.
func (s *Selector) compareGoFile(commit, rel string, wasRecorded bool) (goFileComparison, error) {
	key := commit + "\x00" + rel
	if comparison, cached := s.compared[key]; cached {
		return comparison, nil
	}
	var before goSource
	if wasRecorded {
		data, found, err := s.config.Commits.CodeFileAt(commit, rel)
		switch {
		case err != nil:
			return missed(MissCommitUnreadable, err.Error()), nil
		case !found:
			return missed(MissCommitUnreadable, fmt.Sprintf("%s is absent at the map's commit %s", rel, commit)), nil
		}
		before = goSource{text: data, present: true}
	}
	data, found, err := s.readWorkingFile(rel)
	if err != nil {
		return goFileComparison{}, err
	}
	comparison := compareGoSources(rel, before, goSource{text: data, present: found})
	s.compared[key] = comparison
	return comparison, nil
}

// importAllowed reports whether a changed file of a linked package may add an import of importPath:
// a package m's linked set holds, or a standard-library package.
func (s *Selector) importAllowed(m Map, importPath string) bool {
	if _, linked := m.Linked[importPath]; linked {
		return true
	}
	return s.standardLibrary(importPath)
}

// standardLibrary reports whether importPath names a package of the standard library, a directory under GoRoot's src.
// It reports false when GoRoot is empty.
func (s *Selector) standardLibrary(importPath string) bool {
	if s.config.GoRoot == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(s.config.GoRoot, "src", filepath.FromSlash(importPath)))
	return err == nil && info.IsDir()
}

// otherFileMiss returns the miss of a changed file that is no Go file directly in the package at dir.
// dir is a worktree-relative slash directory, and rel is the file's slash path relative to dir.
// A file inert_paths matches is no miss, unless a go:embed pattern of the package names it.
// ownPackage is set when dir is the importing package, whose test files are in its binary, so their patterns count.
func (s *Selector) otherFileMiss(dir, rel string, ownPackage bool) (*Miss, error) {
	file := path.Join(dir, rel)
	if s.config.Inert == nil || !s.config.Inert(file) {
		return &Miss{Reason: MissNonGoFile, Detail: file}, nil
	}
	patterns, miss, err := s.embedPatterns(dir, ownPackage)
	if miss != nil || err != nil {
		return miss, err
	}
	if embedded(patterns, rel) {
		return &Miss{Reason: MissNonGoFile, Detail: file + ", which a go:embed pattern names"}, nil
	}
	return nil, nil
}

// embedPatterns returns the go:embed patterns of the package at dir, read from the working tree's Go files.
// dir is a worktree-relative slash directory; withTests also reads the package's test files.
// A Go file that does not parse gives its miss instead.
func (s *Selector) embedPatterns(dir string, withTests bool) ([]string, *Miss, error) {
	scope := embedScope{dir: dir, withTests: withTests}
	if patterns, cached := s.embeds[scope]; cached {
		return patterns, nil, nil
	}
	var patterns []string
	for _, rel := range slices.Sorted(maps.Keys(s.ownedFiles(dir))) {
		if !isGoSourceFile(rel) && !(withTests && isGoTestFile(rel)) {
			continue
		}
		file := path.Join(dir, rel)
		data, found, err := s.readWorkingFile(file)
		if err != nil {
			return nil, nil, err
		}
		if !found {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), "", data, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, &Miss{Reason: MissParse, Detail: fmt.Sprintf("%s in the working tree: %v", file, err)}, nil
		}
		for _, group := range parsed.Comments {
			for _, comment := range group.List {
				args, isEmbed := strings.CutPrefix(comment.Text, embedDirective)
				if !isEmbed || (args != "" && !unicode.IsSpace(rune(args[0]))) {
					continue
				}
				linePatterns, err := parseEmbedPatterns(args)
				if err != nil {
					return nil, &Miss{Reason: MissParse, Detail: fmt.Sprintf("%s: %s line: %v", file, embedDirective, err)}, nil
				}
				patterns = append(patterns, linePatterns...)
			}
		}
	}
	s.embeds[scope] = patterns
	return patterns, nil, nil
}

// parseEmbedPatterns splits a go:embed line's arguments into its patterns.
// It unquotes quoted patterns and drops an `all:` prefix.
func parseEmbedPatterns(args string) ([]string, error) {
	var patterns []string
	for args = strings.TrimSpace(args); args != ""; args = strings.TrimSpace(args) {
		var pattern string
		switch args[0] {
		case '"', '`':
			quoted, err := strconv.QuotedPrefix(args)
			if err != nil {
				return nil, err
			}
			if pattern, err = strconv.Unquote(quoted); err != nil {
				return nil, err
			}
			args = args[len(quoted):]
		default:
			end := strings.IndexFunc(args, unicode.IsSpace)
			if end < 0 {
				end = len(args)
			}
			pattern, args = args[:end], args[end:]
		}
		patterns = append(patterns, strings.TrimPrefix(pattern, "all:"))
	}
	return patterns, nil
}

// embedded reports whether a pattern names rel, a slash path relative to the package directory.
// A pattern names rel itself or a directory above it, since a directory pattern names every file below it.
func embedded(patterns []string, rel string) bool {
	for candidate := rel; candidate != "."; candidate = path.Dir(candidate) {
		for _, pattern := range patterns {
			if matched, err := path.Match(pattern, candidate); err == nil && matched {
				return true
			}
		}
	}
	return false
}

// importDir returns the worktree-relative slash directory of the module package at importPath.
func (s *Selector) importDir(importPath string) string {
	if importPath == s.config.ModulePath {
		return s.config.ModuleRel
	}
	return path.Join(s.config.ModuleRel, strings.TrimPrefix(importPath, s.config.ModulePath+"/"))
}

// selectTests returns the names of m's tests that covered a function of changed, sorted, each once.
// With any function changed, it also returns every test the spawn scan marks.
func selectTests(m Map, changed map[FuncKey]bool) []string {
	if len(changed) == 0 {
		return nil
	}
	var names []string
	for _, test := range m.Tests {
		marked := test.Spawn.OutOfProcess || test.Spawn.Unresolved
		if marked || slices.ContainsFunc(test.Funcs, func(index int) bool { return changed[m.Funcs[index]] }) {
			names = append(names, test.Name)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// isGoSourceFile reports whether rel, a slash path relative to a package directory, is a non-test Go file directly in it.
func isGoSourceFile(rel string) bool {
	return path.Dir(rel) == "." && strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go")
}

// isGoTestFile reports whether rel, a slash path relative to a package directory, is a test file directly in it.
func isGoTestFile(rel string) bool {
	return path.Dir(rel) == "." && strings.HasSuffix(rel, "_test.go")
}
