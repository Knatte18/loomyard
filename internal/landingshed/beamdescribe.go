// beamdescribe.go implements NewBeamDescribe, the beam recipe's Describe row: a Go producer that writes a beam's change description from its board entry, its project record and the commits it adds, with no agent.

package landingshed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

const (
	// beamDescriptionTitleMax is the longest title the description carries, in characters, which is the limit summaryparser.ValidateDescription enforces.
	beamDescriptionTitleMax = 72
	// beamChangesHeading heads the list of the commits the beam adds.
	beamChangesHeading = "## Changes"
	// beamCommitSHALength is how many characters of a commit's object name an entry shows, enough for GitHub to link it.
	beamCommitSHALength = 12
	// beamCommitSummaryMax is the longest summary an entry carries, in characters, before it is cut.
	beamCommitSummaryMax = 200
	// beamCommitTimeLayout formats the time an entry shows, in UTC.
	beamCommitTimeLayout = "2006-01-02 15:04 UTC"
)

// pullRequestSubject matches the subject of a commit a pull request landed, `<title> (#<number>)`, capturing the title and the number.
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

// BeamDescribeDeps carries the told values NewBeamDescribe needs.
// Every field is required.
type BeamDescribeDeps struct {
	// Slug is the run's slug, which names the landing in a refusal's way forward.
	Slug string
	// DescriptionPath is the absolute path of the change description the producer writes.
	DescriptionPath string
	// ProjectDecisions reads the entries added to the beam's project record after it was written, each heading and body verbatim, in record order: the project's recorded scope changes.
	ProjectDecisions func() ([]string, error)
	// Entry reads the title and brief of the board entry of the run's slug, on every call.
	Entry func() (title, brief string, err error)
	// Commits lists the commits on the beam branch's first-parent line that its parent branch lacks, oldest first: each landing into the beam, each merge from the parent and each commit made on the beam directly.
	Commits func() ([]BeamCommit, error)
	// Commit commits the description's directory; a repeat commit over unchanged content is a no-op.
	Commit func() error
}

// beamDescribeProducer writes a beam's change description.
type beamDescribeProducer struct {
	name string
	deps BeamDescribeDeps
}

var _ shedengine.ShedProducer = (*beamDescribeProducer)(nil)

// NewBeamDescribe returns the ShedProducer that writes and commits the change description of a beam.
// The description is the entry's title cut to 72 characters as a heading, its brief, a `## Changes` list of the beam's commits oldest first and the project record's added entries verbatim under a `## Project decisions` heading.
// A commit whose subject ends in ` (#<number>)`, as a pull request's merge writes it, is listed by its pull request's number, title and merge time; any other commit, a merge from the parent included, by its object name, subject and commit time.
// Either entry carries the first sentence of the commit's body as its summary, when the body has one.
// GitHub links the number and the object name in a pull request's body.
// The description is validated like the Describe row's, and a finding is an error naming the finding and the way to end the landing and fix the entry.
// An empty brief with no commits leaves the body empty, which that validation refuses.
// A re-run rewrites the same bytes.
func NewBeamDescribe(name string, deps BeamDescribeDeps) (shedengine.ShedProducer, error) {
	required := []struct {
		field string
		set   bool
	}{
		{"Slug", deps.Slug != ""},
		{"DescriptionPath", deps.DescriptionPath != ""},
		{"ProjectDecisions", deps.ProjectDecisions != nil},
		{"Entry", deps.Entry != nil},
		{"Commits", deps.Commits != nil},
		{"Commit", deps.Commit != nil},
	}
	for _, r := range required {
		if !r.set {
			return nil, fmt.Errorf("landingshed: NewBeamDescribe: %s is empty", r.field)
		}
	}
	return &beamDescribeProducer{name: name, deps: deps}, nil
}

// Call implements shedengine.ShedProducer.
// It reads the entry, the record's added entries and the commits, writes the description, validates it and commits it.
// A read, write or commit failure and a finding are errors; only a cancelled context is reported as the cancellation.
func (p *beamDescribeProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	fail := func(action string, err error) (shedengine.Outcome, shedengine.OutputPointer, error) {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, fmt.Errorf("landingshed: %s: %s: %w", p.name, action, err)
	}

	title, brief, err := p.deps.Entry()
	if err != nil {
		return fail("read the board entry", err)
	}
	decisions, err := p.deps.ProjectDecisions()
	if err != nil {
		return fail("read the project record", err)
	}
	commits, err := p.deps.Commits()
	if err != nil {
		return fail("read the beam's commits", err)
	}
	if err := os.MkdirAll(filepath.Dir(p.deps.DescriptionPath), 0o755); err != nil {
		return fail("create the description directory", err)
	}
	if err := os.WriteFile(p.deps.DescriptionPath, []byte(beamDescription(title, brief, decisions, commits)), 0o644); err != nil {
		return fail("write the description", err)
	}
	findings, err := summaryparser.ValidateDescription(p.deps.DescriptionPath)
	if err != nil {
		return fail("validate the description", err)
	}
	if len(findings) > 0 {
		parts := make([]string, len(findings))
		for i, finding := range findings {
			parts[i] = finding.Error()
		}
		// The landing's claim holds the entry under the run lock, so the entry is fixed only after the landing is ended.
		return fail("validate the description", fmt.Errorf("%s; way forward: run \"lym batten cancel %s\" to end the landing, fix the title or brief with \"lym board upsert\", then run \"lym beam land %s\" again", strings.Join(parts, "; "), p.deps.Slug, p.deps.Slug))
	}
	if err := p.deps.Commit(); err != nil {
		return fail("commit the description", err)
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// beamDescription renders the description file: the title cut to beamDescriptionTitleMax characters as a heading, then the brief, the list of commits and the project decisions, each part set off by a blank line.
func beamDescription(title, brief string, decisions []string, commits []BeamCommit) string {
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
	if len(decisions) > 0 {
		sections = append(sections, "## Project decisions\n\n"+strings.Join(decisions, "\n\n"))
	}
	description := "# " + title
	if len(sections) > 0 {
		description += "\n\n" + strings.Join(sections, "\n\n")
	}
	return description + "\n"
}

// beamCommitEntry renders one commit as a list item: `#<number> <title>, merged <time>` for a pull request's merge, `<object name> <subject>, committed <time>` otherwise, and the body's summary indented on the next line when there is one.
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

// commitSummary returns the first sentence of the first paragraph of body that is not a heading, on one line, cut to beamCommitSummaryMax characters with an ellipsis; "" when body has no such paragraph.
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
