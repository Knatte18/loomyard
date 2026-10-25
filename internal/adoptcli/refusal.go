// refusal.go holds the refusals that run before adopt's first mutation: a standing anchor change, an entry adopted without the parent that drives it, and a beam or the prime adopted without the members another machine owns.

package adoptcli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// primeSubject names the prime among the subjects adoptRefusal reasons over; no slug is empty.
const primeSubject = ""

// refuseStandingMove refuses an adopt while an anchor change stands in a worktree it would act under: the prime, each named entry's own worktree when this machine holds its pair, and each named entry's parent worktree.
// The refusal carries the way forward the standing record names, then the re-run, so no pair is added, no run resumed and no gate suite started under a moving anchor.
// A read error refuses naming it, and nil MoveInProgress reads as no record standing.
func refuseStandingMove(deps adoptDeps, slugs []string) error {
	if deps.MoveInProgress == nil {
		return nil
	}
	subjects := []string{primeSubject}
	for _, slug := range slugs {
		if deps.LocalPair(slug) {
			subjects = append(subjects, slug)
		}
		parent, err := deps.ParentOf(slug)
		if err != nil {
			return fmt.Errorf("lyx adopt: read the parent of %s, so nothing ran: %w; %s", slug, err, reentry)
		}
		subjects = append(subjects, parent)
	}
	checked := map[string]bool{}
	for _, subject := range subjects {
		if checked[subject] {
			continue
		}
		checked[subject] = true
		wayForward, standing, err := deps.MoveInProgress(subject)
		if err != nil {
			return fmt.Errorf("lyx adopt: read whether an anchor change stands in %q, so nothing ran: %w; %s", subject, err, reentry)
		}
		if standing {
			return fmt.Errorf("lyx adopt: an anchor change stands, so nothing ran: %s; %s", wayForward, reentry)
		}
	}
	return nil
}

// wouldStop answers, from the read-only part of step 1 alone, whether takeOverSlug would stop slug there: its pair holds local commits origin lacks or is live, or --drop-local meets a live pair.
// An entry no other machine owns, one with no pair here and any entry under --keep-local pass step 1.
func wouldStop(deps adoptDeps, req request, report fabricengine.OriginReport, slug string) (bool, error) {
	if !ownedElsewhere(report, slug) || !deps.LocalPair(slug) || req.KeepLocal {
		return false, nil
	}
	live, err := deps.LivePair(slug)
	if err != nil {
		return false, err
	}
	if req.DropLocal {
		return live, nil
	}
	return live || len(stoppedBranches(report, slug)) > 0, nil
}

// adoptNeeds answers what adopting a subject, a slug or the prime, drags in.
type adoptNeeds struct {
	deps    adoptDeps
	here    string
	owners  boardengine.Owners
	parents map[string]string
}

// needs is what one subject needs adopted with it: its parent when another machine owns that, and for a beam or the prime its members another machine owns.
type needs struct {
	// parent is the parent subject, and hasParent whether it is needed.
	parent    string
	hasParent bool
	// members are the slugs of the not-done members recorded as owned by another machine.
	members []string
}

// subjects lists the subjects the needs name: the parent first, then the members.
func (n needs) subjects() []string {
	var all []string
	if n.hasParent {
		all = append(all, n.parent)
	}
	return append(all, n.members...)
}

// parentOf returns the slug of the pair slug was forked from, "" for the prime, once per slug.
func (a *adoptNeeds) parentOf(slug string) (string, error) {
	if parent, known := a.parents[slug]; known {
		return parent, nil
	}
	parent, err := a.deps.ParentOf(slug)
	if err != nil {
		return "", fmt.Errorf("lyx adopt: read the parent of %s, so nothing ran: %w; %s", slug, err, reentry)
	}
	a.parents[slug] = parent
	return parent, nil
}

// of returns what adopting subject needs.
func (a *adoptNeeds) of(subject string) (needs, error) {
	var found needs
	if subject != primeSubject {
		parent, err := a.parentOf(subject)
		if err != nil {
			return needs{}, err
		}
		owner := a.owners.Prime
		if parent != primeSubject {
			if owner, err = a.owners.Effective(parent, a.deps.ParentOf); err != nil {
				return needs{}, fmt.Errorf("lyx adopt: read the owner of %s, so nothing ran: %w; %s", parent, err, reentry)
			}
		}
		if boardengine.OwnedElsewhere(owner, a.here) {
			found.parent, found.hasParent = parent, true
		}
	}
	if entry := a.owners.Entries[subject]; subject == primeSubject || (entry.IsProject && entry.Beam) {
		for slug, member := range a.owners.Entries {
			isMember := member.IsTask || (member.IsProject && member.Beam)
			if member.Done || !isMember || !boardengine.OwnedElsewhere(member.Owner, a.here) {
				continue
			}
			parent, err := a.parentOf(slug)
			if err != nil {
				return needs{}, err
			}
			if parent == subject {
				found.members = append(found.members, slug)
			}
		}
		slices.Sort(found.members)
	}
	return found, nil
}

// closure returns subject followed by every subject it needs, followed to a fixed point, in discovery order.
func (a *adoptNeeds) closure(subject string) ([]string, error) {
	reached := []string{subject}
	for i := 0; i < len(reached); i++ {
		found, err := a.of(reached[i])
		if err != nil {
			return nil, err
		}
		for _, next := range found.subjects() {
			if !slices.Contains(reached, next) {
				reached = append(reached, next)
			}
		}
	}
	return reached, nil
}

// adoptRefusal refuses an adopt that would leave no dobby here to drive what it takes over, before any step writes.
// It refuses an entry it records an owner for, a task or a beam, whose parent reads owned by another machine, unless the same call adopts the parent, and an adopted beam, the prime included, while a member that is not done records another machine as its owner, unless the call adopts the member too.
// going lists the entries step 1 does not stop and prime says whether the prime is adopted, so a parent or member step 1 stops does not count as adopted.
// Each refusal names one call that meets both rules, with --keep-local when an entry it reaches would itself stop at step 1.
func adoptRefusal(deps adoptDeps, req request, report fabricengine.OriginReport, here string, owners boardengine.Owners, going []string, prime bool) error {
	graph := &adoptNeeds{deps: deps, here: here, owners: owners, parents: map[string]string{}}
	adopted := slices.Clone(going)
	subjects := slices.Clone(going)
	if prime {
		adopted = append(adopted, primeSubject)
		subjects = append(subjects, primeSubject)
	}

	var refusals []error
	for _, subject := range subjects {
		found, err := graph.of(subject)
		if err != nil {
			return err
		}
		var missing needs
		missing.parent, missing.hasParent = found.parent, found.hasParent && !slices.Contains(adopted, found.parent)
		for _, member := range found.members {
			if !slices.Contains(adopted, member) {
				missing.members = append(missing.members, member)
			}
		}
		if !missing.hasParent && len(missing.members) == 0 {
			continue
		}

		reached, err := graph.closure(subject)
		if err != nil {
			return err
		}
		keepLocal := req.KeepLocal
		for _, slug := range reached[1:] {
			if slug == primeSubject {
				continue
			}
			stops, err := wouldStop(deps, req, report, slug)
			if err != nil {
				return fmt.Errorf("lyx adopt: read whether the pair of %s is live, so nothing ran: %w; %s", slug, err, reentry)
			}
			keepLocal = keepLocal || stops
		}
		refusals = append(refusals, fmt.Errorf("lyx adopt: %s needs %s adopted with it, since this machine's dobby drives only runs whose parent is its own worktree; way forward: run `%s`",
			subjectLabel(subject), missing.describe(), adoptCall(reached, req, keepLocal)))
	}
	return errors.Join(refusals...)
}

// describe words the parent and the members a refusal asks to adopt.
func (n needs) describe() string {
	var parts []string
	if n.hasParent {
		if n.parent == primeSubject {
			parts = append(parts, "the prime")
		} else {
			parts = append(parts, "its parent `"+n.parent+"`")
		}
	}
	if len(n.members) > 0 {
		parts = append(parts, "its members `"+strings.Join(n.members, "`, `")+"`")
	}
	return strings.Join(parts, " and ")
}

// subjectLabel names a subject in a refusal.
func subjectLabel(subject string) string {
	if subject == primeSubject {
		return "`--prime`"
	}
	return "`" + subject + "`"
}

// adoptCall renders the call that adopts reached, the first subject last: what it needs come first, so a parent precedes its child.
// The prime is named by --prime, and --keep-local or the request's own --drop-local answers local commits.
func adoptCall(reached []string, req request, keepLocal bool) string {
	words := []string{"lyx adopt"}
	if slices.Contains(reached, primeSubject) {
		words = append(words, "--prime")
	}
	switch {
	case keepLocal:
		words = append(words, "--keep-local")
	case req.DropLocal:
		words = append(words, "--drop-local")
	}
	for _, slug := range reached[1:] {
		if slug != primeSubject {
			words = append(words, slug)
		}
	}
	if reached[0] != primeSubject {
		words = append(words, reached[0])
	}
	return strings.Join(words, " ")
}
