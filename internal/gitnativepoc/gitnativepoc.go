// gitnativepoc.go defines the Repo type and its constructor: this is a
// throwaway-but-kept feasibility-spike package evaluating whether go-git can
// replace internal/gitrepo's CLI-exec-backed git access, never wired into
// cmd/lyx and not a registered module. It carries no operation methods of
// its own — those land in read.go and write.go — only the shared handle
// later batches build on.

package gitnativepoc

import (
	"github.com/go-git/go-git/v5"
)

// Repo is a typed handle on one local git checkout, identified by its
// filesystem path, mirroring internal/gitrepo.Repo's role as the spike's
// go-git-backed counterpart. It stores both the checkout path and the
// opened go-git *git.Repository handle so later methods can use either the
// go-git object model directly or fall back to a path-scoped operation.
// Repo does not create or clone a checkout — see OpenRepo.
type Repo struct {
	path string
	repo *git.Repository
}

// OpenRepo opens the existing git checkout at path via go-git's
// git.PlainOpen and returns a Repo wrapping it. It returns go-git's own
// error unchanged on failure (a missing or non-git directory, for example),
// so callers can inspect the underlying go-git error type directly.
func OpenRepo(path string) (*Repo, error) {
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}
	return &Repo{path: path, repo: repo}, nil
}
