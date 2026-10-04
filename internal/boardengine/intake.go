// intake.go — the board side of GitHub intake: importing an inbox issue as a note or folding it into an entry.
//
// boardengine imports nothing GitHub-specific;
// the caller converts a fetched issue into InboxIssue and makes every network call outside the board lock.
// Every refusal happens before the save, so a refused import writes nothing.

package boardengine

import (
	"fmt"
	"slices"
	"strings"
)

// InboxIssue is the part of a GitHub issue that intake reads.
type InboxIssue struct {
	Number      int
	Title       string
	Body        string
	URL         string
	Labels      []string
	Open        bool
	PullRequest bool
}

// ImportRequest asks to record Issue on the board, as a new note named Slug or as a fold into the entry named Into.
// Title, Brief and Labels apply to a new note only; a nil Labels means "not given".
type ImportRequest struct {
	Issue  InboxIssue
	Slug   string
	Title  string
	Brief  string
	Labels []string
	Into   string
}

// ImportResult reports what an import did.
// Recorded is non-empty when the issue was already recorded and the import was a no-op, and then holds the recording entries' slugs.
// Dropped holds the issue labels that were not carried over.
type ImportResult struct {
	Entry    Task
	Recorded []string
	Dropped  []string
}

// RecordingEntries returns the slugs of the entries whose issues list number, in board order.
func (s *Store) RecordingEntries(number int) []string {
	var slugs []string
	for _, t := range s.tasks {
		if slices.Contains(t.Issues, number) {
			slugs = append(slugs, t.Slug)
		}
	}
	return slugs
}

// RecordedIssues maps every recorded issue number to the slugs of its recording entries.
func (s *Store) RecordedIssues() map[int][]string {
	recorded := map[int][]string{}
	for _, t := range s.tasks {
		for _, n := range t.Issues {
			recorded[n] = append(recorded[n], t.Slug)
		}
	}
	return recorded
}

// RecordedIssues reads every recorded issue number and its recording entries' slugs, persisting nothing.
func (b *Board) RecordedIssues() (map[int][]string, error) {
	store, err := b.loadStore()
	if err != nil {
		return nil, err
	}
	return store.RecordedIssues(), nil
}

// ImportIssue records req.Issue on the board under the write lock.
// An issue that an entry already records is a no-op that returns the recording slugs.
func (b *Board) ImportIssue(req ImportRequest) (ImportResult, error) {
	result, err := b.boardCriticalSection(func(store *Store) (any, error) {
		res, err := store.importIssue(req)
		if err != nil {
			return nil, err
		}
		if len(res.Recorded) > 0 {
			return noWrite{result: res}, nil
		}
		return res, nil
	}, nil)
	if err != nil {
		return ImportResult{}, err
	}
	return result.(ImportResult), nil
}

// importIssue applies req to the store, checking the request in the order the refusals are documented.
func (s *Store) importIssue(req ImportRequest) (ImportResult, error) {
	issue := req.Issue
	if issue.PullRequest {
		return ImportResult{}, fmt.Errorf("#%d is a pull request, and only an issue can be imported: pick an issue number", issue.Number)
	}
	if recorded := s.RecordingEntries(issue.Number); len(recorded) > 0 {
		return ImportResult{Recorded: recorded}, nil
	}
	if !issue.Open {
		return ImportResult{}, fmt.Errorf("issue #%d is closed and no entry records it: reopen it on GitHub to import it, or leave it closed", issue.Number)
	}

	if req.Into != "" {
		if req.Title != "" || req.Brief != "" || req.Labels != nil {
			return ImportResult{}, fmt.Errorf("into folds issue #%d into an existing entry and takes no title, brief or labels: drop them, or import as a new note with slug", issue.Number)
		}
		if req.Slug != "" {
			return ImportResult{}, fmt.Errorf("slug and into both given for issue #%d: pass slug to import as a new note, or into to fold into an existing entry", issue.Number)
		}
		return s.foldIssue(issue, req.Into)
	}
	if req.Slug == "" {
		return ImportResult{}, fmt.Errorf("neither slug nor into given for issue #%d: pass slug to import as a new note, or into to fold into an existing entry", issue.Number)
	}
	return s.importNote(req)
}

// vocabulary returns the store's vocabulary, the zero value when it has none.
func (s *Store) vocabulary() Vocabulary {
	if s.vocab == nil {
		return Vocabulary{}
	}
	return *s.vocab
}

// issueContent is the link-back line followed by the issue body.
func issueContent(issue InboxIssue) string {
	content := fmt.Sprintf("Imported from [issue #%d](%s).", issue.Number, issue.URL)
	if issue.Body != "" {
		content += "\n\n" + issue.Body
	}
	return content
}

// importNote writes a new note recording the issue.
func (s *Store) importNote(req ImportRequest) (ImportResult, error) {
	issue := req.Issue
	if _, taken := s.slugIndex()[req.Slug]; taken {
		return ImportResult{}, fmt.Errorf("entry %q already exists: pick another slug, or fold the issue into it with into", req.Slug)
	}

	vocab := s.vocabulary()
	labels := req.Labels
	var dropped []string
	if labels == nil {
		labels = []string{}
		for _, label := range issue.Labels {
			switch {
			case !vocab.Known(label):
				dropped = append(dropped, label)
			case !slices.Contains(labels, label):
				labels = append(labels, label)
			}
		}
		var typeLabels []string
		for _, label := range labels {
			if vocab.IsType(label) {
				typeLabels = append(typeLabels, label)
			}
		}
		switch {
		case len(typeLabels) > 1:
			return ImportResult{}, fmt.Errorf("issue #%d carries more than one type label (%s), and a note takes exactly one: pass labels naming the one to use", issue.Number, strings.Join(typeLabels, ", "))
		case len(typeLabels) == 0:
			return ImportResult{}, fmt.Errorf("issue #%d carries no type label the board knows (%s): pass labels with one type label from the types list of board.yaml", issue.Number, strings.Join(vocab.Types, ", "))
		}
	}

	title := req.Title
	if title == "" {
		title = issue.Title
	}
	entry, err := s.UpsertTask(map[string]any{
		"slug":   req.Slug,
		"title":  title,
		"brief":  req.Brief,
		"body":   issueContent(issue),
		"kind":   KindNote,
		"labels": labels,
		"issues": []int{issue.Number},
	})
	if err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Entry: entry, Dropped: dropped}, nil
}

// foldIssue appends the issue to the entry named into: a section in its body, the issue number and the issue labels it can carry.
func (s *Store) foldIssue(issue InboxIssue, into string) (ImportResult, error) {
	target, found := s.GetTask(into)
	if !found {
		return ImportResult{}, fmt.Errorf("no entry %q to fold issue #%d into: pick an existing slug, or import as a new note with slug", into, issue.Number)
	}

	vocab := s.vocabulary()
	labels := slices.Clone(target.Labels)
	var dropped []string
	for _, label := range issue.Labels {
		switch {
		case slices.Contains(target.Labels, label), slices.Contains(labels, label):
		case !vocab.Known(label), target.Kind == KindNote && vocab.IsType(label):
			dropped = append(dropped, label)
		default:
			labels = append(labels, label)
		}
	}

	body := fmt.Sprintf("## From issue #%d\n\n%s", issue.Number, issueContent(issue))
	if target.Body != "" {
		body = target.Body + "\n\n" + body
	}
	entry, err := s.UpsertTask(map[string]any{
		"slug":   target.Slug,
		"body":   body,
		"labels": labels,
		"issues": append(slices.Clone(target.Issues), issue.Number),
	})
	if err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Entry: entry, Dropped: dropped}, nil
}
