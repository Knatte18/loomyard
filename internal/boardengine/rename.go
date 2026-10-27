// rename.go — changing an entry's slug in one write: the entry keeps every other field, dependents follow, and prose mentions are reported.
//
// Every refusal happens before the save, so a refused rename writes nothing.

package boardengine

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// RenameResult is the outcome of a rename: Task is the renamed entry and From its old slug.
// Mentions lists, in store order, the entries whose body or brief still name the old slug after the write, the renamed entry included, and is empty but not nil when none does.
type RenameResult struct {
	Task     Task
	From     string
	Mentions []string
}

// Rename changes the slug of the entry selector names, a slug string, a numeric id or a Handle, to to under the write lock, and replaces the old slug with to in every other entry's depends_on, project and order.
// The entry keeps its id, handle, position and every other field, and no entry's dates are revised by the rename.
// It refuses an absent entry, an entry a run holds, and a target that is empty, longer than MaxSlugLength or already an entry's slug.
// Body and brief text is never rewritten; the result names the entries that still mention the old slug.
func (b *Board) Rename(selector any, to string) (RenameResult, error) {
	result, err := b.boardCriticalSection(func(store *Store) (any, error) {
		return store.Rename(selector, to)
	}, nil)
	if err != nil {
		return RenameResult{}, err
	}
	return result.(RenameResult), nil
}

// Rename changes the slug of the entry selector names to to and rewrites every other entry's depends_on, project and order to match.
// It refuses an absent entry, an entry a run holds, a beam, an entry whose pair exists, and a target that is empty, longer than MaxSlugLength or already an entry's slug.
// The run-held refusal is a *RunLockedError.
func (s *Store) Rename(selector any, to string) (RenameResult, error) {
	current, found := s.GetTask(selector)
	if !found {
		return RenameResult{}, fmt.Errorf("task not found: %v: check the slug, id or handle with lym board list", selector)
	}
	if IsRunStatus(current.Status) {
		return RenameResult{}, &RunLockedError{Slug: current.Slug, Status: *current.Status}
	}
	if current.Kind == KindProject && current.Beam {
		return RenameResult{}, fmt.Errorf("entry %q is a beam, and a beam's slug stays fixed while it lives: change its title with `lym board upsert` instead", current.Slug)
	}
	if s.pairExists != nil && s.pairExists(current.Slug) {
		return RenameResult{}, fmt.Errorf("entry %q has a pair, and its slug names that pair: end the pair first with `lym batten cancel %s --remove`, then rename", current.Slug, current.Slug)
	}
	if to == "" {
		return RenameResult{}, fmt.Errorf("rename target must be a non-empty slug: pass the new slug in \"to\"")
	}
	if err := validateSlugLength(to); err != nil {
		return RenameResult{}, fmt.Errorf("%w: pick a shorter target slug", err)
	}
	if _, taken := s.GetTask(to); taken {
		return RenameResult{}, fmt.Errorf("slug %q already names an entry: pick another target slug, or fold the two entries with \"lym board merge\"", to)
	}

	renameSlug(s.tasks, current.Slug, to)
	task, _ := s.GetTask(to)
	mentions := []string{}
	for _, t := range s.tasks {
		if mentionsSlug(t.Body, current.Slug) || mentionsSlug(t.Brief, current.Slug) {
			mentions = append(mentions, t.Slug)
		}
	}
	return RenameResult{Task: task, From: current.Slug, Mentions: mentions}, nil
}

// renameSlug replaces from with to in tasks, as an entry's slug, as an element of every depends_on and order, and as every project, in place.
func renameSlug(tasks []Task, from, to string) {
	for i := range tasks {
		if tasks[i].Slug == from {
			tasks[i].Slug = to
		}
		if tasks[i].Project == from {
			tasks[i].Project = to
		}
		for j, dep := range tasks[i].DependsOn {
			if dep == from {
				tasks[i].DependsOn[j] = to
			}
		}
		for j, member := range tasks[i].Order {
			if member == from {
				tasks[i].Order[j] = to
			}
		}
	}
}

// mentionsSlug reports whether text holds slug as a whole slug: an occurrence with no letter, digit, '-' or '_' directly on either side.
func mentionsSlug(text, slug string) bool {
	for offset := 0; ; {
		index := strings.Index(text[offset:], slug)
		if index < 0 {
			return false
		}
		start := offset + index
		end := start + len(slug)
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		after, _ := utf8.DecodeRuneInString(text[end:])
		if !isSlugRune(before) && !isSlugRune(after) {
			return true
		}
		offset = start + 1
	}
}

// isSlugRune reports whether r can continue a slug: a letter, a digit, '-' or '_'.
func isSlugRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
}
