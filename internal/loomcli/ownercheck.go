// ownercheck.go judges who owns a run before `lym loom start` or `lym loom resume` drives it, and refuses a run another machine owns.

package loomcli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// ownerVerdict is who owns a run: Owner is the owning machine and Elsewhere reports whether that machine is another than this one.
// AdoptSubject is the beam or `root` that `lym adopt` takes to move the run, and empty when the run is owned here.
type ownerVerdict struct {
	Owner        string
	AdoptSubject string
	Elsewhere    bool
}

// primeOfHub resolves the prime worktree of the hub l sits in.
func primeOfHub(l *lymcwd.Location) (*lymcwd.Location, error) {
	primeName, err := fabricengine.PrimeName(l)
	if err != nil {
		return nil, fmt.Errorf("resolve the hub's prime: %w", err)
	}
	prime, err := lymcwd.ResolveWorktree(filepath.Join(l.HubPath, primeName))
	if err != nil {
		return nil, fmt.Errorf("locate the hub's prime %q: %w", primeName, err)
	}
	return prime, nil
}

// runOwnerVerdict judges whether another machine owns the run of slug.
// It runs a judge-only run-scope origin reconcile from the hub's prime, with machine as this machine's name, and reads the run's owned_elsewhere entry.
// With origin unreachable it reads the local board's effective owner instead.
// It writes no owner.
// A reconcile or read error is returned.
func runOwnerVerdict(l *lymcwd.Location, slug string, machine func() (string, error)) (ownerVerdict, error) {
	prime, err := primeOfHub(l)
	if err != nil {
		return ownerVerdict{}, err
	}
	teardown, err := pairteardown.New(prime)
	if err != nil {
		return ownerVerdict{}, err
	}
	options := pairteardown.HubSeamOptions(prime)
	options.Machine = machine
	report, err := fabricengine.ReconcileOrigin(prime, fabricengine.ReconcileScope{Run: slug, JudgeOnly: true}, teardown.OriginSeams(options), fabricengine.ReconcileOptions{})
	if err != nil {
		return ownerVerdict{}, fmt.Errorf("origin reconcile of %q: %w", slug, err)
	}
	if report.Unreachable {
		return localOwnerVerdict(l, slug, machine, options.ParentOf)
	}
	for _, entry := range reportOfRun(report, slug).Entries {
		if entry.State == fabricengine.EntryOwnedElsewhere {
			return ownerVerdict{Owner: entry.Owner, AdoptSubject: entry.AdoptSubject, Elsewhere: true}, nil
		}
	}
	return ownerVerdict{}, nil
}

// localOwnerVerdict judges the owner of slug from the hub's local board, for a reconcile that could not reach origin.
func localOwnerVerdict(l *lymcwd.Location, slug string, machine func() (string, error), parentOf func(slug string) (string, error)) (ownerVerdict, error) {
	here, err := machine()
	if err != nil {
		return ownerVerdict{}, err
	}
	owners, err := boardengine.ReadOwners(fabricengine.BoardDir(l.HubPath))
	if err != nil {
		return ownerVerdict{}, err
	}
	owner, err := owners.Effective(slug, parentOf)
	if err != nil {
		return ownerVerdict{}, err
	}
	subject, err := owners.AdoptSubject(slug, parentOf)
	if err != nil {
		return ownerVerdict{}, err
	}
	return ownerVerdict{Owner: owner, AdoptSubject: subject, Elsewhere: boardengine.OwnedElsewhere(owner, here)}, nil
}

// refuseOwnedElsewhere writes the refusal envelope and returns true when the run is owned by another machine or its owner cannot be judged, and returns false when the run may go on.
// It judges only a run whose status file exists, since a spawn with no status file is the fresh bootstrap under a start batten already judged.
// An owner elsewhere is the kind shedrun.StartOwnedElsewhereKind with the owner's name; a verdict error names its cause and retry, the re-run.
// A receiver with no ownerOf reads as owned here.
func (c *loomCLI) refuseOwnedElsewhere(ctx context.Context, out io.Writer, retry string) bool {
	if c.ownerOf == nil {
		return false
	}
	if _, err := os.Stat(c.shedPaths.StatusPath); err != nil {
		return false
	}
	slug := seedSlug(c.location.WorktreeName)
	verdict, err := c.ownerOf(c.location, slug)
	if err != nil {
		clihelp.SetExit(ctx, output.Err(out, fmt.Sprintf("loom: could not judge who owns this run: %v; fix the cause it names, then %s", err, retry)))
		return true
	}
	if !verdict.Elsewhere {
		return false
	}
	subject := verdict.AdoptSubject
	if subject == "" {
		subject = slug
	}
	message := fmt.Sprintf("loom: this run is owned by machine %q, so this machine does not resume it; way forward: run `lym adopt %s` from this machine's prime", verdict.Owner, subject)
	clihelp.SetExit(ctx, output.ErrFields(out, message, map[string]any{"kind": shedrun.StartOwnedElsewhereKind, "owner": verdict.Owner}))
	return true
}
