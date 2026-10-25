// adopt.go is the takeover sequence of "lyx adopt": it validates the request, reconciles the hub against origin, refuses an adopt that would leave no dobby here to drive what it takes over, then per named entry drops or keeps local commits, adds the missing pair, records this machine as owner, lifts the owner pause, resumes the run, starts the dobbies and starts the test suite.

package adoptcli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
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

// request is what the verb asks for: the named slugs, the prime, every adoptable entry, and how local commits origin lacks are answered.
type request struct {
	Slugs     []string
	Prime     bool
	All       bool
	KeepLocal bool
	DropLocal bool
}

// branchStop names one local branch of an entry and the state the reconcile judged it in.
type branchStop struct {
	Branch string `json:"branch"`
	State  string `json:"state"`
}

// slugOutcome is what happened to one entry: a stop with its branches, the pair added, the owner recorded, the pause lifted or standing, the run resumed or none, the error and a warning.
type slugOutcome struct {
	Slug          string       `json:"slug"`
	Stopped       bool         `json:"stopped,omitempty"`
	Branches      []branchStop `json:"branches,omitempty"`
	Live          bool         `json:"live,omitempty"`
	WayForward    string       `json:"way_forward,omitempty"`
	Dropped       []string     `json:"dropped_tags,omitempty"`
	PairAdded     bool         `json:"pair_added,omitempty"`
	OwnerRecorded bool         `json:"owner_recorded,omitempty"`
	PauseLifted   bool         `json:"pause_lifted,omitempty"`
	PauseStanding string       `json:"pause_standing,omitempty"`
	Run           string       `json:"run,omitempty"`
	ChildResumed  bool         `json:"child_resumed,omitempty"`
	Warning       string       `json:"warning,omitempty"`
	Error         string       `json:"error,omitempty"`
}

// suiteStart names the detached test suite an adopt started: its pid and the log its output goes to.
type suiteStart struct {
	PID int    `json:"pid"`
	Log string `json:"log"`
}

// outcome is the verb's envelope: the reconcile's report, the mutation record of every step, whether that record is non-empty, the prime's owner write, each entry's slugOutcome, the warning of a dobby start that failed, and the test suite started or the warning of a start that failed.
type outcome struct {
	Reconcile     fabricengine.OriginReport
	Mutations     []fabricengine.Mutation
	Partial       bool
	PrimeRecorded bool
	PrimeLost     string
	Slugs         []slugOutcome
	DobbyWarning  string
	Suite         *suiteStart
	SuiteWarning  string
}

// adoptDeps holds the seam of every step, which hubDeps fills over the hub and the tests fake.
// LocalPair reports whether this machine holds the slug's pair worktree.
// ParentOf names the slug of the pair a slug was forked from, or "" for the prime.
// ResumeRun starts the run's first pass detached, driven on by its parent's dobby, and returns the envelope of that start, where an envelope whose "ok" is false is a refusal.
// StartDobbies starts the dobby of the prime and of every beam this machine owns that is not live, and runs after the resumes; nil starts none.
// StartSuite starts the hub's finalize gate in the prime as a detached process and returns at once, so a break this machine brings to the landed code shows at adopt time; it runs last, once per call that recorded an owner.
// HubPath roots the mutation record.
type adoptDeps struct {
	Machine      func() (string, error)
	ReadOwners   func() (boardengine.Owners, error)
	ParentOf     func(slug string) (string, error)
	Reconcile    func() (fabricengine.OriginReport, error)
	LocalPair    func(slug string) bool
	LivePair     func(slug string) (bool, error)
	AddPair      func(slug string) (fabricengine.AddResult, error)
	DropLocal    func(rec *fabricengine.Mutations, slug string) (fabricengine.DropLocalResult, error)
	SetOwners    func(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error)
	PauseRecord  func() (pauserecord.Record, error)
	Lift         func(ref tenderengine.EntryRef) error
	RunRecord    func(slug string) (shedengine.Status, bool, error)
	ResumeRun    func(slug string) (map[string]any, error)
	ChildStatus  func(slug string) (shedengine.Status, bool, error)
	ResumeChild  func(slug string) error
	StartDobbies func() error
	StartSuite   func() (suiteStart, error)
	HubPath      string
}

// adopt takes the requested entries over from another machine, and returns what each step did.
// A refusal before any entry is touched returns the error with the outcome gathered so far, so the envelope still carries the reconcile and its mutation record.
func adopt(deps adoptDeps, req request) (outcome, error) {
	if req.KeepLocal && req.DropLocal {
		return outcome{}, errors.New("lyx adopt: --keep-local and --drop-local answer the same local commits in opposite ways: pass one of them")
	}
	if req.All && len(req.Slugs) > 0 {
		return outcome{}, errors.New("lyx adopt: --all takes no slug: drop the slugs, or drop --all")
	}
	if len(req.Slugs) == 0 && !req.Prime && !req.All {
		return outcome{}, errors.New("lyx adopt: name a task slug, or pass --prime or --all: `lyx board list` shows the entries")
	}

	here, err := deps.Machine()
	if err != nil {
		return outcome{}, fmt.Errorf("lyx adopt: read this machine's name: %w", err)
	}
	owners, err := deps.ReadOwners()
	if err != nil {
		return outcome{}, fmt.Errorf("lyx adopt: read the board's owners: %w", err)
	}
	slugs, primeNamed, err := namedEntries(owners, req)
	if err != nil {
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

	outcomes := map[string]*slugOutcome{}
	for _, slug := range slugs {
		outcomes[slug] = &slugOutcome{Slug: slug}
	}

	var going []string
	for _, slug := range slugs {
		stops, err := wouldStop(deps, req, report, slug)
		if err != nil {
			return finish(), fmt.Errorf("lyx adopt: read whether the pair of %s is live, so nothing ran: %w; %s", slug, err, reentry)
		}
		if !stops {
			going = append(going, slug)
		}
	}
	if err := adoptRefusal(deps, req, report, here, owners, going, primeNamed); err != nil {
		return finish(), err
	}

	var adoptable []string
	for _, slug := range slugs {
		if takeOverSlug(deps, req, rec, report, owners, outcomes[slug]) {
			adoptable = append(adoptable, slug)
		}
	}

	if !primeNamed && owners.Prime == "" && len(adoptable) > 0 {
		primeNamed = true
	}
	var adopted []string
	if len(adoptable) > 0 || req.Prime || req.All {
		write, err := deps.SetOwners(boardengine.OwnerRequest{Machine: here, Slugs: adoptable, Prime: primeNamed, Takeover: true, ParentOf: deps.ParentOf})
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
		for _, slug := range adopted {
			resumeSlug(deps, here, after, outcomes[slug])
		}
		resumeHaltedChildren(deps, here, after, adopted, result.PrimeRecorded, outcomes)
	}

	if deps.StartDobbies != nil {
		if err := deps.StartDobbies(); err != nil {
			result.DobbyWarning = fmt.Sprintf("the dobbies were not all started (%v); way forward: fix the cause each worktree names, then run `lyx beam start`", err)
		}
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

// namedEntries resolves the request to the slugs it adopts and whether it names the prime, refusing a slug the board does not hold as a task or a beam project that is not done.
// --all names the prime, every task that is not done and has an owner or a run status, and every beam project that is not done and has an owner.
func namedEntries(owners boardengine.Owners, req request) (slugs []string, prime bool, err error) {
	if req.All {
		for slug, entry := range owners.Entries {
			startedTask := entry.IsTask && (entry.Owner != "" || entry.RunHeld)
			ownedBeam := entry.IsProject && entry.Beam && entry.Owner != ""
			if !entry.Done && (startedTask || ownedBeam) {
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
			return nil, false, fmt.Errorf("lyx adopt: %q is done and already landed, so there is nothing to adopt: `lyx resume %s` lets a run that landed there go on to Land under its owner pause", slug, slug)
		case entry.IsProject && !entry.Beam:
			return nil, false, fmt.Errorf("lyx adopt: %q is a grouping project, not a beam, so there is no pair to take over: `lyx beam create %s` makes it a beam", slug, slug)
		}
		if !slices.Contains(slugs, slug) {
			slugs = append(slugs, slug)
		}
	}
	return slugs, req.Prime, nil
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
		case fabricengine.BranchRewound, fabricengine.BranchRewritten, fabricengine.BranchDiverged:
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
		case fabricengine.BranchNeedsReplay, fabricengine.BranchAhead, fabricengine.BranchRewound, fabricengine.BranchRewritten, fabricengine.BranchDiverged:
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

// takeOverSlug runs steps 1's tail and 2 for one entry: it stops the entry on local commits or a live pair, drops them under --drop-local, and adds the missing pair.
// It reports whether the entry goes on to the owner write; a stop or an error is recorded on its outcome.
func takeOverSlug(deps adoptDeps, req request, rec *fabricengine.Mutations, report fabricengine.OriginReport, owners boardengine.Owners, out *slugOutcome) bool {
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
			out.WayForward = fmt.Sprintf("the pair holds local state origin lacks or a live driver: `lyx adopt %s --keep-local` records the owner over it, and `lyx adopt %s --drop-local` keeps each local tip under an archive tag and moves the branches to origin's", slug, slug)
			return false
		}
	}

	entry := owners.Entries[slug]
	// A beam's pair is added the same way as a task's: Add adopts the live branches and their origin record, the beam mark included.
	if !hasPair && (entry.Owner != "" || entry.RunHeld) {
		added, err := deps.AddPair(slug)
		rec.Extend(added.Mutated())
		if err != nil {
			out.Error = addError(slug, err)
			return false
		}
		out.PairAdded = true
	}
	return true
}

// addError words a refused pair add.
// The leftover local branch's refusal is reworded, since the way forward fabric names would drop this machine's unpushed commits on that branch with no archive.
func addError(slug string, err error) string {
	var exists *fabricengine.ErrBranchExists
	if errors.As(err, &exists) {
		return fmt.Sprintf("the leftover local branch %s may hold commits origin lacks: keep its tip first with `git tag archive/%s/<tip> %s`, then `git branch -D %s`, then re-run `lyx adopt %s`", exists.Branch, slug, exists.Branch, exists.Branch, slug)
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

// resumeSlug runs steps 4 and 5 for an adopted entry: it lifts the owner pause, resumes a run that is not done and warns when this machine does not own the parent.
func resumeSlug(deps adoptDeps, here string, owners boardengine.Owners, out *slugOutcome) {
	slug := out.Slug

	out.Warning = parentWarning(deps, here, owners, slug)

	if err := liftOwnerPauses(deps, out); err != nil {
		out.Error = err.Error()
		return
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

// parentWarning returns the warning for an entry whose parent another machine owns, since its run halts at Finalize here; it is empty otherwise.
func parentWarning(deps adoptDeps, here string, owners boardengine.Owners, slug string) string {
	parent, err := deps.ParentOf(slug)
	if err != nil {
		return fmt.Sprintf("the owner of the parent could not be read: %v", err)
	}
	owner, adoptCommand := owners.Prime, "lyx adopt --prime"
	if parent != "" {
		owner, err = owners.Effective(parent, deps.ParentOf)
		if err != nil {
			return fmt.Sprintf("the owner of the parent %s could not be read: %v", parent, err)
		}
		adoptCommand = "lyx adopt " + parent
	}
	if !boardengine.OwnedElsewhere(owner, here) {
		return ""
	}
	return fmt.Sprintf("this machine does not own the parent, which %s owns, so the run halts at Finalize here: `%s` takes the parent over", owner, adoptCommand)
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
			out.PauseStanding = "a hub pause covers it: `lyx resume` lifts it"
		} else {
			out.PauseStanding = fmt.Sprintf("a pause of its own scope by %s covers it: `lyx resume %s` lifts it", entry.Caller, out.Slug)
		}
	}
	return nil
}

// resumeHaltedChildren resumes each child halted at Finalize's owner halt whose run this machine now owns and whose own slug, or an ancestor pair or the prime, was adopted.
// A child halted for any other reason is left alone.
func resumeHaltedChildren(deps adoptDeps, here string, owners boardengine.Owners, adopted []string, primeAdopted bool, outcomes map[string]*slugOutcome) {
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
		under, err := adoptedUnder(deps, slug, adopted, primeAdopted)
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

// adoptedUnder reports whether slug or one of its ancestor pairs was adopted, or its chain ends at the prime and the prime was.
func adoptedUnder(deps adoptDeps, slug string, adopted []string, primeAdopted bool) (bool, error) {
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
			return primeAdopted, nil
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
