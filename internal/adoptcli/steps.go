// steps.go holds the eight steps of an adopt, in the order the worker runs them:
// reconcile, pairs, pause, owner, deploy, beam, check and done.
// Each is a function over the step run that writes its outcome into the run's working part.

package adoptcli

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// stepReconcile reconciles the hub against origin and judges which entries a takeover would stop.
// It clears every input but the paused slugs first, so a restart never carries an earlier run's decisions.
// It refuses an unreachable origin, a reconcile error and a stop on a shared branch.
// It reads the runs the reconcile may have brought.
// It decides which projects a local-commit or live-pair stop holds back.
func stepReconcile(run *stepRun) error {
	deps := run.Deps
	inputs := &run.Record.Inputs
	*inputs = adoptrecord.Inputs{Paused: inputs.Paused, LocalRuns: run.LocalRuns}

	report, err := deps.Reconcile()
	run.Report = report
	run.Part.Reconcile = report
	run.Mutations.Extend(report.Mutations)
	if err != nil {
		return stopWith("repair what the error names, "+reentry, "lym adopt: the origin reconcile failed, so nothing ran: %w", err)
	}
	if report.Unreachable {
		return stopWith(reentry+" once origin answers", "lym adopt: origin was unreachable, so nothing was judged against it and nothing ran")
	}
	if stops := sharedStopText(report); stops != "" {
		return stopWith(stops, "lym adopt: the origin reconcile stopped, so nothing ran")
	}

	// The reconcile may have brought the run records of the other machine's runs,
	// so the moved runs are read again.
	moved, err := movedRuns(deps, run.Owners, run.Slugs, run.RootNamed)
	if err != nil {
		return stopWith(reentry, "lym adopt: read the runs under the adopted projects, so nothing ran: %w", err)
	}
	inputs.Moved = moved

	// A beam that keeps its own owner on another machine is moved by an adopted root as a run only:
	// it is reported, and neither taken over nor resumed.
	run.RunSlugs = nil
	for _, slug := range moved {
		entry := run.Owners.Entries[slug]
		keepsOwner := entry.IsProject && entry.Beam && boardengine.OwnedElsewhere(entry.Owner, run.Here) && !slices.Contains(run.Slugs, slug)
		out := run.slot(slug)
		if keepsOwner {
			out.Run = ownedElsewhereRun
			out.OwnedBy = entry.Owner
			continue
		}
		run.RunSlugs = append(run.RunSlugs, slug)
	}

	// A project one of whose runs stops is held back whole,
	// so no owner is written over a pair that still holds local state.
	run.Acted = slices.Clone(run.Slugs)
	for _, slug := range run.RunSlugs {
		if !slices.Contains(run.Slugs, slug) {
			run.Acted = append(run.Acted, slug)
		}
	}
	run.Projects = map[string]string{}
	run.Stops = map[string]bool{}
	run.Held = map[string]bool{}
	run.HeldBack = map[string]bool{}
	for _, slug := range run.Acted {
		project, err := projectOf(deps, run.Slugs, slug)
		if err != nil {
			return stopWith(reentry, "lym adopt: read the parent of %s, so nothing ran: %w", slug, err)
		}
		run.Projects[slug] = project
		stopped, err := wouldStop(deps, run.Req, report, slug)
		if err != nil {
			return stopWith(reentry, "lym adopt: read whether the pair of %s is live, so nothing ran: %w", slug, err)
		}
		if stopped {
			run.Stops[slug] = true
			run.Held[project] = true
			run.HeldBack[project] = true
		}
	}
	return nil
}

// stepPairs takes over each entry the reconcile left going and adds the pairs this machine lacks.
// It decides which beams are adoptable and whether the prime goes, and which projects a stop held back.
func stepPairs(run *stepRun) error {
	for _, slug := range run.Acted {
		project := run.Projects[slug]
		out := run.slot(slug)
		if run.Held[project] && !run.Stops[slug] {
			out.Warning = fmt.Sprintf("not taken over: another run under `%s` stopped, so the adopt holds it back whole", project)
			continue
		}
		if !takeOverSlug(run.Deps, run.Req, run.Mutations, run.Report, run.Owners, project, out) {
			run.Held[project] = true
			if out.Stopped {
				run.HeldBack[project] = true
			}
		}
	}

	var adoptable []string
	for _, slug := range run.Slugs {
		if run.Held[slug] {
			run.slot(slug).Warning = "not adopted: it or a run under it stopped, so its owner stays; the stop names the way forward"
			continue
		}
		adoptable = append(adoptable, slug)
	}

	inputs := &run.Record.Inputs
	inputs.Runs = run.RunSlugs
	inputs.Adoptable = adoptable
	inputs.RootGoing = !run.Held[boardengine.RootSubject] && (run.RootNamed || (run.Owners.Prime == "" && len(adoptable) > 0))
	inputs.HeldBack = nil
	for project := range run.HeldBack {
		inputs.HeldBack = append(inputs.HeldBack, project)
	}
	slices.Sort(inputs.HeldBack)
	return nil
}

// stepPause pauses the runs and released tasks picked from the owners as the reconcile left them,
// and the adopted beams that hold an owner pause of their own.
// A pause lasts until an operator lifts it.
// A slug whose scope another caller already pauses keeps that entry and is not paused.
func stepPause(run *stepRun) error {
	deps := run.Deps
	inputs := &run.Record.Inputs
	if len(inputs.Adoptable) == 0 && !inputs.RootGoing {
		return nil
	}
	reconciled, err := deps.ReadOwners()
	if err != nil {
		return stopWith(reentry, "lym adopt: read the board's owners after the reconcile: %w, so no owner was recorded", err)
	}
	tasks, err := deps.Tasks()
	if err != nil {
		return stopWith(reentry, "lym adopt: read the board's entries: %w, so no owner was recorded", err)
	}
	targets, err := pauseTargets(deps, run.Here, reconciled, tasks, inputs.Runs, inputs.Adoptable, inputs.RootGoing, run.Outcomes)
	if err != nil {
		return stopWith(reentry, "lym adopt: %w, so no owner was recorded", err)
	}
	if len(targets)+len(inputs.Adoptable) == 0 {
		return nil
	}

	var paused []string
	var other map[string]string
	if err := deps.UpdatePause(func(record *pauserecord.Record, now time.Time) error {
		paused, other = pauseEntries(record, targets, inputs.Adoptable, inputs.Paused, now)
		return nil
	}); err != nil {
		return stopWith("re-run `"+adoptrecord.Request(run.Req).Command()+"`", "lym adopt: pause the runs in the pause record: %w, so no owner was recorded", err)
	}

	inputs.Paused = append(inputs.Paused, paused...)
	slices.Sort(inputs.Paused)
	inputs.Paused = slices.Compact(inputs.Paused)
	// Every slug the record lists reports its pause, including one an earlier process paused.
	// A re-run of this step replaces that process's part of it.
	for _, slug := range inputs.Paused {
		out := run.slot(slug)
		out.AdoptPaused = true
		out.Run = pausedRun
		out.PauseStanding = fmt.Sprintf("paused by adopt: `lym tender resume %s` lifts it", slug)
	}
	for slug, caller := range other {
		run.slot(slug).PauseStanding = fmt.Sprintf("a pause of its own scope by %s covers it: `lym tender resume %s` lifts it", caller, slug)
	}
	return nil
}

// stepOwner writes this machine as the owner of every beam not held back, and of the prime when it goes.
// It reads the owners back and settles each adopted beam and each run moved with them that no stop or error kept back.
// It ends with the warnings for the branches the reconcile left unmoved.
func stepOwner(run *stepRun) error {
	deps := run.Deps
	inputs := &run.Record.Inputs
	inputs.OwnerStood = nil
	inputs.PrimeRecorded = false

	var adopted []string
	if len(inputs.Adoptable) > 0 || inputs.RootGoing {
		write, err := deps.SetOwners(boardengine.OwnerRequest{Machine: run.Here, Slugs: inputs.Adoptable, Prime: inputs.RootGoing, Takeover: true, ParentOf: deps.ParentOf})
		if err != nil {
			return ownerWriteError(err)
		}
		for _, slug := range write.Recorded {
			if slug == boardengine.OwnerWritePrime {
				run.Part.PrimeRecorded = true
				inputs.PrimeRecorded = true
				continue
			}
			run.slot(slug).OwnerRecorded = true
			adopted = append(adopted, slug)
		}
		for slug, owner := range write.Lost {
			if slug == boardengine.OwnerWritePrime {
				run.Part.PrimeLost = owner
				continue
			}
			run.slot(slug).Error = fmt.Sprintf("another machine recorded %s as the owner while this write was in flight, so the adopt check of %s starts nothing for it: `lym adopt %s` takes it over again", owner, slug, slug)
		}
	}
	inputs.OwnerStood = adopted

	if len(adopted) > 0 || inputs.PrimeRecorded {
		after, err := deps.ReadOwners()
		if err != nil {
			return stopWith(reentry, "lym adopt: the owners were recorded, but reading them back failed, so the beam steps did not run: %w", err)
		}
		// Settled in slug order: the adopted beams, then the runs moved with them that no stop or error kept back.
		settled := slices.Clone(adopted)
		for _, slug := range inputs.Runs {
			out := run.Outcomes[slug]
			if !slices.Contains(settled, slug) && !out.Stopped && out.Error == "" && out.Warning == "" {
				settled = append(settled, slug)
			}
		}
		slices.Sort(settled)
		for _, slug := range settled {
			resumeSlug(deps, run.Here, after, run.slot(slug), slices.Contains(inputs.Paused, slug))
		}
	}

	run.Part.Warnings = reconcileWarnings(run.Report, adopted, run.Outcomes, run.Req)
	return nil
}

// stepDeploy runs the landing deploy when the prime goes and its head is ahead of the running lym's build.
// The owner is written and the adopted runs are paused by then, so a failed deploy stops the record with none going on.
func stepDeploy(run *stepRun) error {
	if !run.Record.Inputs.RootGoing {
		return nil
	}
	deployed, warning, err := deployStale(run.Deps)
	run.Part.Deploy = deployed
	if err != nil {
		return err
	}
	if warning != "" {
		run.Part.Warnings = append(run.Part.Warnings, warning)
	}
	return nil
}

// stepBeam runs the beam steps of the prime when its owner was recorded, then of each beam whose owner write stood.
// It works over the owners as they read now.
func stepBeam(run *stepRun) error {
	inputs := run.Record.Inputs
	if !inputs.PrimeRecorded && len(inputs.OwnerStood) == 0 {
		return nil
	}
	after, err := run.Deps.ReadOwners()
	if err != nil {
		return stopWith(reentry, "lym adopt: the owners were recorded, but reading them back failed, so the beam steps did not run: %w", err)
	}
	run.Part.PrimeRecorded = inputs.PrimeRecorded
	for _, slug := range inputs.OwnerStood {
		run.slot(slug).AdoptPaused = slices.Contains(inputs.Paused, slug)
	}
	runBeamSteps(run.Deps, run.Here, after, inputs.OwnerStood, run.Req.NoOrch, &run.Part, run.PartSlugs)
	return nil
}

// stepCheck starts the adopt check of root when the prime's owner was recorded.
// It then starts the check of each beam whose owner write stood.
// It resets each subject's fix-try count first.
// It ends by building the per-run report over the runs the takeover moved and paused.
func stepCheck(run *stepRun) error {
	deps := run.Deps
	inputs := run.Record.Inputs
	for _, subject := range checkSubjects(inputs.PrimeRecorded, inputs.OwnerStood) {
		if err := deps.ResetTries(subject); err != nil {
			run.Part.CheckWarnings = append(run.Part.CheckWarnings, fmt.Sprintf("the adopt check of %s was not started (%v); way forward: re-run lym adopt %s", subject, err, subject))
			continue
		}
		start, err := deps.StartSuite(subject)
		if err != nil {
			run.Part.CheckWarnings = append(run.Part.CheckWarnings, fmt.Sprintf("the adopt check of %s was not started (%v); way forward: re-run lym adopt %s", subject, err, subject))
			continue
		}
		run.Part.Checks = append(run.Part.Checks, start)
	}
	run.Part.SuiteMessage = suiteMessage(run.Part.Checks)

	// The subjects whose runs this call reports: the ones it names, and root when the prime's owner is written.
	named := slices.Clone(run.Slugs)
	if run.RootNamed || inputs.RootGoing {
		named = append(named, boardengine.RootSubject)
	}
	reports, err := reportRuns(deps, run.Here, inputs.LocalRuns, inputs.Moved, inputs.Paused, named)
	if err != nil {
		run.Part.Warnings = append(run.Part.Warnings, fmt.Sprintf("the report of the runs was not built (%v); way forward: fix the cause it names, %s", err, reentry))
	}
	run.Part.Runs = reports
	return nil
}

// stepDone ends the adopt.
// It stops the record when a local-commit or live-pair stop held a project back.
// The stop names the re-run that records the owner over it and the one that drops the local commits.
// Each re-run names the subjects of the whole adopt, so the standing record resumes under it.
func stepDone(run *stepRun) error {
	heldBack := run.Record.Inputs.HeldBack
	if len(heldBack) == 0 {
		return nil
	}
	rerun := adoptrecord.Request(run.Req)
	keep, drop := rerun, rerun
	keep.KeepLocal, keep.DropLocal = true, false
	drop.KeepLocal, drop.DropLocal = false, true
	wayForward := fmt.Sprintf("`%s` records the owner over the local state, and `%s` keeps each local tip under an archive tag and moves the branches to origin's", keep.Command(), drop.Command())
	return stopWith(wayForward, "lym adopt: %s held back by a stop on local commits origin lacks or on a live pair, so its owner stays", strings.Join(heldBack, ", "))
}
