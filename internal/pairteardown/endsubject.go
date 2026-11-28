// endsubject.go ends the processes a machine runs for a subject that another machine now owns.

package pairteardown

import (
	"fmt"
	"os"
	"slices"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/runproc"
)

// EndSubjectProcesses ends every process this machine runs for a subject another machine now owns, in the hub of prime.
// owner is the owner the subject's own processes register under: a beam's slug, or runproc.HubOwner for the hub's root.
// The subject's runs are the registered owners whose adopt subject is that beam, or the root for the hub's root.
// A registered owner whose subject cannot be resolved is left alone and logged.
// endDobby says whether the subject's dobby is ended last;
// a caller whose dobby hands the subject over itself leaves it unset.
// The calling process and its ancestors are never ended.
func EndSubjectProcesses(prime *lymcwd.Location, owner string, endDobby bool) error {
	owners, err := boardengine.ReadOwners(fabricengine.BoardDir(prime.HubPath))
	if err != nil {
		return fmt.Errorf("pairteardown: read the board's owners: %w", err)
	}
	runProcDir := fabricengine.RunProcDir(prime.HubPath)
	registered, err := runproc.Owners(runProcDir)
	if err != nil {
		return err
	}
	slugs := make([]string, 0, len(registered))
	for slug := range registered {
		slugs = append(slugs, slug)
	}
	runs := subjectRuns(owners, slugs, owner, func(slug string) (string, error) { return hubgeom.ParentOf(prime, slug) })
	ended, err := runproc.EndSubject(runProcDir, owner, runs, os.Getpid(), endDobby)
	logger.Info("pairteardown: ended the processes of a subject another machine owns", "owner", owner, "runs", runs, "ended", len(ended))
	return err
}

// subjectRuns returns, in order, the slugs of registered whose adopt subject is the subject of owner.
// The subject is the beam named by owner, or the root when owner is runproc.HubOwner.
// owner itself is never one of its own runs, and a slug whose subject cannot be resolved is left out and logged.
func subjectRuns(owners boardengine.Owners, registered []string, owner string, parentOf func(slug string) (string, error)) []string {
	subject := boardengine.RootSubject
	if owner != runproc.HubOwner {
		subject = owner
	}
	var runs []string
	for _, slug := range registered {
		if slug == owner {
			continue
		}
		of, err := owners.AdoptSubject(slug, parentOf)
		if err != nil {
			logger.Warn("pairteardown: the subject of a registered run is unreadable, so its processes are left alone", "slug", slug, "error", err)
			continue
		}
		if of == subject {
			runs = append(runs, slug)
		}
	}
	slices.Sort(runs)
	return runs
}
