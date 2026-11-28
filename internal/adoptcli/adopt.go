// adopt.go holds the types and helpers the foreground adopt shares.
// It names the request, the outcome and the seams of every step, and resolves the request over the board's entries.
// The ordered steps are in foreground.go.

package adoptcli

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// reentry is what a refusal before the adopt's first write is followed by.
const reentry = "then re-run lym adopt with the same arguments"

// The run outcomes of a moved run that adopt does not leave to its owner.
const (
	ownedElsewhereRun = "owned elsewhere"
	takenOverRun      = "taken over"
)

// request is what the verb asks for.
// It names the beam project slugs, root and every adoptable project.
// It says whether another machine's hold over the same subjects moves here.
// It also says whether the beam steps leave the orch down.
// The word `root` among the slugs sets Root and is dropped from them.
type request struct {
	Slugs    []string
	Root     bool
	All      bool
	TakeHold bool
	NoOrch   bool
}

// stopError is the error that ends the adopt: what went wrong, and the clause that names what to do next.
type stopError struct {
	err        error
	wayForward string
}

func (e *stopError) Error() string { return e.err.Error() + "; " + e.wayForward }

func (e *stopError) Unwrap() error { return e.err }

// stopWith returns the error that stops the adopt with the message format and args describe, followed by wayForward.
func stopWith(wayForward, format string, args ...any) error {
	return &stopError{err: fmt.Errorf(format, args...), wayForward: wayForward}
}

// slugOutcome is what adopt did to one entry:
// an adopted beam project, or a run moved with a project.
// WayForward is the way forward a held beam names.
// Check is the state of the subject's adopt check.
// An adopted beam whose pawl run holds the landing hold gets the way to follow the landing and to resume it.
type slugOutcome struct {
	Slug          string            `json:"slug"`
	WayForward    string            `json:"way_forward,omitempty"`
	PairAdded     bool              `json:"pair_added,omitempty"`
	OwnerRecorded bool              `json:"owner_recorded,omitempty"`
	Run           string            `json:"run,omitempty"`
	OwnedBy       string            `json:"owned_by,omitempty"`
	Check         string            `json:"check,omitempty"`
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

// outcome is the verb's envelope: the record of the whole takeover.
// Hold is the hold the adopt wrote, nil once a green check cleared it.
// FixTask is the fix task the check filed or found standing.
// PrimeCheck and each slug's Check hold the subject's check state, and CheckLines the lines the check posted.
// Its warnings name a deploy state that did not read and a workspace file that was not regenerated.
type outcome struct {
	Hold             *boardengine.Hold
	Deploy           *deployOutcome
	PrimeRecorded    bool
	Slugs            []slugOutcome
	Warnings         []string
	PrimeBeamSteps   *beamStepsOutcome
	PrimeBeamWarning string
	FixTask          string
	PrimeCheck       string
	CheckLines       []string
}

// adoptDeps holds the seam of every step, which the hub fills and the tests fake.
// Identity reads this machine's identity, which every comparison of a recorded owner with this machine reads;
// a nil Identity reads the name Machine returns alone.
// PullBolt brings the local Bolt level with origin.
// ParentOf names the slug of the pair a slug was forked from, or "" for the prime.
// RunRecord reads the local batten run record of a slug, reporting false when none exists.
// OriginRun reads the run record of slug at the tip of its parent's records branch on origin, parent "" naming the prime.
// LocalPair reports whether this machine holds the slug's pair worktree.
// WriteAdopt writes the hold, the owners and this machine's registry entry in one pushed Bolt commit.
// TakeOver moves one run to this machine and pushes its takeover entry.
// CatchUp runs the hub-wide catch-up from origin.
// MoveInProgress reports whether an anchor change stands in the given worktree, "" naming the prime.
// It returns the way forward the change names; nil reads as no record standing.
// DeployState reads the `landing.deploy` command for the prime's branch.
// It also reads the running lym's build and whether the prime's head is ahead of it;
// nil reads as nothing to deploy.
// RunDeploy runs the deploy command in the prime and returns its log's path.
// It returns an error for a deploy that did not end with exit code 0.
// Workspace regenerates the hub's workspace files.
// HoldCheck runs the adopt check over the subjects of this machine's hold, first first, and settles its end.
// With noOrch set the check brings no orch up.
// BeamUp runs the beam steps of the named worktree, its dobby and its orch, with no hub preparation;
// nil runs none.
// With noOrch set it starts the dobby and leaves the orch down.
// LandingHeld reports whether the pawl run of the beam slug holds the beam's landing hold.
// The hold keeps the beam's steps from running; nil reads as no hold.
// PickAccount picks the Claude account of a worktree the adopt added a pair for;
// a failed pick is logged by the seam and never fails the adopt.
// PrimeName is the prime worktree's directory name.
// Now reads the clock the hold is stamped with.
type adoptDeps struct {
	Machine        func() (string, error)
	Identity       func() (machinename.Identity, error)
	ReadOwners     func() (boardengine.Owners, error)
	PullBolt       func() error
	ParentOf       func(slug string) (string, error)
	RunRecord      func(slug string) (shedengine.Status, bool, error)
	OriginRun      func(slug, parent string) (shedengine.Status, bool, error)
	LocalPair      func(slug string) bool
	AddPair        func(slug string) (fabriclayout.AddResult, error)
	PickAccount    func(slug string)
	WriteAdopt     func(req boardengine.AdoptRequest) (boardengine.AdoptWrite, error)
	TakeOver       func(slug, subject string) (fabriclayout.TakeOverResult, error)
	CatchUp        func() error
	MoveInProgress func(worktree string) (wayForward string, standing bool, err error)
	DeployState    func() (deployState, error)
	RunDeploy      func(command string) (logPath string, err error)
	Workspace      func() error
	HoldCheck      func(first string, noOrch bool) (holdCheckResult, error)
	BeamUp         func(worktree string, noOrch bool) (orchcli.BeamUpReport, error)
	LandingHeld    func(slug string) (bool, error)
	PrimeName      string
	HubPath        string
	Now            func() time.Time
}

// hereIdentity returns this machine's identity from the Identity seam, or here alone as a name when the seam is nil.
func (deps adoptDeps) hereIdentity(here string) (machinename.Identity, error) {
	if deps.Identity == nil {
		return machinename.Identity{Name: here}, nil
	}
	return deps.Identity()
}

// rerunOf returns the command that adopts subjects again, the way forward every stop of an adopt names.
func rerunOf(subjects []string) string {
	return "lym adopt " + strings.Join(subjects, " ")
}

// checkSubjects lists the subjects whose adopt check a call runs:
// root first when the prime is a subject, then the adopted beams in write order.
func checkSubjects(primeRecorded bool, adopted []string) []string {
	var subjects []string
	if primeRecorded {
		subjects = append(subjects, boardengine.RootSubject)
	}
	return append(subjects, adopted...)
}

// normalizedRequest returns req with the word `root` among its slugs turned into Root.
func normalizedRequest(req request) request {
	if !slices.Contains(req.Slugs, boardengine.RootSubject) {
		return req
	}
	req.Root = true
	req.Slugs = slices.DeleteFunc(slices.Clone(req.Slugs), func(slug string) bool { return slug == boardengine.RootSubject })
	return req
}

// validateRequest holds the checks an adopt makes before any step.
// It refuses a request that names no subject, one that pairs --all with slugs, and the take-hold flag with --all.
// It then resolves the request over the board's owners and returns the beam slugs it adopts and whether it adopts root.
func validateRequest(deps adoptDeps, req request) (slugs []string, root bool, err error) {
	req = normalizedRequest(req)
	if req.All && len(req.Slugs) > 0 {
		return nil, false, errors.New("lym adopt: --all takes no slug: drop the slugs, or drop --all")
	}
	if req.All && req.TakeHold {
		return nil, false, errors.New("lym adopt: the take-hold flag takes no --all, since a hold names its subjects: read them in `lym beam status` and name them")
	}
	if len(req.Slugs) == 0 && !req.Root && !req.All {
		return nil, false, errors.New("lym adopt: name a beam project or `root`, or pass --all: `lym board list` shows the entries")
	}
	owners, err := deps.ReadOwners()
	if err != nil {
		return nil, false, fmt.Errorf("lym adopt: read the board's owners: %w", err)
	}
	return namedEntries(deps, owners, req)
}

// namedEntries resolves the request to the beam project slugs it adopts and whether it adopts root.
// validateRequest calls it over the owners it reads.
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

// openRun is a run under the adopted subjects whose record is not done, with the takeover entry that record holds.
type openRun struct {
	Slug     string
	Takeover *shedengine.Takeover
}

// movedRuns lists, sorted by slug, the open runs under the beams adopted, and under root when rootAdopted.
// A run is open when its entry is an open task or beam project and the record read returns for it is not done:
// its parent is an adopted beam, or root when root is adopted;
// the pawl run of each adopted beam is listed with it.
// A beam that is a member of an adopted beam, or of root, is listed too, though it keeps its own owner.
// read returns a slug's run record given its parent, reporting false when none exists.
func movedRuns(deps adoptDeps, owners boardengine.Owners, adopted []string, rootAdopted bool, read func(slug, parent string) (shedengine.Status, bool, error)) ([]openRun, error) {
	var runs []openRun
	for slug, entry := range owners.Entries {
		if entry.Done || !(entry.IsTask || (entry.IsProject && entry.Beam)) {
			continue
		}
		parent, err := deps.ParentOf(slug)
		if err != nil {
			return nil, fmt.Errorf("read the parent of %s: %w", slug, err)
		}
		if !slices.Contains(adopted, slug) && !slices.Contains(adopted, parent) && !(parent == "" && rootAdopted) {
			continue
		}
		status, found, err := read(slug, parent)
		if err != nil {
			return nil, fmt.Errorf("read the run record of %s: %w", slug, err)
		}
		if !found || status.State == shedengine.StateDone {
			continue
		}
		runs = append(runs, openRun{Slug: slug, Takeover: status.Takeover})
	}
	slices.SortFunc(runs, func(a, b openRun) int { return strings.Compare(a.Slug, b.Slug) })
	return runs, nil
}

// openRunSlugs returns the slugs of runs in their order.
func openRunSlugs(runs []openRun) []string {
	slugs := make([]string, len(runs))
	for i, run := range runs {
		slugs[i] = run.Slug
	}
	return slugs
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

// addError words a refused pair add of slug, whose hold the command rerun adopts again.
// A refusal for a leftover local branch is reworded.
// Its own way forward would drop this machine's unpushed commits on that branch with no archive.
// The reworded text says to keep the tip under an archive tag first, and it ends with rerun.
func addError(slug, rerun string, err error) string {
	var exists *fabricengine.ErrBranchExists
	if errors.As(err, &exists) {
		return fmt.Sprintf("the leftover local branch %s may hold commits origin lacks: keep its tip first with `git tag archive/%s/<tip> %s`, then `git branch -D %s`, then re-run `%s`", exists.Branch, slug, exists.Branch, exists.Branch, rerun)
	}
	return err.Error()
}

// ownerWriteError words a refused adopt write with its way forward.
// A hold or registration refusal carries its own way forward; any other refusal names rerun.
func ownerWriteError(err error, rerun string) error {
	var holdRefusal *boardengine.HoldRefusal
	var registration *boardengine.RegistrationError
	switch {
	case errors.Is(err, boardengine.ErrOwnerOriginUnreachable):
		return stopWith("re-run `"+rerun+"` once origin answers", "lym adopt: the adopt write needs origin and it was unreachable, so no run moved and no check ran: %w", err)
	case errors.As(err, &holdRefusal), errors.As(err, &registration):
		return fmt.Errorf("lym adopt: %w", err)
	}
	return stopWith("re-run `"+rerun+"`", "lym adopt: the adopt write was refused, so nothing more ran: %w", err)
}

// runBeamSteps runs the beam steps of the prime when root was adopted, then of each adopted beam.
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
				out.WayForward = heldBeamWayForward(deps, here, owners, slug)
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
// The landing resumes by hand with `lym loom resume` in the beam's pair.
// A parent another machine owns is taken over by its adopt.
func heldBeamWayForward(deps adoptDeps, here string, owners boardengine.Owners, slug string) string {
	text := fmt.Sprintf("its landing holds the beam, so its dobby and orch are not started: run `lym batten status %s` to follow the landing; resume it with `lym loom resume` in its pair", slug)
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
	identity, err := deps.hereIdentity(here)
	if err != nil {
		return "", "", err
	}
	if !boardengine.OwnedElsewhere(owner, owners.Registry, identity) {
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
