// adopt.go holds the types and helpers the steps of `lym adopt` share.
// It names the request, the outcome and the seams the steps share.
// It also holds the entry resolution and takeover they use.
// The ordered steps themselves are in steps.go, and the worker that runs them in worker.go.
// Each subject's adopt check, started last, only reports; the pause adopt sets lasts until an operator lifts it.

package adoptcli

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// reentry is what a refusal that adopt's own steps leave undone is followed by.
const reentry = "then re-run lym adopt with the same arguments"

// The repository sides a branch stop names;
// every pair shares them.
const (
	sideBolt    = "bolt"
	sideRecords = "records"
)

// The run outcomes of a moved run that adopt does not leave to its owner.
// A run is owned elsewhere after the owner write, or paused by adopt.
const (
	ownedElsewhereRun = "owned elsewhere"
	pausedRun         = "paused by adopt"
)

// request is what the verb asks for.
// It names the beam project slugs, root and every adoptable project.
// It also says how local commits origin lacks are answered, and whether the beam steps leave the orch down.
// The word `root` among the slugs sets Root and is dropped from them.
type request struct {
	Slugs     []string
	Root      bool
	All       bool
	KeepLocal bool
	DropLocal bool
	NoOrch    bool
}

// branchStop names one local branch of an entry and the state the reconcile judged it in.
type branchStop struct {
	Branch string `json:"branch"`
	State  string `json:"state"`
}

// slugOutcome is what adopt did to one entry:
// an adopted beam project, a run moved with a project, or a released task paused with it.
// WayForward is the stop's way forward.
// An adopted beam whose pawl run holds the landing hold gets the way to follow the landing and to resume it.
type slugOutcome struct {
	Slug          string            `json:"slug"`
	Stopped       bool              `json:"stopped,omitempty"`
	Branches      []branchStop      `json:"branches,omitempty"`
	Live          bool              `json:"live,omitempty"`
	WayForward    string            `json:"way_forward,omitempty"`
	Dropped       []string          `json:"dropped_tags,omitempty"`
	PairAdded     bool              `json:"pair_added,omitempty"`
	OwnerRecorded bool              `json:"owner_recorded,omitempty"`
	PauseStanding string            `json:"pause_standing,omitempty"`
	Run           string            `json:"run,omitempty"`
	OwnedBy       string            `json:"owned_by,omitempty"`
	AdoptPaused   bool              `json:"adopt_paused,omitempty"`
	BeamSteps     *beamStepsOutcome `json:"beam_steps,omitempty"`
	Warning       string            `json:"warning,omitempty"`
	Error         string            `json:"error,omitempty"`
}

// beamStepsOutcome is what the beam steps did for one adopted worktree, including the warnings that did not stop them.
type beamStepsOutcome struct {
	Dobby            string   `json:"dobby,omitempty"`
	DobbyDetail      string   `json:"dobby_detail,omitempty"`
	Action           string   `json:"action,omitempty"`
	PromptSource     string   `json:"prompt_source,omitempty"`
	AbandonedSession string   `json:"abandoned_session,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
}

// beamStepsOf returns the beam steps outcome of report, copying the fields adopt reports.
func beamStepsOf(report orchcli.BeamUpReport) *beamStepsOutcome {
	return &beamStepsOutcome{
		Dobby: report.Dobby, DobbyDetail: report.DobbyDetail, Action: report.Action,
		PromptSource: report.PromptSource, AbandonedSession: report.AbandonedSession, Warnings: report.Warnings,
	}
}

// suiteStart records the detached adopt check that adopt started, or found running, for one subject.
type suiteStart struct {
	// Subject is the beam slug or `root` the check runs for.
	Subject string `json:"subject"`
	PID     int    `json:"pid"`
	Log     string `json:"log"`
	// Record is the path of the check's progress record.
	Record string `json:"record"`
	// Tiers are the hub's adopt gate key tiers as adopt read them; empty when the config did not read or has no adopt key.
	Tiers []int `json:"tiers,omitempty"`
	// AlreadyRunning is set when the adopt found a check running and started none.
	AlreadyRunning bool `json:"already_running,omitempty"`
}

// outcome is the verb's envelope: the record of the whole takeover.
// It covers the reconcile, the deploy, the prime, each entry, and the adopt checks, root first.
// Its runs are the ones under the named subjects.
// Its warnings name the branches the reconcile left unmoved, a deploy state that did not read, and each failed start.
type outcome struct {
	Reconcile        fabricengine.OriginReport
	Mutations        []fabricengine.Mutation
	Partial          bool
	Deploy           *deployOutcome
	PrimeRecorded    bool
	PrimeLost        string
	Slugs            []slugOutcome
	Warnings         []string
	PrimeBeamSteps   *beamStepsOutcome
	PrimeBeamWarning string
	Checks           []suiteStart
	SuiteMessage     string
	CheckWarnings    []string
	Runs             []runReport
}

// adoptDeps holds the seam of every step, which the hub fills and the tests fake.
// LocalPair reports whether this machine holds the slug's pair worktree.
// ParentOf names the slug of the pair a slug was forked from, or "" for the prime.
// RunRecord reads the batten run record of a slug, reporting false when none exists.
// Tasks reads the board's entries.
// UpdatePause applies mutate to the pause record in one locked update, handing it the update's time.
// ResetTries writes the subject's fix-try count to 0.
// MoveInProgress reports whether an anchor change stands in the given worktree, "" naming the prime.
// It returns the way forward the change names; nil reads as no record standing.
// BeamUp runs the beam steps of the named worktree, its dobby and its orch, with no hub preparation.
// The adopt's first step already reconciled origin; nil runs none.
// With noOrch set it starts the dobby and leaves the orch down.
// It runs after the owner write for each adopted beam whose owner write stood and, when root was adopted, for the prime.
// LandingHeld reports whether the pawl run of the beam slug holds the beam's landing hold.
// The hold keeps the beam's steps from running; nil reads as no hold.
// PrimeName is the prime worktree's directory name.
// StartSuite starts the adopt check of a subject, a beam slug or `root`, as a detached process in its worktree.
// When a check already runs there, it finds that one instead.
// It returns at once, so a break this machine brings to the landed code shows at adopt time.
// It runs last.
// It runs once for root when the prime's owner was recorded.
// It runs once for each adopted beam whose owner write stood.
// EscalationEligible reports whether a slug's awaiting run stands at a producer whose escalation the arbiter admits.
// DeployState reads the landing deploy command for the prime's branch.
// It also reads the running lym's build and whether the prime's head is ahead of it;
// nil reads as nothing to deploy.
// RunDeploy runs the deploy command in the prime and returns its log's path.
// It returns an error for a deploy that did not end with exit code 0.
// HubPath roots the mutation record.
type adoptDeps struct {
	Machine            func() (string, error)
	ReadOwners         func() (boardengine.Owners, error)
	ParentOf           func(slug string) (string, error)
	Reconcile          func() (fabricengine.OriginReport, error)
	LocalPair          func(slug string) bool
	LivePair           func(slug string) (bool, error)
	AddPair            func(slug string) (fabricengine.AddResult, error)
	DropLocal          func(rec *fabricengine.Mutations, slug string) (fabricengine.DropLocalResult, error)
	SetOwners          func(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error)
	PauseRecord        func() (pauserecord.Record, error)
	RunRecord          func(slug string) (shedengine.Status, bool, error)
	Tasks              func() ([]boardengine.Task, error)
	UpdatePause        func(mutate func(record *pauserecord.Record, now time.Time) error) error
	ResetTries         func(subject string) error
	MoveInProgress     func(worktree string) (wayForward string, standing bool, err error)
	BeamUp             func(worktree string, noOrch bool) (orchcli.BeamUpReport, error)
	LandingHeld        func(slug string) (bool, error)
	StartSuite         func(subject string) (suiteStart, error)
	PrimeName          string
	EscalationEligible func(slug string) bool
	DeployState        func() (deployState, error)
	RunDeploy          func(command string) (logPath string, err error)
	HubPath            string
}

// checkSubjects lists the subjects whose adopt check a call starts:
// root first when the prime's owner was recorded, then the adopted beams in write order.
func checkSubjects(primeRecorded bool, adopted []string) []string {
	var subjects []string
	if primeRecorded {
		subjects = append(subjects, boardengine.RootSubject)
	}
	return append(subjects, adopted...)
}

// reconcileWarnings returns, for each branch of the prime or an adopted beam, a warning naming `lym fabric pull`.
// A branch qualifies when the reconcile left it unmoved: rewritten, diverged, or ahead with Moved unset.
// A branch of a slug whose own stop is on its outcome, and a branch the shared-stop refusal already judged, get none.
// A records branch names its code worktree, where its fabric verbs run, and a code branch its own worktree;
// the reconcile's way forward, when it set one, follows in parentheses.
func reconcileWarnings(report fabricengine.OriginReport, adopted []string, outcomes map[string]*slugOutcome, _ request) []string {
	var warnings []string
	for _, branch := range report.Branches {
		if !branch.Prime && !slices.Contains(adopted, branch.Slug) {
			continue
		}
		switch branch.State {
		case fabricengine.BranchRewritten, fabricengine.BranchDiverged:
		case fabricengine.BranchAhead:
			if branch.Moved {
				continue
			}
		default:
			continue
		}
		if out := outcomes[branch.Slug]; branch.Slug != "" && out != nil && out.Stopped {
			continue
		}
		if sharedStopText(fabricengine.OriginReport{Branches: []fabricengine.BranchOutcome{branch}}) != "" {
			continue
		}
		worktree := branch.Worktree
		if branch.Repo == sideRecords {
			worktree = branch.CodeWorktree
		}
		line := fmt.Sprintf("%s is %s against origin and was left unmoved: run lym fabric pull in %s", branch.Branch, branch.State, worktree)
		if branch.WayForward != "" {
			line += fmt.Sprintf(" (%s)", branch.WayForward)
		}
		warnings = append(warnings, line+", "+reentry)
	}
	return warnings
}

// suiteMessage is the plain text that names the adopt checks, one line per check.
// Each line gives the check's subject, whether it started or was already running, and the tiers it runs when known.
// The prime's reed status bar shows each check's progress and result.
func suiteMessage(checks []suiteStart) string {
	lines := make([]string, len(checks))
	for i, check := range checks {
		verb := "started"
		if check.AlreadyRunning {
			verb = "already running"
		}
		tiers := ""
		if len(check.Tiers) > 0 {
			numbers := make([]string, len(check.Tiers))
			for j, tier := range check.Tiers {
				numbers[j] = strconv.Itoa(tier)
			}
			tiers = ", tiers " + strings.Join(numbers, ", ")
		}
		lines[i] = fmt.Sprintf("adopt check of %s %s in its worktree%s; the prime's reed status bar shows its progress and result", check.Subject, verb, tiers)
	}
	return strings.Join(lines, "\n")
}

// namedEntries resolves the request to the beam project slugs it adopts and whether it adopts root.
// validateRequest and the worker call it over the owners they read.
// --all names root and every beam project that is not done.
// It refuses a slug the board does not hold, a note, or a done entry.
// It refuses a task and names the beam or `root` that moves its run.
// It refuses a grouping project and names the adopt that moves its tasks, and `lym beam create`, which makes it a beam.
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
			return nil, false, fmt.Errorf("lym adopt: %q is not on the board: `lym board list` shows the entries", slug)
		case !entry.IsTask && !entry.IsProject:
			return nil, false, fmt.Errorf("lym adopt: %q is a note, not a task: `lym board promote %s` makes it a task first", slug, slug)
		case entry.Done:
			return nil, false, fmt.Errorf("lym adopt: %q is done and already landed, so there is nothing to adopt: `lym tender resume %s` lets a run that landed there go on to Land under its owner pause", slug, slug)
		case entry.IsTask || !entry.Beam:
			subject, err := owners.AdoptSubject(slug, deps.ParentOf)
			if err != nil {
				return nil, false, fmt.Errorf("lym adopt: read the parent of %s: %w", slug, err)
			}
			if entry.IsTask {
				return nil, false, fmt.Errorf("lym adopt: %q is a task, and a task is moved with its project: `lym adopt %s` moves it", slug, subject)
			}
			return nil, false, fmt.Errorf("lym adopt: %q is a grouping project, not a beam, so there is no pair to take over: `lym adopt %s` moves its tasks, and `lym beam create %s` makes it a beam", slug, subject, slug)
		}
		if !slices.Contains(slugs, slug) {
			slugs = append(slugs, slug)
		}
	}
	return slugs, req.Root, nil
}

// movedRuns lists, sorted, the runs an adopt of the beams adopted, and of root when rootAdopted, moves with them.
// A run moves when its entry is an open task or beam project and holds a batten run record that is not done:
// its parent is an adopted beam, or root when root is adopted;
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

// projectOf returns the adopted project that moves slug:
// slug itself when it is an adopted beam, else its parent beam, else `root` for a child of the prime.
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

// sharedStopText renders each stop on Bolt or the prime's records branch as its way forward.
// A stop that a retry re-enters also carries the re-entry text.
// It is empty when no such branch stopped.
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

// takeOverSlug takes over one entry an adopt moves, an adopted beam or a run moved with project.
// It stops the entry on local commits or a live pair, drops the local commits under --drop-local, and adds the missing pair.
// project is the adopted beam or `root` whose adopt moves the entry,
// and the stop's way forward names it, since a task is never an adopt's argument.
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
			keep, drop := adoptrecord.Request(req), adoptrecord.Request(req)
			keep.KeepLocal, keep.DropLocal = true, false
			drop.KeepLocal, drop.DropLocal = false, true
			out.WayForward = fmt.Sprintf("the pair holds local state origin lacks or a live driver: `%s` records the owner over it, and `%s` keeps each local tip under an archive tag and moves the branches to origin's", keep.Command(), drop.Command())
			return false
		}
	}

	entry := owners.Entries[slug]
	// A beam's pair is added through `AddPair`, which adopts the live branches and their origin record, the beam mark included.
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

// addError words a refused pair add of slug, which the adopt of project moves.
// A refusal for a leftover local branch is reworded.
// Its own way forward would drop this machine's unpushed commits on that branch with no archive.
// The reworded text says to keep the tip under an archive tag first,
// and it ends with the re-run of the adopt of project, the beam or `root` that moves slug.
func addError(slug, project string, err error) string {
	var exists *fabricengine.ErrBranchExists
	if errors.As(err, &exists) {
		return fmt.Sprintf("the leftover local branch %s may hold commits origin lacks: keep its tip first with `git tag archive/%s/<tip> %s`, then `git branch -D %s`, then re-run `lym adopt %s`", exists.Branch, slug, exists.Branch, exists.Branch, project)
	}
	return err.Error()
}

// ownerWriteError words a refused owner write with its way forward and the re-run.
func ownerWriteError(err error) error {
	if errors.Is(err, boardengine.ErrOwnerOriginUnreachable) {
		return stopWith(reentry+" once origin answers", "lym adopt: the owner write needs origin and it was unreachable, so no deploy, beam step or check ran: %w", err)
	}
	return stopWith(reentry, "lym adopt: the owner write was refused, so the adopt check of the subject starts nothing until its owner is recorded: %w", err)
}

// resumeSlug settles an adopted beam or a moved run after the owner write, over the owners read back.
// It starts and lifts nothing: an adopt pause stands until an operator lifts it.
// An entry that reads owned by another machine is left as it stands,
// and its outcome reports it as owned elsewhere, with its owner.
// paused reports whether adopt paused the slug; otherwise a pause already standing over it is noted.
// An entry with no run record, or whose run is done, reads none.
// It warns when this machine does not own the parent.
func resumeSlug(deps adoptDeps, here string, owners boardengine.Owners, out *slugOutcome, paused bool) {
	slug := out.Slug

	out.Warning = parentWarning(deps, here, owners, slug)

	owner, err := owners.Effective(slug, deps.ParentOf)
	if err != nil {
		out.Error = fmt.Sprintf("read the owner: %v", err)
		return
	}
	if boardengine.OwnedElsewhere(owner, here) {
		if !paused {
			out.Run, out.OwnedBy = ownedElsewhereRun, owner
		}
		return
	}

	if !paused {
		if err := noteStanding(deps, out); err != nil {
			out.Error = err.Error()
			return
		}
	}

	if entry := owners.Entries[slug]; entry.IsProject && entry.Beam {
		parentOwner, _, err := parentOwnerOf(deps, owners, slug)
		if err != nil {
			out.Error = fmt.Sprintf("read the owner of the parent: %v", err)
			return
		}
		if boardengine.OwnedElsewhere(parentOwner, here) {
			if !paused {
				out.Run, out.OwnedBy = ownedElsewhereRun, parentOwner
			}
			return
		}
	}

	if paused {
		return
	}
	status, found, err := deps.RunRecord(slug)
	if err != nil {
		out.Error = fmt.Sprintf("read the batten run record: %v", err)
		return
	}
	if !found || status.State == shedengine.StateDone {
		out.Run = "none"
	}
}

// noteStanding records on out the pause that covers its slug, when one does.
func noteStanding(deps adoptDeps, out *slugOutcome) error {
	record, err := deps.PauseRecord()
	if err != nil {
		return fmt.Errorf("read the pause record: %w", err)
	}
	if entry, covered := record.Covering(out.Slug); covered {
		if entry.Scope == pauserecord.ScopeHub {
			out.PauseStanding = "a hub pause covers it: `lym tender resume` lifts it"
		} else {
			out.PauseStanding = fmt.Sprintf("a pause of its own scope by %s covers it: `lym tender resume %s` lifts it", entry.Caller, out.Slug)
		}
	}
	return nil
}

// runBeamSteps runs the beam steps of the prime when root was adopted, then of each adopted beam.
// It works over the owners read back after the owner write.
// noOrch leaves the orch of each worktree down.
// A beam whose pawl run holds the landing hold gets none.
// Its outcome names the way to follow the landing and to resume it by hand.
// When another machine owns its parent, the outcome also names the adopt of the parent.
// A failed step is reported as its worktree's warning, naming `lym beam start`,
// and the other worktrees go on.
func runBeamSteps(deps adoptDeps, here string, owners boardengine.Owners, adopted []string, noOrch bool, result *outcome, outcomes map[string]*slugOutcome) {
	if deps.BeamUp == nil {
		return
	}
	if result.PrimeRecorded {
		report, err := deps.BeamUp(deps.PrimeName, noOrch)
		result.PrimeBeamSteps = beamStepsOf(report)
		if err != nil {
			result.PrimeBeamWarning = fmt.Sprintf("the beam steps of the prime failed (%v); way forward: fix the cause it names, then run `lym beam start root`", err)
		}
	}
	for _, slug := range adopted {
		out := outcomes[slug]
		if deps.LandingHeld != nil {
			held, err := deps.LandingHeld(slug)
			if err != nil {
				addWarning(out, fmt.Sprintf("whether the landing holds the beam could not be read (%v), so its beam steps did not run; way forward: fix the cause it names, then run `lym beam start %s`", err, slug))
				continue
			}
			if held {
				out.WayForward = heldBeamWayForward(deps, here, owners, slug, out.AdoptPaused)
				continue
			}
		}
		report, err := deps.BeamUp(slug, noOrch)
		out.BeamSteps = beamStepsOf(report)
		if err != nil {
			addWarning(out, fmt.Sprintf("the beam steps failed (%v); way forward: fix the cause it names, then run `lym beam start %s`", err, slug))
		}
	}
}

// heldBeamWayForward words the outcome of an adopted beam whose pawl run holds the landing hold.
// The beam steps are left to that run, which `lym batten status` follows.
// The landing resumes by hand with `lym loom resume` in the beam's pair, after `lym tender resume` when paused is set.
// A parent another machine owns is taken over by its adopt.
func heldBeamWayForward(deps adoptDeps, here string, owners boardengine.Owners, slug string, paused bool) string {
	text := fmt.Sprintf("its landing holds the beam, so its dobby and orch are not started: run `lym batten status %s` to follow the landing", slug)
	if paused {
		text += fmt.Sprintf("; resume it with `lym tender resume %s` and then `lym loom resume` in its pair", slug)
	} else {
		text += "; resume it with `lym loom resume` in its pair"
	}
	owner, subject, err := foreignParent(deps, here, owners, slug)
	switch {
	case err != nil:
		return text + fmt.Sprintf("; the owner of its parent could not be read (%v)", err)
	case owner != "":
		return text + fmt.Sprintf("; machine %s owns its parent, so the landing halts at Finalize here: `lym adopt %s` takes the parent over", owner, subject)
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

// foreignParent returns the machine that owns slug's parent and what `lym adopt` takes to move the parent:
// the nearest beam project above it, `root` for the prime.
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

// parentWarning returns the warning for an entry whose parent another machine owns, since its run halts at Finalize here;
// it is empty otherwise.
// The warning names the adopt of the parent's project, `lym adopt root` when the parent is the prime.
func parentWarning(deps adoptDeps, here string, owners boardengine.Owners, slug string) string {
	owner, subject, err := foreignParent(deps, here, owners, slug)
	if err != nil {
		return fmt.Sprintf("the owner of the parent could not be read: %v", err)
	}
	if owner == "" {
		return ""
	}
	return fmt.Sprintf("this machine does not own the parent, which %s owns, so the run halts at Finalize here: `lym adopt %s` takes the parent over", owner, subject)
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
