// beamdescribe.go implements NewBeamDescribe, the beam recipe's Describe row: a Go producer that writes a beam's change description from its board entry and the commits it adds, with no agent.

package landingshed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// beamDescriptionTitleMax is the longest title the description carries, in characters, which is the limit summaryparser.ValidateDescription enforces.
const beamDescriptionTitleMax = 72

// BeamDescribeDeps carries the told values NewBeamDescribe needs.
// Every field is required.
type BeamDescribeDeps struct {
	// Slug is the run's slug, which names the landing in a refusal's way forward.
	Slug string
	// DescriptionPath is the absolute path of the change description the producer writes.
	DescriptionPath string
	// Entry reads the title and brief of the board entry of the run's slug, on every call.
	Entry func() (title, brief string, err error)
	// CommitSubjects lists the subjects of the commits the beam adds over its merge base with the parent, oldest first, merge commits left out.
	CommitSubjects func() ([]string, error)
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
// The description is the entry's title cut to 72 characters as a heading, its brief and one bullet per commit subject.
// It is validated like the Describe row's, and a finding is an error naming the finding and the way to end the landing and fix the entry.
// An empty brief with no commit subjects leaves the body empty, which that validation refuses.
// A re-run rewrites the same bytes.
func NewBeamDescribe(name string, deps BeamDescribeDeps) (shedengine.ShedProducer, error) {
	required := []struct {
		field string
		set   bool
	}{
		{"Slug", deps.Slug != ""},
		{"DescriptionPath", deps.DescriptionPath != ""},
		{"Entry", deps.Entry != nil},
		{"CommitSubjects", deps.CommitSubjects != nil},
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
// It reads the entry and the commit subjects, writes the description, validates it and commits it.
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
	subjects, err := p.deps.CommitSubjects()
	if err != nil {
		return fail("read the beam's commit subjects", err)
	}
	if err := os.MkdirAll(filepath.Dir(p.deps.DescriptionPath), 0o755); err != nil {
		return fail("create the description directory", err)
	}
	if err := os.WriteFile(p.deps.DescriptionPath, []byte(beamDescription(title, brief, subjects)), 0o644); err != nil {
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

// beamDescription renders the description file: the title cut to beamDescriptionTitleMax characters as a heading, then the brief and one bullet per subject, each part set off by a blank line.
func beamDescription(title, brief string, subjects []string) string {
	title = strings.TrimSpace(title)
	if runes := []rune(title); len(runes) > beamDescriptionTitleMax {
		title = strings.TrimSpace(string(runes[:beamDescriptionTitleMax]))
	}
	var sections []string
	if brief = strings.TrimSpace(brief); brief != "" {
		sections = append(sections, brief)
	}
	if len(subjects) > 0 {
		bullets := make([]string, len(subjects))
		for i, subject := range subjects {
			bullets[i] = "- " + subject
		}
		sections = append(sections, strings.Join(bullets, "\n"))
	}
	description := "# " + title
	if len(sections) > 0 {
		description += "\n\n" + strings.Join(sections, "\n\n")
	}
	return description + "\n"
}
