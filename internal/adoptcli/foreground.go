// foreground.go runs `lym adopt` in the caller's process over tracked state.
// It writes the hold, moves each run from origin and runs the catch-up, the deploy and the workspace.
// It runs the adopt check in the foreground and brings each subject up.
// It leaves no record: a re-run goes on from what Bolt and the run records hold.

package adoptcli

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// printStep prints the line that says a step of the adopt ended.
func printStep(steps io.Writer, name string) {
	fmt.Fprintf(steps, "adopt: %s done\n", name)
}

// holdSubjectNames returns the subjects a hold covers, in the hold's order.
func holdSubjectNames(hold boardengine.Hold) []string {
	names := make([]string, len(hold.Subjects))
	for i, subject := range hold.Subjects {
		names[i] = subject.Subject
	}
	return names
}

// sameSubjects reports whether a and b name the same subjects, order ignored.
func sameSubjects(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}

// localRunRead returns the run read that reads the batten run records this machine holds.
func localRunRead(deps adoptDeps) func(slug, parent string) (shedengine.Status, bool, error) {
	return func(slug, _ string) (shedengine.Status, bool, error) { return deps.RunRecord(slug) }
}

// runForeground runs the adopt of req in this process and prints `adopt: <step> done` on steps as each step ends.
// It validates the request and refuses a standing anchor change under the subjects before any write.
// It writes the hold, the owners and this machine's registry entry in one Bolt commit.
// It adds the pairs of the adopted beams and takes each run moved from another machine over from origin.
// It then runs the catch-up over the hub, the deploy when root is a subject, and the workspace.
// Last it runs the adopt check over the subjects of the hold.
// Before each of those steps it pulls Bolt and reads the hold again:
// a hold another machine took, or one that cleared, ends the adopt and starts nothing.
// A green check clears the hold, and a failing one files a fix task and keeps the hold under it.
// Each subject is brought up after the check, on green and on red alike.
// A failing check ends the adopt with an error.
// The outcome holds what the steps did even when an error ends the adopt.
func runForeground(deps adoptDeps, req request, steps io.Writer) (outcome, error) {
	var result outcome
	req = normalizedRequest(req)
	slugs, rootNamed, err := validateRequest(deps, req)
	if err != nil {
		return result, err
	}
	here, err := deps.Machine()
	if err != nil {
		return result, fmt.Errorf("lym adopt: read this machine's name: %w", err)
	}
	subjects := slices.Clone(slugs)
	if rootNamed {
		subjects = append(subjects, boardengine.RootSubject)
	}
	rerun := rerunOf(subjects)

	owners, err := deps.ReadOwners()
	if err != nil {
		return result, fmt.Errorf("lym adopt: read the board's owners: %w", err)
	}
	local, err := movedRuns(deps, owners, slugs, rootNamed, localRunRead(deps))
	if err != nil {
		return result, stopWith(reentry, "lym adopt: read the runs under the adopted projects, so nothing ran: %w", err)
	}
	if err := refuseStandingMove(deps, slices.Concat(slugs, openRunSlugs(local))); err != nil {
		return result, err
	}

	identity, err := deps.hereIdentity(here)
	if err != nil {
		return result, fmt.Errorf("lym adopt: read this machine's identity: %w", err)
	}
	write, err := deps.WriteAdopt(boardengine.AdoptRequest{
		Machine: here, Identity: identity, Slugs: slugs, Prime: rootNamed,
		TakeHold: req.TakeHold, ParentOf: deps.ParentOf, Now: deps.Now(),
	})
	if err != nil {
		return result, ownerWriteError(err, rerun)
	}
	hold := write.Hold
	result.Hold = &hold
	result.PrimeRecorded = slices.Contains(write.Recorded, boardengine.OwnerWritePrime)
	outcomes := map[string]*slugOutcome{}
	for _, slug := range slugs {
		outcomes[slug] = &slugOutcome{Slug: slug, OwnerRecorded: slices.Contains(write.Recorded, slug)}
	}
	finish := func(err error) (outcome, error) {
		result.Slugs = collect(slugs, outcomes)
		return result, err
	}
	printStep(steps, "hold")

	// The write may have registered this machine, so the identity is read again for the hold comparisons.
	if identity, err = deps.hereIdentity(here); err != nil {
		return finish(fmt.Errorf("lym adopt: read this machine's identity: %w", err))
	}
	if err := holdStands(deps, identity, subjects); err != nil {
		return finish(err)
	}
	if owners, err = deps.ReadOwners(); err != nil {
		return finish(fmt.Errorf("lym adopt: read the board's owners: %w", err))
	}
	for _, slug := range slugs {
		if deps.LocalPair(slug) {
			continue
		}
		if _, err := deps.AddPair(slug); err != nil {
			return finish(stopWith("re-run `"+rerun+"`", "lym adopt: add the pair of %s: %s", slug, addError(slug, rerun, err)))
		}
		outcomes[slug].PairAdded = true
		deps.PickAccount(slug)
	}
	printStep(steps, "pairs")

	if err := holdStands(deps, identity, subjects); err != nil {
		return finish(err)
	}
	if err := takeOverRuns(deps, hold, owners, identity, slugs, rootNamed, outcomes); err != nil {
		return finish(err)
	}
	printStep(steps, "takeover")

	if err := holdStands(deps, identity, subjects); err != nil {
		return finish(err)
	}
	if err := deps.CatchUp(); err != nil {
		wayForward := "re-run `" + rerun + "`"
		if errors.Is(err, dobbyengine.ErrOriginUnreachable) {
			wayForward += " once origin answers"
		}
		return finish(stopWith(wayForward, "lym adopt: the catch-up failed, so the deploy and the check did not run: %w", err))
	}
	printStep(steps, "catchup")

	if rootNamed {
		if err := holdStands(deps, identity, subjects); err != nil {
			return finish(err)
		}
		deployed, warning, err := deployStale(deps, rerun)
		result.Deploy = deployed
		if err != nil {
			return finish(err)
		}
		if warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}
		printStep(steps, "deploy")
	}

	if err := holdStands(deps, identity, subjects); err != nil {
		return finish(err)
	}
	if err := deps.Workspace(); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("the hub workspace files were not regenerated (%v); way forward: run `lym fabric reconcile` in the prime", err))
	}
	printStep(steps, "workspace")

	if err := holdStands(deps, identity, subjects); err != nil {
		return finish(err)
	}
	checked, err := deps.HoldCheck(checkSubjects(rootNamed, slugs)[0], req.NoOrch)
	recordCheck(&result, outcomes, checked)
	if err != nil {
		return finish(err)
	}
	lines := strings.Join(checked.Lines, "; ")
	switch {
	case checked.HoldLost:
		return finish(fmt.Errorf("lym adopt: nothing more runs here, and no dobby starts: %s", lines))
	case !checked.Ran || checked.State == holdStateNoHold || checked.State == holdStateNotCleared:
		return finish(fmt.Errorf("lym adopt: the adopt check did not finish, so the hold stands and no dobby starts: %s", lines))
	}
	printStep(steps, "check")

	if checked.Green {
		result.Hold = nil
	} else if err := holdStands(deps, identity, subjects); err != nil {
		return finish(err)
	}
	runBeamSteps(deps, here, owners, slugs, req.NoOrch, &result, outcomes)
	printStep(steps, "beam")
	if checked.Green {
		return finish(nil)
	}
	return finish(failedCheckError(checked, rerun))
}

// recordCheck copies what the adopt check did into the outcome: the fix task, each subject's state and the lines posted.
func recordCheck(result *outcome, outcomes map[string]*slugOutcome, checked holdCheckResult) {
	result.FixTask = checked.FixTask
	result.CheckLines = checked.Lines
	for subject, state := range checked.States {
		if subject == boardengine.RootSubject {
			result.PrimeCheck = state
		} else if out := outcomes[subject]; out != nil {
			out.Check = state
		}
	}
}

// failedCheckError ends an adopt whose check failed, naming the fix task and the lines the check posted.
func failedCheckError(checked holdCheckResult, rerun string) error {
	named := "the adopt check failed"
	if checked.FixTask != "" {
		named += ", and fix task " + checked.FixTask + " stands"
	}
	return stopWith("`"+rerun+"` checks the hold again", "lym adopt: %s, so the hold stands until a check goes green: %s", named, strings.Join(checked.Lines, "; "))
}

// holdStands pulls Bolt and reads the hold again.
// It returns nil while a hold of this machine over subjects stands.
// Once another machine holds the hub, or the hold cleared, it returns a stop.
// The stop says the adopt moved or cleared and names the subjects' owner, so nothing more runs here.
func holdStands(deps adoptDeps, identity machinename.Identity, subjects []string) error {
	rerun := rerunOf(subjects)
	if err := deps.PullBolt(); err != nil {
		return stopWith("re-run `"+rerun+"` once origin answers", "lym adopt: pull Bolt to read the hold again, so nothing more ran: %w", err)
	}
	owners, err := deps.ReadOwners()
	if err != nil {
		return stopWith("re-run `"+rerun+"`", "lym adopt: read the hold again, so nothing more ran: %w", err)
	}
	var other *boardengine.Hold
	for _, hold := range owners.Holds {
		switch {
		case owners.Registry.ReadsAsHere(hold.Machine, identity):
			if sameSubjects(holdSubjectNames(hold), subjects) {
				return nil
			}
		case other == nil:
			standing := hold
			other = &standing
		}
	}
	if other != nil {
		return stopWith("wait for that machine's adopt to finish, and if that machine has stopped, ask the operator",
			"lym adopt: machine %s holds the hub now, so the adopt of %s moved to that machine and nothing more runs here, and no dobby starts", other.Machine, strings.Join(subjects, " "))
	}
	owner := owners.Prime
	if subjects[0] != boardengine.RootSubject {
		if owner, err = owners.Effective(subjects[0], deps.ParentOf); err != nil {
			owner = "unknown"
		}
	}
	if owner == "" {
		owner = "no machine"
	}
	return stopWith("run `"+rerun+"` if the runs do not go on",
		"lym adopt: the hold over %s no longer stands: it cleared meanwhile and %s owns %s, so nothing more runs here, and no dobby starts", strings.Join(subjects, " "), owner, subjects[0])
}

// movedRun reports whether the run slug, moved with subject, was left on another machine by the adopt hold names.
// takeover is the takeover entry the run's record at origin holds, nil when it holds none.
// An entry naming this machine means an earlier pass of the adopt took the run, so it is not moved,
// and an entry naming another machine moves it, a fix task's run included.
// With no entry the fix task of an adopt-fix hold that this machine filed is not moved.
// Any other run is moved when the subject's prior owner in the hold is another machine.
// A re-run after a kill so moves the same runs.
func movedRun(hold boardengine.Hold, registry boardengine.Registry, identity machinename.Identity, subject, slug string, takeover *shedengine.Takeover) bool {
	if takeover != nil {
		return !registry.ReadsAsHere(takeover.Machine, identity)
	}
	if hold.Kind == boardengine.HoldKindAdoptFix && slug == hold.FixTask {
		filer := hold.FixMachine
		if filer == "" {
			filer = hold.Machine
		}
		if registry.ReadsAsHere(filer, identity) {
			return false
		}
	}
	for _, held := range hold.Subjects {
		if held.Subject == subject {
			return boardengine.OwnedElsewhere(held.PriorOwner, registry, identity)
		}
	}
	return false
}

// takeOverRuns takes each run moved from another machine over from origin.
// The runs are the open runs under the adopted beams, and under root when rootAdopted, judged by `movedRun` over hold.
// A run whose pair this machine lacks gets it from origin first.
// A beam under an adopted root that keeps its own owner on another machine is reported as owned elsewhere and left.
// A refused pair add or a failed takeover ends the adopt, naming the run, the error and the hold's re-run.
func takeOverRuns(deps adoptDeps, hold boardengine.Hold, owners boardengine.Owners, identity machinename.Identity, adopted []string, rootAdopted bool, outcomes map[string]*slugOutcome) error {
	rerun := rerunOf(holdSubjectNames(hold))
	open, err := movedRuns(deps, owners, adopted, rootAdopted, deps.OriginRun)
	if err != nil {
		return stopWith("re-run `"+rerun+"` once origin answers", "lym adopt: read the runs at origin, so no run moved: %w", err)
	}
	for _, run := range open {
		out := outcomes[run.Slug]
		if out == nil {
			out = &slugOutcome{Slug: run.Slug}
			outcomes[run.Slug] = out
		}
		entry := owners.Entries[run.Slug]
		if entry.IsProject && entry.Beam && boardengine.OwnedElsewhere(entry.Owner, owners.Registry, identity) && !slices.Contains(adopted, run.Slug) {
			out.Run, out.OwnedBy = ownedElsewhereRun, entry.Owner
			continue
		}
		subject, err := projectOf(deps, adopted, run.Slug)
		if err != nil {
			return stopWith("re-run `"+rerun+"`", "lym adopt: read the parent of %s, so no run moved: %w", run.Slug, err)
		}
		if !movedRun(hold, owners.Registry, identity, subject, run.Slug, run.Takeover) {
			continue
		}
		if !deps.LocalPair(run.Slug) {
			if _, err := deps.AddPair(run.Slug); err != nil {
				return stopWith("re-run `"+rerun+"`", "lym adopt: add the pair of the moved run %s: %s", run.Slug, addError(run.Slug, rerun, err))
			}
			out.PairAdded = true
			deps.PickAccount(run.Slug)
		}
		if _, err := deps.TakeOver(run.Slug, subject); err != nil {
			return stopWith("re-run `"+rerun+"`", "lym adopt: the takeover of run %s failed: %w", run.Slug, err)
		}
		out.Run = takenOverRun
	}
	return nil
}
