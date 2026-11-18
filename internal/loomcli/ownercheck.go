// ownercheck.go judges who owns a run and whether the act rule lets it act.
// It runs before `lym loom start` or `lym loom resume` drives the run.
// It refuses a run that another machine owns, and a run the act rule waits on.

package loomcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
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
// It runs a judge-only origin reconcile of the run from the hub's prime, with identity as this machine's identity.
// It then reads the run's owned_elsewhere entry.
// With origin unreachable it reads the local board's effective owner instead.
// It writes no owner.
// A reconcile or read error is returned.
func runOwnerVerdict(l *lymcwd.Location, slug string, identity func() (machinename.Identity, error)) (ownerVerdict, error) {
	prime, err := primeOfHub(l)
	if err != nil {
		return ownerVerdict{}, err
	}
	teardown, err := pairteardown.New(prime)
	if err != nil {
		return ownerVerdict{}, err
	}
	options := pairteardown.HubSeamOptions(prime)
	options.Machine = identity
	report, err := fabricengine.ReconcileOrigin(prime, fabricengine.ReconcileScope{Run: slug, JudgeOnly: true}, teardown.OriginSeams(options), fabricengine.ReconcileOptions{})
	if err != nil {
		return ownerVerdict{}, fmt.Errorf("origin reconcile of %q: %w", slug, err)
	}
	if report.Unreachable {
		return localOwnerVerdict(l, slug, identity, options.ParentOf)
	}
	for _, entry := range reportOfRun(report, slug).Entries {
		if entry.State == fabricengine.EntryOwnedElsewhere {
			return ownerVerdict{Owner: entry.Owner, AdoptSubject: entry.AdoptSubject, Elsewhere: true}, nil
		}
	}
	return ownerVerdict{}, nil
}

// localOwnerVerdict judges the owner of slug from the hub's local board, for a reconcile that could not reach origin.
func localOwnerVerdict(l *lymcwd.Location, slug string, identity func() (machinename.Identity, error), parentOf func(slug string) (string, error)) (ownerVerdict, error) {
	here, err := identity()
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
	return ownerVerdict{Owner: owner, AdoptSubject: subject, Elsewhere: boardengine.OwnedElsewhere(owner, owners.Registry, here)}, nil
}

// refuseOwnedElsewhere writes the refusal envelope when another machine owns the run or its owner cannot be judged.
// It returns true when it wrote a refusal, and false when the run may go on.
// It judges only a run whose status file exists.
// A spawn with no status file is the fresh bootstrap that a start batten already judged.
// An owner elsewhere is the kind shedrun.StartOwnedElsewhereKind with the owner's name.
// A verdict error names its cause and the retry the caller passes.
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
	writeOwnedElsewhere(ctx, out, slug, verdict.Owner, verdict.AdoptSubject)
	return true
}

// writeOwnedElsewhere writes the owned_elsewhere refusal for slug, owned by owner and moved by `lym adopt` of subject.
// An empty subject names the slug.
func writeOwnedElsewhere(ctx context.Context, out io.Writer, slug, owner, subject string) {
	if subject == "" {
		subject = slug
	}
	message := fmt.Sprintf("loom: this run is owned by machine %q, so this machine does not resume it; way forward: run `lym adopt %s` from this machine's prime", owner, subject)
	clihelp.SetExit(ctx, output.ErrFields(out, message, map[string]any{"kind": shedrun.StartOwnedElsewhereKind, "owner": owner}))
}

// hubRunVerdict judges whether the run of slug may act, over the hub of l.
// The verdict is judged over the run's tracked records and this machine's inputs.
// A shed pause is set aside, since the verbs asking resume the child's own shed pause.
// The hub's Bolt is pulled first, so a hold another machine wrote is read.
func hubRunVerdict(l *lymcwd.Location, slug string) (runfsm.Verdict, error) {
	prime, err := primeOfHub(l)
	if err != nil {
		return runfsm.Verdict{}, err
	}
	records, inputs, err := pairteardown.ReadRunPulled(prime, slug)
	if err != nil {
		return runfsm.Verdict{}, err
	}
	_, verdict := runfsm.Judge(records, inputs, runfsm.Aside{ShedPause: true})
	return verdict, nil
}

// refuseByAct writes the refusal envelope when the act rule does not let this run act or its state cannot be read.
// It returns true when it wrote a refusal, and false when the run may go on.
// It asks only for a run whose status file exists.
// A spawn with no status file is the fresh bootstrap that a start batten already judged.
// A hand-over is the kind shedrun.StartOwnedElsewhereKind with the owner's name, as the owner refusal writes it.
// Any other wait carries the rule's refusal, with retry as the quoted command to run again.
// A read error names its cause and retry, except a record that does not decode, which goes on for the driver to diagnose.
// A receiver with no runVerdict checks nothing.
func (c *loomCLI) refuseByAct(ctx context.Context, out io.Writer, retry string) bool {
	if c.runVerdict == nil {
		return false
	}
	if _, err := os.Stat(c.shedPaths.StatusPath); err != nil {
		return false
	}
	slug := seedSlug(c.location.WorktreeName)
	verdict, err := c.runVerdict(c.location, slug)
	if errors.Is(err, state.ErrDecode) {
		// A record that does not decode is diagnosed by the spawned driver's own read of the status file.
		// The seed step defers it the same way.
		// Refusing here would fail before any driver ran.
		return false
	}
	if err != nil {
		clihelp.SetExit(ctx, output.Err(out, fmt.Sprintf("loom: could not read the state of this run to decide whether it may act: %v; way forward: fix the cause it names, then run %s again", err, retry)))
		return true
	}
	if verdict.Answer == runfsm.AnswerHandOver {
		writeOwnedElsewhere(ctx, out, slug, verdict.Owner, verdict.AdoptSubject)
		return true
	}
	refusal := verdict.Refusal(slug, retry)
	if refusal == nil {
		return false
	}
	clihelp.SetExit(ctx, output.Err(out, refusal.Error()))
	return true
}
