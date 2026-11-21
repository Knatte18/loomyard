// Package dotgit reads a directory's `.git` entry, and a repository's worktree list, in-process.
// It yields the git dir, the common dir, whether they form a usable repository, the worktree root above a path,
// and the worktrees `git worktree list --porcelain` reports.
// It is the one `.git` parser and imports the standard library alone.
//
// It is a separate package because `gitrepo`'s in-package test imports `gitkit`.
// `gitkit` reaches `lymcwd` through `configengine` and `logger`;
// `lymcwd` importing `gitrepo` would close a cycle in that test.
//
// Each of these is ErrNotRepository, wrapped with the path:
// a missing `.git`, a gitfile without a `gitdir:` line, and a `gitdir:` or `commondir` target that does not exist.
//
// FindRoot walks up like `git rev-parse --show-toplevel`:
// an empty or broken `.git` directory is skipped;
// a gitfile whose target is missing or not a repository ends the walk and is refused, never resolved outward.
//
// ListWorktrees matches `git worktree list --porcelain` from git 2.43 on and starts no git process.
// A repository whose `extensions.refStorage` is not `files` keeps no ref files.
// ListWorktrees refuses it with ErrRefStorage, and the caller asks git.
package dotgit
