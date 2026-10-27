// anchor.go is `lym fabric anchor [<slug>] [--set <dir>]`: it prints a worktree's anchor, or moves it to another directory of the warp.
// The refusals, the waits and the session, run and dobby steps around the move are this file's; the move itself is fabricengine's AnchorChange.

package fabriccli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/spf13/cobra"
)

// anchorPassPoll is how long the wait for passes in flight sleeps between two readings.
const anchorPassPoll = 2 * time.Second

// beamStartWayForward is the verb that starts the dobby a finished change could not.
const beamStartWayForward = "lym beam start"

// anchorRequest is one call of the verb.
type anchorRequest struct {
	// Slug names the worktree, the prime by its worktree name; empty means the acting worktree.
	Slug string
	// Set is the directory to move the worktree's anchor to; empty only prints the anchor.
	Set string
}

// anchorResult is what one call reports.
type anchorResult struct {
	// Slug is the worktree the call read or changed.
	Slug string
	// Anchor is the worktree's anchor after the call.
	Anchor string
	// From is the anchor a change left; empty when the call changed nothing.
	From string
	// Changed reports that the call moved the worktree.
	Changed bool
	// NoOp reports that the worktree already sat at the requested anchor.
	NoOp bool
	// PushError is the failure of the detached push a finished change spawns, which the next push of the worktree makes up.
	PushError string
	// DobbyStartError is the failure to start the worktree's dobby after a finished change, whose way forward is `lym beam start`.
	DobbyStartError string
	// Mutations is the record of what the call changed.
	Mutations fabricengine.Mutations
}

// fields returns the envelope fields of the result.
func (r anchorResult) fields() map[string]any {
	fields := map[string]any{"slug": r.Slug, "anchor": r.Anchor, "changed": r.Changed}
	if r.NoOp {
		fields["noop"] = true
	}
	if r.From != "" {
		fields["from"] = r.From
	}
	if r.PushError != "" {
		fields["push_error"] = r.PushError
	}
	if r.DobbyStartError != "" {
		fields["dobby_start_error"] = r.DobbyStartError
		fields["way_forward"] = beamStartWayForward
	}
	return fields
}

// anchorChange is the engine's AnchorChange as the verb drives it.
type anchorChange interface {
	NoOp() bool
	From() string
	To() string
	Apply(rec *fabricengine.Mutations) (landed bool, err error)
	Finish(rec *fabricengine.Mutations) error
	Close()
}

// anchorDeps is every read and action the verb makes beyond its own sequence, one seam each.
type anchorDeps struct {
	// GOOS names the platform; on Windows a process end signals nothing, so a live dobby is refused instead.
	GOOS string
	// Acting and Prime name the acting worktree and the hub's prime worktree.
	Acting, Prime string
	// Worktree returns the Location of the worktree called slug at its own anchor, and reports false when this machine holds no such worktree.
	Worktree func(slug string) (location *lymcwd.Location, held bool, err error)
	// IsBeam reports whether the pair called slug is a beam, which runs a dobby.
	IsBeam func(slug string) (bool, error)
	// LocalBeams lists the beams this machine holds.
	LocalBeams func() ([]string, error)
	// ForeignOwner returns the machine other than this one that the board names as the worktree's effective owner, empty when none does.
	ForeignOwner func(slug string) (string, error)
	// AdoptSubject returns the beam or `root` that `lym adopt` takes to move the worktree called slug, read from the local board.
	AdoptSubject func(slug string) (string, error)
	// UpstreamAnchor returns the anchor the worktree's weft records at its fetched upstream tip, and found false when it has no upstream.
	UpstreamAnchor func(location *lymcwd.Location) (anchor string, found bool, err error)
	// MoveRecord reads the move record of the worktree's weft.
	MoveRecord func(location *lymcwd.Location) (record fabricengine.MoveRecord, found bool, err error)
	// ReedHeld reports whether the worktree's reed state holds a session or a strand, live or stored.
	ReedHeld func(location *lymcwd.Location) (bool, error)
	// DriverLockHeld reports whether the worktree's run driver lock is held.
	DriverLockHeld func(location *lymcwd.Location) (bool, error)
	// BattenLive reports whether a registered batten run of the worktree is live.
	BattenLive func(location *lymcwd.Location) (bool, error)
	// RunUnfinished reports whether the worktree's own run status, as checked out, names an unfinished run.
	RunUnfinished func(location *lymcwd.Location) (bool, error)
	// UpstreamRunUnfinished reports the same from the tree of the worktree's weft's fetched upstream tip.
	UpstreamRunUnfinished func(location *lymcwd.Location) (bool, error)
	// PrimeRunUnfinished reports whether the prime's batten status of the run called slug names an unfinished run.
	PrimeRunUnfinished func(slug string) (bool, error)
	// DobbyPID returns the pid of the live dobby of the worktree, and live false when it runs none.
	DobbyPID func(worktree string) (pid int, live bool, err error)
	// HubReedHeld lists the worktrees of the hub whose reed state holds a session or a strand.
	HubReedHeld func() ([]string, error)
	// HubBattenLive lists the runs of the hub that have a live batten.
	HubBattenLive func() ([]string, error)
	// GateHolders describes each gate slot that is held.
	GateHolders func() ([]string, error)
	// HibernateLive reports whether the hub's hibernate daemon runs.
	HibernateLive func() (bool, error)
	// PassInFlight describes the pass of the worktree's dobby that is in flight, empty when none is.
	PassInFlight func(worktree string) (string, error)
	// Sleep waits d, and returns the context's error when it ends first.
	Sleep func(ctx context.Context, d time.Duration) error
	// Note tells the operator what the verb waits on.
	Note func(line string)
	// AcquireParentLock takes the parent lock of the worktree's branch as the verb's own owner, queuing behind a holder, and returns its release.
	AcquireParentLock func(ctx context.Context, location *lymcwd.Location) (release func(), err error)
	// EndDobbies ends the dobbies of the worktrees, leaving alone the processes registered with a role in spare, and returns the ones it ended.
	EndDobbies func(worktrees []string, spare ...string) (ended []string, err error)
	// EnsureDobbies starts the dobby of each worktree named that is not live, refusing one this machine does not own or whose landing hold stands.
	EnsureDobbies func(worktrees []string) error
	// Begin begins the change of the worktree's anchor in the engine.
	Begin func(location *lymcwd.Location, target string) (anchorChange, error)
	// ReopenLog moves the process's trace sink to dir.
	ReopenLog func(dir string) error
}

// changeAnchor prints the anchor of the worktree req names, or with req.Set moves it.
// A move is refused before anything is touched when the board names another machine as owner, when the worktree's reed state, run or dobby is live, and for the prime when anything of the hub is.
// The engine's own refusals follow, and a worktree already at the anchor is reported as such.
// Otherwise it waits out the passes in flight, takes the parent lock, ends the dobbies, moves the worktree and starts the dobby again.
// A failure before the record the engine wrote lands leaves nothing standing and restarts the dobbies; a failure with the record standing names the re-run of the same --set and starts none.
func changeAnchor(ctx context.Context, deps anchorDeps, req anchorRequest) (anchorResult, error) {
	slug := req.Slug
	if slug == "" {
		slug = deps.Acting
	}
	location, held, err := deps.Worktree(slug)
	if err != nil {
		return anchorResult{}, err
	}
	if !held {
		return anchorResult{}, fmt.Errorf("no worktree called %q is present on this machine; `lym fabric pairs` lists the pairs it holds", slug)
	}
	result := anchorResult{Slug: slug, Anchor: location.AnchorRel}
	if req.Set == "" {
		return result, nil
	}

	target, err := lymcwd.ValidateAnchorRel(req.Set)
	if err != nil {
		return result, fmt.Errorf("%w; choose another directory of the warp checkout", err)
	}
	if err := refuseAnchorChange(deps, location, target); err != nil {
		return result, err
	}
	return moveAnchor(ctx, deps, location, target, result)
}

// refuseAnchorChange returns the refusal that stops the move of location's anchor to target before anything is touched, nil when none does.
func refuseAnchorChange(deps anchorDeps, location *lymcwd.Location, target string) error {
	slug := location.WorktreeName
	again := fmt.Sprintf("then run `lym fabric anchor %s --set %s` again", slug, target)

	_, standing, err := deps.MoveRecord(location)
	if err != nil {
		return err
	}
	owner, err := deps.ForeignOwner(slug)
	if err != nil {
		return err
	}
	if owner != "" && !standing {
		if err := refuseForeignOwner(deps, location, owner, target); err != nil {
			return err
		}
	}

	if reedHeld, err := deps.ReedHeld(location); err != nil {
		return err
	} else if reedHeld {
		return fmt.Errorf("the reed state of %s holds a session or a strand; run `lym reed down` in that worktree, %s", slug, again)
	}
	if locked, err := deps.DriverLockHeld(location); err != nil {
		return err
	} else if locked {
		return fmt.Errorf("the run driver of %s is live, its driver lock being held; wait for it, or run `lym batten cancel %s`, %s", slug, slug, again)
	}
	if live, err := deps.BattenLive(location); err != nil {
		return err
	} else if live {
		return fmt.Errorf("a batten run of %s is live; wait for it to halt or finish, or run `lym batten cancel %s`, %s", slug, slug, again)
	}
	if err := refuseUnfinishedRun(deps, location, owner, target); err != nil {
		return err
	}
	return refuseLiveDobbies(deps, location, target)
}

// refuseForeignOwner refuses a move of a worktree another machine owns, unless target is the anchor the weft's fetched upstream tip already records.
// The refusal names the three ways forward: adopting the worktree here, running the move on the owner machine, and taking the upstream tip's anchor when it differs from the checked-out one.
func refuseForeignOwner(deps anchorDeps, location *lymcwd.Location, owner, target string) error {
	slug := location.WorktreeName
	upstream, hasUpstream, err := deps.UpstreamAnchor(location)
	if err != nil {
		return err
	}
	if hasUpstream && filepath.Clean(upstream) == filepath.Clean(target) {
		return nil
	}
	subject := boardengine.RootSubject
	if slug != deps.Prime {
		if subject, err = deps.AdoptSubject(slug); err != nil {
			return err
		}
	}
	adopt := fmt.Sprintf("`lym adopt %s` here, which records this machine as the owner, or `lym adopt %s --keep-local` over a weft stopped by an anchor change, which records the owner over the stopped weft", subject, subject)
	if slug == deps.Prime {
		adopt = "`lym adopt root` here, which records this machine as the owner, and whose refusal over a prime weft stopped by an anchor change names the `--set` to its upstream tip's marker"
	}
	message := fmt.Sprintf("the board names %s as the owner of %s, so this machine does not change its anchor; run %s; or run `lym fabric anchor %s --set %s` on %s", owner, slug, adopt, slug, target, owner)
	if hasUpstream && filepath.Clean(upstream) != filepath.Clean(location.AnchorRel) {
		message += fmt.Sprintf("; or run `lym fabric anchor %s --set %s`, the marker at the fetched upstream tip of its weft, which this refusal lets through", slug, upstream)
	}
	return errors.New(message)
}

// refuseUnfinishedRun refuses a worktree whose run status names an unfinished run.
// The status is the worktree's own, read from the weft's fetched upstream tip when another machine owns the worktree, since the weft pull that would refresh the checked-out copy stops at the marker change.
// A pair's batten status in the prime counts too.
func refuseUnfinishedRun(deps anchorDeps, location *lymcwd.Location, owner, target string) error {
	slug := location.WorktreeName
	again := fmt.Sprintf("`lym fabric anchor %s --set %s`", slug, target)

	if owner != "" {
		unfinished, err := deps.UpstreamRunUnfinished(location)
		if err != nil {
			return err
		}
		if unfinished {
			return fmt.Errorf("the run of %s reads unfinished at the fetched upstream tip of its weft, and %s owns it; run `lym batten cancel %s` on %s, which accepts it, then, once the cancel's push has reached origin, run `lym fabric pull` in the worktree, which fetches before it stops and so moves the fetched tip, then run %s with the marker at that tip",
				slug, owner, slug, owner, again)
		}
	} else {
		unfinished, err := deps.RunUnfinished(location)
		if err != nil {
			return err
		}
		if unfinished {
			return fmt.Errorf("%s has an unfinished run; wait for it to finish, or run `lym batten cancel %s`, which an entry stopped only by this change lets through, then run %s again", slug, slug, again)
		}
	}
	if slug == deps.Prime {
		return nil
	}
	unfinished, err := deps.PrimeRunUnfinished(slug)
	if err != nil {
		return err
	}
	if unfinished {
		return fmt.Errorf("the batten run of %s reads unfinished in the prime; wait for it to finish, or run `lym batten cancel %s`, then run %s again", slug, slug, again)
	}
	return nil
}

// refuseLiveDobbies refuses the move on Windows when a dobby it would end is live, and for the prime when anything of the hub is.
func refuseLiveDobbies(deps anchorDeps, location *lymcwd.Location, target string) error {
	slug := location.WorktreeName
	again := fmt.Sprintf("then run `lym fabric anchor %s --set %s` again", slug, target)

	if deps.GOOS == "windows" {
		worktrees, err := dobbyWorktrees(deps, slug)
		if err != nil {
			return err
		}
		for _, worktree := range worktrees {
			pid, live, err := deps.DobbyPID(worktree)
			if err != nil {
				return err
			}
			if live {
				return fmt.Errorf("the dobby of %s is live as process %d, and nothing here can end a process on Windows; end it with `taskkill /PID %d /T /F`, %s", worktree, pid, pid, again)
			}
		}
	}
	if slug != deps.Prime {
		return nil
	}

	if reed, err := deps.HubReedHeld(); err != nil {
		return err
	} else if len(reed) > 0 {
		return fmt.Errorf("reed holds a session or a strand in %s; run `lym reed down` in each of those worktrees, the orch's included, %s", strings.Join(reed, ", "), again)
	}
	if runs, err := deps.HubBattenLive(); err != nil {
		return err
	} else if len(runs) > 0 {
		return fmt.Errorf("batten runs are live: %s; wait for each to halt or finish, which `lym batten status` in that run's worktree reports, or pause it with `lym batten pause`, %s", strings.Join(runs, ", "), again)
	}
	if holders, err := deps.GateHolders(); err != nil {
		return err
	} else if len(holders) > 0 {
		return fmt.Errorf("gate slots are held: %s; wait for each holder to finish, %s", strings.Join(holders, ", "), again)
	}
	if live, err := deps.HibernateLive(); err != nil {
		return err
	} else if live {
		return fmt.Errorf("the hub's hibernate daemon is live; cancel it with `lym hibernate cancel`, %s", again)
	}
	return nil
}

// dobbyWorktrees returns the worktrees whose dobby a move of slug ends: the prime's dobby and every local beam's for the prime, the beam's own for a beam, and none for any other pair.
func dobbyWorktrees(deps anchorDeps, slug string) ([]string, error) {
	if slug == deps.Prime {
		beams, err := deps.LocalBeams()
		if err != nil {
			return nil, err
		}
		return append([]string{deps.Prime}, beams...), nil
	}
	beam, err := deps.IsBeam(slug)
	if err != nil || !beam {
		return nil, err
	}
	return []string{slug}, nil
}

// moveAnchor carries the move of location's anchor to target out, around the engine's three calls.
func moveAnchor(ctx context.Context, deps anchorDeps, location *lymcwd.Location, target string, result anchorResult) (anchorResult, error) {
	slug := location.WorktreeName
	rec := fabricengine.NewMutations(location.HubPath)
	change, err := deps.Begin(location, target)
	if err != nil {
		return result, err
	}
	if change.NoOp() {
		change.Close()
		result.NoOp = true
		return result, nil
	}
	dobbies, err := dobbyWorktrees(deps, slug)
	if err != nil {
		change.Close()
		return result, err
	}

	var releaseParentLock func()
	var ended []string
	// fail ends the verb on cause: it releases the parent lock, closes the change, and decides by the record that is left whether the dobbies it ended start again.
	fail := func(cause error) (anchorResult, error) {
		if releaseParentLock != nil {
			releaseParentLock()
			releaseParentLock = nil
		}
		change.Close()
		result.Mutations = rec.Snapshot()
		_, standing, readErr := deps.MoveRecord(location)
		rerun := fmt.Sprintf("`lym fabric anchor %s --set %s`", slug, target)
		switch {
		case readErr != nil:
			return result, fmt.Errorf("%w; whether the change record stands could not be read (%v), so no dobby is started: run %s again", cause, readErr, rerun)
		case standing:
			return result, fmt.Errorf("%w; the change record stands, so no dobby is started: run %s again, which recovers it and starts the dobby", cause, rerun)
		}
		if startErr := startDobbies(deps, ended); startErr != nil {
			return result, errors.Join(cause, fmt.Errorf("start the dobby again: %w; run `%s`", startErr, beamStartWayForward))
		}
		return result, cause
	}

	for {
		busy, err := passesInFlight(deps, dobbies)
		if err != nil {
			return fail(err)
		}
		if len(busy) == 0 {
			break
		}
		deps.Note("waiting for " + strings.Join(busy, ", "))
		if err := deps.Sleep(ctx, anchorPassPoll); err != nil {
			return fail(err)
		}
	}
	release, err := deps.AcquireParentLock(ctx, location)
	if err != nil {
		return fail(err)
	}
	releaseParentLock = release
	if len(dobbies) > 0 {
		ended, err = deps.EndDobbies(dobbies, runproc.RolePush, runproc.RoleSync)
		if err != nil {
			return fail(err)
		}
	}

	if err := deps.ReopenLog(fabricengine.RecordsLockDirPath(fabricengine.RecordsWorktreePath(location, slug))); err != nil {
		return fail(err)
	}
	landed, applyErr := change.Apply(rec)
	settled := change.From()
	if landed {
		settled = change.To()
	}
	reopenErr := deps.ReopenLog(logger.LogsDirAt(filepath.Join(fabricengine.WorktreePath(location, slug), settled)))
	if applyErr != nil {
		if reopenErr != nil {
			applyErr = errors.Join(applyErr, reopenErr)
		}
		return fail(applyErr)
	}
	if reopenErr != nil {
		return fail(reopenErr)
	}

	if finishErr := change.Finish(rec); finishErr != nil {
		_, standing, readErr := deps.MoveRecord(location)
		if readErr != nil || standing {
			return fail(finishErr)
		}
		result.PushError = finishErr.Error()
	}
	result.From, result.Anchor, result.Changed = change.From(), change.To(), true
	if startErr := startDobbies(deps, dobbies); startErr != nil {
		result.DobbyStartError = startErr.Error()
	}
	releaseParentLock()
	releaseParentLock = nil
	change.Close()
	result.Mutations = rec.Snapshot()
	return result, nil
}

// passesInFlight describes every pass in flight of the dobbies of worktrees.
func passesInFlight(deps anchorDeps, worktrees []string) ([]string, error) {
	var busy []string
	for _, worktree := range worktrees {
		pass, err := deps.PassInFlight(worktree)
		if err != nil {
			return nil, err
		}
		if pass != "" {
			busy = append(busy, pass)
		}
	}
	return busy, nil
}

// startDobbies starts the dobbies of worktrees and reports a start that failed.
// A move that finished names every dobby it was to end, and a move that failed names the ones it ended.
// It starts nothing when none are named.
func startDobbies(deps anchorDeps, ended []string) error {
	if len(ended) == 0 {
		return nil
	}
	return deps.EnsureDobbies(ended)
}

// runAnchor executes `lym fabric anchor [<slug>] [--set <dir>]`.
func runAnchor(ctx context.Context, out io.Writer, args []string, set string) int {
	_, location, err := resolveWarpLocation(ctx)
	if err != nil {
		return output.Err(out, err.Error())
	}
	deps, err := hubAnchorDeps(location)
	if err != nil {
		return output.Err(out, err.Error())
	}
	req := anchorRequest{Set: set}
	if len(args) > 0 {
		req.Slug = args[0]
	}
	result, err := changeAnchor(ctx, deps, req)
	if err != nil {
		return errWithRecord(out, result.Mutations, err)
	}
	return okWithRecord(out, result.Mutations, result.fields())
}

// anchorCommand builds the `anchor` subcommand of the fabric group.
func anchorCommand() *cobra.Command {
	var anchorCmd *cobra.Command
	anchorCmd = &cobra.Command{
		Use:         "anchor [<slug>]",
		Args:        cobra.MaximumNArgs(1),
		Short:       "print a worktree's anchor, or move it to another directory of the warp",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `anchor prints the anchor of <slug>, the acting worktree when it is left out, which
is the directory of the warp checkout lym works in for it. <slug> names the prime by
its worktree name, or a pair present on this machine.

With --set <dir> it moves the worktree's anchor there: its junctions and launchers,
and the content of its records branch, which a single commit on that branch re-roots
and whose marker names the new directory. <dir> must be a directory the warp checkout
tracks, so commit it there first when it is new.

The commit is shared by every machine that checks the records branch out. Their
pulls and pushes of it stop at the marker change until "lym fabric anchor <slug> --set
<dir>" integrates the change there, carrying their unpushed records commits along.
A worktree already at <dir> is left as it is, and a killed move is finished by running
the same --set again.

The move is refused, naming its way forward, while the board names another machine as
the owner, while the worktree's reed session or strands, its run driver, a batten run or
an unfinished run are live, and, on Windows, while its dobby is. For the prime it is
refused while any reed session, batten run, gate slot or hibernate daemon of the hub is
live. It waits out the passes of the dobbies it ends, takes the parent lock of the
worktree's branch, ends the worktree's dobby, or every dobby of this machine for the
prime, and starts it again afterwards, reporting a start that failed as
dobby_start_error with the way forward "lym beam start".

Start no session, hand pass or "lym beam dobby" in the worktree while it runs, and for
the prime none in the hub: the verb holds no lock against them.

Example:
  lym fabric anchor
  lym fabric anchor my-task --set backend`,
		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int {
			set, _ := anchorCmd.Flags().GetString("set")
			return runAnchor(ctx, out, args, set)
		}),
	}
	anchorCmd.Flags().String("set", "", "move the anchor to this directory of the warp checkout (default: print the anchor)")
	return anchorCmd
}
