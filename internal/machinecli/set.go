// set.go sets this machine's name in the machine file.
// It then registers the machine on every swept hub and sweeps there the owners recorded under the former names.

package machinecli

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"runtime"
	"slices"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
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
	"A pause left standing stays: run `lym tender resume <slug>` for it.",
	"A board commit left ahead of origin is pushed by the re-run: run `lym machine name <name>` again with the same name.",
	"A registration that failed on a hub is made by the re-run: run `lym machine name <name>` again with the same name.",
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
	UnpushedNote string `json:"unpushed_read_note,omitempty"`
	// Registered reports whether the registration wrote this machine's entry into the hub's registry.
	Registered bool        `json:"registered,omitempty"`
	Outcome    string      `json:"outcome"`
	Slugs      []string    `json:"slugs,omitempty"`
	Lost       []lostEntry `json:"lost,omitempty"`
	Error      string      `json:"error,omitempty"`
	WayForward string      `json:"way_forward,omitempty"`
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

// nextFile returns the machine file a set of name writes with the machine id id, and how many empty entries it dropped.
// The names start from a decoded file's names.
// They gain the old name and the hostname rule's name when absent and not forgotten.
// They lose every empty entry and every refused name, which another registered machine holds, and end with name.
// The decoded file's forgotten names are kept, so a name a forget dropped never comes back.
func nextFile(cur current, name, id string, refused []string) (machinename.File, int) {
	var names, forgotten []string
	if cur.exists && cur.decodeErr == nil {
		names = slices.Clone(cur.decoded.Names)
		forgotten = slices.Clone(cur.decoded.Forgotten)
	}
	dropped := 0
	names = slices.DeleteFunc(names, func(entry string) bool {
		if entry == "" {
			dropped++
		}
		return entry == ""
	})
	for _, old := range []string{cur.name, cur.hostname} {
		if old != "" && !slices.Contains(names, old) && !slices.Contains(forgotten, old) {
			names = append(names, old)
		}
	}
	names = slices.DeleteFunc(names, func(entry string) bool { return entry == name || slices.Contains(refused, entry) })
	return machinename.File{Name: name, Names: append(names, name), ID: id, Forgotten: forgotten}, dropped
}

// needsWrite reports whether file differs from what the machine file holds.
func needsWrite(cur current, file machinename.File) bool {
	return !cur.exists || cur.decodeErr != nil || cur.nameErr != nil ||
		file.Name != cur.decoded.Name || !slices.Equal(file.Names, cur.decoded.Names) ||
		file.ID != cur.decoded.ID || !slices.Equal(file.Forgotten, cur.decoded.Forgotten)
}

// identityOf returns the identity the machine file file yields.
// Its names other than the current one are the former names.
func identityOf(file machinename.File) machinename.Identity {
	former := slices.DeleteFunc(slices.Clone(file.Names), func(entry string) bool { return entry == file.Name })
	return machinename.Identity{ID: file.ID, Name: file.Name, Former: former, Forgotten: file.Forgotten}
}

// hubCheck is what checkHub reads of one hub: its outcome so far, whether its board is ahead of origin and its registry.
type hubCheck struct {
	outcome  hubOutcome
	ahead    bool
	registry boardengine.Registry
}

// checkHub reads the state of the hub's board that the sweep judges, and writes nothing.
// It returns the hub's outcome so far, whether its board is ahead of origin and the board's machine registry.
// It also returns a refusal when the board does not read or records the new name under another machine.
func checkHub(d deps, hub, name string, own []string, file string) (hubCheck, error) {
	boardDir := fabriclayout.BoardDir(hub)
	check := hubCheck{outcome: hubOutcome{Hub: hub}}
	ahead, err := d.unpushed(boardDir)
	if err != nil {
		check.outcome.UnpushedNote = fmt.Sprintf("read whether the board is ahead of origin: %v", err)
		ahead = false
	}
	skip, detail, err := d.pull(boardDir)
	switch {
	case err != nil:
		check.outcome.CheckedLocally = err.Error()
	case skip != "":
		check.outcome.CheckedLocally = fmt.Sprintf("pull skipped (%s): %s", skip, detail)
	}
	owners, err := d.readOwners(boardDir)
	if err != nil {
		return check, fmt.Errorf("the board of hub %s does not read: %w; way forward: repair that hub's board as the error says, then %s", hub, err, rerun(name))
	}
	check.registry = owners.Registry
	if slices.Contains(own, name) {
		check.ahead = ahead
		return check, nil
	}
	if owners.Prime == name {
		return check, collision(hub, boardengine.OwnerWritePrime, name, file)
	}
	for _, slug := range slices.Sorted(maps.Keys(owners.Entries)) {
		if owners.Entries[slug].Owner == name {
			return check, collision(hub, slug, name, file)
		}
	}
	check.ahead = ahead
	return check, nil
}

// collision returns the refusal of a name another machine owns.
func collision(hub, subject, name, file string) error {
	return fmt.Errorf(
		"hub %s records %s as owned by %q, so that name belongs to another machine; "+
			"way forward: choose another name; for a lost former name, write {\"name\": \"%s\"} to %s by hand, "+
			"and take back with `lym adopt` an owner left under another lost name",
		hub, subject, name, name, file)
}

// machineID returns the id the machine file is written with.
// The file's id stands, unless a hub's registry holds it under a current name that is not one of the file's names.
// The file's names are its current name, its names and its forgotten names.
// A registry name outside them marks the file a copy, and it draws a fresh id.
// A file with no id draws one too.
func machineID(cur current, checks []hubCheck, newID func() (string, error)) (string, error) {
	var id string
	if cur.exists && cur.decodeErr == nil {
		id = cur.decoded.ID
	}
	if id == "" {
		return newID()
	}
	for _, check := range checks {
		entry, registered := check.registry[id]
		if registered && entry.Name != cur.name && !slices.Contains(cur.decoded.Names, entry.Name) && !slices.Contains(cur.decoded.Forgotten, entry.Name) {
			return newID()
		}
	}
	return id, nil
}

// refusedNames returns the machine's current name and hostname rule's name.
// It keeps those that a hub's registry reads as another machine's.
// That machine holds the name as its current or a former name, so the name is not this machine's with id.
func refusedNames(cur current, checks []hubCheck, id string) []string {
	var refused []string
	for _, candidate := range []string{cur.name, cur.hostname} {
		for _, check := range checks {
			if holder, _, found := check.registry.Resolve(candidate); found && holder != id && !slices.Contains(refused, candidate) {
				refused = append(refused, candidate)
			}
		}
	}
	return refused
}

// sweepHub takes over, on the hub's board, the owners recorded under replace for name.
// It reports `nothing to sweep` when nothing was recorded and nothing was kept by another owner;
// the caller refines that to `pushed`.
func sweepHub(d deps, hub, name string, replace []string) hubOutcome {
	outcome := hubOutcome{Hub: hub, Outcome: outcomeNothing}
	written, err := d.setOwners(fabriclayout.BoardDir(hub), boardengine.OwnerRequest{Machine: name, Replace: replace})
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

// registerAndSweep registers identity on the hub, then sweeps the owners recorded under replace for its name.
// The sweep's request carries the identity, so it writes the machine's current name.
// A failed registration is the hub's failure with the re-run as its way forward, and the hub is not swept.
func registerAndSweep(d deps, hub string, identity machinename.Identity, replace []string) hubOutcome {
	registered, err := d.register(fabriclayout.BoardDir(hub), boardengine.Registration{Identity: identity})
	if err != nil {
		return hubOutcome{Hub: hub, Outcome: outcomeFailed, Error: fmt.Sprintf("register this machine: %v", err), WayForward: rerun(identity.Name)}
	}
	sweeping := d
	sweeping.setOwners = func(boardDir string, req boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
		req.Identity = identity
		return d.setOwners(boardDir, req)
	}
	outcome := sweepHub(sweeping, hub, identity.Name, replace)
	outcome.Registered = registered
	return outcome
}

// setName writes name as this machine's name and registers the machine on each hub.
// It then sweeps the owners recorded under its former names.
// It refuses, writing nothing, on an invalid name or an unresolvable or unreadable machine file.
// It also refuses on an unreadable registry or board.
// It refuses too on a name another machine owns, and on a registration the registry refuses.
// The file keeps its machine id.
// A file whose id a hub's registry holds under another machine's name gets a fresh one.
// A name a hub's registry reads as another machine's is kept out of the written names.
// A hub whose registration or sweep fails is reported in the result and the other hubs go on.
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
	checks := make([]hubCheck, len(hubs))
	for i, hub := range hubs {
		if checks[i], err = checkHub(d, hub, name, own, cur.file); err != nil {
			return setResult{}, err
		}
	}

	id, err := machineID(cur, checks, d.newID)
	if err != nil {
		return setResult{}, err
	}
	file, dropped := nextFile(cur, name, id, refusedNames(cur, checks, id))
	identity := identityOf(file)
	for i, hub := range hubs {
		if err := checks[i].registry.Check(boardengine.Registration{Identity: identity}); err != nil {
			return setResult{}, fmt.Errorf("hub %s: %w", hub, err)
		}
	}

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
		outcome := registerAndSweep(d, hub, identity, replace)
		outcome.CheckedLocally, outcome.UnpushedNote = checks[i].outcome.CheckedLocally, checks[i].outcome.UnpushedNote
		if outcome.Outcome == outcomeNothing && checks[i].ahead {
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
