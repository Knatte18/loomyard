// derive.go declares Derive and the pure core that turns a package graph, the changed paths and the guard tests into the round gate's command or a fallback reason.

package impactset

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/logger"
)

// maxCommandLength is the longest command Derive returns; a longer one falls back to the full plan verify.
const maxCommandLength = 8000

// vetTags are the tags the vet step runs under, one `go vet` per tag over the set.
var vetTags = []string{"integration", "tmux", "llm"}

// Derivation is the outcome of Derive.
type Derivation struct {
	// Command is the round gate's derived command; empty when Fallback is set.
	Command string
	// Fallback is why the caller runs the full plan verify instead; empty when Command is set.
	Fallback string
	// Base is the commit the diff started from, set whenever the base was usable.
	Base string
}

// Derive derives the round gate's command for the diff between base and HEAD in worktree.
// A fallback reason is not an error; an error is a failure to read git or a package directory, or a *GuardScanError for a test file the guard scan cannot accept.
func Derive(worktree, base string) (Derivation, error) {
	if _, err := os.Stat(filepath.Join(worktree, "go.mod")); err != nil {
		return Derivation{Fallback: "no go.mod at the worktree root"}, nil
	}
	if base == "" {
		return Derivation{Fallback: "no base commit to diff from"}, nil
	}
	resolvedBase, ancestor := resolveAncestorBase(worktree, base)
	if !ancestor {
		return Derivation{Fallback: fmt.Sprintf("base %s is not an ancestor of HEAD", base)}, nil
	}
	derivation := Derivation{Base: resolvedBase}

	changedPaths, err := changedPathsSince(worktree, resolvedBase)
	if err != nil {
		return Derivation{}, err
	}

	pkgs, err := listPackages(worktree)
	if err != nil {
		derivation.Fallback = "go list failed: " + err.Error()
		return derivation, nil
	}
	graph := newModuleGraph(pkgs)
	dirs := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		dirs = append(dirs, p.Dir)
	}
	guards, err := findGuardTests(worktree, dirs)
	if err != nil {
		return Derivation{}, err
	}

	derivation.Command, derivation.Fallback = commandFor(graph, changedPaths, guards)
	return derivation, nil
}

// resolveAncestorBase returns the commit hash base names, and whether it is an ancestor of HEAD.
// Any git refusal, such as an unknown commit, reads as not an ancestor.
func resolveAncestorBase(worktree, base string) (string, bool) {
	resolved, err := gitexec.Run([]string{"rev-parse", "--verify", base + "^{commit}"}, worktree)
	if err != nil {
		return "", false
	}
	resolved = strings.TrimSpace(resolved)
	mergeBase, err := gitexec.Run([]string{"merge-base", resolved, "HEAD"}, worktree)
	if err != nil || strings.TrimSpace(mergeBase) != resolved {
		return "", false
	}
	return resolved, true
}

// changedPathsSince returns every path the diff from base to HEAD touches, a renamed file by both its old and new path.
func changedPathsSince(worktree, base string) ([]string, error) {
	output, err := gitexec.Run([]string{"diff", "--name-status", "-z", "--find-renames", base, "HEAD"}, worktree)
	if err != nil {
		return nil, fmt.Errorf("impactset: list the files changed since %s: %w", base, err)
	}
	return parseChangedPaths(output), nil
}

// parseChangedPaths reads `git diff --name-status -z` output: a status, then one path, or two for a rename or copy.
func parseChangedPaths(output string) []string {
	fields := strings.Split(output, "\x00")
	var paths []string
	for i := 0; i < len(fields) && fields[i] != ""; {
		pathCount := 1
		if status := fields[i][0]; status == 'R' || status == 'C' {
			pathCount = 2
		}
		end := i + 1 + pathCount
		if end > len(fields) {
			break
		}
		paths = append(paths, fields[i+1:end]...)
		i = end
	}
	return paths
}

// listPackages runs `go list` over the module under every tag and returns its packages.
func listPackages(worktree string) ([]pkg, error) {
	absolute, err := filepath.Abs(worktree)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("go", "list", "-tags", "integration,tmux,llm", "-f", goListFormat, "./...")
	cmd.Dir = worktree
	logger.Info("impactset: spawning go list", "dir", worktree)
	output, err := cmd.Output()
	logger.Info("impactset: go list exited", "dir", worktree, "err", err)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}
	return parseGoList(string(output), absolute), nil
}

// commandFor is the pure core of Derive: the command for the changed paths over the graph, or the reason it falls back.
// guards maps a package directory to the names of its guard tests.
func commandFor(graph moduleGraph, changedPaths []string, guards map[string][]string) (command, fallback string) {
	changed := map[string]bool{}
	for _, changedPath := range changedPaths {
		if changedPath == "go.mod" || changedPath == "go.sum" {
			return "", changedPath + " changed"
		}
		importPath, mapped, ok := packageOfPath(graph, changedPath)
		if !ok {
			return "", fmt.Sprintf("%s is neither a Markdown file nor under a Go package directory", changedPath)
		}
		if mapped {
			changed[importPath] = true
		}
	}

	set := graph.impactedSet(changed)
	command = buildCommand(graph, set, guards)
	if len(command) > maxCommandLength {
		return "", fmt.Sprintf("the derived command is %d characters, over the %d limit", len(command), maxCommandLength)
	}
	return command, ""
}

// packageOfPath maps one changed path to the import path of the package it belongs to.
// A `.go` file belongs to its directory's package, and fails when that directory is not a package.
// Any other file belongs to the nearest enclosing package directory; a Markdown file with none belongs to no package (mapped false), and any other file with none fails (ok false).
func packageOfPath(graph moduleGraph, changedPath string) (importPath string, mapped, ok bool) {
	dir := path.Dir(changedPath)
	if strings.HasSuffix(changedPath, ".go") {
		p, found := graph.byDir[dir]
		return p.ImportPath, found, found
	}
	for {
		if p, found := graph.byDir[dir]; found {
			return p.ImportPath, true, true
		}
		if dir == "." {
			break
		}
		dir = path.Dir(dir)
	}
	if strings.HasSuffix(changedPath, ".md") {
		return "", false, true
	}
	return "", false, false
}

// buildCommand assembles the command for an impacted set: build, vet per tag, untagged tests, integration tests, then the guard tests of the packages outside the set.
func buildCommand(graph moduleGraph, set map[string]bool, guards map[string][]string) string {
	steps := []string{"go build ./..."}
	if len(set) > 0 {
		args := strings.Join(packageArgs(graph.packageDirs(set)), " ")
		for _, tag := range vetTags {
			steps = append(steps, "go vet -tags "+tag+" "+args)
		}
		steps = append(steps, "go test "+args, "go test -tags integration "+args)
	}

	var outsideDirs []string
	for dir, names := range guards {
		if len(names) > 0 && !set[graph.byDir[dir].ImportPath] {
			outsideDirs = append(outsideDirs, dir)
		}
	}
	sort.Strings(outsideDirs)
	for _, dir := range outsideDirs {
		steps = append(steps, fmt.Sprintf("go test -tags integration -run '^(%s)$' %s", strings.Join(guards[dir], "|"), packageArgs([]string{dir})[0]))
	}
	return strings.Join(steps, " && ")
}

// packageArgs spells worktree-relative directories as `go` package arguments.
func packageArgs(dirs []string) []string {
	args := make([]string, len(dirs))
	for i, dir := range dirs {
		if dir == "." {
			args[i] = "."
		} else {
			args[i] = "./" + dir
		}
	}
	return args
}
