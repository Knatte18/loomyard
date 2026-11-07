// Package dotgit reads a directory's `.git` entry in-process.
// It yields the git dir, the common dir, whether they form a usable repository, and the worktree root above a path.
// It is the one `.git` parser and imports the standard library alone.
//
// It is a separate package because `gitrepo`'s in-package test imports `gitkit`.
// `gitkit` reaches `lymcwd` through `configengine` and `logger`;
// `lymcwd` importing `gitrepo` would close a cycle in that test.
//
// A `.git` directory is the git dir.
// A `.git` file is a gitfile whose `gitdir:` line names it, relative to the directory when not absolute.
// The git dir's `commondir` file, relative to the git dir when not absolute, names the common dir;
// the git dir is the common dir otherwise.
// Each of these is ErrNotRepository, wrapped with the path:
// a missing `.git`; a gitfile without a `gitdir:` line; a `gitdir:` or `commondir` target that does not exist.
//
// IsRepository is the repository test: HEAD present in the git dir, objects and refs present in the common dir.
// FindRoot walks up like `git rev-parse --show-toplevel`:
// an empty or broken `.git` directory is skipped;
// a gitfile whose target is missing or not a repository ends the walk and is refused, never resolved outward.
package dotgit
