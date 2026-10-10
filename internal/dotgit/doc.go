// Package dotgit reads a directory's `.git` entry in-process: the git dir, the common dir, whether they form a usable repository, and the worktree root above a path.
// It is the one `.git` parser and imports the standard library alone.
//
// It is its own package because `gitrepo`'s in-package test imports `gitkit`, which reaches `lyxcwd` through `configengine` and `logger`; `lyxcwd` importing `gitrepo` would close a cycle in that test.
//
// A `.git` directory is the git dir.
// A `.git` file is a gitfile whose `gitdir:` line names it, relative to the directory when not absolute.
// The git dir's `commondir` file, relative to the git dir when not absolute, names the common dir; the git dir is the common dir otherwise.
// A missing `.git`, a gitfile without a `gitdir:` line, and a `gitdir:` or `commondir` target that does not exist are all ErrNotRepository, wrapped with the path.
//
// IsRepository is the repository test: HEAD present in the git dir, objects and refs present in the common dir.
// FindRoot walks up like `git rev-parse --show-toplevel`: an empty or broken `.git` directory is skipped, while a gitfile ends the walk and is refused, never resolved outward, when its target is missing or not a repository.
package dotgit
