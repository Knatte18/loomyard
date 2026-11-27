// forget.go drops a former name from this machine's file and registry entry, and lists what still records it.

package machinecli

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/state"
)

// outcomeRegistered is the outcome of a hub whose registry took the forget.
const outcomeRegistered = "registered"

// What a reference to a dropped name is recorded in.
const (
	referenceOwner    = "owner"
	referenceTakeover = "takeover"
	referenceCancel   = "cancel"
)

// formerReference is one record on a hub that still names a dropped former name.
type formerReference struct {
	Hub string `json:"hub"`
	// Kind is referenceOwner, referenceTakeover or referenceCancel.
	Kind string `json:"kind"`
	// Entry is what records the name: `root` for the prime's owner, a beam's slug, or a run's slug.
	Entry string `json:"entry"`
	// Subject is what `lym adopt` takes to move the entry.
	Subject    string `json:"subject"`
	WayForward string `json:"way_forward"`
}

// runClaims are the machines a run's batten record names in its takeover entry and its unfinished cancel intent.
// An empty field names none.
type runClaims struct {
	takeover, cancel string
}

// forgetResult is what a forget reports under `machine`.
type forgetResult struct {
	// Name is the dropped former name.
	Name string `json:"name"`
	File string `json:"file"`
	// Names is the written file's names, and Forgotten its forgotten names, the dropped name last.
	Names     []string `json:"names"`
	Forgotten []string `json:"forgotten"`
	// Written reports whether the machine file was written.
	Written bool         `json:"written"`
	Hubs    []hubOutcome `json:"hubs"`
	// References lists every record on a swept hub that still names the dropped name.
	References []formerReference `json:"references"`
	// ReferenceErrors lists each hub whose references could not be read.
	ReferenceErrors []string `json:"reference_errors,omitempty"`
}

// forgetRerun returns the way forward of a hub whose registration did not land.
func forgetRerun(name string) string {
	return fmt.Sprintf("re-run `lym machine name --forget-former %s`", name)
}

// forgetFormer drops name from the machine file's names into its forgotten names, keeping the id.
// It then registers this machine on each swept hub with name forgotten, so the name leaves its registry entry.
// It lists every reference to name it can see on those hubs.
// It refuses a name the file does not hold as a former name, naming the file's former names;
// a name the file already forgot passes, so the re-run of a registration that failed finishes it.
// A hub whose registration fails is reported with the re-run and the others go on.
func forgetFormer(d deps, cwd, name string) (forgetResult, error) {
	cur := readCurrent(d)
	switch {
	case cur.locateErr != nil:
		return forgetResult{}, cur.locateErr
	case !cur.hasDirectory:
		return forgetResult{}, errors.New(noDirectoryRefusal())
	case cur.readErr != nil:
		return forgetResult{}, fmt.Errorf("the machine file %s does not read: %w; way forward: remove the file or fix its permissions, then %s", cur.file, cur.readErr, forgetRerun(name))
	case cur.decodeErr != nil:
		return forgetResult{}, fmt.Errorf("the machine file %s does not decode: %w; way forward: run `lym machine name <name>`, which rewrites the file, or remove the file", cur.file, cur.decodeErr)
	case cur.nameErr != nil:
		return forgetResult{}, fmt.Errorf("the machine file %s holds no valid name: %w; way forward: run `lym machine name <name>`, which rewrites the file, or remove the file", cur.file, cur.nameErr)
	}
	if !slices.Contains(cur.former, name) && !slices.Contains(cur.decoded.Forgotten, name) {
		return forgetResult{}, fmt.Errorf("%q is not a former name of this machine; the machine file %s holds the former names %v; way forward: name one of them", name, cur.file, cur.former)
	}
	id := cur.decoded.ID
	if id == "" {
		var err error
		if id, err = d.newID(); err != nil {
			return forgetResult{}, err
		}
	}
	hubs, err := hubsToSweep(d, cwd)
	if err != nil {
		return forgetResult{}, err
	}

	file := cur.decoded
	file.Names = slices.DeleteFunc(slices.Clone(cur.decoded.Names), func(entry string) bool { return entry == name })
	file.Forgotten = slices.Clone(cur.decoded.Forgotten)
	if !slices.Contains(file.Forgotten, name) {
		file.Forgotten = append(file.Forgotten, name)
	}
	file.ID = id
	result := forgetResult{Name: name, File: cur.file, Names: file.Names, Forgotten: file.Forgotten, Hubs: []hubOutcome{}, References: []formerReference{}}
	if needsWrite(cur, file) {
		if err := state.WriteJSON(cur.file, cur.lock, file); err != nil {
			return forgetResult{}, fmt.Errorf("write the machine file %s: %w; way forward: fix the file's directory, then %s", cur.file, err, forgetRerun(name))
		}
		result.Written = true
	}

	identity := identityOf(file)
	for _, hub := range hubs {
		result.Hubs = append(result.Hubs, forgetOnHub(d, hub, identity, name))
		references, err := d.references(hub, name)
		if err != nil {
			result.ReferenceErrors = append(result.ReferenceErrors, fmt.Sprintf("hub %s: %v", hub, err))
			continue
		}
		result.References = append(result.References, references...)
	}
	return result, nil
}

// forgetOnHub registers identity on the hub with name forgotten.
// A failed registration is the hub's failure with the re-run as its way forward.
func forgetOnHub(d deps, hub string, identity machinename.Identity, name string) hubOutcome {
	registered, err := d.register(fabriclayout.BoardDir(hub), boardengine.Registration{Identity: identity, Forget: []string{name}})
	if err != nil {
		return hubOutcome{Hub: hub, Outcome: outcomeFailed, Error: fmt.Sprintf("register this machine: %v", err), WayForward: forgetRerun(name)}
	}
	return hubOutcome{Hub: hub, Outcome: outcomeRegistered, Registered: registered}
}

// runForget runs a forget of name and prints its result under `machine`.
// A hub whose registration failed makes the exit non-zero, with the same fields.
func runForget(out io.Writer, d deps, cwd, name string) int {
	result, err := forgetFormer(d, cwd, name)
	if err != nil {
		return output.Err(out, err.Error())
	}
	fields := map[string]any{"machine": result}
	failed := 0
	for _, hub := range result.Hubs {
		if hub.Outcome == outcomeFailed {
			failed++
		}
	}
	if failed > 0 {
		return output.ErrFields(out, fmt.Sprintf("the registration did not land on %d hub(s), listed under machine.hubs; way forward: %s", failed, forgetRerun(name)), fields)
	}
	return output.Ok(out, fields)
}

// hubReferences lists the records of the hub that still name name.
// They are the owners its local board records under name, the prime as `root` and a beam by slug.
// They also include each takeover entry and unfinished cancel intent that names it in a batten record.
// Only a run whose pair stands here has one that is read.
func hubReferences(hub, name string) ([]formerReference, error) {
	owners, err := boardengine.ReadOwners(fabriclayout.BoardDir(hub))
	if err != nil {
		return nil, fmt.Errorf("read the board's owners: %w", err)
	}
	prime, err := primeOfHub(hub)
	if err != nil {
		return nil, err
	}
	entries, err := fabricengine.List(prime.AnchorPath())
	if err != nil {
		return nil, fmt.Errorf("list the pairs: %w", err)
	}
	runs := map[string]runClaims{}
	for _, entry := range entries {
		if entry.Main || entry.Prunable {
			continue
		}
		slug := filepath.Base(filepath.Clean(filepath.FromSlash(entry.Path)))
		status, found, err := pairteardown.ReadBattenRecord(prime, slug)
		if err != nil {
			return nil, fmt.Errorf("read the batten record of %s: %w", slug, err)
		}
		if !found {
			continue
		}
		var claims runClaims
		if status.Takeover != nil {
			claims.takeover = status.Takeover.Machine
		}
		if status.Cancel != nil && status.Cancel.Finish == nil {
			claims.cancel = status.Cancel.Machine
		}
		runs[slug] = claims
	}
	parentOf := func(slug string) (string, error) { return hubgeom.ParentOf(prime, slug) }
	return collectReferences(hub, name, owners, parentOf, runs)
}

// primeOfHub returns the location of the prime worktree of the hub at hub.
// It asks the first worktree of the hub that resolves which worktree is the main one.
func primeOfHub(hub string) (*lymcwd.Location, error) {
	children, err := os.ReadDir(hub)
	if err != nil {
		return nil, fmt.Errorf("read the hub %s: %w", hub, err)
	}
	for _, child := range children {
		if !child.IsDir() || child.Name() == fabriclayout.BoltDirName {
			continue
		}
		location, err := lymcwd.ResolveWorktree(filepath.Join(hub, child.Name()))
		if err != nil {
			continue
		}
		primeName, err := fabricengine.PrimeName(location)
		if err != nil {
			continue
		}
		return lymcwd.NamedWorktree(hub, primeName)
	}
	return nil, fmt.Errorf("no worktree of the hub %s resolves, so its prime is unknown", hub)
}

// collectReferences returns the references to name in owners and in the claims of the runs, by slug.
// Each carries the subject `lym adopt` takes to move its entry.
// That is the beam's slug, `root` for the prime, and for a run the beam or root that parentOf names above it.
func collectReferences(hub, name string, owners boardengine.Owners, parentOf func(slug string) (string, error), runs map[string]runClaims) ([]formerReference, error) {
	var references []formerReference
	add := func(kind, entry, subject string) {
		references = append(references, formerReference{
			Hub: hub, Kind: kind, Entry: entry, Subject: subject,
			WayForward: fmt.Sprintf("once the registration has landed, run `lym adopt %s` to take it back", subject),
		})
	}
	if owners.Prime == name {
		add(referenceOwner, boardengine.OwnerWritePrime, boardengine.RootSubject)
	}
	for _, slug := range slices.Sorted(maps.Keys(owners.Entries)) {
		if owners.Entries[slug].Owner == name {
			add(referenceOwner, slug, slug)
		}
	}
	for _, slug := range slices.Sorted(maps.Keys(runs)) {
		claims := runs[slug]
		if claims.takeover != name && claims.cancel != name {
			continue
		}
		subject, err := owners.AdoptSubject(slug, parentOf)
		if err != nil {
			return nil, fmt.Errorf("resolve what adopt takes for %s: %w", slug, err)
		}
		if claims.takeover == name {
			add(referenceTakeover, slug, subject)
		}
		if claims.cancel == name {
			add(referenceCancel, slug, subject)
		}
	}
	return references, nil
}
