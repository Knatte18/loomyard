// testselect.go selects a package's tests from its coverage map:
// the tests that covered a function that changed since the map's commit, in the package or a linked one,
// and the Test functions changed or added in its test files.
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

// SelectorConfig is what a Selector reads coverage maps and its target with.
type SelectorConfig struct {
	// MapsDir is the directory holding the per-package coverage maps.
	MapsDir string
	// WorktreeRoot is the worktree's absolute path, from which a working-tree target is read.
	WorktreeRoot string
	// ModuleRel is the module's slash directory relative to the worktree root, "." for the root.
	ModuleRel string
	// ModulePath is the module's path, as its go.mod declares it.
	ModulePath string
	// GoRoot is the Go toolchain's root directory, whose src directory holds the standard library.
	// It must be set: with it empty, no import counts as standard library.
	GoRoot string
	// Commits reads files at a map's commit, and a commit target's files.
	Commits CommitReader
	// Inert reports whether a worktree-relative slash path matches the hub's inert_paths; nil matches none.
	Inert func(rel string) bool
	// Environment is the current environment stamp, bar its GoModHash and GoSumHash.
	// The Selector reads those two from its target's go.mod and go.sum.
	Environment EnvironmentStamp
}

// Target is the tree a Selector compares coverage maps with: the working tree or one commit.
type Target struct {
	// Commit names the commit whose tree the target is, read through SelectorConfig.Commits.
	// It is empty for the working tree under SelectorConfig.WorktreeRoot.
	Commit string
	// Files are the worktree-relative slash paths of every file the target holds.
	// For the working tree they are every file git does not ignore, tracked or untracked;
	// for a commit, every file its tree holds.
	Files []string
}

// MissReason names why a coverage map cannot select a package's tests.
type MissReason string

// The reasons a Selector misses for.
const (
	MissNoMap            MissReason = "no coverage map"
	MissMapUnreadable    MissReason = "coverage map unreadable"
	MissBuildFailure     MissReason = "coverage map records a build failure"
	MissUnmarked         MissReason = "coverage map unmarked"
	MissEnvironment      MissReason = "environment stamp differs"
	MissCommitUnreadable MissReason = "map commit unreadable"
	MissNonGoFile        MissReason = "non-Go file changed"
	MissParse            MissReason = "Go file does not parse"
	MissNonBody          MissReason = "change outside function declarations"
	MissTestFile         MissReason = "test file changed outside its Test functions"
	MissDirective        MissReason = "compiler directive changed"
	MissMethodSet        MissReason = "method set changed"
	MissInit             MissReason = "init function added"
	MissPredeclared      MissReason = "predeclared name declared"
)

// Miss is why a coverage map cannot select a package's tests, so the package runs whole.
type Miss struct {
	Reason MissReason
	// Detail names the file, declaration or failure the reason is about.
	Detail string
}

// String renders the miss as its reason and detail.
func (m Miss) String() string {
	return string(m.Reason) + ": " + m.Detail
}

// Selection is the outcome of selecting one package's tests.
type Selection struct {
	// Tests are the affected tests' names, sorted, each once.
	Tests []string
	// Miss is set when the package runs whole instead, and Tests is then empty.
	Miss *Miss
	// Recorded are the tests the map records a passing measurement for, in its order; empty with a Miss.
	// An entry that records an outcome or a spawn verdict instead is no recorded test.
	// A caller tells from them which of the package's tests the map has no coverage for.
	Recorded []MapTest
}

// Selector selects packages' tests from their coverage maps against one target.
// It caches the target's file hashes, each directory's owned files and each Go file's comparison.
// Selecting many packages then reads each file once.
// A working-tree target must not change while the Selector is in use.
// It is not safe for concurrent use.
type Selector struct {
	config SelectorConfig
	target Target
	// environment is the target's environment stamp, nil until the first selection reads it.
	environment *EnvironmentStamp
	// owned holds the target files each package directory owns, by slash path relative to it.
	owned map[string]map[string]struct{}
	// hashes holds each target file's hash, by worktree-relative slash path.
	hashes map[string]targetHash
	// compared holds each Go file's comparison, by the map's commit and the file's worktree-relative slash path.
	compared map[string]goFileComparison
	// embeds holds the go:embed patterns of each package directory, read with or without its test files.
	embeds map[embedScope][]string
}

// embedScope keys the go:embed patterns of one package directory.
// withTests is set when the package's test files count, which holds for the selected package alone.
type embedScope struct {
	dir       string
	withTests bool
}

// targetHash is a target file's content hash, or its absence.
type targetHash struct {
	hash  string
	found bool
}

// packageChange is what one package's changed files select.
type packageChange struct {
	// funcs are the package's functions whose declaration changed or that only the map's commit holds.
	funcs []FuncKey
	// tests name the Test functions of the package's test files that changed or are new.
	tests []string
}

// NewSelector returns a Selector over target, read as config describes.
func NewSelector(config SelectorConfig, target Target) *Selector {
	return &Selector{
		config:   config,
		target:   target,
		owned:    map[string]map[string]struct{}{},
		hashes:   map[string]targetHash{},
		compared: map[string]goFileComparison{},
		embeds:   map[embedScope][]string{},
	}
}

// Select selects the tests of the package pkg, a module-relative slash path, from its coverage map.
// A recorded test is affected when it covered a function that changed or was removed since the map's commit.
// The function belongs to the package or to a linked package.
// A Test function whose declaration changed, or that is new, is affected whatever the map records for it.
// A change the map cannot speak for gives a Miss instead, as does an environment stamp that differs from the target's.
// The error reports a target file that cannot be read.
func (s *Selector) Select(pkg string) (Selection, error) {
	m, found, err := ReadMap(s.config.MapsDir, pkg)
	if miss := mapMiss(m, found, err); miss != nil {
		return Selection{Miss: miss}, nil
	}
	if miss, err := s.environmentMiss(m.Format.Environment); miss != nil || err != nil {
		return Selection{Miss: miss}, err
	}
	ownDir := path.Join(s.config.ModuleRel, pkg)
	if miss := s.commitMiss(ownDir, m); miss != nil {
		return Selection{Miss: miss}, nil
	}
	ownImportPath := s.importPath(pkg)
	recorded := map[string]string{}
	maps.Copy(recorded, m.OtherFiles)
	maps.Copy(recorded, m.Files)
	own, miss, err := s.packageChanges(m, ownImportPath, ownDir, recorded, true)
	if miss != nil || err != nil {
		return Selection{Miss: miss}, err
	}
	changed := map[FuncKey]bool{}
	for _, key := range own.funcs {
		changed[key] = true
	}
	for _, linked := range slices.Sorted(maps.Keys(m.Linked)) {
		change, miss, err := s.packageChanges(m, linked, s.importDir(linked), m.Linked[linked], false)
		if miss != nil || err != nil {
			return Selection{Miss: miss}, err
		}
		for _, key := range change.funcs {
			changed[key] = true
		}
	}
	return Selection{Tests: selectTests(m, ownImportPath, changed, own.tests), Recorded: measuredTests(m)}, nil
}

// mapMiss returns the miss of a map a selection cannot use, or nil for a usable map.
// Unusable maps are missing, unreadable, failed to build with no test entry, unmarked, or rewritten by another writer.
// A build-failure record beside test entries leaves the map usable.
// found and err are ReadMap's.
func mapMiss(m Map, found bool, err error) *Miss {
	switch {
	case err != nil:
		return &Miss{Reason: MissMapUnreadable, Detail: err.Error()}
	case !found:
		return &Miss{Reason: MissNoMap, Detail: "no map was built for the package"}
	case m.BuildFailure != "" && len(m.Tests) == 0:
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
		for _, index := range test.Blocks {
			if index < 0 || index >= len(m.Blocks) {
				return &Miss{Reason: MissMapUnreadable, Detail: fmt.Sprintf("test %s names block %d of a table of %d", test.Name, index, len(m.Blocks))}
			}
		}
	}
	return nil
}

// environmentMiss returns the miss of a map whose environment stamp, recorded, differs from the target's.
// It reads the target's go.mod and go.sum at its first call; the error reports one that cannot be read.
func (s *Selector) environmentMiss(recorded EnvironmentStamp) (*Miss, error) {
	if s.environment == nil {
		current, err := StampModuleFiles(s.config.Environment, s.config.ModuleRel, s.readTargetFile)
		if err != nil {
			return nil, err
		}
		s.environment = &current
	}
	if differing := stampDifferences(recorded, *s.environment); len(differing) > 0 {
		return &Miss{Reason: MissEnvironment, Detail: "the map's stamp differs in " + strings.Join(differing, ", ")}, nil
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

// packageChanges returns what the changed files of the package at importPath select, or the miss their changes give.
// dir is the package's worktree-relative slash directory, and recorded its files' hashes as the map holds them.
// own is set for the selected package, whose test files are compared and whose test files' go:embed patterns count;
// a linked package's test files are in no binary the map measured, so they are skipped.
func (s *Selector) packageChanges(m Map, importPath, dir string, recorded map[string]string, own bool) (packageChange, *Miss, error) {
	changedFiles, err := s.changedFiles(dir, recorded)
	if err != nil {
		return packageChange{}, nil, err
	}
	var change packageChange
	methodsBefore, methodsAfter := map[string][]*ast.FuncDecl{}, map[string][]*ast.FuncDecl{}
	for _, rel := range changedFiles {
		file := path.Join(dir, rel)
		_, wasRecorded := recorded[rel]
		switch {
		case isGoTestFile(rel) && !own:
			continue
		case isGoTestFile(rel):
			comparison, err := s.compareTestFile(m.Commit, file, wasRecorded)
			if err != nil || comparison.miss != nil {
				return packageChange{}, comparison.miss, err
			}
			for _, imported := range comparison.addedImports {
				if imported != importPath && !s.importAllowed(m, imported) {
					return packageChange{}, &Miss{Reason: MissTestFile, Detail: fmt.Sprintf(
						"%s adds an import of %s, which is neither the package, in the map's linked set, nor in the standard library", file, imported)}, nil
				}
			}
			for _, removed := range comparison.removedImports {
				if removed != importPath && !s.standardLibrary(removed) {
					return packageChange{}, &Miss{Reason: MissTestFile, Detail: fmt.Sprintf(
						"%s removes an import of %s, which may leave the build with it", file, removed)}, nil
				}
			}
			change.tests = append(change.tests, comparison.tests...)
			continue
		case !isGoSourceFile(rel):
			if miss, err := s.otherFileMiss(dir, rel, own); miss != nil || err != nil {
				return packageChange{}, miss, err
			}
			continue
		}
		comparison, err := s.compareGoFile(m.Commit, file, wasRecorded)
		if err != nil || comparison.miss != nil {
			return packageChange{}, comparison.miss, err
		}
		for _, imported := range comparison.addedImports {
			if !s.importAllowed(m, imported) {
				return packageChange{}, &Miss{Reason: MissNonBody, Detail: fmt.Sprintf(
					"%s adds an import of %s, which is neither in the map's linked set nor in the standard library", file, imported)}, nil
			}
		}
		for _, removed := range comparison.removedImports {
			if !s.standardLibrary(removed) {
				return packageChange{}, &Miss{Reason: MissNonBody, Detail: fmt.Sprintf(
					"%s is deleted and imported %s, which may leave the build with it", file, removed)}, nil
			}
		}
		for _, name := range comparison.changed {
			change.funcs = append(change.funcs, FuncKey{Package: importPath, File: rel, Func: name})
		}
		for key, decls := range comparison.methodsBefore {
			methodsBefore[key] = append(methodsBefore[key], decls...)
		}
		for key, decls := range comparison.methodsAfter {
			methodsAfter[key] = append(methodsAfter[key], decls...)
		}
	}
	if miss := methodSetMiss(dir, methodsBefore, methodsAfter); miss != nil {
		return packageChange{}, miss, nil
	}
	return change, nil, nil
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
		var current targetHash
		if _, inTarget := owned[rel]; inTarget {
			var err error
			if current, err = s.targetHash(path.Join(dir, rel)); err != nil {
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

// ownedFiles returns the target files that the package at the worktree-relative slash directory dir owns.
// Paths are slash paths relative to dir.
// A package owns the files under dir and under no directory below it that holds another package or module.
func (s *Selector) ownedFiles(dir string) map[string]struct{} {
	if owned, cached := s.owned[dir]; cached {
		return owned
	}
	var below []string
	for _, file := range s.target.Files {
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

// targetHash returns the hash of the target file at the worktree-relative slash path rel.
func (s *Selector) targetHash(rel string) (targetHash, error) {
	if current, cached := s.hashes[rel]; cached {
		return current, nil
	}
	data, found, err := s.readTargetFile(rel)
	if err != nil {
		return targetHash{}, err
	}
	current := targetHash{found: found}
	if found {
		current.hash = ContentHash(data)
	}
	s.hashes[rel] = current
	return current, nil
}

// readTargetFile reads the target file at the worktree-relative slash path rel; found is false for an absent file.
func (s *Selector) readTargetFile(rel string) (data []byte, found bool, err error) {
	if s.target.Commit != "" {
		return s.config.Commits.CodeFileAt(s.target.Commit, rel)
	}
	data, err = os.ReadFile(filepath.Join(s.config.WorktreeRoot, filepath.FromSlash(rel)))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, false, nil
	case err != nil:
		return nil, false, err
	}
	return data, true, nil
}

// readSides reads the Go file at the worktree-relative slash path rel as commit stores it and as the target holds it.
// wasRecorded tells whether the map's record holds the file, and so whether the commit does.
// A commit that cannot give the file returns its miss instead.
func (s *Selector) readSides(commit, rel string, wasRecorded bool) (before, after goSource, miss *Miss, err error) {
	if wasRecorded {
		data, found, err := s.config.Commits.CodeFileAt(commit, rel)
		switch {
		case err != nil:
			return goSource{}, goSource{}, &Miss{Reason: MissCommitUnreadable, Detail: err.Error()}, nil
		case !found:
			return goSource{}, goSource{}, &Miss{Reason: MissCommitUnreadable, Detail: fmt.Sprintf("%s is absent at the map's commit %s", rel, commit)}, nil
		}
		before = goSource{text: data, present: true}
	}
	data, found, err := s.readTargetFile(rel)
	if err != nil {
		return goSource{}, goSource{}, nil, err
	}
	return before, goSource{text: data, present: found}, nil, nil
}

// compareGoFile compares the Go file at the worktree-relative slash path rel as commit stores it with the target's.
// wasRecorded tells whether the map's record holds the file, and so whether the commit does.
func (s *Selector) compareGoFile(commit, rel string, wasRecorded bool) (goFileComparison, error) {
	key := commit + "\x00" + rel
	if comparison, cached := s.compared[key]; cached {
		return comparison, nil
	}
	before, after, miss, err := s.readSides(commit, rel, wasRecorded)
	if err != nil {
		return goFileComparison{}, err
	}
	comparison := goFileComparison{miss: miss}
	if miss == nil {
		comparison = compareGoSources(rel, before, after)
	}
	s.compared[key] = comparison
	return comparison, nil
}

// compareTestFile compares the test file at the worktree-relative slash path rel as commit stores it with the target's.
// wasRecorded tells whether the map's record holds the file, and so whether the commit does.
func (s *Selector) compareTestFile(commit, rel string, wasRecorded bool) (testFileComparison, error) {
	before, after, miss, err := s.readSides(commit, rel, wasRecorded)
	if err != nil || miss != nil {
		return testFileComparison{miss: miss}, err
	}
	return compareTestSources(rel, before, after), nil
}

// importAllowed reports whether a changed Go file may add an import of importPath:
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
// ownPackage is set when dir is the selected package, whose test files are in its binary, so their patterns count.
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

// embedPatterns returns the go:embed patterns of the package at dir, read from the target's Go files.
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
		data, found, err := s.readTargetFile(file)
		if err != nil {
			return nil, nil, err
		}
		if !found {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), "", data, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, &Miss{Reason: MissParse, Detail: fmt.Sprintf("%s in the target: %v", file, err)}, nil
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

// importPath returns the import path of the module-relative slash package directory pkg.
func (s *Selector) importPath(pkg string) string {
	if pkg == "." {
		return s.config.ModulePath
	}
	return s.config.ModulePath + "/" + pkg
}

// selectTests returns the affected tests' names, sorted, each once.
// They are changedTests and each test m records a passing measurement for that covered a function of changed:
// a linked package's function through its Funcs, or a function of the package at importPath through its Blocks.
func selectTests(m Map, importPath string, changed map[FuncKey]bool, changedTests []string) []string {
	names := slices.Clone(changedTests)
	for _, test := range m.Tests {
		if !measured(test) {
			continue
		}
		coversLinked := slices.ContainsFunc(test.Funcs, func(index int) bool { return changed[m.Funcs[index]] })
		coversOwn := slices.ContainsFunc(test.Blocks, func(index int) bool {
			block := m.Blocks[index]
			return changed[FuncKey{Package: importPath, File: block.File, Func: block.Func}]
		})
		if coversLinked || coversOwn {
			names = append(names, test.Name)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// measuredTests returns the tests m records a passing measurement for, in its order.
func measuredTests(m Map) []MapTest {
	var tests []MapTest
	for _, test := range m.Tests {
		if measured(test) {
			tests = append(tests, test)
		}
	}
	return tests
}

// measured reports whether a map's entry records a passing measurement:
// no outcome, and no spawn verdict that marks the test out of process or unresolved.
func measured(test MapTest) bool {
	return test.Outcome == "" && !test.Spawn.OutOfProcess && !test.Spawn.Unresolved
}

// isGoSourceFile reports whether rel, a slash path relative to a package directory, is a non-test Go file directly in it.
func isGoSourceFile(rel string) bool {
	return path.Dir(rel) == "." && strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go")
}

// isGoTestFile reports whether rel, a slash path relative to a package directory, is a test file directly in it.
func isGoTestFile(rel string) bool {
	return path.Dir(rel) == "." && strings.HasSuffix(rel, "_test.go")
}
