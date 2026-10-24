// derive.go declares the two entry points of the derivation, DeriveDiff and DeriveCards, and the tree scan they share.

package dotnetimpact

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// skippedDirs are the directory names scanTree never enters.
var skippedDirs = []string{"bin", "obj", ".scratch", ".git", "_lyx", ".lyx"}

// DeriveDiff derives the Scope of the diff between base and the working tree in worktree, for the tier whose directory is dir, relative to the worktree.
// inert reports whether a worktree-relative path is inert; an inert path is dropped before anything else is read.
// A fallback is the Whole Scope with its reason and is no error; the one error is a cancelled ctx, checked before each git read.
func DeriveDiff(ctx context.Context, worktree, dir, base string, inert func(rel string) bool) (Scope, error) {
	dir = path.Clean(filepath.ToSlash(dir))
	if base == "" {
		return wholeScope("no base commit to diff from"), nil
	}
	if err := ctx.Err(); err != nil {
		return Scope{}, err
	}
	resolved, ancestor := impactset.AncestorBase(worktree, base)
	if !ancestor {
		return wholeScope(fmt.Sprintf("base %s is not an ancestor of HEAD", base)), nil
	}
	changed, err := impactset.ChangedPaths(ctx, worktree, resolved)
	if err != nil {
		return readFallback(ctx, err)
	}

	var files []string
	for _, changedPath := range changed {
		if inert(changedPath) {
			continue
		}
		if !strings.HasSuffix(changedPath, ".cs") {
			return wholeScope(fmt.Sprintf("%s is neither inert nor a .cs file", changedPath)), nil
		}
		if _, inside := relativeToDir(dir, changedPath); !inside {
			return wholeScope(fmt.Sprintf("%s lies outside %s", changedPath, dir)), nil
		}
		files = append(files, changedPath)
	}
	if len(files) == 0 {
		return Scope{}, nil
	}
	slices.Sort(files)
	files = slices.Compact(files)

	s := seed{Files: map[string]bool{}, Types: map[string]bool{}}
	for _, file := range files {
		rel, _ := relativeToDir(dir, file)
		s.Files[rel] = true
		declared, reason, err := declaredTypes(ctx, worktree, resolved, file)
		if err != nil {
			return readFallback(ctx, err)
		}
		if reason != "" {
			return wholeScope(reason), nil
		}
		for _, name := range declared {
			s.Types[name] = true
		}
	}
	return scanAndNarrow(worktree, dir, s), nil
}

// DeriveCards derives the Scope of the card targets in seeds, for the tier whose directory is dir, relative to the worktree.
// inert reports whether a worktree-relative path is inert; a seed whose file is inert is dropped.
// It reads no git; a seed list that is empty, or whose seeds are all dropped as inert, is the empty scope.
// A fallback is the Whole Scope with its reason and is no error; the one error is a cancelled ctx.
func DeriveCards(ctx context.Context, worktree, dir string, seeds []planparser.CSharpSeed, inert func(rel string) bool) (Scope, error) {
	dir = path.Clean(filepath.ToSlash(dir))
	if err := ctx.Err(); err != nil {
		return Scope{}, err
	}
	s := seed{Files: map[string]bool{}, Types: map[string]bool{}}
	for _, target := range seeds {
		if target.File == "" && len(target.Types) == 0 {
			return wholeScope(fmt.Sprintf("%s names neither a file nor a type", target.Ref)), nil
		}
		if target.File != "" {
			if inert(target.File) {
				continue
			}
			if !strings.HasSuffix(target.File, ".cs") {
				return wholeScope(fmt.Sprintf("%s is neither inert nor a .cs file", target.File)), nil
			}
			rel, inside := relativeToDir(dir, target.File)
			if !inside {
				return wholeScope(fmt.Sprintf("%s lies outside %s", target.File, dir)), nil
			}
			if _, err := os.Stat(filepath.Join(worktree, filepath.FromSlash(target.File))); err == nil {
				s.Files[rel] = true
			} else if len(target.Types) == 0 {
				return wholeScope(fmt.Sprintf("%s is absent from the working tree and names no type", target.File)), nil
			}
		}
		for _, name := range target.Types {
			s.Types[name] = true
		}
	}
	if len(s.Files) == 0 && len(s.Types) == 0 {
		return Scope{}, nil
	}
	return scanAndNarrow(worktree, dir, s), nil
}

// scanAndNarrow scans the tier's tree and narrows the seed over it, or returns the Whole Scope naming the scan's failure.
func scanAndNarrow(worktree, dir string, s seed) Scope {
	sources, projects, err := scanTree(filepath.Join(worktree, filepath.FromSlash(dir)))
	if err != nil {
		return wholeScope(fmt.Sprintf("scan failed: %v", err))
	}
	return narrow(sources, projects, s)
}

// readFallback turns a failed git or source read into the cancelled ctx's error, or into the Whole Scope naming it.
func readFallback(ctx context.Context, err error) (Scope, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Scope{}, ctxErr
	}
	return wholeScope(fmt.Sprintf("read failed: %v", err)), nil
}

// relativeToDir returns the path relative to the tier's dir, and whether the path lies inside it.
func relativeToDir(dir, rel string) (string, bool) {
	if dir == "." {
		return rel, true
	}
	return strings.CutPrefix(rel, dir+"/")
}

// declaredTypes returns the names of the types the C# file declares in its base version and in the working tree, so a renamed or deleted type still seeds.
// A missing version declares none.
// reason is non-empty, with an empty error, when neither version declares a type.
func declaredTypes(ctx context.Context, worktree, base, file string) (names []string, reason string, err error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	listed, err := gitexec.Run([]string{"ls-tree", "--name-only", "-z", base, "--", file}, worktree)
	if err != nil {
		return nil, "", err
	}
	if listed != "" {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		content, err := gitexec.Run([]string{"show", base + ":" + file}, worktree)
		if err != nil {
			return nil, "", err
		}
		names = typeNames(file, []byte(content))
	}
	data, readErr := os.ReadFile(filepath.Join(worktree, filepath.FromSlash(file)))
	switch {
	case readErr == nil:
		names = append(names, typeNames(file, data)...)
	case !os.IsNotExist(readErr):
		return nil, "", readErr
	}
	if len(names) == 0 {
		return nil, fmt.Sprintf("%s declares no type", file), nil
	}
	return names, "", nil
}

// typeNames returns the names of the types the C# source declares.
func typeNames(rel string, data []byte) []string {
	var names []string
	for _, decl := range parseSource(rel, data).Types {
		names = append(names, decl.Name)
	}
	return names
}

// scanTree reads every `.cs` file under dir into a sourceFile, keyed by its slash path relative to dir, and lists every `.csproj`, in path order.
// It skips the directories named in skippedDirs; an unreadable file or directory is its error.
func scanTree(dir string) (sources []sourceFile, projects []string, err error) {
	err = filepath.WalkDir(dir, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if p != dir && slices.Contains(skippedDirs, entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch path.Ext(rel) {
		case ".cs":
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			sources = append(sources, parseSource(rel, data))
		case ".csproj":
			projects = append(projects, rel)
		}
		return nil
	})
	return sources, projects, err
}
