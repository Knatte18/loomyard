// derive.go declares the pure core that turns a package graph and the changed paths into an impacted set, and the git and `go list` reads that feed it.

package impactset

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/logger"
)

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
// ctx bounds the spawn: a cancelled ctx kills a stalled `go list`.
func listPackages(ctx context.Context, worktree string) ([]pkg, error) {
	absolute, err := filepath.Abs(worktree)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "go", "list", "-tags", "tier0,tier1,tier2,tier3", "-f", goListFormat, "./...")
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

// impactedFor returns the impacted set of the changed paths over the graph, or the reason the diff cannot be narrowed to one.
func impactedFor(graph moduleGraph, changedPaths []string) (set map[string]bool, fallback string) {
	changed := map[string]bool{}
	for _, changedPath := range changedPaths {
		if changedPath == "go.mod" || changedPath == "go.sum" {
			return nil, changedPath + " changed"
		}
		importPath, mapped, ok := packageOfPath(graph, changedPath)
		if !ok {
			return nil, fmt.Sprintf("%s is neither a Markdown file nor under a Go package directory", changedPath)
		}
		if mapped {
			changed[importPath] = true
		}
	}
	return graph.impactedSet(changed), ""
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
