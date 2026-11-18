// start.go brings back the hubs this machine owns with `lym machine start`:
// it starts each hub's owned worktrees through `lym beam start`, opens their windows and keeps the desktop launchers.

package machinecli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/beamcli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/desktoplauncher"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubregistry"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shell"
)

// The outcomes of a worktree under `lym machine start`.
const (
	startStarted = "started"
	startSkipped = "skipped"
	startFailed  = "failed"
)

// The launcher actions a hub reports.
const (
	launcherAdded   = "added"
	launcherRemoved = "removed"
)

// beamStartEntry is one worktree's entry in the envelope `lym beam start` prints.
type beamStartEntry struct {
	// Worktree is dobbyengine.PrimeLabel for the prime, else the beam's slug.
	Worktree string `json:"worktree"`
	// Outcome is the dobby's outcome, empty when no dobby start was attempted.
	Outcome string `json:"outcome,omitempty"`
	// Detail says why the dobby was refused or failed.
	Detail string `json:"detail,omitempty"`
	// Warnings are the notes that did not stop the worktree's start.
	Warnings []string `json:"warnings,omitempty"`
	// Error is why the worktree did not come up, empty when it did.
	Error string `json:"error,omitempty"`
}

// startWorktree is one worktree's outcome under `lym machine start`.
type startWorktree struct {
	// Name is the hub's shortname and the worktree, which label the worktree in the readable summary.
	Name string `json:"name"`
	// Shortname is the shortname of the hub the worktree lies in; startHub keys the same value as name.
	Shortname string `json:"shortname"`
	// Worktree is dobbyengine.PrimeLabel for the prime, else the beam's slug.
	Worktree string `json:"worktree"`
	// Outcome is startStarted, startSkipped or startFailed.
	Outcome string `json:"outcome"`
	// Detail says why the worktree was skipped or failed, with its way forward.
	Detail string `json:"detail,omitempty"`
	// Attach is the command that attaches to a started worktree's reed session, set when no terminal window opened for it.
	Attach string `json:"attach,omitempty"`
	// Line is the outcome with its detail or attach command on one line, which the readable summary prints.
	Line string `json:"line"`
}

// startHold is a standing adopt hold as `lym machine start` prints it.
type startHold struct {
	Machine  string   `json:"machine"`
	Subjects []string `json:"subjects"`
	// Rerun is the `lym adopt <subjects>` re-run, set for a hold this machine wrote.
	Rerun string `json:"rerun,omitempty"`
}

// startHub is one hub's outcome under `lym machine start`.
type startHub struct {
	Shortname string `json:"name"`
	Hub       string `json:"hub"`
	// Worktrees are the hub's worktrees, which the envelope lists in startResult.Worktrees.
	Worktrees []startWorktree `json:"-"`
	Hold      *startHold      `json:"hold,omitempty"`
	// Line is the hold on one line, which the readable summary prints; empty when no hold stands.
	Line string `json:"line,omitempty"`
	// Workspace is the hub's VS Code workspace file the verb opened, empty when it opened none.
	Workspace string `json:"workspace,omitempty"`
	// Launcher is launcherAdded or launcherRemoved, empty when the machine keeps no launcher or its write failed.
	Launcher string `json:"launcher,omitempty"`
	// Warnings are the hub's steps that failed without failing the verb.
	Warnings []string `json:"warnings,omitempty"`
	// Error is why the hub could not be read, so nothing of it started.
	Error string `json:"error,omitempty"`
}

// failed reports whether the hub did not read or one of its worktrees did not start.
func (h startHub) failed() bool {
	return h.Error != "" || slices.ContainsFunc(h.Worktrees, func(w startWorktree) bool { return w.Outcome == startFailed })
}

// summarize fills the labels and one-line texts the readable summary prints for the hub and its worktrees.
func (h *startHub) summarize() {
	if h.Hold != nil {
		h.Line = fmt.Sprintf("hold: %s holds %s", h.Hold.Machine, strings.Join(h.Hold.Subjects, ", "))
		if h.Hold.Rerun != "" {
			h.Line += "; re-run: " + h.Hold.Rerun
		}
	}
	for i := range h.Worktrees {
		worktree := &h.Worktrees[i]
		worktree.Shortname = h.Shortname
		worktree.Name = h.Shortname + "/" + worktree.Worktree
		switch {
		case worktree.Detail != "":
			worktree.Line = worktree.Outcome + ": " + worktree.Detail
		case worktree.Attach != "":
			worktree.Line = worktree.Outcome + "; attach: " + worktree.Attach
		default:
			worktree.Line = worktree.Outcome
		}
	}
}

// startResult is the envelope of `lym machine start`.
type startResult struct {
	Hubs []startHub `json:"hubs"`
	// Worktrees lists every hub's worktrees, each named by its hub and worktree.
	Worktrees []startWorktree `json:"worktrees"`
	// LaunchersRemoved lists the shortnames whose launcher went because the machine record no longer lists their hub as live.
	LaunchersRemoved []string `json:"launchers_removed,omitempty"`
	// Warnings are the steps that failed for every hub without failing the verb.
	Warnings []string `json:"warnings,omitempty"`
}

// startSession is what one run of `lym machine start` shares across its hubs.
type startSession struct {
	d       deps
	lymPath string
	// openTerminal opens a terminal window attached to a worktree's reed session; nil when no launcher resolves.
	openTerminal func(dir, title, lymPath string) error
	// desktop holds the launchers; nil when this machine has none.
	desktop *desktoplauncher.Desktop
}

// startRun is one `lym beam start` child: the worktree it is reported under when it fails as a whole, and its arguments.
type startRun struct {
	label string
	args  []string
}

// startPlan is what `lym machine start` starts in one hub and what it skips.
type startPlan struct {
	// primeOwned reports that this machine owns the prime, so `lym beam start --all` starts it and every owned beam.
	primeOwned bool
	// beams are the owned beams started one by one when the prime is not owned here.
	beams   []string
	skipped []startWorktree
}

// runStart runs `lym machine start` over d for the hub shortname names, or for every live hub when it is empty.
// It returns the exit code.
func runStart(out io.Writer, d deps, shortname string) int {
	live, err := d.liveHubEntries()
	if err != nil {
		return output.Err(out, fmt.Sprintf("lym machine start: %v; way forward: fix the record it names, then run lym machine start again", err))
	}
	targets := live
	if shortname != "" {
		hub, err := d.lookupHub(shortname)
		if err != nil {
			return output.Err(out, fmt.Sprintf("lym machine start: no live hub reads the shortname %s; way forward: run lym machine start with no argument to start every live hub, or run lym creel inbox in that hub once to record it: %v", shortname, err))
		}
		targets = []hubregistry.Entry{{HubPath: hub, Shortname: shortname}}
	}
	lymPath, err := d.lymBinary()
	if err != nil {
		return output.Err(out, fmt.Sprintf("lym machine start: resolve the running lym binary: %v; way forward: run lym machine start from an installed lym", err))
	}
	session := startSession{d: d, lymPath: lymPath}
	result := startResult{Hubs: []startHub{}, Worktrees: []startWorktree{}}
	if open, err := d.terminal(); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("no terminal window opens, so each started worktree carries its attach command: %v", err))
	} else {
		session.openTerminal = open
	}
	if desktop, err := d.desktop(); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("no desktop launcher is kept: %v", err))
	} else {
		session.desktop = &desktop
	}

	failed := false
	for _, entry := range targets {
		hub := session.startHub(entry)
		hub.summarize()
		failed = failed || hub.failed()
		result.Hubs = append(result.Hubs, hub)
		result.Worktrees = append(result.Worktrees, hub.Worktrees...)
	}
	if session.desktop != nil {
		removed, err := removeGoneLaunchers(*session.desktop, live)
		result.LaunchersRemoved = removed
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("the launchers of hubs no longer live were not all removed: %v", err))
		}
	}
	fields := map[string]any{"start": result}
	if failed {
		return output.ErrFields(out, "lym machine start: a hub did not start in full; way forward: fix the cause each hub or worktree names, then run lym machine start again", fields)
	}
	return output.Ok(out, fields)
}

// startHub brings back the hub of entry.
// It pulls the board and reads the owners and the hold.
// It then starts the worktrees this machine owns, opens their windows and keeps the hub's launcher.
// A pull that fails or skips is a warning, and the local board's owners are read.
func (s startSession) startHub(entry hubregistry.Entry) startHub {
	hub := startHub{Shortname: entry.Shortname, Hub: entry.HubPath, Worktrees: []startWorktree{}}
	boardDir := fabricengine.BoardDir(entry.HubPath)
	if skip, detail, err := s.d.pull(boardDir); err != nil {
		hub.Warnings = append(hub.Warnings, fmt.Sprintf("the board did not pull, so the local board's owners are read: %v", err))
	} else if skip != "" {
		hub.Warnings = append(hub.Warnings, fmt.Sprintf("the board pull skipped (%s), so the local board's owners are read: %s", skip, detail))
	}
	owners, err := s.d.readOwners(boardDir)
	if err != nil {
		hub.Error = fmt.Sprintf("read the board's owners: %v", err)
		return hub
	}
	here, err := s.d.identity()
	if err != nil {
		hub.Error = fmt.Sprintf("read this machine's name: %v", err)
		return hub
	}
	hub.Hold = holdOf(owners, here)
	prime, start, err := s.d.hubStart(entry.HubPath)
	if err != nil {
		hub.Error = err.Error()
		return hub
	}
	plan, err := planStart(start)
	if err != nil {
		hub.Error = err.Error()
		return hub
	}
	hub.Worktrees = append(hub.Worktrees, plan.skipped...)
	s.runBeamStarts(&hub, prime, plan)
	s.openWindows(&hub, prime)
	s.keepLauncher(&hub, owners, here)
	return hub
}

// planStart sorts the worktrees dobbyengine.OwnedWorktrees lists into those to start and those to skip.
// The prime and each beam another machine owns are skipped, naming that machine and the adopt that takes it here.
// A prime or beam with no recorded owner is skipped too, since starting it would claim it.
func planStart(start dobbyengine.StartDeps) (startPlan, error) {
	owned, err := dobbyengine.OwnedWorktrees(start)
	if err != nil {
		return startPlan{}, err
	}
	primeOwner, primeElsewhere, err := start.PrimeOwner()
	if err != nil {
		return startPlan{}, fmt.Errorf("read the prime owner: %w", err)
	}
	beams, err := start.Beams()
	if err != nil {
		return startPlan{}, fmt.Errorf("list the local beams: %w", err)
	}
	var plan startPlan
	skip := func(worktree, detail string) {
		plan.skipped = append(plan.skipped, startWorktree{Worktree: worktree, Outcome: startSkipped, Detail: detail})
	}
	switch {
	case primeElsewhere:
		skip(dobbyengine.PrimeLabel, fmt.Sprintf("owned by %s; way forward: lym adopt root takes it over here", primeOwner))
	case primeOwner == "":
		skip(dobbyengine.PrimeLabel, "no owner is recorded, and this verb claims none; way forward: lym beam start root in the prime claims it for this machine")
	default:
		plan.primeOwned = slices.Contains(owned, start.PrimeName)
	}
	for _, beam := range beams {
		switch {
		case beam.Elsewhere:
			skip(beam.Slug, fmt.Sprintf("owned by %s; way forward: lym adopt %s takes it over here", beam.Owner, beam.Slug))
		case beam.Owner == "":
			skip(beam.Slug, fmt.Sprintf("no owner is recorded for it or the worktrees above it; way forward: lym adopt %s takes it here", beam.Slug))
		case slices.Contains(owned, beam.Slug):
			plan.beams = append(plan.beams, beam.Slug)
		}
	}
	return plan, nil
}

// runBeamStarts starts the plan's worktrees through `lym beam start` children in the hub's prime.
// It records each worktree's outcome on hub.
// An owned prime is started with `--all`, which brings up every owned beam with it.
// Otherwise each owned beam is started by its slug, with the flag that keeps the child from claiming the prime.
// A child that does not report fails the worktrees it was started for.
// A worktree whose dobby was refused or failed is failed, since the dobby that takes its runs up is down.
func (s startSession) runBeamStarts(hub *startHub, prime *lymcwd.Location, plan startPlan) {
	var runs []startRun
	if plan.primeOwned {
		runs = append(runs, startRun{label: dobbyengine.PrimeLabel, args: []string{"beam", "start", "--all"}})
	} else {
		for _, slug := range plan.beams {
			runs = append(runs, startRun{label: slug, args: []string{"beam", "start", slug, "--" + beamcli.NoPrimeClaimFlag}})
		}
	}
	for _, run := range runs {
		entries, err := s.d.beamStart(s.lymPath, prime.AnchorPath(), run.args)
		if err != nil {
			hub.Worktrees = append(hub.Worktrees, startWorktree{Worktree: run.label, Outcome: startFailed, Detail: err.Error()})
			continue
		}
		for _, entry := range entries {
			for _, warning := range entry.Warnings {
				hub.Warnings = append(hub.Warnings, entry.Worktree+": "+warning)
			}
			switch {
			case entry.Error != "":
				hub.Worktrees = append(hub.Worktrees, startWorktree{Worktree: entry.Worktree, Outcome: startFailed, Detail: entry.Error})
			case entry.Outcome == dobbyengine.OutcomeRefused || entry.Outcome == dobbyengine.OutcomeFailed:
				detail := fmt.Sprintf("the dobby did not start (%s): %s", entry.Outcome, entry.Detail)
				hub.Worktrees = append(hub.Worktrees, startWorktree{Worktree: entry.Worktree, Outcome: startFailed, Detail: detail})
			default:
				hub.Worktrees = append(hub.Worktrees, startWorktree{Worktree: entry.Worktree, Outcome: startStarted})
			}
		}
	}
}

// openWindows opens the hub's VS Code workspace file.
// It also opens one terminal window per started worktree, attached to its reed session.
// A hub that started nothing opens nothing.
// A worktree whose window does not open carries its attach command instead.
// So does every one when no terminal launcher resolves.
func (s startSession) openWindows(hub *startHub, prime *lymcwd.Location) {
	if !slices.ContainsFunc(hub.Worktrees, func(w startWorktree) bool { return w.Outcome == startStarted }) {
		return
	}
	workspace := fabricengine.HubWorkspacePath(prime)
	if err := s.d.openWorkspace(workspace); err != nil {
		hub.Warnings = append(hub.Warnings, fmt.Sprintf("the workspace file %s did not open: %v", workspace, err))
	} else {
		hub.Workspace = workspace
	}
	for i := range hub.Worktrees {
		worktree := &hub.Worktrees[i]
		if worktree.Outcome != startStarted {
			continue
		}
		name := worktree.Worktree
		if name == dobbyengine.PrimeLabel {
			name = prime.WorktreeName
		}
		location, err := lymcwd.NamedWorktree(prime.HubPath, name)
		if err != nil {
			hub.Warnings = append(hub.Warnings, fmt.Sprintf("the worktree %s did not locate, so no window opened for it: %v", worktree.Worktree, err))
			continue
		}
		dir := location.AnchorPath()
		if s.openTerminal != nil {
			err := s.openTerminal(dir, boardengine.ConsoleWindowTitle(prime.HubPath, prime.WorktreeName, name), s.lymPath)
			if err == nil {
				continue
			}
			hub.Warnings = append(hub.Warnings, fmt.Sprintf("the terminal window of %s did not open: %v", worktree.Worktree, err))
		}
		sh := shell.ForGOOS()
		worktree.Attach = sh.Chain(sh.ChangeDir(dir), sh.Invoke(s.lymPath)+" reed attach")
	}
}

// keepLauncher adds the hub's desktop launcher while this machine owns anything in the hub.
// It removes the launcher once this machine owns nothing there.
// A machine with no desktop keeps none, and a launcher that does not write is a warning.
func (s startSession) keepLauncher(hub *startHub, owners boardengine.Owners, here machinename.Identity) {
	if s.desktop == nil {
		return
	}
	if dobbyengine.OwnsAnything(owners, here) {
		if err := s.desktop.Add(s.lymPath, hub.Shortname); err != nil {
			hub.Warnings = append(hub.Warnings, fmt.Sprintf("the hub's desktop launcher was not added: %v", err))
			return
		}
		hub.Launcher = launcherAdded
		return
	}
	if err := s.desktop.Remove(hub.Shortname); err != nil {
		hub.Warnings = append(hub.Warnings, fmt.Sprintf("the hub's desktop launcher was not removed: %v", err))
		return
	}
	hub.Launcher = launcherRemoved
}

// holdOf returns the standing adopt hold of owners as the verb prints it, or nil when none stands.
// It carries the re-run when here wrote the hold.
func holdOf(owners boardengine.Owners, here machinename.Identity) *startHold {
	if len(owners.Holds) == 0 {
		return nil
	}
	hold := owners.Holds[0]
	view := &startHold{Machine: hold.Machine, Subjects: make([]string, len(hold.Subjects))}
	for i, held := range hold.Subjects {
		view.Subjects[i] = held.Subject
	}
	if owners.Registry.ReadsAsHere(hold.Machine, here) {
		view.Rerun = "lym adopt " + strings.Join(view.Subjects, " ")
	}
	return view
}

// removeGoneLaunchers removes each hub launcher on desktop whose hub live does not list, and returns their shortnames.
func removeGoneLaunchers(desktop desktoplauncher.Desktop, live []hubregistry.Entry) ([]string, error) {
	shortnames, err := desktop.Shortnames()
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, shortname := range shortnames {
		if slices.ContainsFunc(live, func(entry hubregistry.Entry) bool { return entry.Shortname == shortname }) {
			continue
		}
		if err := desktop.Remove(shortname); err != nil {
			return removed, err
		}
		removed = append(removed, shortname)
	}
	return removed, nil
}

// runBeamStart runs `<lymPath> <args>` in dir as a child and waits for it.
// It returns the worktree entries of the envelope the child prints.
// A child that does not run, or whose envelope carries no result, is an error.
// The error carries the envelope's error or the child's error output.
func runBeamStart(lymPath, dir string, args []string) ([]beamStartEntry, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(lymPath, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	logger.Info("machinecli: spawning lym beam start", "exe", lymPath, "args", args, "dir", dir)
	runErr := cmd.Run()
	logger.Info("machinecli: lym beam start ended", "args", args, "dir", dir, "error", runErr)

	entries, err := parseBeamStartOutput(stdout.Bytes(), stderr.Bytes(), runErr)
	if err != nil {
		return nil, fmt.Errorf("lym %s in %s: %w", strings.Join(args, " "), dir, err)
	}
	return entries, nil
}

// parseBeamStartOutput reads the worktree entries from the output of a `lym beam start` child that ended with runErr.
// An envelope that carries a result gives its entries, whether or not the child failed with it.
// Any other output is an error.
// Its reason is the envelope's error, else the child's error output, else runErr, else that the child printed no result.
func parseBeamStartOutput(stdout, stderr []byte, runErr error) ([]beamStartEntry, error) {
	var envelope struct {
		Error  string `json:"error"`
		Result *struct {
			Beams []beamStartEntry `json:"beams"`
		} `json:"result"`
	}
	decodeErr := json.Unmarshal(lastJSONLine(stdout), &envelope)
	if decodeErr == nil && envelope.Result != nil {
		return envelope.Result.Beams, nil
	}
	reason := envelope.Error
	if reason == "" {
		reason = strings.TrimSpace(string(stderr))
	}
	if reason == "" && runErr != nil {
		reason = runErr.Error()
	}
	if reason == "" {
		reason = fmt.Sprintf("it printed no result: %v", decodeErr)
	}
	return nil, errors.New(reason)
}

// lastJSONLine returns the last line of output that opens a JSON object.
// That line is the envelope a lym verb prints last; nil when none does.
func lastJSONLine(output []byte) []byte {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		if line := bytes.TrimSpace(lines[i]); bytes.HasPrefix(line, []byte("{")) {
			return line
		}
	}
	return nil
}
