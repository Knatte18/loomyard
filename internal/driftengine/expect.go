// expect.go implements the Drift-Expect producer, which seals the changes the expect agent expects a task to make.
// It also holds the `expectation` gate entry that the agent's table runs.

package driftengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// DriftExpect is the ShedProducer that runs the expect agent and seals the changes it expects.
type DriftExpect struct {
	name        string
	row         Row
	expectation shuttleengine.GateEntry
	deps        Deps
}

var _ shedengine.ShedProducer = (*DriftExpect)(nil)

// NewDriftExpect returns the Drift-Expect producer identified as name.
// expectation is the row's resolved `expectation` gate entry, which the agent's table runs at each turn end.
func NewDriftExpect(name string, row Row, expectation shuttleengine.GateEntry, deps Deps) *DriftExpect {
	return &DriftExpect{name: name, row: row, expectation: expectation, deps: deps}
}

// Call runs one Drift-Expect pass, which seals the changes the expect agent expects a task to make.
// It is Done at once, spawning no agent, when drift is switched off or no scope is recorded.
// It is also Done at once when the expectation is already sealed.
// When the scope is off or the baseline capture was deferred, the pass records the expectation Unsealed first.
// Containment then reads the empty expectation, and the pass is Done.
// The run's parent gets a Stuck when the capture never counted, or when the baseline is missing or fails its digest.
// It also gets one when the code HEAD is not the capture's, or when the records differ from their newest commit.
// Each such reason opens with shedadapters.DecisionHaltPrefix.
// The capture, baseline and HEAD reasons also carry DeferHaltPrefix.
// A session the agent already closed, with an expected.yaml that validates, is sealed without spawning the agent.
// Otherwise the pass runs the expect agent, which resumes a live chair when one exists.
// A Done seals expected.yaml, adding the baseline digest as a second trailer when no commit has recorded it yet.
// An exhausted `expectation` entry is the shed's Stuck, and its resume spawns the agent again.
func (p *DriftExpect) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	err := requireSeams(p.name,
		seam{"Config", d.Config != nil},
		seam{"Head", d.Head != nil},
		seam{"CommitsSince", d.CommitsSince != nil},
		seam{"CommitWithTrailers", d.CommitWithTrailers != nil},
		seam{"TrailerCommit", d.TrailerCommit != nil},
		seam{"FileAt", d.FileAt != nil},
		seam{"Seats", d.Seats != nil},
		seam{"ExpectTable", d.ExpectTable != nil},
	)
	if err == nil && p.expectation.Gate == nil {
		err = fmt.Errorf("drift: %s is wired without an expectation gate entry", p.name)
	}
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if outcome, pointer, stop := passWhenSwitchedOff(d.Config); stop {
		return outcome, pointer, nil
	}
	return p.pass(ctx)
}

// haltForParent returns a Stuck whose reason opens with shedadapters.DecisionHaltPrefix, so the stop chain hands it to the run's parent.
func haltForParent(reason string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return shedengine.Stuck, shedengine.OutputPointer{Reason: shedadapters.DecisionHaltPrefix + reason}, nil
}

// pass runs one Drift-Expect pass once drift is switched on, ending in a seal, a halt, or a run of the expect agent.
func (p *DriftExpect) pass(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	rec, err := ReadRecords(d)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if !rec.ScopeFound {
		return doneBecause("no scope is recorded, so there is no expectation to seal")
	}
	_, sealed, err := d.TrailerCommit(SealTrailerKey, false)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("drift: %s: find the oldest %s commit: %w", p.name, SealTrailerKey, err)
	}
	switch {
	case sealed:
		return doneBecause("the expectation is already sealed")
	case rec.Scope.Scope == ScopeOff:
		return p.doneUnsealed(rec, "the scope is off, so no expectation is sealed")
	case rec.Scope.Deferred:
		return p.doneUnsealed(rec, "the baseline capture was deferred, so no expectation is sealed")
	case rec.Scope.AwaitedAt == "":
		return haltForParent(DeferHaltPrefix + "the baseline capture never counted, so there is no baseline to expect changes against; way forward: the parent runs lym drift defer, then lym loom resume")
	}

	if rec.Mismatch {
		return haltForParent(recordsMismatchReason)
	}

	baseline, err := CheckBaseline(d)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if baseline.Fault != "" {
		return haltForParent(baseline.Fault)
	}
	if outcome, pointer, stopped, err := p.checkHead(rec.Scope); stopped || err != nil {
		return outcome, pointer, err
	}

	if rec.Scope.Diff == "" {
		cfg, err := d.Config()
		if err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("drift: %s: load the drift config: %w", p.name, err)
		}
		err = WriteRecords(d, func(scope *ScopeRecord, _ *JudgedRecord) error {
			scope.Diff, scope.KeyPatterns, scope.Volatile = cfg.Diff, cfg.KeyPatterns, cfg.Volatile
			return nil
		})
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		rec.Scope.Diff, rec.Scope.KeyPatterns, rec.Scope.Volatile = cfg.Diff, cfg.KeyPatterns, cfg.Volatile
	}

	if session := rec.Scope.ExpectSession; !session.Open && session.OpenedAt != "" {
		present, _, findings, err := readExpectationFile(d)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		if present && findings == "" {
			return p.seal(baseline.Unguarded)
		}
	}
	return p.runAgent(ctx, rec.Scope, baseline.Unguarded)
}

// doneUnsealed records Unsealed once and returns a Done carrying why.
func (p *DriftExpect) doneUnsealed(rec Records, why string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if !rec.Scope.Unsealed {
		if rec.Mismatch {
			return haltForParent(recordsMismatchReason)
		}
		err := WriteRecords(p.deps, func(scope *ScopeRecord, _ *JudgedRecord) error {
			scope.Unsealed = true
			return nil
		})
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	return doneBecause(why)
}

// checkHead returns a Stuck when the code HEAD differs from the head the baseline was captured at.
// That head is the scope record's BaselineHead, or the capture record's head when the scope record holds none.
// stopped reports whether it returned a Stuck; a run that knows no capture head is not checked.
func (p *DriftExpect) checkHead(scope ScopeRecord) (outcome shedengine.Outcome, pointer shedengine.OutputPointer, stopped bool, err error) {
	d := p.deps
	captureHead := scope.BaselineHead
	if captureHead == "" {
		capture, found, err := ReadCapture(d.Paths)
		if err != nil {
			return "", shedengine.OutputPointer{}, false, err
		}
		if found {
			captureHead = capture.Head
		}
	}
	if captureHead == "" {
		return "", shedengine.OutputPointer{}, false, nil
	}
	head, err := d.Head()
	if err != nil {
		return "", shedengine.OutputPointer{}, false, fmt.Errorf("drift: %s: read the code HEAD: %w", p.name, err)
	}
	if head == captureHead {
		return "", shedengine.OutputPointer{}, false, nil
	}
	commits, err := d.CommitsSince(captureHead)
	if err != nil {
		return "", shedengine.OutputPointer{}, false, fmt.Errorf("drift: %s: list the commits since the baseline: %w", p.name, err)
	}
	outcome, pointer, err = haltForParent(DeferHaltPrefix + fmt.Sprintf(
		"the code moved after the baseline was captured: HEAD is %s and the capture ran at %s, with these commits since: %s; way forward: the parent runs lym drift defer, then lym loom resume, or the operator resets the run's code branch to %s in the run's worktree, then lym loom resume",
		head, captureHead, strings.Join(commits, ", "), captureHead))
	return outcome, pointer, true, err
}

// now returns the told clock, or the system clock when none was told.
func (p *DriftExpect) now() time.Time {
	if p.deps.Now == nil {
		return time.Now()
	}
	return p.deps.Now()
}

// setSession records the expect agent's session mark through the records guard.
func (p *DriftExpect) setSession(mark SessionMark) error {
	return WriteRecords(p.deps, func(scope *ScopeRecord, _ *JudgedRecord) error {
		scope.ExpectSession = mark
		return nil
	})
}

// runAgent writes the listing, opens the session mark if needed, and runs the agent or resumes its live chair.
// It closes the mark when the context is still live, and seals the expectation when the agent is Done.
// The mark stays open when the context ended, which leaves a possibly live agent for the next entry to reattach to.
func (p *DriftExpect) runAgent(ctx context.Context, scope ScopeRecord, unguarded bool) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	patterns, err := CompileKeyPatterns(scope.KeyPatterns)
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("drift: %s: %w", p.name, err)
	}
	if err := writeListing(d.Paths.Listing, scope.Diff, d.Paths.BaselineDir, patterns, scope.Volatile); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	openedAt := scope.ExpectSession.OpenedAt
	if !scope.ExpectSession.Open {
		openedAt = stamp(p.now())
		if err := p.setSession(SessionMark{Open: true, OpenedAt: openedAt}); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}

	tables := func() (seatengine.Table, error) {
		table, err := d.ExpectTable(d.Paths.Listing)
		if err != nil {
			return seatengine.Table{}, err
		}
		table.Gate = shuttleengine.GateSpec{p.expectation}
		return table, nil
	}
	agent := shedadapters.NewMultiLLMProducerSourced(p.name, tables, d.Seats, d.Now, nil).KeepGateFindings(d.GateFindingsFile)
	outcome, pointer, err := agent.Call(ctx)
	if ctx.Err() == nil {
		closed := SessionMark{}
		if err == nil {
			closed = SessionMark{OpenedAt: openedAt}
		}
		err = errors.Join(err, p.setSession(closed))
	}
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if outcome != shedengine.Done {
		return outcome, pointer, nil
	}
	return p.seal(unguarded)
}

// seal records the metadata marks of a validating expected.yaml in the scope record, when it holds any.
// It then commits expected.yaml with its seal trailer.
// When unguarded is true, no commit has recorded the baseline digest yet, so it is added as a second trailer.
func (p *DriftExpect) seal(unguarded bool) (shedengine.Outcome, shedengine.OutputPointer, error) {
	d := p.deps
	_, expectation, _, err := readExpectationFile(d)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if marks := expectation.Metadata; len(marks.Files)+len(marks.Keys) > 0 {
		err := WriteRecords(d, func(scope *ScopeRecord, _ *JudgedRecord) error {
			scope.Metadata = marks
			return nil
		})
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	var extra []fabricengine.Trailer
	if unguarded {
		digest, err := BaselineDigest(d.Paths.BaselineDir)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		extra = append(extra, fabricengine.Trailer{Key: BaselineTrailerKey, Value: digest})
	}
	if err := SealExpectation(d, extra...); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return shedengine.Done, shedengine.OutputPointer{Path: d.Paths.Expected}, nil
}

// NewExpectationGate returns the gate of the `expectation` entry of Drift-Expect's row.
// It parses expected.yaml with ParseExpectation against the cases of the baseline listing.
// The diff mode and key patterns come from the scope record, or from the config when the record holds none.
// A parse failure fails the gate with the parser's message, which re-prompts the agent.
// A missing file fails the gate asking for it.
func NewExpectationGate(d Deps) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		present, _, findings, err := readExpectationFile(d)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("drift: the expectation gate: %w", err)
		}
		switch {
		case !present:
			return shuttleengine.GateResult{Findings: fmt.Sprintf("expected.yaml is missing; write the expectation to %s and end your turn again.", d.Paths.Expected)}, nil
		case findings != "":
			return shuttleengine.GateResult{Findings: findings + "\nCorrect expected.yaml and end your turn again."}, nil
		}
		return shuttleengine.GateResult{Passed: true}, nil
	}
}

// readExpectationFile reports whether expected.yaml exists and, when it does, parses it.
// findings is the parser's refusal of the file, empty when it validates.
func readExpectationFile(d Deps) (present bool, expectation Expectation, findings string, err error) {
	if err := requireSeams("the expectation check", seam{"Config", d.Config != nil}); err != nil {
		return false, Expectation{}, "", err
	}
	data, present, err := readRecordFile(d.Paths.Expected)
	if err != nil || !present {
		return false, Expectation{}, "", err
	}
	rec, err := ReadRecords(d)
	if err != nil {
		return true, Expectation{}, "", err
	}
	mode, patterns := rec.Scope.Diff, rec.Scope.KeyPatterns
	if mode == "" {
		cfg, err := d.Config()
		if err != nil {
			return true, Expectation{}, "", fmt.Errorf("load the drift config: %w", err)
		}
		mode, patterns = cfg.Diff, cfg.KeyPatterns
	}
	compiled, err := CompileKeyPatterns(patterns)
	if err != nil {
		return true, Expectation{}, "", err
	}
	var baseline []CaseListing
	if mode == DiffLines {
		if baseline, err = ListCaptures(d.Paths.BaselineDir, compiled); err != nil {
			return true, Expectation{}, "", err
		}
	}
	expectation, err = ParseExpectation(data, mode, baseline)
	if err != nil {
		return true, Expectation{}, err.Error(), nil
	}
	return true, expectation, "", nil
}

// listingLine is one line of a listed case; N is written only for a key that occurs more than once in its case.
type listingLine struct {
	Key  string `yaml:"key,omitempty"`
	N    int    `yaml:"n,omitempty"`
	Line int    `yaml:"line,omitempty"`
	Text string `yaml:"text"`
}

// listingCase is one baseline case; Lines is empty under diff command.
type listingCase struct {
	Case  string        `yaml:"case"`
	Lines []listingLine `yaml:"lines,omitempty"`
}

// listingFile is the identity listing the expect agent reads.
type listingFile struct {
	Mode  string        `yaml:"mode"`
	Cases []listingCase `yaml:"cases"`
}

// writeListing writes the listing of every case under baselineDir to path.
// Under diff lines it writes each line's identity and text, minus the lines volatile names, which the diff ignores.
// Under diff command it writes the case names alone.
func writeListing(path, mode, baselineDir string, patterns []*regexp.Regexp, volatile Volatile) error {
	listings, err := ListCaptures(baselineDir, patterns)
	if err != nil {
		return fmt.Errorf("drift: list the baseline for the expect agent: %w", err)
	}
	if mode == DiffLines {
		listings = volatile.withoutListed(listings)
	}
	file := listingFile{Mode: mode, Cases: make([]listingCase, 0, len(listings))}
	for _, listing := range listings {
		listed := listingCase{Case: listing.Case}
		if mode == DiffLines {
			occurrences := map[string]int{}
			for _, line := range listing.Lines {
				occurrences[line.Identity.Key]++
			}
			for _, line := range listing.Lines {
				entry := listingLine{Key: line.Identity.Key, Line: line.Identity.Line, Text: line.Text}
				if line.Identity.Key != "" && occurrences[line.Identity.Key] > 1 {
					entry.N = line.Identity.N
				}
				listed.Lines = append(listed.Lines, entry)
			}
		}
		file.Cases = append(file.Cases, listed)
	}
	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("drift: render the listing: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("drift: create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("drift: write the listing %s: %w", path, err)
	}
	return nil
}
