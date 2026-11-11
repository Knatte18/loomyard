// parentreview.go keeps a run's parent-review asks: the never-tracked record of each ask to the parent orch,
// and the parent's report for each answered one.
// Every state move is forward-only and runs under the record's one lock, so an answer and a timeout never both land.

package burlerengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
	"gopkg.in/yaml.v3"
)

// ParentAskState is where one parent-review ask stands.
type ParentAskState string

// The states of a parent-review ask.
// An ask moves only from pending-post to open, and from either of those to an end state.
const (
	// ParentAskPendingPost is an ask recorded before its mail was posted; the mail may have gone out before a crash.
	ParentAskPendingPost ParentAskState = "pending-post"
	// ParentAskOpen is an ask whose mail went out and that the parent has not answered.
	ParentAskOpen ParentAskState = "open"
	// ParentAskDone is an ask the parent answered; its report sits beside the record.
	ParentAskDone ParentAskState = "done"
	// ParentAskTimeout is an ask that reached its deadline, or whose round closed, with no answer.
	ParentAskTimeout ParentAskState = "timeout"
	// ParentAskNoParent is an ask that never went out because no parent orch resolved.
	ParentAskNoParent ParentAskState = "no-parent"
	// ParentAskNotSent is an ask that never went out because its deadline cap left no room; Reason says why.
	ParentAskNotSent ParentAskState = "not-sent"
)

// Ended reports whether s is an end state, from which an ask never moves.
// A state this binary does not know counts as ended, so it is never moved.
func (s ParentAskState) Ended() bool {
	return s != ParentAskPendingPost && s != ParentAskOpen
}

// ParentAsk is one ask to the parent orch to review a round.
type ParentAsk struct {
	// Number is the ask's number within the run, from 1 up in the order the asks were recorded.
	Number int `json:"number"`
	// Round is the review round the ask belongs to.
	Round int            `json:"round"`
	State ParentAskState `json:"state"`
	// Strand is the orch strand the ask went to, which may answer it whichever orch the run's parent resolves to later.
	Strand  string    `json:"strand,omitempty"`
	AskedAt time.Time `json:"asked_at"`
	// Deadline is when an ask still pending-post or open is set timeout; every ask that goes out carries one.
	Deadline time.Time `json:"deadline"`
	// Reason says why an ask never went out.
	Reason string `json:"reason,omitempty"`
	// Report is the parent report's path, set once the ask is done.
	Report     string    `json:"report,omitempty"`
	AnsweredAt time.Time `json:"answered_at,omitzero"`
}

// ParentAskRecord is a run's record of its parent-review asks, oldest first.
type ParentAskRecord struct {
	Asks []ParentAsk `json:"asks"`
}

// Latest returns the run's latest ask, and false when the run holds none.
func (r ParentAskRecord) Latest() (ParentAsk, bool) {
	if len(r.Asks) == 0 {
		return ParentAsk{}, false
	}
	return r.Asks[len(r.Asks)-1], true
}

// Ask returns the ask numbered number, and false when the run holds no such ask.
func (r ParentAskRecord) Ask(number int) (ParentAsk, bool) {
	for _, ask := range r.Asks {
		if ask.Number == number {
			return ask, true
		}
	}
	return ParentAsk{}, false
}

// ParentReviewFiles locates a run's parent-review files: the ask record, its lock and the directory holding the reports.
// None is tracked.
type ParentReviewFiles struct {
	Dir    string
	Record string
	Lock   string
}

// parentReviewLockName is the name of the ask record's lock in the checkout's lock directory.
const parentReviewLockName = "parent-review.lock"

// ParentReviewFilesOf returns the parent-review files of the run whose worktree is anchored at anchorPath.
func ParentReviewFilesOf(anchorPath string) ParentReviewFiles {
	dir := filepath.Join(anchorPath, lymdirs.DotLymDirName, lymdirs.ParentReviewDirName)
	return ParentReviewFiles{
		Dir:    dir,
		Record: filepath.Join(dir, "asks.json"),
		Lock:   lock.TrackedLockPath(anchorPath, parentReviewLockName),
	}
}

// ReportPath returns the path of the parent's report for the ask numbered number, beside the ask record.
func (f ParentReviewFiles) ReportPath(number int) string {
	return filepath.Join(f.Dir, fmt.Sprintf("ask-%d.report.yaml", number))
}

// ParentFinding is one finding of the parent's report.
// Change names what to change and where; Severity and Class are optional and, when set, in the review format's words.
type ParentFinding struct {
	ID       string   `yaml:"id"`
	Severity Severity `yaml:"severity,omitempty"`
	Class    Class    `yaml:"class,omitempty"`
	Change   string   `yaml:"change"`
}

// ParentReport is the parent's answer to one ask, written as YAML beside the ask record.
// Satisfied is true for a report with no findings.
type ParentReport struct {
	Ask       int             `yaml:"ask"`
	Satisfied bool            `yaml:"satisfied"`
	Findings  []ParentFinding `yaml:"findings"`
}

// ReadParentAsks reads the run's ask record; found is false when the run never recorded an ask.
func ReadParentAsks(files ParentReviewFiles) (ParentAskRecord, bool, error) {
	return state.ReadJSON[ParentAskRecord](files.Record, files.Lock)
}

// RecordParentAsk appends ask as the run's next ask, numbered one past the latest, and returns it as recorded.
// Its state is the caller's.
// It is pending-post for an ask about to be posted, and no-parent or not-sent for one that never goes out.
func RecordParentAsk(files ParentReviewFiles, ask ParentAsk) (ParentAsk, error) {
	err := state.UpdateJSON(files.Record, files.Lock, func(record ParentAskRecord, _ bool) (ParentAskRecord, error) {
		ask.Number = 1
		if latest, ok := record.Latest(); ok {
			ask.Number = latest.Number + 1
		}
		record.Asks = append(record.Asks, ask)
		return record, nil
	})
	if err != nil {
		return ParentAsk{}, fmt.Errorf("burler: record a parent-review ask: %w", err)
	}
	return ask, nil
}

// errParentAskUnchanged aborts a locked update that has nothing to write.
var errParentAskUnchanged = errors.New("burler: the parent-review ask is unchanged")

// MarkParentAskPosted moves the ask numbered number from pending-post to open once its mail went out.
// An ask past pending-post is left as it stands, and so is a number the run holds no ask for.
func MarkParentAskPosted(files ParentReviewFiles, number int) error {
	err := state.UpdateJSON(files.Record, files.Lock, func(record ParentAskRecord, _ bool) (ParentAskRecord, error) {
		for i := range record.Asks {
			if record.Asks[i].Number == number && record.Asks[i].State == ParentAskPendingPost {
				record.Asks[i].State = ParentAskOpen
				return record, nil
			}
		}
		return record, errParentAskUnchanged
	})
	if err != nil && !errors.Is(err, errParentAskUnchanged) {
		return fmt.Errorf("burler: mark parent-review ask %d posted: %w", number, err)
	}
	return nil
}

// ParentAnswerRefusalKind names why an answer was refused.
type ParentAnswerRefusalKind string

// The reasons an answer is refused.
const (
	// ParentAnswerNoAsk: the run holds no ask.
	ParentAnswerNoAsk ParentAnswerRefusalKind = "no-ask"
	// ParentAnswerNotLatest: the answer names an ask other than the run's latest.
	ParentAnswerNotLatest ParentAnswerRefusalKind = "not-latest"
	// ParentAnswerEnded: the ask is already in an end state.
	ParentAnswerEnded ParentAnswerRefusalKind = "ended"
	// ParentAnswerPastDeadline: the ask was at or past its deadline, and the answer set it timeout.
	ParentAnswerPastDeadline ParentAnswerRefusalKind = "past-deadline"
)

// ParentAnswerRefusal is the error AnswerParentAsk returns for an answer it did not record.
// Latest is the run's latest ask as it stands after the call, and the zero ask when the run holds none.
type ParentAnswerRefusal struct {
	Kind   ParentAnswerRefusalKind
	Latest ParentAsk
}

func (r *ParentAnswerRefusal) Error() string {
	return fmt.Sprintf("burler: parent-review answer refused (%s); the latest ask is %d, %s", r.Kind, r.Latest.Number, r.Latest.State)
}

// AnswerParentAsk records the parent's answer to the ask numbered number as one step under the record's lock.
// It writes the report and sets the ask done.
// findings become the report's items in their order, with ids P1, P2 and so on; no findings make a satisfied report.
// It answers only the run's latest ask, and only while that ask is pending-post or open and now is before its deadline.
// An ask still pending-post or open at or past its deadline is set timeout instead, and the answer refused.
// Every refusal is a *ParentAnswerRefusal and writes no report.
func AnswerParentAsk(files ParentReviewFiles, number int, now time.Time, findings []ParentFinding) (ParentAsk, error) {
	var refusal *ParentAnswerRefusal
	var answered ParentAsk
	err := state.UpdateJSON(files.Record, files.Lock, func(record ParentAskRecord, _ bool) (ParentAskRecord, error) {
		latest, ok := record.Latest()
		switch {
		case !ok:
			refusal = &ParentAnswerRefusal{Kind: ParentAnswerNoAsk}
			return record, errParentAskUnchanged
		case latest.Number != number:
			refusal = &ParentAnswerRefusal{Kind: ParentAnswerNotLatest, Latest: latest}
			return record, errParentAskUnchanged
		case latest.State.Ended():
			refusal = &ParentAnswerRefusal{Kind: ParentAnswerEnded, Latest: latest}
			return record, errParentAskUnchanged
		}
		ask := &record.Asks[len(record.Asks)-1]
		if !now.Before(ask.Deadline) {
			ask.State = ParentAskTimeout
			refusal = &ParentAnswerRefusal{Kind: ParentAnswerPastDeadline, Latest: *ask}
			return record, nil
		}
		path := files.ReportPath(number)
		if err := writeParentReport(path, number, findings); err != nil {
			return record, err
		}
		ask.State, ask.Report, ask.AnsweredAt = ParentAskDone, path, now
		answered = *ask
		return record, nil
	})
	if err != nil && !errors.Is(err, errParentAskUnchanged) {
		return ParentAsk{}, fmt.Errorf("burler: answer parent-review ask %d: %w", number, err)
	}
	if refusal != nil {
		return ParentAsk{}, refusal
	}
	return answered, nil
}

// writeParentReport writes the report of findings for the ask numbered number to path, numbering the findings P1 up.
func writeParentReport(path string, number int, findings []ParentFinding) error {
	report := ParentReport{Ask: number, Satisfied: len(findings) == 0, Findings: make([]ParentFinding, len(findings))}
	for i, finding := range findings {
		finding.ID = fmt.Sprintf("P%d", i+1)
		report.Findings[i] = finding
	}
	data, err := yaml.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal the parent report: %w", err)
	}
	return fsx.AtomicWriteBytes(path, data)
}

// ExpireParentAsk moves the run's latest ask to timeout as one step under the record's lock.
// It does so only when the ask is pending-post or open and now is at or past its deadline.
// It returns the latest ask as it then stands.
// found is false when the run holds no ask, and nothing is written then.
func ExpireParentAsk(files ParentReviewFiles, now time.Time) (ask ParentAsk, found bool, err error) {
	err = state.UpdateJSON(files.Record, files.Lock, func(record ParentAskRecord, _ bool) (ParentAskRecord, error) {
		latest, ok := record.Latest()
		if !ok {
			return record, errParentAskUnchanged
		}
		found = true
		ask = latest
		if latest.State.Ended() || now.Before(latest.Deadline) {
			return record, errParentAskUnchanged
		}
		record.Asks[len(record.Asks)-1].State = ParentAskTimeout
		ask.State = ParentAskTimeout
		return record, nil
	})
	if err != nil && !errors.Is(err, errParentAskUnchanged) {
		return ParentAsk{}, false, fmt.Errorf("burler: expire the parent-review ask: %w", err)
	}
	return ask, found, nil
}
