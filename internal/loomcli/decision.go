// decision.go implements the `decision` loom group:
// `add` records a design call made after the Discussion ended as an append-only entry in the run's decision record.
// `replace` writes a whole checked record from a file under a trailer that cuts the record's history check.
// `store` copies the committed record onto the run's board entry, idempotently.
// An `add` call takes one entry or a batch from a `--from` file, and commits the record once.
// Every verb holds the run's decision lock from its first precondition through its commit and board copy.
// A call made while `Discussion-Bouncer` is settling is not copied to the board,
// and `store` closes that window by hand.
// Each live shuttle run of the target worktree gets a creel notice, except the tackler, the arbiter and the caller.
// No verb routes the run or touches the support log or the plan.
// The caller check guards against a mistaken call, not a forged strand name, so `--by` stays an unverified label.

package loomcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/raddleparser"
	"github.com/Knatte18/loomyard/internal/samplerengine"
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

	decisionWayEntryLost = `way forward: re-run "lym loom decision add"`
	decisionWayNoHeading = `way forward: restore the "## Decisions" heading in the decision record, then re-run the verb`
	decisionWayRetry     = `way forward: re-run "lym loom decision add"; nothing is appended`
	decisionWayFindings  = `way forward: the record is restored; fix the named section and re-run the verb`
	decisionWayArbiter   = `way forward: the "arbiter" label is the run's own arbiter's; pass --by parent from the run's parent, or --by operator from the operator's own shell`
	decisionWaySampler   = `way forward: pass --by operator for a call the operator settled in the pane, or ask the run's parent to record its own call`
	decisionWayPattern   = `way forward: pass --pattern-entry and --pattern-action together, and --pattern-text for adds or changes, never for withdraws`
	decisionWayWithdraw  = `way forward: read the record's "## PATTERN changes" section, then re-run the verb naming an entry it declares, or without the withdrawal when the entry was never declared`

	decisionWayRecordStart     = `the record exists once Record-Write has run; way forward: run "lym loom start", then re-run the verb once the run passed Record-Write`
	decisionWayRecordResume    = `the record exists once Record-Write has run; way forward: run "lym loom resume", then re-run the verb`
	decisionWayRecordGoto      = `way forward: run "lym loom goto --to Record-Write", then "lym loom resume", then re-run the verb`
	decisionWayUncommitted     = `way forward: run "lym loom resume", which commits the record and support log, then re-run the verb`
	decisionWayUncommittedGoto = `way forward: run "lym loom goto --to Record-Write", then "lym loom resume", which commits the existing record and support log without overwriting them, then re-run the verb`
	decisionWayEntryMissing    = `way forward: restore the entry and commit the record under _lym, then re-run the verb`

	decisionWayFromAlone = `way forward: put every entry's fields in the --from file and drop --title, --decision, --rationale and the --pattern-* flags`
	decisionWayFromFile  = `way forward: write the --from file as a non-empty JSON array of objects with the keys title, decision, rationale and, for a PATTERN change, pattern_entry, pattern_action and pattern_text, then re-run the verb`
	decisionWayFromKeys  = `way forward: set pattern_entry and pattern_action together, and pattern_text for adds or changes, never for withdraws, then re-run the verb`
	decisionWayFromTwice = `way forward: give each entry of the --from file its own title and its own PATTERN entry, or record the second in a later call`

	decisionWayFabricCommit    = `way forward: run "lym fabric commit" to commit the record; a session lym refuses the verb from reports status: FAILED and the orch runs it`
	decisionWayStore           = `way forward: once the cause above is cleared, run "lym loom decision store" to copy the committed record onto the board entry`
	decisionWayAmendsPlanRetry = `way forward: the entry and the amendment record are restored and nothing landed; re-run "lym loom decision add ... --amends-plan"`
	decisionWayPlanUnparsed    = `way forward: run "lym loom validate-plan" to show the plan's defect, and record the decision without --amends-plan until the plan parses`
	decisionWayAmendsNoPlan    = `way forward: record the decision without --amends-plan; Plan-Write and Plan-Review read the decision record`
	decisionWayAmendsDarn      = `way forward: record the decision without --amends-plan`
	decisionWayNoticeTooLong   = "way forward: shorten the decision or rationale, put the detail in a file and name that file in the entry"
	decisionWayLock            = `way forward: re-run the verb; nothing was written`

	replaceWayRebuild   = `way forward: re-read the record, rebuild the --from file from it and re-run "lym loom decision replace" with the sha256 of the record you read as --base-sha256`
	replaceWayAdd       = `way forward: record the call with "lym loom decision add"`
	replaceWayFormat    = `way forward: fix each finding in the --from file, then re-run "lym loom decision replace"; the record is unchanged`
	replaceWayCarryOver = `way forward: re-read the record, copy every carry-over block of its "## Open risks" section into the --from file unchanged, then re-run "lym loom decision replace"; the record is unchanged`
	replaceWaySubset    = `way forward: re-read the record, copy each added entry the --from file keeps unchanged, record a new call with "lym loom decision add", and re-run "lym loom decision replace"; the record is unchanged`
	replaceWayRetry     = `way forward: re-run "lym loom decision replace"; the record is unchanged`
	replaceWayRestored  = `way forward: the record is restored and nothing landed; re-run "lym loom decision replace", never "lym fabric commit"`
	replaceWayTrailer   = `way forward: HEAD holds the file's bytes without the replace trailer; re-read the record, rebuild the --from file from it and re-run "lym loom decision replace", which lands the trailer commit`
	replaceWayTitle     = `way forward: shorten --title, then re-run "lym loom decision replace"`
)

// decisionWayRecordRestore returns the way forward for a run past its record row.
// It applies when the record is missing from the working copy but committed at HEAD.
// underLym is the record's path inside _lym.
func decisionWayRecordRestore(underLym string) string {
	return fmt.Sprintf(`way forward: restore the record from HEAD with "git -C _lym checkout HEAD -- %s", then re-run the verb`, underLym)
}

// decisionWayDiffers returns the way forward for a record whose working copy differs from its committed copy.
// underLym is the record's path inside _lym.
func decisionWayDiffers(underLym string) string {
	return fmt.Sprintf(`way forward: when the working record differs from the committed one only by the entries or PATTERN bullets a failed call's commit-failed refusal reported, run "lym fabric commit" under _lym, which records that call's entries, then re-run the refused call alone, never the failed one; otherwise restore the record to its committed copy with "git -C _lym checkout HEAD -- %s", then re-run the verb`, underLym)
}

// errPlanUnparsed marks an approval read that failed because the plan does not parse.
// A failed read of a file does not return this error.
var errPlanUnparsed = errors.New("the plan does not parse")

// decisionWayNoRecord is the way forward when a Discussion-Write run has no decision record.
const decisionWayNoRecord = `way forward: the record exists once Discussion-Write has written it; "lym loom status <slug>" shows whether the run has reached that row, and "lym loom start" begins or resumes the run`

// decisionInput holds the flag values of one `decision add` call.
// The three pattern fields are the optional PATTERN-change form and are all empty without it.
// from names the batch file whose entries replace the per-entry fields, and is empty for a one-entry call.
// amendsPlan marks a decision that amends the approved plan, so the call also writes the plan amendment record.
type decisionInput struct {
	by, title, decision, rationale           string
	patternEntry, patternAction, patternText string
	from                                     string
	amendsPlan                               bool
}

// replaceInput holds the flag values of one `decision replace` call.
// from names the file whose bytes become the record, and baseSHA256 the hex sha256 of the record the caller read.
type replaceInput struct {
	by, from, title, baseSHA256 string
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

// patternFormBroken reports whether an entry's PATTERN-change fields are partial, or carry text beside a withdrawal.
func patternFormBroken(entry, action, text string) bool {
	return (entry != "") != (action != "") || (entry == "" && text != "") ||
		(strings.TrimSpace(action) == discussionparser.PatternActionWithdraws && text != "")
}

// decisionEntries returns the entries input carries, or the refusal text when they are not a well-formed set.
// A one-entry call refuses an empty title, decision or rationale, and a partial PATTERN-change form.
// A --from call refuses any per-entry flag beside the file.
// It refuses a file that is unreadable, is not one JSON array of entries with nothing after it, or holds no entry.
// It refuses an entry with an empty title, decision or rationale, or a partial PATTERN-change form.
// It also refuses an unknown key, and two entries that share a title or a PATTERN entry ID.
func decisionEntries(input decisionInput) ([]decisionEntry, string) {
	if input.from == "" {
		for _, flag := range []struct{ name, value string }{{"title", input.title}, {"decision", input.decision}, {"rationale", input.rationale}} {
			if strings.TrimSpace(flag.value) == "" {
				return nil, fmt.Sprintf("--%s is empty; way forward: pass --%s with the text to record", flag.name, flag.name)
			}
		}
		if patternFormBroken(input.patternEntry, input.patternAction, input.patternText) {
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
		if patternFormBroken(entry.PatternEntry, entry.PatternAction, entry.PatternText) {
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

// unknownEntryKey returns the 1-based number of the first entry in data with an undefined key, and that key.
// It returns 0 and an empty key when every key is defined, or when data is not an array of objects.
// The check is its own pass because the entries decode must stay tolerant of unknown keys.
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

// decisionDeps holds every side effect of a decision call, injected so that tests can supply fakes.
type decisionDeps struct {
	// strandName is the full name reed exported to the calling session, empty for a shell reed did not spawn.
	strandName string
	// programName resolves the hub's full LOOM program name, '<shortname>:LOOM', which sends notices as the program.
	programName func() (string, error)
	// parent resolves the nearest live orch up the beam chain, which receives the run's notices, for a named caller only.
	parent func() (hubgeom.Parent, error)
	// arbiterStrand reports whether name is the target run's own arbiter strand, called only for a named caller.
	arbiterStrand func(name string) bool
	// samplerStrand reports whether name is the strand the Sampler-Session row recorded, while the run stands at that row.
	samplerStrand func(name string) (bool, error)
	// readStatus reads the target worktree's default run status; found is false when the run has no status file.
	readStatus reviewStatusReader
	// recordPath is the decision record the entry lands in, reported on the success envelope.
	recordPath string
	// recordUnderLym is the record's path inside the _lym directory, which the restore way forward names.
	recordUnderLym string
	// recordRel is the record's anchor-relative, slash-separated path, which a replace's trailer names.
	recordRel string
	// lockRecord takes the run's decision lock, waiting while another holder has it, and returns its release.
	lockRecord func() (release func() error, err error)
	// writeRecord writes data as the decision record's working copy.
	writeRecord func(data []byte) error
	// commitReplace commits the record alone with msg and trailers; committed is false when nothing was staged.
	commitReplace func(msg string, trailers []fabricengine.Trailer) (committed bool, err error)
	// commitEmpty commits the record's path alone with msg and trailers, landing an empty commit when it is unchanged.
	commitEmpty func(msg string, trailers []fabricengine.Trailer) error
	// headCarries reports whether HEAD is a commit whose replace trailer carries value.
	headCarries func(value string) (bool, error)
	// implementerSpan reports whether the run at currentRow stands past its plan's approval.
	// It answers false past the recipe's last Webster row.
	implementerSpan func(currentRow string) (bool, error)
	// boardRecordStored reports whether the run's board entry carries a stored decision record.
	// It answers false for a run whose record is a beam's project record.
	boardRecordStored func() (bool, error)
	// append adds the decisions in one write, re-runs the discussion check, and restores the record on a finding or error.
	append func(ds []discussionparser.AddedDecision) ([]discussionparser.Finding, error)
	// commit commits the discussion artifacts, the record included.
	commit func() error
	// now is the clock the entry's date comes from.
	now func() time.Time
	// hasPlan reports whether the target's recipe has a Plan-Write row, so a plan exists to amend.
	hasPlan func() (bool, error)
	// rowWritten reports whether the target's recipe writes the decision record in its Record-Write row.
	rowWritten func() (bool, error)
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
	// pastApproval reports whether the run at currentRow (empty if no status file) has passed Discussion approval.
	pastApproval func(currentRow string) bool
	// storeRecord copies record onto the run's board entry.
	storeRecord func(record []byte) error
	// approvalRow reports whether the run's recipe has a Discussion approval row.
	approvalRow func() bool
	// raddleFileRow answers the name of the recipe's RaddleFile row, and false when the recipe has none.
	raddleFileRow func() (string, bool, error)
	// raddlePerch reports whether the run's raddle state records perch mode.
	raddlePerch func() (bool, error)
	// raddleFiled returns the slugs the run's raddle filing put on the board, and false while the run has filed none.
	raddleFiled func() ([]string, bool, error)
	// raddleEntryTask reports whether the run's board entry is still a task; an absent entry counts as one.
	raddleEntryTask func() (bool, error)
	// raddleBoardFiling returns the part slugs of the run's raddle proposal.
	// It also reports whether the board holds every one of them as filed from the run.
	// An absent or unparseable proposal holds no parts.
	raddleBoardFiling func() ([]string, bool, error)
}

// decisionNoticeError is one notice the verb could not send, reported on its success envelope.
// Strand is empty when the live runs could not be listed.
type decisionNoticeError struct {
	Strand string `json:"strand"`
	Error  string `json:"error"`
}

// decisionNoticeOpening opens every notice of an `add` call.
// decisionReplaceNoticeOpening opens every notice of a `replace` call.
// The parent directive stencil names both.
// That lets a spawned role tell the notice from any other message signed as the LOOM program.
const (
	decisionNoticeOpening        = "Parent decision recorded with lym loom decision add"
	decisionReplaceNoticeOpening = "Parent decision record replaced with lym loom decision replace"
)

// decisionReplaceNoticeLine is the notice a live agent of the run gets once a replace wrote the record at recordPath.
// title is the replace's title.
// Every agent re-reads the record whole, since the replace may have changed or dropped any part of it.
func decisionReplaceNoticeLine(recordPath, title string) string {
	return fmt.Sprintf("%s, titled %q, in the decision record %s. Act on it as your parent's instruction: re-read the record whole and follow it.",
		decisionReplaceNoticeOpening, strings.TrimSpace(title), recordPath)
}

// decisionNoticeLine is the notice a live agent of the run gets for the entries added in the record at recordPath.
// A one-entry notice carries each entry's decision and rationale.
// So does every notice with inline set, since a role such as Merriam may not read the record.
// An agent that may read the record re-reads its Added after Discussion entries;
// the committed record is the authority.
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

// noticeFits reports whether line fits one creel hook delivery under every sender name a notice may be posted as:
// the calling strand, when named, and the hub's LOOM program.
func noticeFits(deps decisionDeps, line string) bool {
	if deps.strandName != "" && !creelengine.LineFits(deps.strandName, line) {
		return false
	}
	programName, err := deps.programName()
	if err != nil {
		// The post as the program fails on the same geometry and is reported per strand.
		return true
	}
	return creelengine.LineFits(programName, line)
}

// decisionNoticeInline reports whether the live run's strand named strand reads no decision record.
// Its notice then carries every entry's text.
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

// decisionNoticeHeldBack is one live strand the verb did not wake, reported on its success envelope.
type decisionNoticeHeldBack struct {
	Strand string `json:"strand"`
	Reason string `json:"reason"`
}

// heldBackReason returns why the live strand named strand gets no notice from a run standing in state halted.
// It returns empty when the strand gets one.
// halted is empty for a running run or one with no status file.
// A Webster Master session of a halted run is left over from a run that ended:
// woken, it would drive the run's bracket verbs with no `lym webster run` owning them.
// The fresh session a re-entry starts carries the decision in its prompt instead.
func heldBackReason(strand string, halted shedengine.State) string {
	if halted == "" {
		return ""
	}
	name, err := agentname.Parse(strand)
	if err != nil || !agentname.MatchesRole(name.Role, websterengine.MerriamStrandRole) {
		return ""
	}
	return fmt.Sprintf("the run is %s, so this Webster session's run has ended and waking it would start work no run owns; the fresh session the run's re-entry starts carries the decision", halted)
}

// addedNoticeLines returns the notice lines the live strand named strand gets for the entries added.
// A batch whose one notice would not fit creel's line goes as one line per entry.
func addedNoticeLines(deps decisionDeps, added []discussionparser.AddedDecision, strand string) []string {
	lines := []string{decisionNoticeLine(deps.recordPath, added, decisionNoticeInline(strand))}
	if len(added) > 1 && !noticeFits(deps, lines[0]) {
		// Each one-entry notice fits, which decisionVerb checked before it recorded anything.
		lines = lines[:0]
		for _, d := range added {
			lines = append(lines, decisionNoticeLine(deps.recordPath, []discussionparser.AddedDecision{d}, true))
		}
	}
	return lines
}

// notifyLiveRuns posts the notice linesFor renders to every live run of the target worktree the notice is for.
// halted is the run's state when it is not running, and empty otherwise; heldBackReason picks the strands it keeps from.
// It returns the strands posted to, the strands held back, and the failures, each logged at Warn;
// a failure never stops the next post.
func notifyLiveRuns(deps decisionDeps, halted shedengine.State, linesFor func(strand string) []string) ([]string, []decisionNoticeHeldBack, []decisionNoticeError) {
	notified := []string{}
	heldBack := []decisionNoticeHeldBack{}
	failures := []decisionNoticeError{}
	strands, err := deps.liveStrands()
	if err != nil {
		logger.Warn("loom: a decision call could not list the run's live agents; no notice sent", "error", err)
		return notified, heldBack, append(failures, decisionNoticeError{Error: "list the run's live agents: " + err.Error()})
	}
	for _, strand := range strands {
		if !decisionNoticeReceiver(strand, deps.strandName) {
			continue
		}
		if reason := heldBackReason(strand, halted); reason != "" {
			logger.Warn("loom: a decision call held its notice back from a live agent of a halted run", "strand", strand, "state", string(halted))
			heldBack = append(heldBack, decisionNoticeHeldBack{Strand: strand, Reason: reason})
			continue
		}
		var err error
		for _, line := range linesFor(strand) {
			if err = deps.notify(strand, line); err != nil {
				break
			}
		}
		if err != nil {
			logger.Warn("loom: a decision call could not notify a live agent", "strand", strand, "error", err)
			failures = append(failures, decisionNoticeError{Strand: strand, Error: err.Error()})
			continue
		}
		notified = append(notified, strand)
	}
	return notified, heldBack, failures
}

// recordWritePending reports whether the run's current row is Record-Write or an earlier one, so Record-Write has yet to finish.
// An empty row, a run with no status file, is such a run.
func recordWritePending(currentRow string) bool {
	return currentRow == "" || currentRow == loomshed.NamePreflight || currentRow == loomshed.NameRecordWrite
}

// checkDarnRecord returns the refusal text for a Record-Write run's record that is not safe to append to, or empty when it is.
// The record is safe once Record-Write has committed it and nothing touched it since:
// it exists, is committed, equals its committed copy and holds every added entry its history held.
// Each refusal names the way forward by currentRow, the run's current row, empty for a run with no status file.
// A failed read of the record, its committed copy or its history is returned as an error.
func checkDarnRecord(deps decisionDeps, currentRow string) (string, error) {
	working, err := deps.readRecord()
	if errors.Is(err, fs.ErrNotExist) {
		return missingRecordRefusal(deps, currentRow)
	}
	if err != nil {
		return "", err
	}
	committed, found, err := deps.committed()
	if err != nil {
		return "", err
	}
	if !found {
		if recordWritePending(currentRow) {
			return "the decision record is not committed at HEAD; " + decisionWayUncommitted, nil
		}
		return "the decision record is not committed at HEAD; " + decisionWayUncommittedGoto, nil
	}
	if !bytes.Equal(working, committed) {
		return "the decision record differs from its committed copy; " + decisionWayDiffers(deps.recordUnderLym), nil
	}
	versions, err := deps.history()
	if err != nil {
		return "", err
	}
	if findings := discussionparser.CheckAddedDecisionsKept(working, versions); len(findings) > 0 {
		return "the decision record is missing an entry its history held: " + strings.Join(renderFindings(findings), "; ") + "; " + decisionWayEntryMissing, nil
	}
	return "", nil
}

// missingRecordRefusal returns the refusal text for a Record-Write run whose decision record is absent from the working copy.
// A run past Record-Write is sent to restore the record from HEAD when HEAD holds one, else to redo the row.
func missingRecordRefusal(deps decisionDeps, currentRow string) (string, error) {
	prefix := fmt.Sprintf("the run has no decision record at %s; ", deps.recordPath)
	switch {
	case currentRow == "":
		return prefix + decisionWayRecordStart, nil
	case recordWritePending(currentRow):
		return prefix + decisionWayRecordResume, nil
	}
	_, found, err := deps.committed()
	if err != nil {
		return "", err
	}
	if found {
		return prefix + decisionWayRecordRestore(deps.recordUnderLym), nil
	}
	return prefix + decisionWayRecordGoto, nil
}

// committedLacksEntry returns the heading of the first entry of added that HEAD's record does not hold.
// An entry is held when the committed record has an `Added after Discussion` entry with the same heading.
// It returns empty when every entry is held, and the first entry when HEAD holds no record.
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

// boardCopyDue reports whether a decision call on the run standing at currentRow refreshes the board copy of its record.
// A run past its Discussion approval refreshes it.
// So does a run whose recipe has no Discussion approval row when its board entry carries a stored record.
// Such a run is a raddle run of a promoted Sketch or a darn part a raddle run filed; a beam's project record never is.
// A run of a recipe with an approval row keeps the past-approval gate alone.
// A run sent back into its Discussion segment therefore copies nothing.
func boardCopyDue(deps decisionDeps, currentRow string) (bool, error) {
	if deps.pastApproval(currentRow) {
		return true, nil
	}
	if deps.approvalRow() {
		return false, nil
	}
	return deps.boardRecordStored()
}

// copyRecordToBoard copies the committed decision record onto the run's board entry when boardCopyDue says so.
// It returns "" when no copy is due or the copy succeeded.
// Otherwise it returns the text the success envelope reports as record_store_error.
// A done entry's refusal reports that the entry keeps the record it last stored, with no pointer at the store verb.
// Any other failure, a missing committed record included, reports the error and the way forward that names the store verb.
// Every failure is logged at Warn, and none fails the verb, since the record is already committed.
func copyRecordToBoard(deps decisionDeps, currentRow string) string {
	due, err := boardCopyDue(deps, currentRow)
	if err == nil && !due {
		return ""
	}
	var committed []byte
	if err == nil {
		var found bool
		committed, found, err = deps.committed()
		if err == nil && !found {
			err = errors.New("HEAD holds no decision record")
		}
	}
	if err == nil {
		err = deps.storeRecord(committed)
	}
	switch {
	case err == nil:
		return ""
	case errors.Is(err, boardengine.ErrRecordEntryDone):
		logger.Warn("loom: a decision call did not copy the record: the board entry has landed", "error", err)
		return "the entry has landed and keeps the record it last stored"
	}
	logger.Warn("loom: a decision call could not copy the record onto the board entry", "error", err)
	return err.Error() + "; " + decisionWayStore
}

// decisionLockSuffix ends the file name of a decision record's per-run decision lock.
const decisionLockSuffix = ".decision.lock"

// holdDecisionLock takes the per-run decision lock of the record at recordPath, waiting while another holder has it.
// Every Go writer of the record past approval holds it across its checks, its write and its commit,
// so a check of the record and the write it guards are one step against every other such writer.
// The lock file sits in the never-tracked lock directory of the checkout that holds the record.
func holdDecisionLock(recordPath string) (release func() error, err error) {
	path := lock.TrackedFileLockPath(recordPath, decisionLockSuffix)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("take the run's decision lock: %w", err)
	}
	held, err := lock.AcquireWriteLock(path)
	if err != nil {
		return nil, fmt.Errorf("take the run's decision lock: %w", err)
	}
	return held.Release, nil
}

// decisionCallerRefusal returns the refusal text for a caller that may not make a decision call labelled by.
// It returns empty when the caller may.
// The operator's own unnamed shell always may.
// The arbiter label is the run's own arbiter strand's alone.
// The run's sampler strand passes only with the operator label.
// Any other named caller must be the run's parent orch.
func decisionCallerRefusal(deps decisionDeps, by string) string {
	if by == decisionByArbiter {
		if deps.strandName == "" || !deps.arbiterStrand(deps.strandName) {
			return fmt.Sprintf("the caller %q is not the run's arbiter; %s", deps.strandName, decisionWayArbiter)
		}
		return ""
	}
	if deps.strandName == "" {
		return ""
	}
	sampler, samplerErr := deps.samplerStrand(deps.strandName)
	if sampler && samplerErr == nil {
		if by != decisionByOperator {
			return fmt.Sprintf("the caller %q is the run's sampler session, which records only what the operator settled; %s", deps.strandName, decisionWaySampler)
		}
		return ""
	}
	// An unreadable sampler record refuses only a caller that is not the run's parent, whose call never depended on it.
	samplerRead := fmt.Sprintf("read the run's sampler session: %s; %s", samplerErr, decisionWayRetry)
	parent, err := deps.parent()
	if err != nil {
		if samplerErr != nil {
			return samplerRead
		}
		return fmt.Sprintf("the caller %q is a named session; way forward: the run's parent could not be resolved (%s); the operator's own unnamed shell may still run the verb", deps.strandName, err)
	}
	switch {
	case deps.strandName == parent.Name:
		return ""
	case samplerErr != nil:
		return samplerRead
	case parent.Name == "":
		return fmt.Sprintf("the caller %q is a task session; way forward: a design call after the Discussion is the operator's to record; the operator's own unnamed shell may always run it", deps.strandName)
	}
	return fmt.Sprintf("the caller %q is a task session; way forward: a design call after the Discussion is the parent's to record; ask %s to run the verb; the operator's own unnamed shell may always run it", deps.strandName, parent.Name)
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
// The restored record is committed when a status commit already landed this call's amendment bytes.
// That keeps HEAD from holding a title whose entry did not land.
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
// It reports landed when HEAD holds both this call's amendment record and every entry,
// so the commit went through despite its error.
// Otherwise it returns the refusal text.
// Both files are kept when HEAD holds the amendment record alone or cannot be read;
// they are restored when HEAD differs.
// wayCommit is the way forward the refusals that keep the files end in.
func (a *amendmentCall) afterFailedCommit(deps decisionDeps, added []discussionparser.AddedDecision, commitErr error, wayCommit string) (landed bool, refusal string) {
	failed := fmt.Sprintf("the entry is appended but the commit failed: %s", commitErr)
	committed, found, err := deps.committedAmendment()
	if err != nil {
		return false, fmt.Sprintf("%s; the amendment record at HEAD could not be read: %s; %s", failed, err, wayCommit)
	}
	if found && bytes.Equal(committed, a.written) {
		lacking, err := committedLacksEntry(deps, added)
		switch {
		case err != nil:
			return false, fmt.Sprintf("%s; the decision record at HEAD could not be read: %s; %s", failed, err, wayCommit)
		case lacking == "":
			return true, ""
		}
		return false, fmt.Sprintf("%s; a status commit landed the amendment record without the entry; %s", failed, wayCommit)
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

// decisionVerb appends the decisions in input to the record and commits them in one commit.
// A refusal carries a way forward.
//
// With amendsPlan the verb also appends each entry's title to the plan amendment record.
// The title is committed with the entries.
// It is refused on a recipe with no Plan-Write row and for a plan that is not approved,
// and a failed append or commit puts both records back.
// The optional PATTERN-change form appends one bullet beside its entry, in the same write.
// The bullet follows the record's PATTERN-change shape.
// It passes through the verb's existing checks and its --by parent|operator|arbiter caller guards.
// Only the run's parent, the operator's own shell or the run's own arbiter therefore declares a change after Discussion.
// The form rewrites only the record's bullet for the same entry ID, which its new bullet replaces.
// Its `withdraws` action strikes that bullet instead, and is refused when the record declares none.
// A perch-mode raddle run standing at its RaddleFile row is refused, since no split reads a decision recorded there.
// So is a raddle run whose filing is done.
// The call holds the run's decision lock from its first precondition through its commit and board copy.
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
	if refusal := decisionCallerRefusal(deps, input.by); refusal != "" {
		return refuse("%s", refusal)
	}

	release, err := deps.lockRecord()
	if err != nil {
		return refuse("%s; %s", err, decisionWayLock)
	}
	defer func() { _ = release() }()

	st, found, err := deps.readStatus()
	if err != nil {
		return refuse("read the run status: %s; %s", err, decisionWayRetry)
	}
	if found && st.State == shedengine.StateRunning && st.CurrentProducer == loomshed.NameDiscussionWrite {
		return refuse("%s is running and owns the decision record; %s", loomshed.NameDiscussionWrite, decisionWayWriter)
	}
	currentRow := ""
	if found {
		currentRow = st.CurrentProducer
	}
	filingRefusal, err := raddleFilingRefusal(deps, currentRow)
	if err != nil {
		return refuse("read the run's raddle filing: %s; %s", err, decisionWayRetry)
	}
	if filingRefusal != "" {
		return refuse("%s", filingRefusal)
	}
	// A run whose board entry holds a copy of the record has that copy to refresh once the commit lands.
	wayCommit := decisionWayFabricCommit
	if due, _ := boardCopyDue(deps, currentRow); due {
		wayCommit += `, then "lym loom decision store" once "lym fabric commit" has landed the record`
	}

	hasPlan, err := deps.hasPlan()
	if err != nil {
		return refuse("read the run's recipe: %s; %s", err, decisionWayRetry)
	}
	if input.amendsPlan && !hasPlan {
		return refuse("--amends-plan has no plan to amend on this run's recipe, which has no Plan-Write row; %s", decisionWayAmendsDarn)
	}
	rowWritten, err := deps.rowWritten()
	if err != nil {
		return refuse("read the run's recipe: %s; %s", err, decisionWayRetry)
	}
	if rowWritten {
		refusal, err := checkDarnRecord(deps, currentRow)
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
	withdrawn := []string{}
	for _, entry := range entries {
		d := discussionparser.AddedDecision{
			By:        input.by,
			Title:     entry.Title,
			Decision:  entry.Decision,
			Rationale: entry.Rationale,
			Date:      date,
		}
		switch id, action := strings.TrimSpace(entry.PatternEntry), strings.TrimSpace(entry.PatternAction); {
		case action == discussionparser.PatternActionWithdraws:
			d.WithdrawnPattern = id
			withdrawn = append(withdrawn, id)
		case id != "":
			change := discussionparser.PatternChange{ID: id, Action: action, Text: strings.TrimSpace(entry.PatternText)}
			d.PatternChange = &change
			patternBullets = append(patternBullets, change.Bullet())
		}
		added = append(added, d)
		titles = append(titles, strings.TrimSpace(entry.Title))
		headings = append(headings, addedDecisionHeading(d))
	}
	for i, d := range added {
		if !noticeFits(deps, decisionNoticeLine(deps.recordPath, []discussionparser.AddedDecision{d}, true)) {
			return refuse("the decision notice for %q would pass the line creel delivers in one hook field; "+decisionWayNoticeTooLong, headings[i])
		}
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
	case errors.Is(err, discussionparser.ErrPatternBulletAbsent):
		return refuse("%s; %s", err, decisionWayWithdraw)
	case err != nil:
		return refuse("%s; %s", err, decisionWayRetry)
	case len(findings) > 0:
		return refuse("the discussion check flags the record after the append: %s; %s", strings.Join(renderFindings(findings), "; "), decisionWayFindings)
	}
	commitErr := deps.commit()
	if commitErr != nil && amendment == nil {
		return refuse("the entry is appended but the commit failed: %s; %s", commitErr, wayCommit)
	}
	if commitErr != nil {
		landed, refusal := amendment.afterFailedCommit(deps, added, commitErr, wayCommit)
		if !landed {
			return refuse("%s", refusal)
		}
	}
	if rowWritten {
		lacking, err := committedLacksEntry(deps, added)
		if err != nil {
			return refuse("the committed record could not be read: %s; %s", err, decisionWayEntryLost)
		}
		if lacking != "" {
			return refuse("the committed record holds no entry under %q; %s", lacking, decisionWayEntryLost)
		}
	}
	recordStoreError := copyRecordToBoard(deps, currentRow)
	var halted shedengine.State
	if found && st.State != shedengine.StateRunning {
		halted = st.State
	}
	notified, heldBack, noticeErrors := notifyLiveRuns(deps, halted, func(strand string) []string {
		return addedNoticeLines(deps, added, strand)
	})
	result := map[string]any{
		"slug":          slug,
		"record":        deps.recordPath,
		"headings":      headings,
		"notified":      notified,
		"notice_errors": noticeErrors,
	}
	if len(heldBack) > 0 {
		result["notice_held_back"] = heldBack
	}
	if recordStoreError != "" {
		result["record_store_error"] = recordStoreError
	}
	if len(patternBullets) > 0 {
		result["pattern_changes"] = patternBullets
	}
	if len(withdrawn) > 0 {
		result["pattern_withdrawn"] = withdrawn
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

// raddleFilingRefusal returns the refusal text for a raddle run whose filing bars a decision call.
// It returns empty for any other run.
// A perch-mode run standing at its RaddleFile row before its filing is refused:
// the decision still reaches a new split through a goto to Raddle-Write.
// Once the filing is done, a call is refused in every mode and at any row, naming the filed slugs.
// The decision belongs on the filed part it concerns.
// A family filing made the run's entry the family's project with a record of its own, which a board copy would overwrite.
// The filing counts as done when the raddle store records it or when the run's entry is no longer a task.
// It also counts as done when the board holds every part of the run's proposal as filed from the run.
// In the last two cases the store lacks its done marker.
// The way forward then resumes the run first, which finishes the filing.
func raddleFilingRefusal(deps decisionDeps, currentRow string) (string, error) {
	row, hasRow, err := deps.raddleFileRow()
	if err != nil || !hasRow {
		return "", err
	}
	perch, err := deps.raddlePerch()
	if err != nil {
		return "", err
	}
	filed, done, err := deps.raddleFiled()
	if err != nil {
		return "", err
	}
	resumeFirst := ""
	if !done {
		isTask, err := deps.raddleEntryTask()
		if err != nil {
			return "", err
		}
		parts, held, err := deps.raddleBoardFiling()
		if err != nil {
			return "", err
		}
		if isTask && !held {
			if perch && currentRow == row {
				return fmt.Sprintf(`the run stands at %s in perch mode, filing the proposal its split perch approved, so no split reads a decision recorded now; way forward: run "lym loom goto --to %s", re-run this verb, then run "lym loom resume", which splits again with the decision recorded first`, row, loomshed.NameRaddleWrite), nil
			}
			return "", nil
		}
		filed = parts
		resumeFirst = `run "lym loom resume" on this run first, which finishes the filing and writes its done marker; then, `
	}
	names := strings.Join(filed, ", ")
	if names == "" {
		names = "its parts"
	}
	partCall := `run "lym loom decision add <part-slug>" or "lym loom decision replace <part-slug>" on that part`
	if perch {
		return fmt.Sprintf(`the run in perch mode has filed %s, so no split reads a decision recorded on its entry; way forward: %sonce the operator has released the part the decision concerns and its run has passed its record row, %s`, names, resumeFirst, partCall), nil
	}
	return fmt.Sprintf(`the run has filed %s, and its family filing made the run's entry the family's project with a record of its own, which a call on this run would overwrite; way forward: %sfor a call that concerns one part, once the operator has released it and its run has passed its record row, %s; for a call that concerns the whole family, run "lym board upsert <project-slug> --record-file <file>" on the family's project`, names, resumeFirst, partCall), nil
}

// addedDecisionHeading is the heading of a decision added after a Discussion, without its leading marker.
func addedDecisionHeading(d discussionparser.AddedDecision) string {
	return fmt.Sprintf("Added after Discussion (%s, %s): %s", strings.TrimSpace(d.By), d.Date.UTC().Format("2006-01-02"), strings.TrimSpace(d.Title))
}

// decisionStoreRetry is the way forward of a store refusal that clearing its cause fixes.
const decisionStoreRetry = `way forward: re-run "lym loom decision store"`

// decisionStoreVerb copies the committed decision record onto the run's board entry, or refuses with a way forward.
// It admits the operator's own unnamed shell, the run's own arbiter strand and the run's parent orch.
// It is admitted on the runs whose decision calls refresh the board copy:
// a run past its Discussion approval, or one whose recipe has no approval row and whose entry carries a stored record.
// It refuses a raddle run whose filing is done.
// It changes nothing in the run: no commit, no routing and no notice.
func decisionStoreVerb(out io.Writer, slug string, deps decisionDeps) int {
	refuse := func(format string, args ...any) int {
		return output.Err(out, fmt.Sprintf("loom: %s store: ", decisionGroup)+fmt.Sprintf(format, args...))
	}

	if deps.strandName != "" && !deps.arbiterStrand(deps.strandName) {
		parent, err := deps.parent()
		switch {
		case err != nil:
			return refuse("the caller %q is a named session; way forward: the run's parent could not be resolved (%s); the operator's own unnamed shell may still run the verb", deps.strandName, err)
		case deps.strandName != parent.Name && parent.Name == "":
			return refuse(`the caller %q is a task session; way forward: the operator's own unnamed shell may always run "lym loom decision store"`, deps.strandName)
		case deps.strandName != parent.Name:
			return refuse(`the caller %q is a task session; way forward: ask %s to run "lym loom decision store"; the operator's own unnamed shell may always run it`, deps.strandName, parent.Name)
		}
	}

	release, err := deps.lockRecord()
	if err != nil {
		return refuse("%s; %s", err, decisionStoreRetry)
	}
	defer func() { _ = release() }()

	st, found, err := deps.readStatus()
	if err != nil {
		return refuse("read the run status: %s; %s", err, decisionStoreRetry)
	}
	currentRow := ""
	if found {
		currentRow = st.CurrentProducer
	}
	filingRefusal, err := raddleFilingRefusal(deps, currentRow)
	if err != nil {
		return refuse("read the run's raddle filing: %s; %s", err, decisionStoreRetry)
	}
	if filingRefusal != "" {
		return refuse("%s", filingRefusal)
	}
	if !deps.approvalRow() {
		stored, err := deps.boardRecordStored()
		if err != nil {
			return refuse("read the run's board entry: %s; %s", err, decisionStoreRetry)
		}
		if !stored {
			return refuse("the run's recipe has no Discussion approval and its board entry carries no stored record, so its record is never stored on the board; way forward: read the record in the run's worktree at %s", deps.recordPath)
		}
	} else if !deps.pastApproval(currentRow) {
		return refuse(`the run has not passed its Discussion approval, which stores the record itself; way forward: let the run settle Discussion-Review, by "lym loom resume" or, when it waits on an escalation, by answering it with the "lym loom circling" verb its escalation names, then re-run this verb if "lym loom status" shows it past Discussion-Bouncer without a stored record`)
	}

	committed, committedFound, err := deps.committed()
	if err != nil {
		return refuse("read the committed decision record: %s; %s", err, decisionStoreRetry)
	}
	if !committedFound {
		return refuse(`the run has no committed decision record; way forward: run "lym fabric commit" to commit it, then re-run "lym loom decision store"; a session lym refuses "lym fabric commit" from reports status: FAILED and the orch runs it`)
	}
	err = deps.storeRecord(committed)
	switch {
	case errors.Is(err, boardengine.ErrRecordEntryDone):
		return refuse("the entry has landed and keeps the record it last stored; nothing to store")
	case err != nil:
		return refuse("%s; %s", err, decisionStoreRetry)
	}
	return output.Ok(out, map[string]any{"slug": slug, "record": deps.recordPath, "stored": true})
}

// sha256Hex returns the lower-case hex sha256 of data.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// replaceFileRefusal returns the refusal text for a --from file that may not replace the record committed as current.
// It returns empty when the file may.
// The file must pass the record's section check and its decision-id and acceptance check.
// It must keep every carry-over block byte for byte, and each of its added entries must equal one of current's.
// It may drop added entries and appended PATTERN bullets.
func replaceFileRefusal(current, file []byte) string {
	findings := append(discussionparser.CheckRecordSections(file), discussionparser.CheckDecisionIDs(file)...)
	if len(findings) > 0 {
		return "the --from file fails the decision record's format check: " + strings.Join(renderFindings(findings), "; ") + "; " + replaceWayFormat
	}
	if findings := discussionparser.CheckCarryOversKept(current, file); len(findings) > 0 {
		return "the --from file changes the record's carry-over blocks, which only Go writes: " + strings.Join(renderFindings(findings), "; ") + "; " + replaceWayCarryOver
	}
	if findings := discussionparser.CheckAddedEntriesSubset(current, file); len(findings) > 0 {
		return "the --from file inserts or rewrites an added entry: " + strings.Join(renderFindings(findings), "; ") + "; " + replaceWaySubset
	}
	return ""
}

// decisionReplaceVerb writes the --from file as the run's decision record and commits it under a replace trailer.
// A refusal carries a way forward.
// It takes the caller guards of decisionVerb and is refused on every run decisionVerb refuses.
// It holds the run's decision lock from its first precondition through its board copy.
// It is also refused before a Discussion approval the recipe has.
// It is refused between the plan's approval and the last Webster row too.
// Each fresh Webster prompt there renders only added entries.
// The record committed at HEAD must hash to --base-sha256 and equal the working copy, so no concurrent write is lost.
// The file must pass replaceFileRefusal; a refused file leaves the record as it was.
// The commit carries the Lym-Record-Replaced trailer, and when HEAD already holds the bytes an empty commit carries it,
// so the record's history check reads versions from this replace on.
// A failed commit restores the prior record only when HEAD lacks the written bytes.
// Each live agent of the run is told to re-read the record whole.
func decisionReplaceVerb(out io.Writer, slug string, deps decisionDeps, input replaceInput) int {
	refuse := func(format string, args ...any) int {
		return output.Err(out, fmt.Sprintf("loom: %s replace: ", decisionGroup)+fmt.Sprintf(format, args...))
	}

	if input.by != decisionByParent && input.by != decisionByOperator && input.by != decisionByArbiter {
		return refuse("--by is %q; way forward: pass --by %s or --by %s", input.by, decisionByParent, decisionByOperator)
	}
	for _, flag := range []struct{ name, value string }{{"from", input.from}, {"title", input.title}, {"base-sha256", input.baseSHA256}} {
		if strings.TrimSpace(flag.value) == "" {
			return refuse("--%s is empty; way forward: pass --from with the record file to write, --title naming the change and --base-sha256 with the sha256 of the record you read", flag.name)
		}
	}
	file, err := os.ReadFile(input.from)
	if err != nil {
		return refuse("read the --from file: %s; way forward: pass the path of a readable record file, then re-run the verb", err)
	}
	if refusal := decisionCallerRefusal(deps, input.by); refusal != "" {
		return refuse("%s", refusal)
	}

	release, err := deps.lockRecord()
	if err != nil {
		return refuse("%s; %s", err, decisionWayLock)
	}
	defer func() { _ = release() }()

	st, found, err := deps.readStatus()
	if err != nil {
		return refuse("read the run status: %s; %s", err, replaceWayRetry)
	}
	if found && st.State == shedengine.StateRunning && st.CurrentProducer == loomshed.NameDiscussionWrite {
		return refuse("%s is running and owns the decision record; %s", loomshed.NameDiscussionWrite, decisionWayWriter)
	}
	currentRow := ""
	if found {
		currentRow = st.CurrentProducer
	}
	filingRefusal, err := raddleFilingRefusal(deps, currentRow)
	if err != nil {
		return refuse("read the run's raddle filing: %s; %s", err, replaceWayRetry)
	}
	if filingRefusal != "" {
		return refuse("%s", filingRefusal)
	}
	if deps.approvalRow() && !deps.pastApproval(currentRow) {
		return refuse("the run has not passed its Discussion approval, and before it the Discussion's own review folds each call into the main decisions; %s", replaceWayAdd)
	}
	inSpan, err := deps.implementerSpan(currentRow)
	if err != nil {
		return refuse("read the run's recipe: %s; %s", err, replaceWayRetry)
	}
	if inSpan {
		return refuse(`the run stands at %s, past its plan's approval and not past its last Webster row, where each fresh Webster prompt renders only the record's Added after Discussion entries, so a fold would hide a call from every Webster session spawned later; %s, whose notice reaches Webster inline, and fold it with "lym loom decision replace" once the run has left its Webster rows`, currentRow, replaceWayAdd)
	}
	rowWritten, err := deps.rowWritten()
	if err != nil {
		return refuse("read the run's recipe: %s; %s", err, replaceWayRetry)
	}
	if rowWritten {
		refusal, err := checkDarnRecord(deps, currentRow)
		if err != nil {
			return refuse("read the darn run's decision record: %s; %s", err, replaceWayRetry)
		}
		if refusal != "" {
			return refuse("%s", refusal)
		}
	}

	working, err := deps.readRecord()
	if errors.Is(err, fs.ErrNotExist) {
		return refuse("the run has no decision record at %s; %s", deps.recordPath, decisionWayNoRecord)
	}
	if err != nil {
		return refuse("read the decision record: %s; %s", err, replaceWayRetry)
	}
	committed, committedFound, err := deps.committed()
	if err != nil {
		return refuse("read the committed decision record: %s; %s", err, replaceWayRetry)
	}
	if !committedFound {
		return refuse(`the decision record is not committed at HEAD; way forward: run "lym fabric commit" to commit it, then %s`, strings.TrimPrefix(replaceWayRebuild, "way forward: "))
	}
	if sha256Hex(committed) != strings.ToLower(strings.TrimSpace(input.baseSHA256)) {
		return refuse("the decision record committed at HEAD is not the record --base-sha256 names, so it changed since it was read; %s", replaceWayRebuild)
	}
	if !bytes.Equal(working, committed) {
		return refuse("the decision record differs from its committed copy; %s", decisionWayDiffers(deps.recordUnderLym))
	}
	if refusal := replaceFileRefusal(committed, file); refusal != "" {
		return refuse("%s", refusal)
	}
	line := decisionReplaceNoticeLine(deps.recordPath, input.title)
	if !noticeFits(deps, line) {
		return refuse("the replace notice would pass the line creel delivers in one hook field; %s", replaceWayTitle)
	}

	if err := deps.writeRecord(file); err != nil {
		if restoreErr := deps.writeRecord(working); restoreErr != nil {
			return refuse("write the decision record: %s; restoring it failed too: %s; %s", err, restoreErr, decisionWayRestoreRecords)
		}
		return refuse("write the decision record: %s; %s", err, replaceWayRetry)
	}
	value := recordReplacedValue(input.by, deps.recordRel, file)
	trailers := []fabricengine.Trailer{{Key: recordReplacedTrailerKey, Value: value}}
	msg := fmt.Sprintf("loom: decision record replaced for %s: %s", slug, strings.TrimSpace(input.title))
	commitError, refusal := landReplace(deps, file, working, msg, trailers)
	if refusal != "" {
		return refuse("%s", refusal)
	}

	recordStoreError := copyRecordToBoard(deps, currentRow)
	var halted shedengine.State
	if found && st.State != shedengine.StateRunning {
		halted = st.State
	}
	notified, heldBack, noticeErrors := notifyLiveRuns(deps, halted, func(string) []string { return []string{line} })
	result := map[string]any{
		"slug":          slug,
		"record":        deps.recordPath,
		"title":         strings.TrimSpace(input.title),
		"sha256":        sha256Hex(file),
		"notified":      notified,
		"notice_errors": noticeErrors,
	}
	if len(heldBack) > 0 {
		result["notice_held_back"] = heldBack
	}
	if recordStoreError != "" {
		result["record_store_error"] = recordStoreError
	}
	if commitError != "" {
		result["commit_error"] = commitError
	}
	return output.Ok(out, result)
}

// landReplace commits the written record with msg and trailers, so HEAD ends at a commit carrying them.
// A commit that stages nothing, since HEAD already holds the bytes, is followed by an empty commit carrying the trailers.
// A commit that errors is judged by what HEAD holds afterwards:
// the written bytes in a commit carrying this call's trailer succeed, reporting the error as commitError;
// the bytes without that trailer get the empty trailer commit;
// and a HEAD without the bytes gets prior written back as the record.
// It returns the refusal text when the call fails.
func landReplace(deps decisionDeps, written, prior []byte, msg string, trailers []fabricengine.Trailer) (commitError, refusal string) {
	committed, err := deps.commitReplace(msg, trailers)
	if err == nil {
		if !committed {
			if err := deps.commitEmpty(msg, trailers); err != nil {
				return "", fmt.Sprintf("the record was written but its replace trailer commit failed: %s; %s", err, replaceWayTrailer)
			}
		}
		return "", ""
	}

	atHead, found, readErr := deps.committed()
	if readErr != nil {
		return "", fmt.Sprintf("the commit failed: %s; the record at HEAD could not be read: %s; %s", err, readErr, replaceWayRebuild)
	}
	if !found || !bytes.Equal(atHead, written) {
		if restoreErr := deps.writeRecord(prior); restoreErr != nil {
			return "", fmt.Sprintf("the commit failed: %s; restoring the record failed too: %s; %s", err, restoreErr, decisionWayRestoreRecords)
		}
		return "", fmt.Sprintf("the commit failed: %s; %s", err, replaceWayRestored)
	}
	carries, carriesErr := deps.headCarries(trailers[0].Value)
	if carriesErr == nil && carries {
		return err.Error(), ""
	}
	if carriesErr != nil {
		return "", fmt.Sprintf("the commit reported %s and HEAD's trailer could not be read: %s; %s", err, carriesErr, replaceWayTrailer)
	}
	if emptyErr := deps.commitEmpty(msg, trailers); emptyErr != nil {
		return "", fmt.Sprintf("the commit reported %s and the replace trailer commit failed: %s; %s", err, emptyErr, replaceWayTrailer)
	}
	return "", ""
}

// decisionState carries what the decision group's pre-run resolved for its verb.
type decisionState struct {
	deps    decisionDeps
	slug    string
	input   decisionInput
	replace replaceInput
}

// preRun resolves the invoking cwd, then the target worktree, then the verb's side effects.
func (s *decisionState) preRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == decisionGroup {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	location, err := lymcwd.Resolve(cwd)
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
	recipe, err := seedRecipe(target)
	if err != nil {
		output.Err(out, fmt.Sprintf("loom: %s %s: read the run's seed: %s; way forward: restore the run's seed, or begin the run again with `lym loom start`", decisionGroup, cmd.Name(), err))
		clihelp.Abort(ctx, 1)
		return nil
	}
	record := runRecordOf(target, recipe)
	recordPath := record.path
	supportLogPath := record.supportLog
	recordUnderLym, err := filepath.Rel(lymdirs.LymDirName, record.rel)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	s.deps = decisionDeps{
		strandName: os.Getenv(agentname.StrandNameEnv),
		programName: func() (string, error) {
			geometry, err := hubgeom.ReedGeometry(target)
			if err != nil {
				return "", err
			}
			return agentname.FormatProgram(geometry.NameShortname, loomProgram)
		},
		parent: func() (hubgeom.Parent, error) { return decisionParent(target, record, orchcli.OrchLive) },
		arbiterStrand: func(name string) bool {
			geometry, err := hubgeom.ReedGeometry(target)
			if err != nil {
				return false
			}
			parsed, err := agentname.Parse(name)
			return err == nil && parsed.Shortname == geometry.NameShortname && parsed.Slug == geometry.NameSlug && loomengine.IsArbiterStrand(name)
		},
		samplerStrand:  samplerStrandReader(target, recipe),
		readStatus:     circlingStatusReader(shedrun.StatusFile(target, shedrun.SelfRunID), shedrun.StatusLock(target, shedrun.SelfRunID)),
		recordPath:     recordPath,
		recordUnderLym: filepath.ToSlash(recordUnderLym),
		recordRel:      filepath.ToSlash(record.rel),
		lockRecord:     func() (func() error, error) { return holdDecisionLock(recordPath) },
		writeRecord:    func(data []byte) error { return os.WriteFile(recordPath, data, 0o644) },
		commitReplace: func(msg string, trailers []fabricengine.Trailer) (bool, error) {
			_, committed, err := fabricengine.CommitAnchoredPathsWithTrailers(fabricengine.NewMutations(""), target, []string{record.rel}, msg, trailers, fabricengine.EnvSyncOptions())
			return committed, err
		},
		commitEmpty: func(msg string, trailers []fabricengine.Trailer) error {
			_, _, err := fabricengine.CommitEmptyWithTrailers(fabricengine.NewMutations(""), target, record.rel, msg, trailers, fabricengine.EnvSyncOptions())
			return err
		},
		headCarries: func(value string) (bool, error) { return headCarriesReplace(target, value) },
		implementerSpan: func(currentRow string) (bool, error) {
			return loomrecipe.InImplementerSpan(recipe, currentRow)
		},
		boardRecordStored: boardRecordStoredOf(record, func() (bool, error) {
			board, err := openHubBoard(target)
			if err != nil {
				return false, err
			}
			_, found, err := board.DecisionRecord(seedSlug(target.WorktreeName))
			return found, err
		}),
		raddleEntryTask: func() (bool, error) {
			board, err := openHubBoard(target)
			if err != nil {
				return false, err
			}
			task, found, err := board.GetTask(seedSlug(target.WorktreeName))
			return !found || task.Kind == boardengine.KindTask, err
		},
		raddleBoardFiling: func() ([]string, bool, error) { return raddleBoardFiling(target) },
		append: func(ds []discussionparser.AddedDecision) ([]discussionparser.Finding, error) {
			return discussionparser.AppendDecisions(recordPath, supportLogPath, ds)
		},
		commit: func() error { return commitRunRecord(target, record) },
		now:    time.Now,
		hasPlan: func() (bool, error) {
			recipe, err := seedRecipe(target)
			if err != nil {
				return false, err
			}
			return loomrecipe.HasEngine(recipe, "PlanWrite")
		},
		rowWritten: func() (bool, error) {
			recipe, err := seedRecipe(target)
			if err != nil {
				return false, err
			}
			engine, err := loomrecipe.RecordEngine(recipe)
			return engine == "RecordWrite", err
		},
		readRecord: func() ([]byte, error) { return os.ReadFile(recordPath) },
		committed: func() ([]byte, bool, error) {
			return fabricengine.CommittedAnchoredFile(target, record.rel)
		},
		history: recordHistory(target, record),
		liveStrands: func() ([]string, error) {
			return decisionReceivers(target, record, liveRunStrands)
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
		// The recipe's parse error is dropped: an unparseable recipe is refused before any dep is built.
		// A recipe with no approval row answers false, and so does a run whose record is the beam's project record: neither copies a past approval.
		pastApproval: func(currentRow string) bool {
			past, _, _ := loomrecipe.PastDiscussionApproval(recipe, currentRow)
			return past
		},
		storeRecord: func(record []byte) error {
			return storeDecisionRecord(target, seedSlug(target.WorktreeName), record)
		},
		approvalRow: func() bool {
			_, hasRow, _ := loomrecipe.DiscussionApprovalRow(recipe)
			return hasRow
		},
		raddleFileRow: func() (string, bool, error) { return loomrecipe.EngineRow(recipe, raddleFileEngine) },
		raddlePerch: func() (bool, error) {
			recorded, err := loomshed.ReadRaddleState(loomengine.RaddleStatePath(target), loomengine.RaddleStateLockPath(target))
			return recorded.Mode == loomengine.RaddleModePerch, err
		},
		raddleFiled: func() ([]string, bool, error) { return raddleRunFiling(target, seedSlug(target.WorktreeName)) },
	}
	return nil
}

// headCarriesReplace reports whether the fabric HEAD of target is a commit whose replace trailer carries value.
func headCarriesReplace(target *lymcwd.Location, value string) (bool, error) {
	fabric, err := fabricengine.Open(target)
	if err != nil {
		return false, err
	}
	head, err := fabric.RecordsHeadSHA()
	if err != nil {
		return false, err
	}
	commits, err := fabricengine.TrailerCommits(target, recordReplacedTrailerKey)
	if err != nil {
		return false, err
	}
	return len(commits) > 0 && commits[0].SHA == head && commits[0].Value == value, nil
}

// boardRecordStoredOf returns the seam that reports whether the run's board entry carries a stored record.
// The run is the one that holds record.
// A beam's project record is never stored on its entry, so that run answers false without reading the entry.
// Any other run answers entryStoresRecord's result.
func boardRecordStoredOf(record runRecord, entryStoresRecord func() (bool, error)) func() (bool, error) {
	return func() (bool, error) {
		if record.project {
			return false, nil
		}
		return entryStoresRecord()
	}
}

// raddleBoardFiling returns the part slugs of target's raddle proposal.
// It also reports whether the board holds every one of them as filed from the proposal's Sketch.
// An absent or unparseable proposal holds no parts and no filing.
func raddleBoardFiling(target *lymcwd.Location) ([]string, bool, error) {
	data, err := os.ReadFile(loomengine.RaddleProposal(target))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	proposal, err := raddleparser.Parse(data)
	if err != nil {
		return nil, false, nil
	}
	parts := make([]string, 0, len(proposal.Parts))
	for _, part := range proposal.Parts {
		parts = append(parts, part.Slug)
	}
	board, err := openHubBoard(target)
	if err != nil {
		return nil, false, err
	}
	held, err := board.HoldsRaddleFiling(proposal.Sketch, parts)
	return parts, held, err
}

// samplerStrandReader returns the seam that reports whether name is the target run's sampler session:
// the strand its Sampler-Session row has recorded, while the run's current row is that row.
// It answers false for a recipe with no SamplerSession row.
// It also answers false for a status file whose current producer is another row, and for an absent record.
// The run's state is not read,
// so a run halted at the row still admits its strand, which the row re-records on each of its own starts and resumes.
func samplerStrandReader(target *lymcwd.Location, recipe string) func(name string) (bool, error) {
	readStatus := circlingStatusReader(shedrun.StatusFile(target, shedrun.SelfRunID), shedrun.StatusLock(target, shedrun.SelfRunID))
	return func(name string) (bool, error) {
		row, hasRow, err := loomrecipe.EngineRow(recipe, samplerSessionEngine)
		if err != nil || !hasRow {
			return false, err
		}
		status, found, err := readStatus()
		if err != nil || !found || status.CurrentProducer != row {
			return false, err
		}
		record, found, err := samplerengine.ReadRecord(shedrun.SamplerRecord(target, shedrun.SelfRunID), shedrun.SamplerRecordLock(target, shedrun.SelfRunID))
		if err != nil || !found {
			return false, err
		}
		return record.StrandName == name, nil
	}
}

// seedRecipe returns the recipe name target's seed records, tapestry for a run with no seed.
func seedRecipe(target *lymcwd.Location) (string, error) {
	seed, found, err := shedrun.ReadSeed(target, shedrun.SelfRunID)
	if err != nil {
		return "", err
	}
	return resolveSeedRecipe(seed, found), nil
}

// decisionParent resolves the orch that may record a decision on target's run.
// A beam's project record is the beam's own orch's to change: the nearest live orch from the beam itself.
// Any other record is the run's parent orch's: the nearest live one up from the run's parent.
// orchLive reports whether a worktree's orch is live.
func decisionParent(target *lymcwd.Location, record runRecord, orchLive func(*lymcwd.Location) (bool, error)) (hubgeom.Parent, error) {
	if record.project {
		parent, err := hubgeom.NearestLiveOrch(target, orchLive)
		if err != nil {
			return hubgeom.Parent{}, err
		}
		if parent.Name == "" {
			return hubgeom.Parent{}, fmt.Errorf("the hub records no repo shortname, so the orch of the beam %s, the one session that may record on the project, cannot be named; way forward: run `lym fabric shortname <shortname>`", seedSlug(target.WorktreeName))
		}
		return parent, nil
	}
	return orchcli.NearestLiveParent(target)
}

// decisionReceivers returns the strand names of the live shuttle runs a decision on target's record reaches.
// A project record reaches the beam's own runs and the runs of every member pair under the beam.
// Every member works under the project's scope, so the project record reaches their runs too.
// Any other record reaches target's own runs.
// runStrands reads the live strand names of one worktree.
func decisionReceivers(target *lymcwd.Location, record runRecord, runStrands func(*lymcwd.Location) ([]string, error)) ([]string, error) {
	strands, err := runStrands(target)
	if err != nil || !record.project {
		return strands, err
	}
	pairs, err := memberPairs(target, seedSlug(target.WorktreeName))
	if err != nil {
		return nil, err
	}
	for _, pair := range pairs {
		member, err := runStrands(pair)
		if err != nil {
			return nil, err
		}
		strands = append(strands, member...)
	}
	return strands, nil
}

// liveRunStrands returns the strand names of the live shuttle runs of target's worktree, read through the registered engines.
// A running run whose waiting process is gone is not live.
func liveRunStrands(target *lymcwd.Location) ([]string, error) {
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
		if !reading.WaiterGone {
			strands = append(strands, reading.StrandName)
		}
	}
	return strands, nil
}

// noticePoster is the one creel call a notice makes: leaving a body as mail for a receiver.
type noticePoster interface {
	Post(receiver, body string) (int64, error)
}

// noticeClients builds the creel clients a notice posts through:
// one signing as a strand, one signing as the LOOM program.
type noticeClients struct {
	strand  func(strandClaim string) (noticePoster, error)
	program func() (noticePoster, error)
}

// postDecisionNotice posts line to receiver for a caller whose exported strand name is caller.
// Creel accepts a program claim only from an orch pane or from a process in no pane.
// So a named caller that is not an orch, such as the arbiter, posts as its own strand.
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
func decisionNoticeClients(target *lymcwd.Location) noticeClients {
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
		Long: `decision holds the verbs through which a design call made after the Discussion
ended is recorded in the decision record. "decision add" records it,
"decision replace" writes a whole checked record that folds, changes or
withdraws earlier calls, and "decision store" re-copies the committed record
onto the run's board entry.

The verb takes an optional slug naming the task worktree. A task worktree
addresses itself; from the prime the slug is required. On a beam,
"decision add <beam-slug>" or "decision replace <beam-slug>" records a change
of the project's scope in the beam's project record; the beam's own orch runs
it, and its notice reaches
the live agents of every member run under the beam. It refuses a task
session's label: a caller whose reed strand name differs from the run's
parent orch is refused, and the operator's own unnamed shell always passes.
The one other caller is the run's own arbiter strand, which passes --by arbiter.
On a sampler run the strand its Sampler-Session row recorded as its session is
also admitted while the run stands at that row, and only with --by operator.
--by is a label that is not verified.

Example:
  lym loom decision add <slug> --by parent --title "..." --decision "..." --rationale "..."
  lym loom decision add <slug> --by parent --from decisions.json`,
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: st.preRun,
	}

	add := &cobra.Command{
		Use:         "add [<slug>]",
		Short:       "append decisions to the decision record and commit them",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `add appends an entry under an "Added after Discussion" heading, carrying the
--by label and today's date, to the record's Decisions section, re-runs the
discussion check (restoring the record on a finding) and commits.
It changes neither the support log nor the plan. Past Discussion approval it
also copies the record onto the run's board entry; a failed copy is reported
as "record_store_error", never fails the verb. It then notifies the run's other
live agents ("notified", "notice_errors", "notice_held_back").

--by is parent, operator or arbiter (the last only from the run's arbiter
strand). --title, --decision and --rationale are required without --from. A named session other than the run's parent orch, arbiter or sampler
session (--by operator only) is refused, as is any call while Discussion-Write
runs or, on a Record-Write recipe, before it has committed the record.

--amends-plan marks a decision that amends the approved plan: the call also
appends the title to the plan amendment record in the same commit. It is
refused on a recipe with no Plan-Write row and with no approved plan.
Afterwards the parent runs "lym loom goto --to Plan-Burler-Review" and
"lym loom resume".

A decision on a PATTERN entry also passes --pattern-entry <PATTERN-id>,
--pattern-action adds|changes|removes and, for adds or changes, --pattern-text
<new entry text>, which writes its bullet to the "## PATTERN changes" section.
--pattern-action withdraws strikes the entry's declared bullet in the same
commit, takes no --pattern-text and is refused when the record declares none.

--from <file> records several decisions from a JSON array of objects with the
keys title, decision, rationale and optional pattern_entry, pattern_action,
pattern_text. --by and --amends-plan apply to every entry. All land in one
commit or none does; no two share a title or a PATTERN entry.

Example:
  lym loom decision add <slug> --by parent --title "T" --decision "D" --rationale "R"`,
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
	add.Flags().StringVar(&st.input.patternEntry, "pattern-entry", "", "the PATTERN entry the decision adds, changes, removes or withdraws, as PATTERN-<id>")
	add.Flags().StringVar(&st.input.patternAction, "pattern-action", "", "what the decision does to the entry: adds, changes, removes or withdraws")
	add.Flags().StringVar(&st.input.patternText, "pattern-text", "", "the entry's new text, for adds or changes")
	add.Flags().StringVar(&st.input.from, "from", "", "a JSON file of several decisions to record in one call, in place of the per-entry flags")
	add.Flags().BoolVar(&st.input.amendsPlan, "amends-plan", false, "the decision amends the approved plan: also record it in the plan amendment record")

	store := &cobra.Command{
		Use:         "store [<slug>]",
		Short:       "copy the committed decision record onto the run's board entry",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `store copies the decision record as committed at HEAD onto the run's board
entry, the same copy "decision add" makes after its commit. It is idempotent:
a record the entry already holds is not written again. It takes no flags and
changes nothing in the run: no commit, no routing and no notice.

It works once the run is past its Discussion approval, or, on a recipe with
no Discussion approval, when the run's entry carries a stored record. It
refuses a run still inside Discussion-Review, a record HEAD does not hold,
an entry that has landed, which keeps the record it last stored, and a raddle
run whose filing is done. The operator's own unnamed shell, the run's own
arbiter strand and the run's parent orch may run it.

Example:
  lym loom decision store <slug>`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), decisionStoreVerb(cmd.OutOrStdout(), st.slug, st.deps))
			return nil
		},
	}

	replace := &cobra.Command{
		Use:         "replace [<slug>]",
		Short:       "write a whole checked decision record from a file and commit it",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `replace writes the --from file as the run's decision record and commits it
with a Lym-Record-Replaced trailer, from which on the record's dropped-entry
check reads its history. It changes or withdraws an earlier decision, and
folds "Added after Discussion" entries into the main decisions.

--base-sha256 is the sha256 of the record the file was built from; the call
is refused when the record committed at HEAD differs, or the working copy
differs from it. The file must pass the section and decision-id checks, keep
every carry-over block byte for byte, and hold no added entry the record does
not hold, compared without trailing whitespace: it may drop added entries and appended PATTERN
bullets, never insert or rewrite one. A new call is recorded with "add".

The callers are "add"'s. It is refused while Discussion-Write runs, before a
Discussion approval the recipe has, between the plan's approval and the last
Webster row, and on a raddle run whose filing is done. It refreshes the board
copy as "add" does and tells each live agent to re-read the record whole.

Example:
  lym loom decision replace <slug> --by parent --from record.md --title "Fold the cache calls" --base-sha256 <hex>`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), decisionReplaceVerb(cmd.OutOrStdout(), st.slug, st.deps, st.replace))
			return nil
		},
	}
	replace.Flags().StringVar(&st.replace.by, "by", "", "who made the call: parent, operator or arbiter")
	replace.Flags().StringVar(&st.replace.from, "from", "", "the file whose bytes become the decision record")
	replace.Flags().StringVar(&st.replace.title, "title", "", "a short title of the change, carried by the notice and the commit")
	replace.Flags().StringVar(&st.replace.baseSHA256, "base-sha256", "", "the sha256 of the committed record the file was built from")

	group.AddCommand(add, replace, store)
	return group
}
