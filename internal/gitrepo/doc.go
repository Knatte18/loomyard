// Package gitrepo provides a typed wrapper over a single local git checkout,
// built on top of internal/gitexec's raw command runner. It exposes the small
// set of semantic operations (current SHA, stage+commit, changed-files-since,
// SHA existence) that every consumer of a git-backed repo (fabric, raddle,
// codeintel, webster) would otherwise reimplement by parsing raw git stdout
// itself. gitrepo does not create, clone, or otherwise manage repo topology —
// that is fabric's job, built directly on gitexec.
package gitrepo
