// adopt.go is the takeover sequence of "lyx adopt": it validates the request, reconciles the hub against origin, then per adopted beam project and per run under it or under root drops or keeps local commits, adds the missing pair, records this machine as owner of the projects, lifts the owner pauses, resumes the runs, runs the beam steps of each adopted beam and starts the test suite.

package adoptcli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

// callerLabel is the strand name adopt acts under when it lifts a pause.
const callerLabel = "adopt"

// reentry is what a refusal that adopt's own steps leave undone is followed by.
const reentry = "then re-run lyx adopt with the same arguments"

// The repository sides a branch outcome names that a stop on is shared by every pair.
const (
	sideBolt    = "bolt"
	sideRecords = "records"
)

// ownedElsewhereRun is the run outcome of a moved run whose entry reads owned by another machine after the owner write.
const ownedElsewhereRun = "owned elsewhere"

// request is what the verb asks for: the named beam project slugs, root, every adoptable project, and how local commits origin lacks are answered.
// The word `root` among the slugs sets Root and is dropped from them.
type request struct {
	Slugs     []string
	Root      bool
	All       bool
	KeepLocal bool
	DropLocal bool
}

// branchStop names one local branch of an entry and the state the reconcile judged it in.
type branchStop struct {
	Branch string `json:"branch"`
	State  string `json:"state"`
}

// slugOutcome is what happened to one entry, an adopted beam project or a run moved with a project: a stop with its branches, the pair added, the owner recorded, the pause lifted or standing, the run resumed, none or owned elsewhere by OwnedBy, the beam steps run, the error and a warning.
// WayForward is the stop's, or for an adopted beam whose pawl run holds the landing hold, the way to follow the landing and to take its parent over.
type slugOutcome struct {
	Slug          string            `json:"slug"`
	Stopped       bool              `json:"stopped,omitempty"`
	Branches      []branchStop      `json:"branches,omitempty"`
	Live          bool              `json:"live,omitempty"`
	WayForward    string            `json:"way_forward,omitempty"`
	Dropped       []string          `json:"dropped_tags,omitempty"`
	PairAdded     bool              `json:"pair_added,omitempty"`
	OwnerRecorded bool              `json:"owner_recorded,omitempty"`
	PauseLifted   bool              `json:"pause_lifted,omitempty"`
	PauseStanding string            `json:"pause_standing,omitempty"`
	Run           string            `json:"run,omitempty"`
	OwnedBy       string            `json:"owned_by,omitempty"`
	ChildResumed  bool              `json:"child_resumed,omitempty"`
	BeamSteps     *beamStepsOutcome `json:"beam_steps,omitempty"`
	Warning       string            `json:"warning,omitempty"`
	Error         string            `json:"error,omitempty"`
}

// beamStepsOutcome is what the beam steps did for one adopted worktree: the dobby's outcome and detail, the orch's action, prompt source and abandoned session, and the warnings that did not stop them.
type beamStepsOutcome struct {
	Dobby            string   `json:"dobby,omitempty"`
	DobbyDetail      string   `json:"dobby_detail,omitempty"`
	Action           string   `json:"action,omitempty"`
	PromptSource     string   `json:"prompt_source,omitempty"`
	AbandonedSession string   `json:"abandoned_session,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
}

// beamStepsOf returns the outcome the report of a BeamUp reads as.
func beamStepsOf(report orchcli.BeamUpReport) *beamStepsOutcome {
	return &beamStepsOutcome{
		Dobby: report.Dobby, DobbyDetail: report.DobbyDetail, Action: report.Action,
		PromptSource: report.PromptSource, AbandonedSession: report.AbandonedSession, Warnings: report.Warnings,
	}
}

// suiteStart names the detached test suite an adopt started: its pid and the log its output goes to.
type suiteStart struct {
	PID int    `json:"pid"`
	Log string `json:"log"`
}

// outcome is the verb's envelope: the reconcile's report, the mutation record of every step, whether that record is non-empty, the prime's owner write, each entry's slugOutcome, the beam steps of the prime and the warning of a prime start that failed, and the test suite started or the warning of a start that failed.
type outcome struct {
	Reconcile        fabricengine.OriginReport
	Mutations        []fabricengine.Mutation
	Partial          bool
	PrimeRecorded    bool
	PrimeLost        string
	Slugs            []slugOutcome
	PrimeBeamSteps   *beamStepsOutcome
	PrimeBeamWarning string
	Suite            *suiteStart
	SuiteWarning     string
}

// adoptDeps holds the seam of every step, which hubDeps fills over the hub and the tests fake.
// LocalPair reports whether this machine holds the slug's pair worktree.
// ParentOf names the slug of the pair a slug was forked from, or "" for the prime.
// RunRecord reads the batten run record of a slug, reporting false when none exists.
// ResumeRun starts the run's first pass detached, driven on by its parent's dobby, and returns the envelope of that start, where an envelope whose "ok" is false is a refusal.
// MoveInProgress reports whether an anchor change stands in the worktree called worktree, "" naming the prime, and returns the way forward it names; nil reads as no record standing.
// BeamUp runs the beam steps of the named worktree, its dobby and its orch, with no hub preparation, since the adopt's own first step reconciled origin; nil runs none.
// It runs after the resumes for each adopted beam whose owner write stood and, when root was adopted, for the prime.
// LandingHeld reports whether the pawl run of the beam slug holds the beam's landing hold, which keeps its beam steps from running; nil reads as no hold.
// PrimeName is the prime worktree's directory name.
// StartSuite starts the hub's finalize gate in the prime as a detached process and returns at once, so a break this machine brings to the landed code shows at adopt time; it runs last, once per call that recorded an owner.
// HubPath roots the mutation record.
type adoptDeps struct {
	Machine        func() (string, error)
	ReadOwners     func() (boardengine.Owners, error)
	ParentOf       func(slug string) (string, error)
	Reconcile      func() (fabricengine.OriginReport, error)
	LocalPair      func(slug string) bool
	LivePair       func(slug string) (bool, error)
	AddPair        func(slug string) (fabricengine.AddResult, error)
	DropLocal      func(rec *fabricengine.Mutations, slug string) (fabricengine.DropLocalResult, error)
	SetOwners      func(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error)
	PauseRecord    func() (pauserecord.Record, error)
	Lift           func(ref tenderengine.EntryRef) error
	RunRecord      func(slug string) (shedengine.Status, bool, error)
	ResumeRun      func(slug string) (map[string]any, error)
	ChildStatus    func(slug string) (shedengine.Status, bool, error)
	ResumeChild    func(slug string) error
	MoveInProgress func(worktree string) (wayForward string, standing bool, err error)
	BeamUp         func(worktree string) (orchcli.BeamUpReport, error)
	LandingHeld    func(slug string) (bool, error)
	StartSuite     func() (suiteStart, error)
	PrimeName      string
	HubPath        string
}

// adopt takes the requested beam projects, and root, over from another machine together with the runs under them, and returns what each step did.
// A refusal before any entry is touched returns the error with the outcome gathered so far, so the envelope still carries the reconcile and its mutation record.
// A standing anchor change in a worktree it would act under refuses before the reconcile, with an empty outcome.
func adopt(deps adoptDeps, req request) (outcome, error) {
	if slices.Contains(req.Slugs, boardengine.RootSubject) {
		req.Root = true
		req.Slugs = slices.DeleteFunc(slices.Clone(req.Slugs), func(slug string) bool { return slug == boardengine.RootSubject })
	}
	if req.KeepLocal && req.DropLocal {
		return outcome{}, errors.New("lyx adopt: --keep-local and --drop-local answer the same local commits in opposite ways: pass one of them")
	}
	if req.All && len(req.Slugs) > 0 {
		return outcome{}, errors.New("lyx adopt: --all takes no slug: drop the slugs, or drop --all")
	}
	if len(req.Slugs) == 0 && !req.Root && !req.All {
		return outcome{}, errors.New("lyx adopt: name a beam project or `root`, or pass --all: `lyx board list` shows the entries")
	}

	here, err := deps.Machine()
	if err != nil {
		return outcome{}, fmt.Errorf("lyx adopt: read this machine's name: %w", err)
	}
	owners, err := deps.ReadOwners()
	if err != nil {
		return outcome{}, fmt.Errorf("lyx adopt: read the board's owners: %w", err)
	}
	slugs, rootNamed, err := namedEntries(deps, owners, req)
	if err != nil {
		return outcome{}, err
	}

	// The runs on this machine's own records stand in for the ones the reconcile may still bring, so a worktree they act under is checked before it.
	localRuns, err := movedRuns(deps, owners, slugs, rootNamed)
	if err != nil {
		return outcome{}, fmt.Errorf("lyx adopt: read the runs under the adopted projects, so nothing ran: %w; %s", err, reentry)
	}
	if err := refuseStandingMove(deps, append(slices.Clone(slugs), localRuns...)); err != nil {
		return outcome{}, err
	}

	rec := fabricengine.NewMutations(deps.HubPath)
	var result outcome
	finish := func() outcome {
		result.Mutations = rec.Entries()
		result.Partial = rec.Len() > 0
		return result
	}

	report, err := deps.Reconcile()
	result.Reconcile = report
	rec.Extend(report.Mutations)
	if err != nil {
		return finish(), fmt.Errorf("lyx adopt: the origin reconcile failed, so nothing ran: %w; repair what the error names, %s", err, reentry)
	}
	if report.Unreachable {
		return finish(), fmt.Errorf("lyx adopt: origin was unreachable, so nothing was judged against it and nothing ran: %s once origin answers", reentry)
	}
	if stops := sharedStopText(report); stops != "" {
		return finish(), fmt.Errorf("lyx adopt: the origin reconcile stopped, so nothing ran: %s", stops)
	}

	// The reconcile may have brought the run records of the other machine's runs, so the moved runs are read again.
	moved, err := movedRuns(deps, owners, slugs, rootNamed)
	if err != nil {
		return finish(), fmt.Errorf("lyx adopt: read the runs under the adopted projects, so nothing ran: %w; %s", err, reentry)
	}

	outcomes := map[string]*slugOutcome{}
	for _, slug := range slugs {
		outcomes[slug] = &slugOutcome{Slug: slug}
	}
	// A beam that keeps its own owner on another machine is moved by an adopted root as a run only: it is reported, and neither taken over nor resumed.
	var runs []string
	for _, slug := range moved {
		entry := owners.Entries[slug]
		keepsOwner := entry.IsProject && entry.Beam && boardengine.OwnedElsewhere(entry.Owner, here) && !slices.Contains(slugs, slug)
		outcomes[slug] = &slugOutcome{Slug: slug}
		if keepsOwner {
			outcomes[slug].Run = ownedElsewhereRun
			outcomes[slug].OwnedBy = entry.Owner
			continue
		}
		runs = append(runs, slug)
	}

	// A project one of whose runs stops is held back whole, so no owner is written over a pair that still holds local state.
	acted := slices.Clone(slugs)
	for _, slug := range runs {
		if !slices.Contains(slugs, slug) {
			acted = append(acted, slug)
		}
	}
	projects := map[string]string{}
	held := map[string]bool{}
	stops := map[string]bool{}
	for _, slug := range acted {
		project, err := projectOf(deps, slugs, slug)
		if err != nil {
			return finish(), fmt.Errorf("lyx adopt: read the parent of %s, so nothing ran: %w; %s", slug, err, reentry)
		}
		projects[slug] = project
		stopped, err := wouldStop(deps, req, report, slug)
		if err != nil {
			return finish(), fmt.Errorf("lyx adopt: read whether the pair of %s is live, so nothing ran: %w; %s", slug, err, reentry)
		}
		if stopped {
			stops[slug] = true
			held[project] = true
		}
	}

	for _, slug := range acted {
		project := projects[slug]
		if held[project] && !stops[slug] {
			outcomes[slug].Warning = fmt.Sprintf("not taken over: another run under `%s` stopped, so the adopt holds it back whole", project)
			continue
		}
		if !takeOverSlug(deps, req, rec, report, owners, project, outcomes[slug]) {
			held[project] = true
		}
	}

	var adoptable []string
	for _, slug := range slugs {
		if held[slug] {
			outcomes[slug].Warning = "not adopted: it or a run under it stopped, so its owner stays; the stop names the way forward"
			continue
		}
		adoptable = append(adoptable, slug)
	}
	rootGoing := !held[boardengine.RootSubject] && (rootNamed || (owners.Prime == "" && len(adoptable) > 0))

	var adopted []string
	if len(adoptable) > 0 || rootGoing {
		write, err := deps.SetOwners(boardengine.OwnerRequest{Machine: here, Slugs: adoptable, Prime: rootGoing, Takeover: true, ParentOf: deps.ParentOf})
		if err != nil {
			result.Slugs = collect(slugs, outcomes)
			return finish(), ownerWriteError(err)
		}
		for _, slug := range write.Recorded {
			if slug == boardengine.OwnerWritePrime {
				result.PrimeRecorded = true
				continue
			}
			outcomes[slug].OwnerRecorded = true
			adopted = append(adopted, slug)
		}
		for slug, owner := range write.Lost {
			if slug == boardengine.OwnerWritePrime {
				result.PrimeLost = owner
				continue
			}
			outcomes[slug].Error = fmt.Sprintf("another machine recorded %s as the owner while this write was in flight, so nothing was resumed: `lyx adopt %s` takes it over again", owner, slug)
		}
	}

	if len(adopted) > 0 || result.PrimeRecorded {
		after, err := deps.ReadOwners()
		if err != nil {
			result.Slugs = collect(slugs, outcomes)
			return finish(), fmt.Errorf("lyx adopt: the owners were recorded, but reading them back failed, so nothing was resumed: %w; %s", err, reentry)
		}
		// Resumed in slug order: the adopted beams, then the runs moved with them that no stop or error kept back.
		resumable := slices.Clone(adopted)
		for _, slug := range runs {
			out := outcomes[slug]
			if !slices.Contains(resumable, slug) && !out.Stopped && out.Error == "" && out.Warning == "" {
				resumable = append(resumable, slug)
			}
		}
		slices.Sort(resumable)
		for _, slug := range resumable {
			resumeSlug(deps, here, after, outcomes[slug])
		}
		resumeHaltedChildren(deps, here, after, adopted, result.PrimeRecorded, outcomes)
		runBeamSteps(deps, here, after, adopted, &result, outcomes)
	}

	if len(adopted) > 0 || result.PrimeRecorded {
		suite, err := deps.StartSuite()
		if err != nil {
			result.SuiteWarning = fmt.Sprintf("the test suite was not started (%v); way forward: run `lyx gate test --gate finalize` in the prime", err)
		} else {
			result.Suite = &suite
		}
	}

	result.Slugs = collect(slugs, outcomes)
	return finish(), nil
}

// namedEntries resolves the request to the beam project slugs it adopts and whether it adopts root.
// --all names root and every beam project that is not done.
// It refuses a slug the board does not hold, a note, a done entry, a task, naming the beam or `root` that moves its run, and a grouping project, naming the same adopt and `lyx beam create`.
func namedEntries(deps adoptDeps, owners boardengine.Owners, req request) (slugs []string, root bool, err error) {
	if req.All {
		for slug, entry := range owners.Entries {
			if !entry.Done && entry.IsProject && entry.Beam {
				slugs = append(slugs, slug)
			}
		}
		slices.Sort(slugs)
		return slugs, true, nil
	}
	for _, slug := range req.Slugs {
		entry, held := owners.Entries[slug]
		switch {
		case !held:
			return nil, false, fmt.Errorf("lyx adopt: %q is not on the board: `lyx board list` shows the entries", slug)
		case !entry.IsTask && !entry.IsProject:
			return nil, false, fmt.Errorf("lyx adopt: %q is a note, not a task: `lyx board promote %s` makes it a task first", slug, slug)
		case entry.Done:
			return nil, false, fmt.Errorf("lyx adopt: %q is done and already landed, so there is nothing to adopt: `lyx tender resume %s` lets a run that landed there go on to Land under its owner pause", slug, slug)
		case entry.IsTask || !entry.Beam:
			subject, err := owners.AdoptSubject(slug, deps.ParentOf)
			if err != nil {
				return nil, false, fmt.Errorf("lyx adopt: read the parent of %s: %w", slug, err)
			}
			if entry.IsTask {
				return nil, false, fmt.Errorf("lyx adopt: %q is a task, and a task is moved with its project: `lyx adopt %s` moves it", slug, subject)
			}
			return nil, false, fmt.Errorf("lyx adopt: %q is a grouping project, not a beam, so there is no pair to take over: `lyx adopt %s` moves its tasks, and `lyx beam create %s` makes it a beam", slug, subject, slug)
		}
		if !slices.Contains(slugs, slug) {
			slugs = append(slugs, slug)
		}
	}
	return slugs, req.Root, nil
}

// movedRuns lists, sorted, the runs an adopt of the beams adopted, and of root when rootAdopted, moves with them.
// A run is moved when its entry is a task or a beam project that is not done, holds a batten run record that is not done, and is a child of an adopted beam, or of root when root is adopted;
// the pawl run of each adopted beam is moved with it.
// A beam that is a member of an adopted beam, or of root, is listed too, though it keeps its own owner.
func movedRuns(deps adoptDeps, owners boardengine.Owners, adopted []string, rootAdopted bool) ([]string, error) {
	var moved []string
	for slug, entry := range owners.Entries {
		if entry.Done || !(entry.IsTask || (entry.IsProject && entry.Beam)) {
			continue
		}
		status, found, err := deps.RunRecord(slug)
		if err != nil {
			return nil, fmt.Errorf("read the batten run record of %s: %w", slug, err)
		}
		if !found || status.State == shedengine.StateDone {
			continue
		}
		if !slices.Contains(adopted, slug) {
			parent, err := deps.ParentOf(slug)
			if err != nil {
				return nil, fmt.Errorf("read the parent of %s: %w", slug, err)
			}
			if !slices.Contains(adopted, parent) && !(parent == "" && rootAdopted) {
				continue
			}
		}
		moved = append(moved, slug)
	}
	slices.Sort(moved)
	return moved, nil
}

// projectOf returns the adopted project that moves slug: the slug itself when it is an adopted beam, else its parent beam, else RootSubject for a child of the prime.
func projectOf(deps adoptDeps, adopted []string, slug string) (string, error) {
	if slices.Contains(adopted, slug) {
		return slug, nil
	}
	parent, err := deps.ParentOf(slug)
	if err != nil {
		return "", err
	}
	if parent == "" {
		return boardengine.RootSubject, nil
	}
	return parent, nil
}

// sharedStopText renders each stop on Bolt or the prime's records branch as its way forward, followed by the re-entry where one retries it.
// It is empty when neither branch stopped.
func sharedStopText(report fabricengine.OriginReport) string {
	var stops []string
	for _, branch := range report.Branches {
		shared := branch.Repo == sideBolt || (branch.Repo == sideRecords && branch.Prime)
		if !shared {
			continue
		}
		reenters := false
		switch branch.State {
		case fabricengine.BranchDirty, fabricengine.BranchGone:
			reenters = true
		case fabricengine.BranchBusy:
			reenters = !branch.PushLockHeld
		case fabricengine.BranchRewound, fabricengine.BranchRewritten, fabricengine.BranchDiverged, fabricengine.BranchAnchorStopped:
		default:
			continue
		}
		line := fmt.Sprintf("%s (%s): %s", branch.Branch, branch.State, branch.WayForward)
		if reenters {
			line += "; " + reentry
		}
		stops = append(stops, line)
	}
	return strings.Join(stops, "; ")
}

// stoppedBranches lists the branches of slug whose local commits origin lacks or whose history origin moved, in the report.
func stoppedBranches(report fabricengine.OriginReport, slug string) []branchStop {
	var stops []branchStop
	for _, branch := range report.Branches {
		if branch.Slug != slug {
			continue
		}
		switch branch.State {
		case fabricengine.BranchNeedsReplay, fabricengine.BranchAhead, fabricengine.BranchRewound, fabricengine.BranchRewritten, fabricengine.BranchDiverged, fabricengine.BranchAnchorStopped:
			stops = append(stops, branchStop{Branch: branch.Branch, State: string(branch.State)})
		}
	}
	return stops
}

// ownedElsewhere reports whether the report reads slug as owned by another machine.
func ownedElsewhere(report fabricengine.OriginReport, slug string) bool {
	for _, entry := range report.Entries {
		if entry.Slug == slug && entry.State == fabricengine.EntryOwnedElsewhere {
			return true
		}
	}
	return false
}

// takeOverSlug runs steps 1's tail and 2 for one entry, an adopted beam or a run moved with project: it stops the entry on local commits or a live pair, drops them under --drop-local, and adds the missing pair.
// project is the adopted beam or `root` whose adopt moves the entry, and the stop's way forward names it, since a task is never an adopt's argument.
// It reports whether the entry goes on; a stop or an error is recorded on its outcome.
func takeOverSlug(deps adoptDeps, req request, rec *fabricengine.Mutations, report fabricengine.OriginReport, owners boardengine.Owners, project string, out *slugOutcome) bool {
	slug := out.Slug
	hasPair := deps.LocalPair(slug)

	if ownedElsewhere(report, slug) && hasPair && !req.KeepLocal {
		branches := stoppedBranches(report, slug)
		live, err := deps.LivePair(slug)
		if err != nil {
			out.Error = fmt.Sprintf("read whether the pair is live: %v", err)
			return false
		}
		switch {
		case req.DropLocal && live:
			out.Error = fmt.Sprintf("the pair of %s is live, so its local commits are not dropped under it: pass --keep-local to record the owner over them, or end the pair's driver first", slug)
			return false
		case req.DropLocal:
			dropped, err := deps.DropLocal(rec, slug)
			out.Dropped = dropped.Tags
			if err != nil {
				out.Error = fmt.Sprintf("drop the local commits: %v", err)
				return false
			}
		case len(branches) > 0 || live:
			out.Stopped = true
			out.Branches = branches
			out.Live = live
			out.WayForward = fmt.Sprintf("the pair holds local state origin lacks or a live driver: `lyx adopt %s --keep-local` records the owner over it, and `lyx adopt %s --drop-local` keeps each local tip under an archive tag and moves the branches to origin's", project, project)
			return false
		}
	}

	entry := owners.Entries[slug]
	// A beam's pair is added the same way as a task's: Add adopts the live branches and their origin record, the beam mark included.
	ownedBeam := entry.IsProject && entry.Beam && entry.Owner != ""
	if !hasPair && (ownedBeam || entry.RunHeld) {
		added, err := deps.AddPair(slug)
		rec.Extend(added.Mutated())
		if err != nil {
			out.Error = addError(slug, project, err)
			return false
		}
		out.PairAdded = true
	}
	return true
}

// addError words a refused pair add.
// The leftover local branch's refusal is reworded, since the way forward fabric names would drop this machine's unpushed commits on that branch with no archive, and it ends with re-running the adopt of project, the beam or `root` that moves slug.
func addError(slug, project string, err error) string {
	var exists *fabricengine.ErrBranchExists
	if errors.As(err, &exists) {
		return fmt.Sprintf("the leftover local branch %s may hold commits origin lacks: keep its tip first with `git tag archive/%s/<tip> %s`, then `git branch -D %s`, then re-run `lyx adopt %s`", exists.Branch, slug, exists.Branch, exists.Branch, project)
	}
	return err.Error()
}

// ownerWriteError words a refused owner write with its way forward and the re-run.
func ownerWriteError(err error) error {
	if errors.Is(err, boardengine.ErrOwnerOriginUnreachable) {
		return fmt.Errorf("lyx adopt: the owner write needs origin and it was unreachable, so nothing was resumed: %w; %s once origin answers", err, reentry)
	}
	return fmt.Errorf("lyx adopt: the owner write was refused, so nothing was resumed: %w; %s", err, reentry)
}

// resumeSlug runs steps 4 and 5 for an adopted beam or a moved run, over the owners read back after the owner write.
// An entry that reads owned by another machine is left as it stands and reported on its outcome as owned elsewhere, with its owner.
// Otherwise it lifts the owner pause, and resumes a run that is not done, a beam's pawl run only when this machine also owns the beam's parent.
// It warns when this machine does not own the parent.
func resumeSlug(deps adoptDeps, here string, owners boardengine.Owners, out *slugOutcome) {
	slug := out.Slug

	out.Warning = parentWarning(deps, here, owners, slug)

	owner, err := owners.Effective(slug, deps.ParentOf)
	if err != nil {
		out.Error = fmt.Sprintf("read the owner: %v", err)
		return
	}
	if boardengine.OwnedElsewhere(owner, here) {
		out.Run, out.OwnedBy = ownedElsewhereRun, owner
		return
	}

	if err := liftOwnerPauses(deps, out); err != nil {
		out.Error = err.Error()
		return
	}

	if entry := owners.Entries[slug]; entry.IsProject && entry.Beam {
		parentOwner, _, err := parentOwnerOf(deps, owners, slug)
		if err != nil {
			out.Error = fmt.Sprintf("read the owner of the parent: %v", err)
			return
		}
		if boardengine.OwnedElsewhere(parentOwner, here) {
			out.Run, out.OwnedBy = ownedElsewhereRun, parentOwner
			return
		}
	}

	status, found, err := deps.RunRecord(slug)
	if err != nil {
		out.Error = fmt.Sprintf("read the batten run record: %v", err)
		return
	}
	if !found || status.State == shedengine.StateDone {
		out.Run = "none"
		return
	}
	envelope, err := deps.ResumeRun(slug)
	if err != nil {
		out.Error = fmt.Sprintf("resume the run: %v", err)
		return
	}
	if ok, _ := envelope["ok"].(bool); !ok {
		message, _ := envelope["error"].(string)
		out.Error = "resume the run: " + message
		return
	}
	out.Run = "resumed"
}

// runBeamSteps runs the beam steps of the prime when root was adopted, then of each adopted beam, over the owners read back after the owner write.
// A beam whose pawl run holds the landing hold gets none, and its outcome names the way to follow the landing and, when another machine owns its parent, the adopt that takes the parent over and so resumes the landing here.
// A step that fails is the worktree's warning naming `lyx beam start`, and the other worktrees go on.
func runBeamSteps(deps adoptDeps, here string, owners boardengine.Owners, adopted []string, result *outcome, outcomes map[string]*slugOutcome) {
	if deps.BeamUp == nil {
		return
	}
	if result.PrimeRecorded {
		report, err := deps.BeamUp(deps.PrimeName)
		result.PrimeBeamSteps = beamStepsOf(report)
		if err != nil {
			result.PrimeBeamWarning = fmt.Sprintf("the beam steps of the prime failed (%v); way forward: fix the cause it names, then run `lyx beam start root`", err)
		}
	}
	for _, slug := range adopted {
		out := outcomes[slug]
		if deps.LandingHeld != nil {
			held, err := deps.LandingHeld(slug)
			if err != nil {
				addWarning(out, fmt.Sprintf("whether the landing holds the beam could not be read (%v), so its beam steps did not run; way forward: fix the cause it names, then run `lyx beam start %s`", err, slug))
				continue
			}
			if held {
				out.WayForward = heldBeamWayForward(deps, here, owners, slug)
				continue
			}
		}
		report, err := deps.BeamUp(slug)
		out.BeamSteps = beamStepsOf(report)
		if err != nil {
			addWarning(out, fmt.Sprintf("the beam steps failed (%v); way forward: fix the cause it names, then run `lyx beam start %s`", err, slug))
		}
	}
}

// heldBeamWayForward words the outcome of an adopted beam whose pawl run holds the landing hold: the beam steps are left to the run, which `lyx batten status` follows, and a parent another machine owns is taken over by its adopt.
func heldBeamWayForward(deps adoptDeps, here string, owners boardengine.Owners, slug string) string {
	text := fmt.Sprintf("its landing holds the beam, so its dobby and orch are not started: run `lyx batten status %s` to follow the landing", slug)
	owner, subject, err := foreignParent(deps, here, owners, slug)
	switch {
	case err != nil:
		return text + fmt.Sprintf("; the owner of its parent could not be read (%v)", err)
	case owner != "":
		return text + fmt.Sprintf("; machine %s owns its parent, so the landing halts at Finalize here: `lyx adopt %s` takes the parent over and resumes the landing here", owner, subject)
	}
	return text
}

// addWarning appends text to the warning of out, after any warning already there.
func addWarning(out *slugOutcome, text string) {
	if out.Warning != "" {
		out.Warning += "; "
	}
	out.Warning += text
}

// foreignParent returns the machine that owns slug's parent and what `lyx adopt` takes to move the parent: the nearest beam project above it, `root` for the prime.
// The owner is empty when this machine owns the parent.
func foreignParent(deps adoptDeps, here string, owners boardengine.Owners, slug string) (owner, subject string, err error) {
	owner, parent, err := parentOwnerOf(deps, owners, slug)
	if err != nil {
		return "", "", err
	}
	if !boardengine.OwnedElsewhere(owner, here) {
		return "", "", nil
	}
	subject = boardengine.RootSubject
	if parent != "" {
		if subject, err = owners.AdoptSubject(parent, deps.ParentOf); err != nil {
			return "", "", err
		}
	}
	return owner, subject, nil
}

// parentOwnerOf returns the owner of slug's parent and the parent's slug, "" for the prime, whose owner is the prime owner.
func parentOwnerOf(deps adoptDeps, owners boardengine.Owners, slug string) (owner, parent string, err error) {
	parent, err = deps.ParentOf(slug)
	if err != nil {
		return "", "", err
	}
	if parent == "" {
		return owners.Prime, "", nil
	}
	owner, err = owners.Effective(parent, deps.ParentOf)
	return owner, parent, err
}

// parentWarning returns the warning for an entry whose parent another machine owns, since its run halts at Finalize here; it is empty otherwise.
// The warning names the adopt of the parent's project, `lyx adopt root` when the parent is the prime.
func parentWarning(deps adoptDeps, here string, owners boardengine.Owners, slug string) string {
	owner, subject, err := foreignParent(deps, here, owners, slug)
	if err != nil {
		return fmt.Sprintf("the owner of the parent could not be read: %v", err)
	}
	if owner == "" {
		return ""
	}
	return fmt.Sprintf("this machine does not own the parent, which %s owns, so the run halts at Finalize here: `lyx adopt %s` takes the parent over", owner, subject)
}

// liftOwnerPauses lifts each live owner-labelled pause entry of the slug's own scope, and records a pause that still covers the slug afterwards.
func liftOwnerPauses(deps adoptDeps, out *slugOutcome) error {
	record, err := deps.PauseRecord()
	if err != nil {
		return fmt.Errorf("read the pause record: %w", err)
	}
	for _, entry := range record.Live {
		if entry.Scope != out.Slug || entry.Caller != pauserecord.OwnerCaller {
			continue
		}
		if err := deps.Lift(tenderengine.EntryRef{Scope: entry.Scope, Caller: entry.Caller, PausedAt: entry.PausedAt}); err != nil {
			return fmt.Errorf("lift the owner pause: %w", err)
		}
		out.PauseLifted = true
	}

	record, err = deps.PauseRecord()
	if err != nil {
		return fmt.Errorf("read the pause record: %w", err)
	}
	if entry, covered := record.Covering(out.Slug); covered {
		if entry.Scope == pauserecord.ScopeHub {
			out.PauseStanding = "a hub pause covers it: `lyx tender resume` lifts it"
		} else {
			out.PauseStanding = fmt.Sprintf("a pause of its own scope by %s covers it: `lyx tender resume %s` lifts it", entry.Caller, out.Slug)
		}
	}
	return nil
}

// resumeHaltedChildren resumes each child halted at Finalize's owner halt whose run this machine now owns and whose own slug, or an ancestor beam, or root when rootAdopted, was adopted.
// A child halted for any other reason is left alone.
func resumeHaltedChildren(deps adoptDeps, here string, owners boardengine.Owners, adopted []string, rootAdopted bool, outcomes map[string]*slugOutcome) {
	slugs := make([]string, 0, len(owners.Entries))
	for slug, entry := range owners.Entries {
		if entry.IsTask && !entry.Done {
			slugs = append(slugs, slug)
		}
	}
	slices.Sort(slugs)

	for _, slug := range slugs {
		status, found, err := deps.ChildStatus(slug)
		if err != nil || !found || status.State != shedengine.StateBlocked || !strings.HasPrefix(status.Error, landingshed.OwnerHaltPrefix) {
			continue
		}
		owner, err := owners.Effective(slug, deps.ParentOf)
		if err != nil || boardengine.OwnedElsewhere(owner, here) {
			continue
		}
		under, err := adoptedUnder(deps, slug, adopted, rootAdopted)
		if err != nil || !under {
			continue
		}
		out := outcomes[slug]
		if out == nil {
			out = &slugOutcome{Slug: slug}
			outcomes[slug] = out
		}
		if err := deps.ResumeChild(slug); err != nil {
			out.Error = fmt.Sprintf("resume the child halted at Finalize's owner check: %v", err)
			continue
		}
		out.ChildResumed = true
	}
}

// adoptedUnder reports whether slug or one of its ancestor beams was adopted, or its chain ends at the prime and root was.
func adoptedUnder(deps adoptDeps, slug string, adopted []string, rootAdopted bool) (bool, error) {
	seen := map[string]bool{}
	for current := slug; !seen[current]; {
		if slices.Contains(adopted, current) {
			return true, nil
		}
		seen[current] = true
		parent, err := deps.ParentOf(current)
		if err != nil {
			return false, err
		}
		if parent == "" {
			return rootAdopted, nil
		}
		current = parent
	}
	return false, nil
}

// collect returns the outcomes of the named slugs in their order, followed by any other entry a step touched, sorted by slug.
func collect(named []string, outcomes map[string]*slugOutcome) []slugOutcome {
	var all []slugOutcome
	for _, slug := range named {
		all = append(all, *outcomes[slug])
	}
	var extra []string
	for slug := range outcomes {
		if !slices.Contains(named, slug) {
			extra = append(extra, slug)
		}
	}
	slices.Sort(extra)
	for _, slug := range extra {
		all = append(all, *outcomes[slug])
	}
	return all
}
