// beamdescribe.go implements the two halves of a beam's change description, which the beam's orch writes.
// DraftBeamDescription offers a starting point from the board entry and the beam's member pull requests;
// RecordBeamDescription validates the orch's text and commits it.

package landingshed

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/summaryparser"
)

const (
	// beamDescriptionTitleMax is the longest title the draft carries, in characters.
	// It is the limit that the description's validation enforces.
	beamDescriptionTitleMax = 72
	// beamChangesHeading heads the draft's list of the commits the beam adds.
	beamChangesHeading = "## Changes"
	// beamCommitSHALength is how many characters of a commit's object name an entry shows, enough for GitHub to link it.
	beamCommitSHALength = 12
	// beamCommitSummaryMax is the longest summary an entry carries, in characters, before it is cut.
	beamCommitSummaryMax = 200
	// beamCommitTimeLayout formats the time an entry shows, in UTC.
	beamCommitTimeLayout = "2006-01-02 15:04 UTC"
)

// pullRequestSubject matches the subject of a commit a pull request landed, `<title> (#<number>)`.
// It captures the title and the number.
var pullRequestSubject = regexp.MustCompile(`^(.*\S) \(#(\d+)\)$`)

// BeamCommit is one commit on the beam branch's own line of history.
type BeamCommit struct {
	// SHA is the commit's full hex object name.
	SHA string
	// Message is the commit's full message: its subject line, then its body.
	Message string
	// Committed is the commit's committer time, which for a pull request GitHub merged is when it merged.
	Committed time.Time
}

// BeamDraftDeps carries the values a beam's draft description is written from, supplied by `lym beam describe --draft`.
// Every field is required.
type BeamDraftDeps struct {
	// Slug names the beam in the way forward of a refusal.
	Slug string
	// DescriptionPath is the absolute path where the draft is written; no file may stand there yet.
	DescriptionPath string
	// Entry reads the title and brief of the beam's board entry.
	Entry func() (title, brief string, err error)
	// Commits lists the commits on the beam branch's first-parent line that its parent branch lacks, oldest first:
	// each landing into the beam, each merge from the parent and each commit made on the beam directly.
	Commits func() ([]BeamCommit, error)
}

// requireBeamDraftDeps returns an error naming the first field of deps that is unset.
func requireBeamDraftDeps(deps BeamDraftDeps) error {
	required := []struct {
		field string
		set   bool
	}{
		{"Slug", deps.Slug != ""},
		{"DescriptionPath", deps.DescriptionPath != ""},
		{"Entry", deps.Entry != nil},
		{"Commits", deps.Commits != nil},
	}
	for _, r := range required {
		if !r.set {
			return fmt.Errorf("landingshed: DraftBeamDescription: %s is empty", r.field)
		}
	}
	return nil
}

// DraftBeamDescription writes a starting point for a beam's change description at deps.DescriptionPath.
// It refuses a description that already stands there,
// so the orch's text and a recorded description are never overwritten.
// The refusal names the file and the way forward: edit it and record it, or delete it to get a fresh draft.
// The draft is the entry's title cut to the title length limit as a heading, then its brief.
// The beam's commits follow oldest first under `## Changes`, so the orch can list every member pull request.
// A pull request's merge writes its subject ending in ` (#<number>)`.
// Such a commit is listed by that pull request's number, title and merge time.
// Any other commit, a merge from the parent included, is listed by its object name, subject and commit time.
// Either entry carries the first sentence of the commit's body as its summary, when the body has one.
// GitHub links the number and the object name in a pull request's body.
// The draft is neither validated nor committed: the orch edits it, and RecordBeamDescription records it.
// It refuses deps with an unset field, naming that field.
// A read or write failure is an error naming the step.
func DraftBeamDescription(deps BeamDraftDeps) error {
	if err := requireBeamDraftDeps(deps); err != nil {
		return err
	}
	fail := func(step string, err error) error {
		return fmt.Errorf("landingshed: draft the beam description: %s: %w", step, err)
	}

	if _, err := os.Stat(deps.DescriptionPath); err == nil {
		return fail("check for an existing description", fmt.Errorf("a description already stands at %s; way forward: edit it and run \"lym beam describe %s\", or delete it to get a fresh draft", deps.DescriptionPath, deps.Slug))
	} else if !os.IsNotExist(err) {
		return fail("check for an existing description", err)
	}
	title, brief, err := deps.Entry()
	if err != nil {
		return fail("read the board entry", err)
	}
	commits, err := deps.Commits()
	if err != nil {
		return fail("read the beam's commits", err)
	}
	if err := os.MkdirAll(filepath.Dir(deps.DescriptionPath), 0o755); err != nil {
		return fail("create the description directory", err)
	}
	if err := os.WriteFile(deps.DescriptionPath, []byte(beamDraft(title, brief, commits)), 0o644); err != nil {
		return fail("write the draft", err)
	}
	return nil
}

// RecordBeamDescription validates the change description at path, which the beam's orch wrote.
// It then commits it through commit.
// slug names the beam in the way forward of a refusal.
// It refuses a missing file, naming the draft verb that offers a starting point.
// It refuses a description with a finding, naming each finding and the file to fix;
// a refused description is not committed.
// A validation or commit failure is an error naming the step.
func RecordBeamDescription(slug, path string, commit func() error) error {
	fail := func(step string, err error) error {
		return fmt.Errorf("landingshed: record the beam description: %s: %w", step, err)
	}
	findings, err := summaryparser.ValidateDescription(path)
	if err != nil {
		return fail("validate the description", err)
	}
	if len(findings) == 1 && findings[0].Check == summaryparser.CheckMissing {
		return fail("read the description", fmt.Errorf("no description is written at %s; way forward: write it there, or run \"lym beam describe %s --draft\" for a starting point and edit that, then run \"lym beam describe %s\" again", path, slug, slug))
	}
	if len(findings) > 0 {
		parts := make([]string, len(findings))
		for i, finding := range findings {
			parts[i] = finding.Error()
		}
		return fail("validate the description", fmt.Errorf("%s; way forward: fix the description at %s, then run \"lym beam describe %s\" again", strings.Join(parts, "; "), path, slug))
	}
	if err := commit(); err != nil {
		return fail("commit the description", err)
	}
	return nil
}

// beamDraft renders the draft: the title cut to beamDescriptionTitleMax characters as a heading.
// Then come the brief and the list of commits, each set off by a blank line.
func beamDraft(title, brief string, commits []BeamCommit) string {
	title = strings.TrimSpace(title)
	if runes := []rune(title); len(runes) > beamDescriptionTitleMax {
		title = strings.TrimSpace(string(runes[:beamDescriptionTitleMax]))
	}
	var sections []string
	if brief = strings.TrimSpace(brief); brief != "" {
		sections = append(sections, brief)
	}
	if len(commits) > 0 {
		entries := make([]string, len(commits))
		for i, commit := range commits {
			entries[i] = beamCommitEntry(commit)
		}
		sections = append(sections, beamChangesHeading+"\n\n"+strings.Join(entries, "\n"))
	}
	description := "# " + title
	if len(sections) > 0 {
		description += "\n\n" + strings.Join(sections, "\n\n")
	}
	return description + "\n"
}

// beamCommitEntry renders one commit as a list item.
// A pull request's merge renders as `#<number> <title>, merged <time>`.
// Any other commit renders as `<object name> <subject>, committed <time>`.
// The body's summary follows on the next line, indented, when there is one.
func beamCommitEntry(commit BeamCommit) string {
	subject, body, _ := strings.Cut(strings.TrimSpace(commit.Message), "\n")
	subject = strings.TrimSpace(subject)
	when := commit.Committed.UTC().Format(beamCommitTimeLayout)
	var entry string
	if match := pullRequestSubject.FindStringSubmatch(subject); match != nil {
		entry = fmt.Sprintf("- #%s %s, merged %s", match[2], match[1], when)
	} else {
		entry = fmt.Sprintf("- %s %s, committed %s", commit.SHA[:min(len(commit.SHA), beamCommitSHALength)], subject, when)
	}
	if summary := commitSummary(body); summary != "" {
		entry += "\n  " + summary
	}
	return entry
}

// commitSummary returns the first sentence of the body's first paragraph that is not a heading, on one line.
// It cuts the sentence to beamCommitSummaryMax characters with an ellipsis.
// It returns "" when body has no such paragraph.
func commitSummary(body string) string {
	for _, paragraph := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n") {
		text := strings.Join(strings.Fields(paragraph), " ")
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if end := strings.Index(text, ". "); end >= 0 {
			text = text[:end+1]
		}
		if runes := []rune(text); len(runes) > beamCommitSummaryMax {
			text = strings.TrimSpace(string(runes[:beamCommitSummaryMax])) + "…"
		}
		return text
	}
	return ""
}
