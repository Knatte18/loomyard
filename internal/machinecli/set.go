// set.go sets this machine's name in the machine file.
// It then sweeps, on every swept hub, the owners recorded under the former names.

package machinecli

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"runtime"
	"slices"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/state"
)

// How a hub's sweep ended.
const (
	outcomeSwept   = "swept"
	outcomePushed  = "pushed"
	outcomeNothing = "nothing to sweep"
	outcomeKept    = "kept by another owner"
	outcomeFailed  = "failed"
)

// lostWayForwardSuffix is added to the re-run as the way forward of an entry another owner keeps.
const lostWayForwardSuffix = "; `lym adopt` takes back an entry whose standing owner is another name"

// recoveries lists how each state the sweep can leave behind is recovered, as constant text.
var recoveries = []string{
	"A run paused for an owner other than this machine resumes once the sweep records this machine as owner: the dobby's probe lifts that owner pause.",
	"A pause the probe does not lift stays: run `lym tender resume <slug>` for it.",
	"A board commit left ahead of origin is pushed by the re-run: run `lym machine name <name>` again with the same name.",
	"A Finalize that halted at its owner check goes on with `lym loom resume` in its pair.",
	"A worktree the owner change skipped or refused starts with `lym beam start <slug>`, or `lym beam start root` for the prime.",
	"Lift a pause with `lym tender resume` only after that hub's sweep reports done.",
}

// lostEntry is an entry the sweep left with the owner that stands.
type lostEntry struct {
	Slug       string `json:"slug"`
	Owner      string `json:"owner"`
	WayForward string `json:"way_forward"`
}

// hubOutcome is how the sweep ended on one hub.
type hubOutcome struct {
	Hub string `json:"hub"`
	// CheckedLocally carries why the hub's board was checked against its local Bolt alone.
	CheckedLocally string `json:"checked_locally,omitempty"`
	// UnpushedNote carries why the read of whether the board is ahead of origin failed.
	UnpushedNote string      `json:"unpushed_read_note,omitempty"`
	Outcome      string      `json:"outcome"`
	Slugs        []string    `json:"slugs,omitempty"`
	Lost         []lostEntry `json:"lost,omitempty"`
	Error        string      `json:"error,omitempty"`
	WayForward   string      `json:"way_forward,omitempty"`
}

// setResult is what a set reports under `machine`.
type setResult struct {
	Name string `json:"name"`
	File string `json:"file"`
	// Names is the written file's names, the new name last.
	Names []string `json:"names"`
	// Written reports whether the machine file was written.
	Written bool `json:"written"`
	// DroppedEmptyEntries counts the empty entries the rewrite dropped from the file's names.
	DroppedEmptyEntries int `json:"dropped_empty_entries,omitempty"`
	// InvalidNameDropped reports that the file's name was invalid and the rewrite replaced it.
	InvalidNameDropped bool `json:"invalid_name_dropped,omitempty"`
	// NamesLost reports that the file did not decode, so the rewrite lost the names it held.
	NamesLost  bool         `json:"names_lost,omitempty"`
	Hubs       []hubOutcome `json:"hubs"`
	Recoveries []string     `json:"recoveries"`
}

// rerun returns the way forward of a hub whose sweep did not finish.
func rerun(name string) string {
	return fmt.Sprintf("re-run `lym machine name %s`", name)
}

// noDirectoryRefusal names the variables the machine file's directory resolves from on this platform.
func noDirectoryRefusal() string {
	variables := "XDG_CONFIG_HOME or HOME"
	if runtime.GOOS == "windows" {
		variables = "LOCALAPPDATA"
	}
	return fmt.Sprintf("no directory for the machine file resolves from the environment; way forward: set %s to an absolute path", variables)
}

// ownNames returns the names that are this machine's own.
// They are the valid current name and the non-empty names of a decoded file.
// The hostname rule's name joins them when it yields one.
func ownNames(cur current) []string {
	var names []string
	add := func(name string) {
		if name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	add(cur.name)
	if cur.exists && cur.decodeErr == nil {
		for _, name := range cur.decoded.Names {
			add(name)
		}
	}
	add(cur.hostname)
	return names
}

// nextFile returns the machine file a set of name writes, and how many empty entries it dropped.
// The names start from a decoded file's names.
// They gain the old name and the hostname rule's name when absent.
// They lose every empty entry and end with name.
func nextFile(cur current, name string) (machinename.File, int) {
	var names []string
	if cur.exists && cur.decodeErr == nil {
		names = slices.Clone(cur.decoded.Names)
	}
	dropped := 0
	names = slices.DeleteFunc(names, func(entry string) bool {
		if entry == "" {
			dropped++
		}
		return entry == ""
	})
	for _, old := range []string{cur.name, cur.hostname} {
		if old != "" && !slices.Contains(names, old) {
			names = append(names, old)
		}
	}
	names = slices.DeleteFunc(names, func(entry string) bool { return entry == name })
	return machinename.File{Name: name, Names: append(names, name)}, dropped
}

// needsWrite reports whether file differs from what the machine file holds.
func needsWrite(cur current, file machinename.File) bool {
	return !cur.exists || cur.decodeErr != nil || cur.nameErr != nil ||
		file.Name != cur.decoded.Name || !slices.Equal(file.Names, cur.decoded.Names)
}

// checkHub reads the state of the hub's board that the sweep judges, and writes nothing.
// It returns the hub's outcome so far and whether its board is ahead of origin.
// It also returns a refusal when the board does not read or records the new name under another machine.
func checkHub(d deps, hub, name string, own []string, file string) (hubOutcome, bool, error) {
	boardDir := fabricengine.BoardDir(hub)
	check := hubOutcome{Hub: hub}
	ahead, err := d.unpushed(boardDir)
	if err != nil {
		check.UnpushedNote = fmt.Sprintf("read whether the board is ahead of origin: %v", err)
		ahead = false
	}
	skip, detail, err := d.pull(boardDir)
	switch {
	case err != nil:
		check.CheckedLocally = err.Error()
	case skip != "":
		check.CheckedLocally = fmt.Sprintf("pull skipped (%s): %s", skip, detail)
	}
	owners, err := d.readOwners(boardDir)
	if err != nil {
		return check, false, fmt.Errorf("the board of hub %s does not read: %w; way forward: repair that hub's board as the error says, then %s", hub, err, rerun(name))
	}
	if slices.Contains(own, name) {
		return check, ahead, nil
	}
	if owners.Prime == name {
		return check, false, collision(hub, boardengine.OwnerWritePrime, name, file)
	}
	for _, slug := range slices.Sorted(maps.Keys(owners.Entries)) {
		if owners.Entries[slug].Owner == name {
			return check, false, collision(hub, slug, name, file)
		}
	}
	return check, ahead, nil
}

// collision returns the refusal of a name another machine owns.
func collision(hub, subject, name, file string) error {
	return fmt.Errorf(
		"hub %s records %s as owned by %q, so that name belongs to another machine; "+
			"way forward: choose another name; for a lost former name, write {\"name\": \"%s\"} to %s by hand, "+
			"and take back with `lym adopt` an owner left under another lost name",
		hub, subject, name, name, file)
}

// sweepHub takes over, on the hub's board, the owners recorded under replace for name.
// It reports `nothing to sweep` when nothing was recorded and nothing was kept by another owner;
// the caller refines that to `pushed`.
func sweepHub(d deps, hub, name string, replace []string) hubOutcome {
	outcome := hubOutcome{Hub: hub, Outcome: outcomeNothing}
	written, err := d.setOwners(fabricengine.BoardDir(hub), boardengine.OwnerRequest{Machine: name, Replace: replace})
	if err != nil {
		outcome.Outcome = outcomeFailed
		outcome.Error = err.Error()
		outcome.WayForward = rerun(name)
		return outcome
	}
	for _, slug := range slices.Sorted(maps.Keys(written.Lost)) {
		outcome.Lost = append(outcome.Lost, lostEntry{Slug: slug, Owner: written.Lost[slug], WayForward: rerun(name) + lostWayForwardSuffix})
	}
	switch {
	case len(written.Recorded) > 0:
		outcome.Outcome = outcomeSwept
		outcome.Slugs = written.Recorded
	case len(outcome.Lost) > 0:
		outcome.Outcome = outcomeKept
	}
	return outcome
}

// setName writes name as this machine's name and sweeps the owners recorded under its former names.
// It refuses, writing nothing, on an invalid name or an unresolvable or unreadable machine file.
// It also refuses on an unreadable registry or board and on a name another machine owns.
// A hub whose sweep fails is reported in the result and the other hubs go on.
func setName(d deps, cwd, name string) (setResult, error) {
	if err := machinename.CheckName(name); err != nil {
		return setResult{}, err
	}
	cur := readCurrent(d)
	switch {
	case cur.locateErr != nil:
		return setResult{}, cur.locateErr
	case !cur.hasDirectory:
		return setResult{}, errors.New(noDirectoryRefusal())
	case cur.readErr != nil:
		return setResult{}, fmt.Errorf("the machine file %s does not read: %w; way forward: remove the file or fix its permissions, then %s", cur.file, cur.readErr, rerun(name))
	}
	own := ownNames(cur)
	hubs, err := hubsToSweep(d, cwd)
	if err != nil {
		return setResult{}, err
	}
	checks := make([]hubOutcome, len(hubs))
	ahead := make([]bool, len(hubs))
	for i, hub := range hubs {
		if checks[i], ahead[i], err = checkHub(d, hub, name, own, cur.file); err != nil {
			return setResult{}, err
		}
	}

	file, dropped := nextFile(cur, name)
	result := setResult{
		Name: name, File: cur.file, Names: file.Names, DroppedEmptyEntries: dropped,
		InvalidNameDropped: cur.nameErr != nil, NamesLost: cur.decodeErr != nil,
		Hubs: []hubOutcome{}, Recoveries: recoveries,
	}
	if needsWrite(cur, file) {
		if err := state.WriteJSON(cur.file, cur.lock, file); err != nil {
			return setResult{}, fmt.Errorf("write the machine file %s: %w; way forward: fix the file's directory, then %s", cur.file, err, rerun(name))
		}
		result.Written = true
	}

	replace := slices.DeleteFunc(slices.Clone(file.Names), func(entry string) bool { return entry == name })
	for i, hub := range hubs {
		outcome := sweepHub(d, hub, name, replace)
		outcome.CheckedLocally, outcome.UnpushedNote = checks[i].CheckedLocally, checks[i].UnpushedNote
		if outcome.Outcome == outcomeNothing && ahead[i] {
			outcome.Outcome = outcomePushed
		}
		result.Hubs = append(result.Hubs, outcome)
	}
	return result, nil
}

// runSet runs a set of name and prints its result under `machine`.
// A hub that failed or kept an owner makes the exit non-zero, with the same fields.
func runSet(out io.Writer, d deps, cwd, name string) int {
	result, err := setName(d, cwd, name)
	if err != nil {
		return output.Err(out, err.Error())
	}
	fields := map[string]any{"machine": result}
	unfinished := 0
	for _, hub := range result.Hubs {
		if hub.Outcome == outcomeFailed || len(hub.Lost) > 0 {
			unfinished++
		}
	}
	if unfinished > 0 {
		return output.ErrFields(out, fmt.Sprintf("the owner sweep did not finish on %d hub(s), listed under machine.hubs; way forward: %s", unfinished, rerun(name)), fields)
	}
	return output.Ok(out, fields)
}
