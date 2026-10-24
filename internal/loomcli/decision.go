// decision.go implements the `decision` loom subtree: the one verb, `add`, through which a design call made after the Discussion ended is recorded in the decision record.
// The run's parent, or an operator, uses it to answer a question the record does not settle, so Plan-Write and a later reader see the call beside the decisions the Discussion made.
// The verb appends one entry, or the batch a `--from` JSON file holds, through `discussionparser.AppendDecisions`, which writes once, re-runs the discussion check and restores the record on a finding, and then commits the record once.
//
// The group carries its own PersistentPreRunE, and the loom parent's pre-run skips it, for the reason review.go gives.
// It resolves the target worktree through the same resolver the review group uses.
//
// The verb refuses a task session's label: a caller whose exported strand name differs from the run's parent orch, the nearest live orch up its beam chain, is refused, and the operator's own unnamed shell always passes.
// The one other caller is the run's own arbiter strand, which alone may pass `--by arbiter`; any other caller of that label is refused.
// The check reads only the exported strand name and the parent orch's name, so it guards a mistaken call, not a session that clears or forges the variable, and `--by` stays an unverified label.
// That is bounded by each entry being append-only, under an `Added after Discussion` heading with the claimed label and the date, in the call's one committed diff.
//
// On a darn run the verb works once Darn-Write has validated and committed its record, and also while Darn-Write runs.
// The caller rule and the append-only, labelled, committed entry are unchanged.
// The one new edge is appending while the writer runs, bounded to a record the writer validated and has not touched.
// The working copy must equal the committed copy and hold every added entry its history held.
// After the commit the committed record must hold every appended entry, so an entry a writer write displaced is refused rather than reported.
//
// The verb never routes the run, never touches the support log or the plan, and an approved plan is not re-planned by it.
//
// Once the entries are committed, the verb sends one creel notice to each live shuttle run of the target worktree but the tackler's, the arbiter's and the caller's own, so the LLM agent working for the run acts on the call.
// The line opens with a fixed phrase the parent directive names, so every spawned role takes it as its parent's, and names the record and each entry's heading.
// A one-entry notice inlines the entry's decision and rationale, so a role that may not read the record, such as webster's Master, still has the call.
// A batch's notice names only the headings, bar Master's, which inlines every entry's text.
// With no live run nothing is sent: a later producer reads the record when it starts.
// A failed listing or post is reported on the success envelope and never fails the verb, since the entry is already committed.

package loomcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/alignment"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

const (
	decisionGroup = "decision"

	decisionByParent   = "parent"
	decisionByOperator = "operator"
	decisionByArbiter  = "arbiter"

	decisionWayWriter = `way forward: Discussion-Write owns the record while it runs; message the Discussion-Write session with the decision instead, or re-run once the row has handed off`

	decisionWayDarnWriter = `way forward: Darn-Write has not finished its record or is editing it after validating it; message the running Darn-Write session, and re-run "lyx loom decision add" once the committed record is unchanged`
	decisionWayEntryLost  = `way forward: a Darn-Write write landed during the append; message the running Darn-Write session to leave the record alone, then re-run "lyx loom decision add"`
	decisionWayNoHeading  = `way forward: restore the "## Decisions" heading in the decision record, then re-run the verb`
	decisionWayRetry      = `way forward: re-run "lyx loom decision add"; nothing is appended`
	decisionWayFindings   = `way forward: the record is restored; fix the named section and re-run the verb`
	decisionWayArbiter    = `way forward: the "arbiter" label is the run's own arbiter's; pass --by parent from the run's parent, or --by operator from the operator's own shell`
	decisionWayPattern    = `way forward: pass --pattern-entry and --pattern-action together, and --pattern-text for adds or changes`

	decisionWayFromAlone = `way forward: put every entry's fields in the --from file and drop --title, --decision, --rationale and the --pattern-* flags`
	decisionWayFromFile  = `way forward: write the --from file as a non-empty JSON array of objects with the keys title, decision, rationale and, for a PATTERN change, pattern_entry, pattern_action and pattern_text, then re-run the verb`
	decisionWayFromKeys  = `way forward: set pattern_entry and pattern_action together, and pattern_text for adds or changes, then re-run the verb`
	decisionWayFromTwice = `way forward: give each entry of the --from file its own title and its own PATTERN entry, or record the second in a later call`

	decisionWayFabricCommit    = `way forward: run "lyx fabric commit" to commit the record; a session lyx refuses the verb from reports status: FAILED and the orch runs it`
	decisionWayAmendsPlanRetry = `way forward: the entry and the amendment record are restored and nothing landed; re-run "lyx loom decision add ... --amends-plan"`
	decisionWayPlanUnparsed    = `way forward: run "lyx loom validate-plan" to show the plan's defect, and record the decision without --amends-plan until the plan parses`
	decisionWayAmendsNoPlan    = `way forward: record the decision without --amends-plan; Plan-Write and Plan-Review read the decision record`
	decisionWayAmendsDarn      = `way forward: record the decision without --amends-plan`
)

// errPlanUnparsed marks an approval read that failed because the plan does not parse, as opposed to one that failed reading a file.
var errPlanUnparsed = errors.New("the plan does not parse")

// decisionWayNoRecord is the way forward of the refusal for a run with no decision record.
// Its last clause names the title form a board note must carry to count as a decision.
var decisionWayNoRecord = `way forward: the record exists once Discussion-Write has written it; "lyx loom status <slug>" shows whether the run has reached that row, and "lyx loom start" begins or resumes the run; on a darn run the record exists once Darn-Write has validated it, and until then message the running Darn-Write session with the call or, with no writer live, record the call as a board note titled "` + alignment.DecisionNoteTitle("<slug>") + ` <the call>"`

// decisionInput is the flag values of one `decision add` call.
// The three pattern fields are the optional PATTERN-change form and are all empty without it.
// from names the batch file whose entries replace the per-entry fields, and is empty for a one-entry call.
// amendsPlan marks a decision that amends the approved plan, so the call also writes the plan amendment record.
type decisionInput struct {
	by, title, decision, rationale           string
	patternEntry, patternAction, patternText string
	from                                     string
	amendsPlan                               bool
}

// decisionEntry is one decision of a call: the per-entry flags of a one-entry call, or one object of the --from file.
type decisionEntry struct {
	Title         string `json:"title"`
	Decision      string `json:"decision"`
	Rationale     string `json:"rationale"`
	PatternEntry  string `json:"pattern_entry"`
	PatternAction string `json:"pattern_action"`
	PatternText   string `json:"pattern_text"`
}

// decisionEntries returns the entries input carries, or the refusal text when they are not a well-formed set.
// A one-entry call refuses an empty field or a partial PATTERN-change form.
// A --from call refuses any per-entry flag beside it and a file that is unreadable, not one JSON array of objects, followed by more data, empty, or carrying a key no entry has.
// It also refuses an entry with an empty field or a partial PATTERN-change form, and two entries sharing a title or a PATTERN entry ID.
func decisionEntries(input decisionInput) ([]decisionEntry, string) {
	if input.from == "" {
		for _, flag := range []struct{ name, value string }{{"title", input.title}, {"decision", input.decision}, {"rationale", input.rationale}} {
			if strings.TrimSpace(flag.value) == "" {
				return nil, fmt.Sprintf("--%s is empty; way forward: pass --%s with the text to record", flag.name, flag.name)
			}
		}
		if (input.patternEntry != "") != (input.patternAction != "") || (input.patternEntry == "" && input.patternText != "") {
			return nil, "--pattern-entry, --pattern-action and --pattern-text are one form; " + decisionWayPattern
		}
		return []decisionEntry{{
			Title: input.title, Decision: input.decision, Rationale: input.rationale,
			PatternEntry: input.patternEntry, PatternAction: input.patternAction, PatternText: input.patternText,
		}}, ""
	}

	if input.title != "" || input.decision != "" || input.rationale != "" || input.patternEntry != "" || input.patternAction != "" || input.patternText != "" {
		return nil, "--from carries every entry, and a per-entry flag was passed beside it; " + decisionWayFromAlone
	}
	data, err := os.ReadFile(input.from)
	if err != nil {
		return nil, fmt.Sprintf("read the --from file: %s; %s", err, decisionWayFromFile)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var entries []decisionEntry
	err = decoder.Decode(&entries)
	if err == nil {
		// Anything after the array, a stray closing bracket included, must be the end of the file.
		if err = decoder.Decode(&json.RawMessage{}); err == io.EOF {
			err = nil
		} else {
			err = errors.New("data follows the array")
		}
	}
	if err != nil {
		return nil, fmt.Sprintf("the --from file %s is not one JSON array of entries: %s; %s", input.from, err, decisionWayFromFile)
	}
	if len(entries) == 0 {
		return nil, fmt.Sprintf("the --from file %s holds no entry; %s", input.from, decisionWayFromFile)
	}
	if n, key := unknownEntryKey(data); key != "" {
		return nil, fmt.Sprintf("entry %d of the --from file has an unknown field %q; %s", n, key, decisionWayFromFile)
	}
	titles := map[string]int{}
	patternIDs := map[string]int{}
	for i, entry := range entries {
		n := i + 1
		for _, field := range []struct{ name, value string }{{"title", entry.Title}, {"decision", entry.Decision}, {"rationale", entry.Rationale}} {
			if strings.TrimSpace(field.value) == "" {
				return nil, fmt.Sprintf("entry %d of the --from file has an empty %s; %s", n, field.name, decisionWayFromFile)
			}
		}
		if (entry.PatternEntry != "") != (entry.PatternAction != "") || (entry.PatternEntry == "" && entry.PatternText != "") {
			return nil, fmt.Sprintf("entry %d of the --from file: pattern_entry, pattern_action and pattern_text are one form; %s", n, decisionWayFromKeys)
		}
		title := strings.TrimSpace(entry.Title)
		if first, ok := titles[title]; ok {
			return nil, fmt.Sprintf("entries %d and %d of the --from file share the title %q; %s", first, n, title, decisionWayFromTwice)
		}
		titles[title] = n
		if id := strings.TrimSpace(entry.PatternEntry); id != "" {
			if first, ok := patternIDs[id]; ok {
				return nil, fmt.Sprintf("entries %d and %d of the --from file both change %s; %s", first, n, id, decisionWayFromTwice)
			}
			patternIDs[id] = n
		}
	}
	return entries, ""
}

// unknownEntryKey returns the 1-based number of the first entry of the --from array data that carries a key no decisionEntry has, with that key.
// The key is empty when every key is known.
// The key check is its own pass because the entries decode must stay tolerant of unknown keys.
func unknownEntryKey(data []byte) (int, string) {
	var objects []map[string]json.RawMessage
	if err := json.Unmarshal(data, &objects); err != nil {
		return 0, ""
	}
	known := map[string]bool{"title": true, "decision": true, "rationale": true, "pattern_entry": true, "pattern_action": true, "pattern_text": true}
	for i, object := range objects {
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if !known[key] {
				return i + 1, key
			}
		}
	}
	return 0, ""
}

// decisionDeps is every side effect decisionVerb performs, injected so tests supply fakes.
type decisionDeps struct {
	// strandName is the full name reed exported to the calling session, empty for a shell reed did not spawn.
	strandName string
	// parent resolves the orch that receives the run's notices, the nearest live one up the beam chain, called only for a named caller.
	parent func() (hubgeom.Parent, error)
	// arbiterStrand reports whether name is the target run's own arbiter strand, called only for a named caller.
	arbiterStrand func(name string) bool
	// readStatus reads the target worktree's default run status; found is false when the run has no status file.
	readStatus reviewStatusReader
	// recordPath is the decision record the entry lands in, reported on the success envelope.
	recordPath string
	// append adds every entry of ds to the decision record in one write and re-runs the discussion check, restoring the record on a finding or an error.
	append func(ds []discussionparser.AddedDecision) ([]discussionparser.Finding, error)
	// commit commits the discussion artifacts, the record included.
	commit func() error
	// now is the clock the entry's date comes from.
	now func() time.Time
	// darn reports whether the target's run is a darn run.
	darn func() (bool, error)
	// readRecord reads the decision record's working copy.
	readRecord func() ([]byte, error)
	// committed reads the decision record as committed at HEAD; found is false when HEAD holds no record.
	committed func() ([]byte, bool, error)
	// history returns every committed version of the decision record, newest first.
	history func() ([][]byte, error)
	// liveStrands returns the strand names of the target worktree's live LLM runs.
	liveStrands func() ([]string, error)
	// notify posts line to the strand named receiver through creel.
	notify func(receiver, line string) error
	// amendmentPath is the plan amendment record a call with --amends-plan writes.
	amendmentPath string
	// approvedPlan reports whether the current generation holds an approved plan.
	approvedPlan func() (bool, error)
	// appendAmendment adds title to the amendment record, creating it when absent.
	appendAmendment func(title string, at time.Time) error
	// committedAmendment reads the amendment record as committed at HEAD; found is false when HEAD holds none.
	committedAmendment func() ([]byte, bool, error)
	// commitAmendment commits the amendment record alone.
	commitAmendment func() error
}

// decisionNoticeError is one notice the verb could not send, reported on its success envelope.
// Strand is empty when the live runs could not be listed.
type decisionNoticeError struct {
	Strand string `json:"strand"`
	Error  string `json:"error"`
}

// decisionNoticeOpening opens every decision notice.
// The parent directive stencil names it, so a spawned role tells the notice from any other message signed as the LOOM program.
const decisionNoticeOpening = "Parent decision recorded with lyx loom decision add"

// decisionNoticeLine is the notice a live agent of the run gets for the entries added in the record at recordPath.
// A one-entry notice, and every notice with inline set, carries each entry's decision and rationale, since a role such as Merriam may not read the record;
// an agent that may read the record re-reads its Added after Discussion entries, the committed record being the authority.
// Any other notice of several entries names only their headings, so a batch reaches the agent as one short line.
func decisionNoticeLine(recordPath string, added []discussionparser.AddedDecision, inline bool) string {
	if len(added) == 1 {
		return fmt.Sprintf("%s, under %q in the decision record %s. Decision: %s Rationale: %s Act on it as your parent's instruction; when you may read the record, re-read its Added after Discussion entries and follow them.",
			decisionNoticeOpening, addedDecisionHeading(added[0]), recordPath, strings.TrimSpace(added[0].Decision), strings.TrimSpace(added[0].Rationale))
	}
	parts := make([]string, 0, len(added))
	for _, d := range added {
		if inline {
			parts = append(parts, fmt.Sprintf("under %q, Decision: %s Rationale: %s", addedDecisionHeading(d), strings.TrimSpace(d.Decision), strings.TrimSpace(d.Rationale)))
			continue
		}
		parts = append(parts, fmt.Sprintf("%q", addedDecisionHeading(d)))
	}
	if inline {
		return fmt.Sprintf("%s, %d entries in the decision record %s: %s. Act on them as your parent's instruction; when you may read the record, re-read its Added after Discussion entries and follow them.",
			decisionNoticeOpening, len(added), recordPath, strings.Join(parts, "; "))
	}
	return fmt.Sprintf("%s, %d entries in the decision record %s, under %s. Act on them as your parent's instruction: re-read the record's Added after Discussion entries and follow them.",
		decisionNoticeOpening, len(added), recordPath, strings.Join(parts, ", "))
}

// decisionNoticeInline reports whether the live run's strand named strand reads no decision record, so its notice carries every entry's text.
func decisionNoticeInline(strand string) bool {
	name, err := agentname.Parse(strand)
	return err == nil && agentname.MatchesRole(name.Role, websterengine.MerriamStrandRole)
}

// decisionNoticeReceiver reports whether the live run's strand named strand gets the notice of a call by caller.
// The tackler and the arbiter judge stops, not the plan, and the caller wrote the decision itself.
func decisionNoticeReceiver(strand, caller string) bool {
	name, err := agentname.Parse(strand)
	return err == nil && strand != caller &&
		!agentname.MatchesRole(name.Role, sheddriver.TacklerRole) && !agentname.MatchesRole(name.Role, loomengine.ArbiterRole)
}

// notifyLiveRuns posts one notice of the entries added to every live run of the target worktree the notice is for.
// It returns the strands posted to and the failures, each logged at Warn; a failure never stops the next post.
func notifyLiveRuns(deps decisionDeps, added []discussionparser.AddedDecision) ([]string, []decisionNoticeError) {
	notified := []string{}
	failures := []decisionNoticeError{}
	strands, err := deps.liveStrands()
	if err != nil {
		logger.Warn("loom: decision add could not list the run's live agents; no notice sent", "error", err)
		return notified, append(failures, decisionNoticeError{Error: "list the run's live agents: " + err.Error()})
	}
	for _, strand := range strands {
		if !decisionNoticeReceiver(strand, deps.strandName) {
			continue
		}
		if err := deps.notify(strand, decisionNoticeLine(deps.recordPath, added, decisionNoticeInline(strand))); err != nil {
			logger.Warn("loom: decision add could not notify a live agent", "strand", strand, "error", err)
			failures = append(failures, decisionNoticeError{Strand: strand, Error: err.Error()})
			continue
		}
		notified = append(notified, strand)
	}
	return notified, failures
}

// checkDarnRecord returns the refusal text for a darn run's record that is not safe to append to, or empty when it is.
// The record is safe once the writer has validated and committed it and not touched it since: it exists, is committed, equals its committed copy and holds every added entry its history held.
// A failed read of any of the four is returned as an error.
func checkDarnRecord(deps decisionDeps) (string, error) {
	working, err := deps.readRecord()
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Sprintf("the run has no decision record at %s; %s", deps.recordPath, decisionWayNoRecord), nil
	}
	if err != nil {
		return "", err
	}
	committed, found, err := deps.committed()
	if err != nil {
		return "", err
	}
	if !found {
		return "the darn run's decision record is not committed yet; " + decisionWayDarnWriter, nil
	}
	if !bytes.Equal(working, committed) {
		return "the decision record differs from its committed copy; " + decisionWayDarnWriter, nil
	}
	versions, err := deps.history()
	if err != nil {
		return "", err
	}
	if findings := discussionparser.CheckAddedDecisionsKept(working, versions); len(findings) > 0 {
		return "the decision record is missing an entry its history held: " + strings.Join(renderFindings(findings), "; ") + "; " + decisionWayDarnWriter, nil
	}
	return "", nil
}

// committedLacksEntry returns the heading of the first entry of added that the record as committed at HEAD holds no `Added after Discussion` entry for, or empty when it holds them all.
// A record HEAD does not hold lacks the first entry.
func committedLacksEntry(deps decisionDeps, added []discussionparser.AddedDecision) (string, error) {
	committed, found, err := deps.committed()
	if err != nil {
		return "", err
	}
	if !found {
		return addedDecisionHeading(added[0]), nil
	}
	entries, err := discussionparser.ParseAddedDecisions(committed)
	if err != nil {
		return "", err
	}
	held := make(map[string]bool, len(entries))
	for _, entry := range entries {
		held[addedDecisionHeading(entry)] = true
	}
	for _, d := range added {
		if !held[addedDecisionHeading(d)] {
			return addedDecisionHeading(d), nil
		}
	}
	return "", nil
}

// amendmentCall holds what a `--amends-plan` call must put back when its append or commit fails:
// both records' bytes before the call, whether each existed, and the amendment record's bytes the call wrote.
type amendmentCall struct {
	priorDecision    []byte
	decisionExisted  bool
	priorAmendment   []byte
	amendmentExisted bool
	written          []byte
}

// decisionWayRestoreRecords is the way forward when a record could not be put back to its prior bytes.
const decisionWayRestoreRecords = `way forward: rewrite the file named above to its content before this call, or remove it when it did not exist, then re-run the verb`

// readPriorRecord reads path and reports whether it existed; an absent file is no error.
func readPriorRecord(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, err
}

// restoreRecord writes prior back to path, or removes the file when it did not exist before.
func restoreRecord(path string, prior []byte, existed bool) error {
	if existed {
		return os.WriteFile(path, prior, 0o644)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// beginAmendment snapshots both records and appends every title, in order, to the amendment record.
// A failure leaves the amendment record as it was.
func beginAmendment(deps decisionDeps, titles []string) (*amendmentCall, error) {
	a := &amendmentCall{}
	var err error
	if a.priorDecision, a.decisionExisted, err = readPriorRecord(deps.recordPath); err != nil {
		return nil, fmt.Errorf("read the decision record: %w", err)
	}
	if a.priorAmendment, a.amendmentExisted, err = readPriorRecord(deps.amendmentPath); err != nil {
		return nil, fmt.Errorf("read the amendment record: %w", err)
	}
	for _, title := range titles {
		if err := deps.appendAmendment(title, deps.now()); err != nil {
			return nil, errors.Join(fmt.Errorf("write the amendment record: %w", err), a.restoreAmendment(deps))
		}
	}
	if a.written, err = os.ReadFile(deps.amendmentPath); err != nil {
		return nil, errors.Join(fmt.Errorf("read the amendment record back: %w", err), a.restoreAmendment(deps))
	}
	return a, nil
}

// restoreAmendment puts the amendment record back to its bytes before the call.
func (a *amendmentCall) restoreAmendment(deps decisionDeps) error {
	return restoreRecord(deps.amendmentPath, a.priorAmendment, a.amendmentExisted)
}

// restoreDecision puts the decision record back to its bytes before the call; a record absent before stays absent.
func (a *amendmentCall) restoreDecision(deps decisionDeps) error {
	if !a.decisionExisted {
		return nil
	}
	return restoreRecord(deps.recordPath, a.priorDecision, true)
}

// rollBackAppend undoes the amendment of a call whose append returned an error or findings, which cause describes.
// A decision record is restored only after an error, since the append restores it itself on findings.
// When a status commit already landed this call's amendment bytes, the restored record is committed, so HEAD never keeps a title whose entry did not land.
// It returns the refusal text when that cleanup fails, and empty when it succeeded.
func (a *amendmentCall) rollBackAppend(deps decisionDeps, appendErrored bool, cause string) string {
	if appendErrored {
		if err := a.restoreDecision(deps); err != nil {
			return fmt.Sprintf("%s; the decision record at %s could not be restored: %s; %s", cause, deps.recordPath, err, decisionWayRestoreRecords)
		}
	}
	if err := a.restoreAmendment(deps); err != nil {
		return fmt.Sprintf("%s; the amendment record at %s could not be restored: %s; %s", cause, deps.amendmentPath, err, decisionWayRestoreRecords)
	}
	committed, found, err := deps.committedAmendment()
	if err != nil {
		return fmt.Sprintf("%s; the amendment record at HEAD could not be read: %s; %s", cause, err, decisionWayFabricCommit)
	}
	if found && bytes.Equal(committed, a.written) {
		if err := deps.commitAmendment(); err != nil {
			return fmt.Sprintf("%s; a status commit landed the amendment title and committing the restored record failed: %s; %s", cause, err, decisionWayFabricCommit)
		}
	}
	return ""
}

// afterFailedCommit decides what a failed commit left at HEAD, which commitErr describes.
// It reports landed when HEAD holds both this call's amendment record and every entry, so the commit went through despite its error.
// Otherwise it returns the refusal text: both files are kept when HEAD holds the amendment record alone or cannot be read, and restored when HEAD differs.
func (a *amendmentCall) afterFailedCommit(deps decisionDeps, added []discussionparser.AddedDecision, commitErr error) (landed bool, refusal string) {
	failed := fmt.Sprintf("the entry is appended but the commit failed: %s", commitErr)
	committed, found, err := deps.committedAmendment()
	if err != nil {
		return false, fmt.Sprintf("%s; the amendment record at HEAD could not be read: %s; %s", failed, err, decisionWayFabricCommit)
	}
	if found && bytes.Equal(committed, a.written) {
		lacking, err := committedLacksEntry(deps, added)
		switch {
		case err != nil:
			return false, fmt.Sprintf("%s; the decision record at HEAD could not be read: %s; %s", failed, err, decisionWayFabricCommit)
		case lacking == "":
			return true, ""
		}
		return false, fmt.Sprintf("%s; a status commit landed the amendment record without the entry; %s", failed, decisionWayFabricCommit)
	}
	if err := a.restoreDecision(deps); err != nil {
		return false, fmt.Sprintf("%s; the decision record at %s could not be restored: %s; %s", failed, deps.recordPath, err, decisionWayRestoreRecords)
	}
	if err := a.restoreAmendment(deps); err != nil {
		return false, fmt.Sprintf("%s; the amendment record at %s could not be restored: %s; %s", failed, deps.amendmentPath, err, decisionWayRestoreRecords)
	}
	return false, fmt.Sprintf("%s; %s", failed, decisionWayAmendsPlanRetry)
}

// planApprovedReader returns the seam that reports whether the plan of the worktree anchored at anchorPath is approved.
// An absent overview answers false.
// A read failure is returned as is, and a plan that does not parse is returned wrapping errPlanUnparsed.
func planApprovedReader(anchorPath string) func() (bool, error) {
	return func() (bool, error) {
		if _, err := os.Stat(planparser.PlanOverview(anchorPath)); errors.Is(err, fs.ErrNotExist) {
			return false, nil
		} else if err != nil {
			return false, err
		}
		plan, err := planparser.ParsePlan(planparser.PlanDir(anchorPath))
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			return false, err
		}
		if err != nil {
			return false, fmt.Errorf("%w: %s", errPlanUnparsed, err)
		}
		return plan.Approved, nil
	}
}

// decisionVerb appends the decisions in input to the record and commits them in one commit, or refuses with a way forward.
//
// With amendsPlan the verb also appends each entry's title to the plan amendment record, committed with the entries.
// It is refused on a darn run and for a plan that is not approved, and a failed append or commit puts both records back.
// The optional PATTERN-change form appends one bullet of the record's PATTERN-change shape beside its entry, in the same write.
// It passes through the verb's existing checks and its --by parent|operator|arbiter caller guards, so only the run's parent, the operator's own shell or the run's own arbiter declares a change after Discussion.
// The form rewrites only the record's bullet for the same entry ID, which its new bullet replaces, and removes nothing.
func decisionVerb(out io.Writer, slug string, deps decisionDeps, input decisionInput) int {
	refuse := func(format string, args ...any) int {
		return output.Err(out, fmt.Sprintf("loom: %s add: ", decisionGroup)+fmt.Sprintf(format, args...))
	}

	if input.by != decisionByParent && input.by != decisionByOperator && input.by != decisionByArbiter {
		return refuse("--by is %q; way forward: pass --by %s or --by %s", input.by, decisionByParent, decisionByOperator)
	}
	entries, refusal := decisionEntries(input)
	if refusal != "" {
		return refuse("%s", refusal)
	}

	if input.by == decisionByArbiter {
		if deps.strandName == "" || !deps.arbiterStrand(deps.strandName) {
			return refuse("the caller %q is not the run's arbiter; %s", deps.strandName, decisionWayArbiter)
		}
	} else if deps.strandName != "" {
		parent, err := deps.parent()
		if err != nil {
			return refuse("the caller %q is a named session; way forward: the run's parent could not be resolved (%s); the operator's own unnamed shell may still run the verb", deps.strandName, err)
		}
		if deps.strandName != parent.Name {
			if parent.Name == "" {
				return refuse("the caller %q is a task session; way forward: a design call after the Discussion is the operator's to record; the operator's own unnamed shell may always run it", deps.strandName)
			}
			return refuse("the caller %q is a task session; way forward: a design call after the Discussion is the parent's to record; ask %s to run the verb; the operator's own unnamed shell may always run it", deps.strandName, parent.Name)
		}
	}

	st, found, err := deps.readStatus()
	if err != nil {
		return refuse("read the run status: %s; %s", err, decisionWayRetry)
	}
	if found && st.State == shedengine.StateRunning && st.CurrentProducer == loomshed.NameDiscussionWrite {
		return refuse("%s is running and owns the decision record; %s", loomshed.NameDiscussionWrite, decisionWayWriter)
	}

	darn, err := deps.darn()
	if err != nil {
		return refuse("read the run's recipe: %s; %s", err, decisionWayRetry)
	}
	if input.amendsPlan && darn {
		return refuse("--amends-plan has no plan to amend on a darn run; %s", decisionWayAmendsDarn)
	}
	if darn {
		refusal, err := checkDarnRecord(deps)
		if err != nil {
			return refuse("read the darn run's decision record: %s; %s", err, decisionWayRetry)
		}
		if refusal != "" {
			return refuse("%s", refusal)
		}
	}
	if input.amendsPlan {
		approved, err := deps.approvedPlan()
		switch {
		case errors.Is(err, errPlanUnparsed):
			return refuse("%s; %s", err, decisionWayPlanUnparsed)
		case err != nil:
			return refuse("read the plan: %s; %s", err, decisionWayRetry)
		case !approved:
			return refuse("the current generation has no approved plan to amend; %s", decisionWayAmendsNoPlan)
		}
	}

	date := deps.now().UTC()
	added := make([]discussionparser.AddedDecision, 0, len(entries))
	titles := make([]string, 0, len(entries))
	headings := make([]string, 0, len(entries))
	patternBullets := []string{}
	for _, entry := range entries {
		d := discussionparser.AddedDecision{
			By:        input.by,
			Title:     entry.Title,
			Decision:  entry.Decision,
			Rationale: entry.Rationale,
			Date:      date,
		}
		if entry.PatternEntry != "" {
			change := discussionparser.PatternChange{
				ID:     strings.TrimSpace(entry.PatternEntry),
				Action: strings.TrimSpace(entry.PatternAction),
				Text:   strings.TrimSpace(entry.PatternText),
			}
			d.PatternChange = &change
			patternBullets = append(patternBullets, change.Bullet())
		}
		added = append(added, d)
		titles = append(titles, strings.TrimSpace(entry.Title))
		headings = append(headings, addedDecisionHeading(d))
	}
	var amendment *amendmentCall
	if input.amendsPlan {
		amendment, err = beginAmendment(deps, titles)
		if err != nil {
			return refuse("%s; %s", err, decisionWayRetry)
		}
	}
	findings, err := deps.append(added)
	if amendment != nil && (err != nil || len(findings) > 0) {
		cause := "the discussion check flags the record after the append: " + strings.Join(renderFindings(findings), "; ")
		if err != nil {
			cause = err.Error()
		}
		if refusal := amendment.rollBackAppend(deps, err != nil, cause); refusal != "" {
			return refuse("%s", refusal)
		}
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return refuse("the run has no decision record at %s; %s", deps.recordPath, decisionWayNoRecord)
	case errors.Is(err, discussionparser.ErrNoDecisionsHeading):
		return refuse("%s; %s", err, decisionWayNoHeading)
	case err != nil:
		return refuse("%s; %s", err, decisionWayRetry)
	case len(findings) > 0:
		return refuse("the discussion check flags the record after the append: %s; %s", strings.Join(renderFindings(findings), "; "), decisionWayFindings)
	}
	commitErr := deps.commit()
	if commitErr != nil && amendment == nil {
		return refuse("the entry is appended but the commit failed: %s; %s", commitErr, decisionWayFabricCommit)
	}
	if commitErr != nil {
		landed, refusal := amendment.afterFailedCommit(deps, added, commitErr)
		if !landed {
			return refuse("%s", refusal)
		}
	}
	if darn {
		lacking, err := committedLacksEntry(deps, added)
		if err != nil {
			return refuse("the committed record could not be read: %s; %s", err, decisionWayEntryLost)
		}
		if lacking != "" {
			return refuse("the committed record holds no entry under %q; %s", lacking, decisionWayEntryLost)
		}
	}
	notified, noticeErrors := notifyLiveRuns(deps, added)
	result := map[string]any{
		"slug":          slug,
		"record":        deps.recordPath,
		"headings":      headings,
		"notified":      notified,
		"notice_errors": noticeErrors,
	}
	if len(patternBullets) > 0 {
		result["pattern_changes"] = patternBullets
	}
	if len(added) == 1 {
		result["heading"] = headings[0]
		if len(patternBullets) == 1 {
			result["pattern_change"] = patternBullets[0]
		}
	}
	if commitErr != nil {
		result["commit_error"] = commitErr.Error()
	}
	return output.Ok(out, result)
}

// addedDecisionHeading is the heading AppendDecision writes for d, without its leading marker.
func addedDecisionHeading(d discussionparser.AddedDecision) string {
	return fmt.Sprintf("Added after Discussion (%s, %s): %s", strings.TrimSpace(d.By), d.Date.UTC().Format("2006-01-02"), strings.TrimSpace(d.Title))
}

// decisionState carries what the decision group's pre-run resolved for its verb.
type decisionState struct {
	deps  decisionDeps
	slug  string
	input decisionInput
}

// preRun resolves the invoking cwd, then the target worktree, then the verb's side effects.
func (s *decisionState) preRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == decisionGroup {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	slug := ""
	if len(args) >= 1 {
		slug = args[0]
	}
	target, err := resolveReviewTarget(decisionGroup, cmd.Name(), location, slug, defaultReviewTargetDeps())
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	s.slug = target.WorktreeName
	recordPath := loomengine.DiscussionDecisionRecord(target)
	supportLogPath := loomengine.DiscussionSupportLog(target)
	s.deps = decisionDeps{
		strandName: os.Getenv(agentname.StrandNameEnv),
		parent:     func() (hubgeom.Parent, error) { return orchcli.NearestLiveParent(target) },
		arbiterStrand: func(name string) bool {
			geometry, err := hubgeom.ReedGeometry(target)
			if err != nil {
				return false
			}
			parsed, err := agentname.Parse(name)
			return err == nil && parsed.Shortname == geometry.NameShortname && parsed.Slug == geometry.NameSlug && loomengine.IsArbiterStrand(name)
		},
		readStatus: circlingStatusReader(shedrun.StatusFile(target, shedrun.SelfRunID), shedrun.StatusLock(target, shedrun.SelfRunID)),
		recordPath: recordPath,
		append: func(ds []discussionparser.AddedDecision) ([]discussionparser.Finding, error) {
			return discussionparser.AppendDecisions(recordPath, supportLogPath, ds)
		},
		commit: func() error { return commitDiscussion(target) },
		now:    time.Now,
		darn: func() (bool, error) {
			seed, found, err := shedrun.ReadSeed(target, shedrun.SelfRunID)
			if err != nil {
				return false, err
			}
			return resolveSeedRecipe(seed, found) == shedrun.RecipeDarn, nil
		},
		readRecord: func() ([]byte, error) { return os.ReadFile(recordPath) },
		committed: func() ([]byte, bool, error) {
			return fabricengine.CommittedAnchoredFile(target, loomengine.DiscussionDecisionRecordRel())
		},
		history: discussionHistory(target),
		liveStrands: func() ([]string, error) {
			return liveRunStrands(target)
		},
		notify: func(receiver, line string) error {
			return postDecisionNotice(decisionNoticeClients(target), os.Getenv(agentname.StrandNameEnv), receiver, line)
		},
		amendmentPath: loomengine.LoomAmendmentRecord(target),
		approvedPlan:  planApprovedReader(target.AnchorPath()),
		appendAmendment: func(title string, at time.Time) error {
			return loomshed.AppendAmendment(loomengine.LoomAmendmentRecord(target), title, at)
		},
		committedAmendment: func() ([]byte, bool, error) {
			return fabricengine.CommittedAnchoredFile(target, loomengine.LoomAmendmentRecordRel())
		},
		commitAmendment: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), target, []string{loomengine.LoomAmendmentRecordRel()}, fmt.Sprintf("loom: plan amendment record for %s", seedSlug(target.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
	}
	return nil
}

// liveRunStrands returns the strand names of the live shuttle runs of target's worktree, read through the registered engines.
func liveRunStrands(target *lyxcwd.Location) ([]string, error) {
	configDirs := fabricengine.ConfigDirs(target)
	cfg, err := shuttleengine.LoadConfig(configDirs, "shuttle")
	if err != nil {
		return nil, err
	}
	engines, err := shuttleengines.New(cfg, configDirs)
	if err != nil {
		return nil, err
	}
	readings, err := shuttleengine.ReadAgentActivity(cfg, target.AnchorPath(), engines)
	if err != nil {
		return nil, err
	}
	strands := make([]string, 0, len(readings))
	for _, reading := range readings {
		strands = append(strands, reading.StrandName)
	}
	return strands, nil
}

// noticePoster is the one creel call a notice makes: leaving a body as mail for a receiver.
type noticePoster interface {
	Post(receiver, body string) (int64, error)
}

// noticeClients builds the creel clients a notice posts through: one signing as a strand, one signing as the LOOM program.
type noticeClients struct {
	strand  func(strandClaim string) (noticePoster, error)
	program func() (noticePoster, error)
}

// postDecisionNotice posts line to receiver for a caller whose exported strand name is caller.
// Creel accepts a program claim only from an orch pane or a process in no pane, so a named non-orch caller, the arbiter, posts as its own strand.
// A strand claim creel refuses as coming from outside that strand's pane falls back to posting once as the program.
func postDecisionNotice(clients noticeClients, caller, receiver, line string) error {
	if name, err := agentname.Parse(caller); err == nil && !agentname.MatchesRole(name.Role, agentname.RoleOrch) {
		poster, err := clients.strand(caller)
		if err != nil {
			return err
		}
		_, err = poster.Post(receiver, line)
		if !errors.Is(err, creelengine.ErrStrandClaimOutsidePane) {
			return err
		}
	}
	poster, err := clients.program()
	if err != nil {
		return err
	}
	_, err = poster.Post(receiver, line)
	return err
}

// decisionNoticeClients builds the creel clients of target's worktree that a notice posts through.
func decisionNoticeClients(target *lyxcwd.Location) noticeClients {
	return noticeClients{
		strand: func(strandClaim string) (noticePoster, error) {
			return creelcli.NewStrandClient(target, strandClaim)
		},
		program: func() (noticePoster, error) {
			return creelcli.NewProgramClient(target, loomProgram)
		},
	}
}

// decisionCmd builds the `decision` subtree.
func (c *loomCLI) decisionCmd() *cobra.Command {
	st := &decisionState{}

	group := &cobra.Command{
		Use:   decisionGroup,
		Short: "record a design call made after the Discussion",
		Long: `decision holds the verb through which a design call made after the Discussion
ended is recorded in the decision record. "decision add" is the only verb.

The verb takes an optional slug naming the task worktree. A task worktree
addresses itself; from the prime the slug is required. It refuses a task
session's label: a caller whose reed strand name differs from the run's
parent orch is refused, and the operator's own unnamed shell always passes.
The one other caller is the run's own arbiter strand, which passes --by arbiter.
--by is a label that is not verified.

Example:
  lyx loom decision add <slug> --by parent --title "..." --decision "..." --rationale "..."
  lyx loom decision add <slug> --by parent --from decisions.json`,
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: st.preRun,
	}

	add := &cobra.Command{
		Use:         "add [<slug>]",
		Short:       "append decisions to the decision record and commit them",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `add appends an entry under an "Added after Discussion" heading, carrying the
--by label and today's date, to the decision record's Decisions section,
re-runs the discussion check (restoring the record on a finding) and commits.
It resumes nothing and changes neither the support log nor the plan. It then
notifies the run's other live agents ("notified", "notice_errors").

--by is parent, operator or arbiter, the last only from the run's own arbiter
strand. --title, --decision and --rationale are required unless --from is
passed. A named session other than the run's parent orch or arbiter is
refused. It is refused while Discussion-Write runs, and on a darn run until
Darn-Write has validated and committed its record.

--amends-plan marks a decision that amends the approved plan: the call also
appends the title to the plan amendment record in the same commit. It is
refused on a darn run and with no approved plan. Afterwards the parent runs
"lyx loom goto --to Plan-Burler-Review" and "lyx loom resume".

A decision on a PATTERN entry also passes --pattern-entry <PATTERN-id>,
--pattern-action adds|changes|removes and, for adds or changes, --pattern-text
<new entry text>, which writes its bullet to the "## PATTERN changes" section.

--from <file> records several decisions in place of the per-entry flags: a
JSON array of objects with the keys title, decision, rationale and optionally
pattern_entry, pattern_action and pattern_text. --by and --amends-plan apply
to every entry. All land in one write and one commit, or none does, and each
live agent gets one notice naming the headings ("headings"). Two entries may
not share a title or a PATTERN entry.

Example:
  lyx loom decision add <slug> --by parent --title "Keep the cache" --decision "Keep the cache per run." --rationale "A shared cache races."
  lyx loom decision add <slug> --by parent --from decisions.json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), decisionVerb(cmd.OutOrStdout(), st.slug, st.deps, st.input))
			return nil
		},
	}
	add.Flags().StringVar(&st.input.by, "by", "", "who made the call: parent, operator or arbiter")
	add.Flags().StringVar(&st.input.title, "title", "", "the decision's heading title")
	add.Flags().StringVar(&st.input.decision, "decision", "", "what was decided")
	add.Flags().StringVar(&st.input.rationale, "rationale", "", "why it was decided")
	add.Flags().StringVar(&st.input.patternEntry, "pattern-entry", "", "the PATTERN entry the decision adds, changes or removes, as PATTERN-<id>")
	add.Flags().StringVar(&st.input.patternAction, "pattern-action", "", "what the decision does to the entry: adds, changes or removes")
	add.Flags().StringVar(&st.input.patternText, "pattern-text", "", "the entry's new text, for adds or changes")
	add.Flags().StringVar(&st.input.from, "from", "", "a JSON file of several decisions to record in one call, in place of the per-entry flags")
	add.Flags().BoolVar(&st.input.amendsPlan, "amends-plan", false, "the decision amends the approved plan: also record it in the plan amendment record")

	group.AddCommand(add)
	return group
}
