// recordwrite.go implements the Record-Write producer: a Go row that writes a loom run's decision record and support log from the run's board entry, with no agent.

package loomshed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// RecordWriteDeps are the injected seams and absolute paths of the Record-Write producer.
type RecordWriteDeps struct {
	// RecordPath is the absolute path of the decision record.
	RecordPath string
	// SupportLogPath is the absolute path of the support log.
	SupportLogPath string
	// DiscussionDir is the absolute path of the directory holding both files.
	DiscussionDir string
	// ReadBody returns the run's board entry body, with found false when the board holds no entry for the run.
	ReadBody func() (body string, found bool, err error)
	// ReadStored returns the stored record; found is true only when the entry is flagged discussed and its file exists.
	ReadStored func() (record []byte, found bool, err error)
	// Commit commits the discussion directory.
	// It is a no-op on a directory with no uncommitted change.
	Commit func() error
}

// recordWrite is the Record-Write producer, and the Record-Seed producer in its stored-only mode.
type recordWrite struct {
	name string
	deps RecordWriteDeps
	// noStoredReason is set in the stored-only mode.
	// It renders the Stuck reason of a missing record whose board entry holds no stored record.
	// This mode never renders that record from the entry's body.
	noStoredReason func() (string, error)
}

var _ shedengine.ShedProducer = (*recordWrite)(nil)

// NewRecordWrite returns the producer of the Record-Write row, identified as name.
func NewRecordWrite(name string, deps RecordWriteDeps) shedengine.ShedProducer {
	return &recordWrite{name: name, deps: deps}
}

// errorEnvelopeBodyReason is the Stuck reason of a board entry body holding a CLI error envelope.
// Its verb is the envelope line.
// Such a body is a failed verb's output captured where the entry's text belongs,
// so a record written from it would carry no scope.
// A beam project's run holds nothing at this row, so its body takes an edit in place.
// Any other run holds the entry, so its body is corrected only once the run is ended and removed.
const errorEnvelopeBodyReason = "the board entry's body holds a CLI error envelope where its text belongs: %s; " +
	"way forward for a beam project: write the entry's own text with \"lym board upsert --body-file <path>\", " +
	"then \"lym loom resume\"; " +
	"for any other run: end it with \"lym batten cancel <slug> --remove\" from the hub's prime worktree, " +
	"write the entry's own text with \"lym board upsert --body-file <path>\", " +
	"release the slug with \"lym board set-status '{\"slug\":\"<slug>\",\"status\":null}'\", then \"lym batten run <slug>\""

// recordSections are the headings of the decision record after its Goal, each written empty.
var recordSections = []string{
	"## Scope",
	"## Decisions",
	"## Constraints",
	"## Auto-mode assumptions",
	"## Open risks",
	"## Acceptance criteria",
}

// BoardRecord renders the decision record of a run whose task is body.
// The Goal section holds body verbatim in one fenced markdown block, and the other required sections follow empty.
// The fence is one backtick longer than the longest backtick run in body, and never shorter than three.
// So no line of body, a heading or a fence of its own included, opens or closes a section of the record.
func BoardRecord(body string) []byte {
	fence := strings.Repeat("`", max(3, longestBacktickRun(body)+1))

	var record strings.Builder
	record.WriteString("## Goal\n\n")
	record.WriteString(fence + "markdown\n")
	if body != "" {
		record.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			record.WriteString("\n")
		}
	}
	record.WriteString(fence + "\n")
	for _, heading := range recordSections {
		record.WriteString("\n" + heading + "\n")
	}

	return []byte(record.String())
}

// longestBacktickRun returns the length of the longest run of consecutive backticks in text.
func longestBacktickRun(text string) int {
	longest, current := 0, 0
	for _, r := range text {
		if r != '`' {
			current = 0
			continue
		}
		current++
		longest = max(longest, current)
	}

	return longest
}

// supportLogWithInterview renders a support log with no review rounds, whose Interview section holds the line interview.
func supportLogWithInterview(interview string) []byte {
	return []byte("## Interview\n\n" +
		interview + "\n\n" +
		"## Rejected alternatives\n\n" +
		"## Review rounds\n\n" +
		"_No rounds yet._\n\n" +
		"## Question ledger\n")
}

// boardSupportLog renders the support log of a run whose record was written from the board entry.
func boardSupportLog() []byte {
	return supportLogWithInterview("The decision record was written from the board entry, with no interview.")
}

// keptRecordSupportLog renders the support log of a run whose record was kept as it stood, of unstated source.
func keptRecordSupportLog() []byte {
	return supportLogWithInterview("The support log was missing beside an existing record; the record was kept as it stood.")
}

// StoredRecordSupportLog renders the support log of a run whose decision record is the board entry's stored record.
// The stored record was approved in an earlier run.
func StoredRecordSupportLog() []byte {
	return supportLogWithInterview("The decision record is the board entry's stored record, approved in an earlier run, so this run held no Discussion.")
}

// Call implements shedengine.ShedProducer.
// It removes the torn temp files a crash left in the discussion directory.
// It writes whichever of the record and the support log is missing, and it commits the pair.
// On success it reports Done with the record as its pointer.
// A missing record is the board entry's stored decision record, written verbatim, when the entry has one;
// otherwise it is rendered from the entry's body.
// A missing support log is the stored-record log when the entry has a stored record;
// otherwise it is the board-rendered log, whether or not the record was missing too.
// In the stored-only mode that otherwise case is a log that makes no claim about the record's source.
// A file that exists is never overwritten, so a resume keeps every entry added to the record after it was written.
// The commit runs on every call, so a pair a crash left written but uncommitted is committed on resume.
// A board with no entry for the run is Stuck, and so is an entry body holding a CLI error envelope, with nothing written.
// In the stored-only mode the body is never read.
// There a missing record with no stored record is Stuck with that mode's reason, and nothing is written.
// A read, write or commit failure is a returned error.
func (p *recordWrite) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	if err := p.removeTornFiles(); err != nil {
		return "", shedengine.OutputPointer{}, p.fail(ctx, "clear the torn temp files", err)
	}

	recordMissing, err := missingFile(p.deps.RecordPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, p.fail(ctx, "stat the decision record", err)
	}
	logMissing, err := missingFile(p.deps.SupportLogPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, p.fail(ctx, "stat the support log", err)
	}

	var stored []byte
	storedFound := false
	if recordMissing || logMissing {
		stored, storedFound, err = p.deps.ReadStored()
		if err != nil {
			return "", shedengine.OutputPointer{}, p.fail(ctx, "read the stored decision record", err)
		}
	}
	if recordMissing {
		switch {
		case storedFound:
			if err := fsx.AtomicWriteBytes(p.deps.RecordPath, stored); err != nil {
				return "", shedengine.OutputPointer{}, p.fail(ctx, "write the decision record", err)
			}
		case p.noStoredReason != nil:
			reason, err := p.noStoredReason()
			if err != nil {
				return "", shedengine.OutputPointer{}, p.fail(ctx, "read the run's seed", err)
			}
			if err := cancelErr(ctx, p.name); err != nil {
				return "", shedengine.OutputPointer{}, err
			}
			return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
		default:
			body, found, err := p.deps.ReadBody()
			if err != nil {
				return "", shedengine.OutputPointer{}, p.fail(ctx, "read the board entry", err)
			}
			if !found {
				if err := cancelErr(ctx, p.name); err != nil {
					return "", shedengine.OutputPointer{}, err
				}
				return shedengine.Stuck, shedengine.OutputPointer{Reason: "the board holds no entry for this run; way forward: restore the entry with \"lym board upsert\", then \"lym loom resume\""}, nil
			}
			if envelope, isEnvelope := output.ErrorEnvelopeLine(body); isEnvelope {
				if err := cancelErr(ctx, p.name); err != nil {
					return "", shedengine.OutputPointer{}, err
				}
				return shedengine.Stuck, shedengine.OutputPointer{Reason: fmt.Sprintf(errorEnvelopeBodyReason, envelope)}, nil
			}
			if err := fsx.AtomicWriteBytes(p.deps.RecordPath, BoardRecord(body)); err != nil {
				return "", shedengine.OutputPointer{}, p.fail(ctx, "write the decision record", err)
			}
		}
	}
	if logMissing {
		var supportLog []byte
		switch {
		case storedFound:
			supportLog = StoredRecordSupportLog()
		case p.noStoredReason != nil:
			supportLog = keptRecordSupportLog()
		default:
			supportLog = boardSupportLog()
		}
		if err := fsx.AtomicWriteBytes(p.deps.SupportLogPath, supportLog); err != nil {
			return "", shedengine.OutputPointer{}, p.fail(ctx, "write the support log", err)
		}
	}

	if err := p.deps.Commit(); err != nil {
		return "", shedengine.OutputPointer{}, p.fail(ctx, "commit the discussion artifacts", err)
	}
	if err := cancelErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	return shedengine.Done, shedengine.OutputPointer{Path: p.deps.RecordPath}, nil
}

// fail returns the error of a failed step, or the cancellation error when ctx was cancelled meanwhile.
func (p *recordWrite) fail(ctx context.Context, step string, err error) error {
	if cancelled := cancelErr(ctx, p.name); cancelled != nil {
		return cancelled
	}

	return fmt.Errorf("loomshed: %s: %s: %w; way forward: the fault is transient, re-step the %s row", p.name, step, err, p.name)
}

// removeTornFiles removes every temp file directly in the discussion directory that an interrupted atomic write left.
// The discussion commit stages the whole directory, so a left file would be committed.
func (p *recordWrite) removeTornFiles() error {
	entries, err := os.ReadDir(p.deps.DiscussionDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), ".tmp-") {
			continue
		}
		if err := os.Remove(filepath.Join(p.deps.DiscussionDir, entry.Name())); err != nil {
			return err
		}
	}

	return nil
}

// missingFile reports whether path does not exist, and returns any other stat failure.
func missingFile(path string) (bool, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return true, nil
	}

	return false, err
}
