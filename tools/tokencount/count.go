package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Usage is a token tally: one message's usage, or a sum over many.
type Usage struct {
	Input       int
	Output      int
	CacheCreate int
	CacheRead   int
	// CacheCreation splits CacheCreate by cache lifetime, which sets a cache write's price.
	CacheCreation CacheCreation
}

// CacheCreation is the part of a tally's cache writes made with the one-hour lifetime;
// the rest of the writes have the default five-minute lifetime.
type CacheCreation struct {
	OneHour int
}

func (u *Usage) add(o Usage) {
	u.Input += o.Input
	u.Output += o.Output
	u.CacheCreate += o.CacheCreate
	u.CacheRead += o.CacheRead
	u.CacheCreation.OneHour += o.CacheCreation.OneHour
}

// Total is every token of the tally: input, cache write, cache read and output.
func (u Usage) Total() int {
	return u.Input + u.CacheCreate + u.CacheRead + u.Output
}

// Weight ranks a tally by rough relative cost; see the package doc for what it ignores.
func (u Usage) Weight() float64 {
	return float64(u.Input) + 1.25*float64(u.CacheCreate) + 0.1*float64(u.CacheRead) + 5*float64(u.Output)
}

// RoleTally is the usage of every session one role ran in one run.
type RoleTally struct {
	Role     string
	Sessions int
	Usage    Usage
	// Cost is the usage priced per message at the message's model.
	Cost   Cost
	Models map[string]int // assistant messages per model
	// Working is the union of the role's sessions' working intervals.
	Working []Interval
}

// Interval is a span of time from From to To.
type Interval struct {
	From, To time.Time
}

// Duration is To - From, 0 when To is not after From.
func (i Interval) Duration() time.Duration {
	if !i.To.After(i.From) {
		return 0
	}
	return i.To.Sub(i.From)
}

// unionIntervals returns the sorted, non-overlapping, non-touching union of set, dropping empty intervals.
func unionIntervals(set []Interval) []Interval {
	sorted := make([]Interval, 0, len(set))
	for _, i := range set {
		if i.Duration() > 0 {
			sorted = append(sorted, i)
		}
	}
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].From.Before(sorted[b].From) })
	var merged []Interval
	for _, i := range sorted {
		if n := len(merged); n > 0 && !i.From.After(merged[n-1].To) {
			if i.To.After(merged[n-1].To) {
				merged[n-1].To = i.To
			}
			continue
		}
		merged = append(merged, i)
	}
	return merged
}

// withoutIntervals returns the union of set less every part the union of cut covers.
func withoutIntervals(set, cut []Interval) []Interval {
	cuts := unionIntervals(cut)
	var rest []Interval
	for _, i := range unionIntervals(set) {
		from := i.From
		for _, c := range cuts {
			if !c.From.Before(i.To) {
				break
			}
			if !c.To.After(from) {
				continue
			}
			if c.From.After(from) {
				rest = append(rest, Interval{from, c.From})
			}
			from = c.To
		}
		if i.To.After(from) {
			rest = append(rest, Interval{from, i.To})
		}
	}
	return rest
}

// covered returns how much of window the union of set covers.
func covered(set []Interval, window Interval) time.Duration {
	var total time.Duration
	for _, i := range unionIntervals(set) {
		total += Interval{max2(i.From, window.From), min2(i.To, window.To)}.Duration()
	}
	return total
}

// max2 returns the later of a and b.
func max2(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// min2 returns the earlier of a and b.
func min2(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

// stallCap is the longest interval counted as working time, three times the harness's ten-minute cap on a foreground Bash call;
// a longer interval is a stall such as a suspended host and counts as idle whole.
const stallCap = 30 * time.Minute

// turnPoint is one transcript entry's timestamp and whether the entry ends or starts its session's turn.
type turnPoint struct {
	At     time.Time
	Ends   bool
	Starts bool
}

// workingIntervals returns the interval from each point to the next, in timestamp order, except after a point that ends the turn, into a point that starts a turn, or longer than stallCap.
func workingIntervals(points []turnPoint) []Interval {
	ordered := slices.Clone(points)
	sort.SliceStable(ordered, func(a, b int) bool { return ordered[a].At.Before(ordered[b].At) })
	var working []Interval
	for i := 0; i+1 < len(ordered); i++ {
		next := ordered[i+1]
		if !ordered[i].Ends && !next.Starts && next.At.Sub(ordered[i].At) <= stallCap {
			working = append(working, Interval{ordered[i].At, next.At})
		}
	}
	return working
}

// ForkTally is the usage of one Webster fork: one sub-agent transcript of a session whose role is webster.
type ForkTally struct {
	Slug string
	// File is the transcript's file name.
	File string
	// Cards are the card ids (NN-<slug>) the fork's prompt names, empty when unattributed.
	Cards []string
	// Messages counts the assistant messages tallied for the fork.
	Messages int
	// StartContext is the input + cache write + cache read of the first counted message, the context Merriam held when it spawned the fork;
	// 0 when no message was counted.
	StartContext int
	// PeakContext is the largest input + cache write + cache read of any counted message.
	PeakContext int
	Usage       Usage
	// Session is the file name of the webster-role session transcript the fork's transcript sits under.
	Session string
	// Started is the timestamp of the transcript's first line that carries one.
	Started time.Time
}

// MerriamStart is the start of one Merriam (webster-role) session.
type MerriamStart struct {
	// Session is the session transcript's file name.
	Session string
	// Started is the timestamp of the transcript's first line that carries one.
	Started time.Time
	// StartContext is the input + cache write + cache read of the first assistant message with usage after the line holding the result of Merriam's Read of its prompt file;
	// 0 when no start was measured.
	StartContext int
	// NoStart says why no start was measured, empty when StartContext was measured.
	NoStart string
}

// RunTally is one task run's usage, split by role.
type RunTally struct {
	Slug  string
	Roles map[string]*RoleTally
	Forks []ForkTally
	// Merriams holds one start per webster-role session, ordered by Started.
	Merriams   []MerriamStart
	Duplicates int // transcript lines skipped as a repeat of a message already counted
	// WebsterEngine is the name of the engine that located the run's first counted webster-role session, in located order;
	// empty when the run has none.
	// Calibration takes the run's instruction files from this engine, so it assumes every webster-role session of the run ran under one engine.
	WebsterEngine string
	// BaseSHA is the start_sha of the earliest successful begin-batch result in the run's webster sessions, the HEAD before its first batch forked;
	// empty when there is none.
	BaseSHA string
	// baseAt is the timestamp of the result BaseSHA came from.
	baseAt time.Time
	// First and Last bound the run's active window: the earliest and latest timestamp of a user or assistant line in any of its transcripts;
	// both are zero when no such line carries one.
	First, Last time.Time
	// Span is the window the time figures are measured over: First..Last.
	Span Interval
	// Working is the union of every transcript's working intervals.
	Working []Interval
	// Sleeps are the machine's sleeps, which every time figure of the run leaves out.
	Sleeps []Interval
	// Shed is the shed record read from the run's archive tag, nil when no shed status was read.
	Shed *ShedRecord
	// Bound is the lower bound applied to the run's transcripts, zero for none.
	Bound time.Time
	// LeftOut counts the main-session transcripts left out for starting before Bound or having no timestamped line.
	LeftOut int
}

// RoleTime is how much of the run's span the role's sessions spent in a turn, 0 for a role the run lacks.
func (run RunTally) RoleTime(role string) time.Duration {
	t := run.Roles[role]
	if t == nil {
		return 0
	}
	return covered(t.Working, run.Span)
}

// Idle is the run's awake span minus the time any session of the run was in a turn.
func (run RunTally) Idle() time.Duration {
	return run.AwakeSpan() - covered(run.Working, run.Span)
}

// AwakeSpan is the run's span less the machine's sleeps in it.
func (run RunTally) AwakeSpan() time.Duration {
	return run.awake(run.Span)
}

// Slept is how much of the run's span the machine slept.
func (run RunTally) Slept() time.Duration {
	return covered(run.Sleeps, run.Span)
}

// awake returns window's duration less the machine's sleeps in it.
func (run RunTally) awake(window Interval) time.Duration {
	return window.Duration() - covered(run.Sleeps, window)
}

// subtractSleeps records the machine's sleeps on the run and removes the parts they cover from every working interval, the run's and each role's.
func (run *RunTally) subtractSleeps(sleeps []Interval) {
	run.Sleeps = unionIntervals(sleeps)
	run.Working = withoutIntervals(run.Working, run.Sleeps)
	for _, tally := range run.Roles {
		tally.Working = withoutIntervals(tally.Working, run.Sleeps)
	}
}

// Cost is the run's estimated cost, summed over its roles.
func (run RunTally) Cost() Cost {
	var total Cost
	for _, t := range run.Roles {
		total.add(t.Cost)
	}
	return total
}

// observeTime widens the run's active window to hold an entry's timestamp, an entry without one leaving it as it is.
func (run *RunTally) observeTime(e shuttleengine.TranscriptEntry) {
	if e.At.IsZero() {
		return
	}
	if run.First.IsZero() || e.At.Before(run.First) {
		run.First = e.At
	}
	if e.At.After(run.Last) {
		run.Last = e.At
	}
}

// websterMasterRole is the role of the session whose sub-agents are Webster forks.
const websterMasterRole = "webster"

var (
	readLinePrefix = regexp.MustCompile(`^\d+\t`)
	cardPointer    = regexp.MustCompile("^- `(?:[^`]*/)?(\\d\\d-[^/`]+)\\.md`$")
)

// cardPointers returns the card ids of every card pointer line in text, in order;
// a Read result's line-number prefix is ignored.
func cardPointers(text string) []string {
	var cards []string
	for _, raw := range strings.Split(text, "\n") {
		m := cardPointer.FindStringSubmatch(strings.TrimRight(readLinePrefix.ReplaceAllString(raw, ""), "\r"))
		if m != nil && m[1] != "00-overview" {
			cards = append(cards, m[1])
		}
	}
	return cards
}

// beginBatchCommand matches a shell command that invokes the webster begin-batch verb.
var beginBatchCommand = regexp.MustCompile(`\blyx\s+webster\s+begin-batch\b`)

// recordBase scans one webster session's entries for begin-batch result envelopes and keeps the start_sha of the earliest-timestamped one across every session of the run.
// An envelope is a tool result answering a begin-batch shell call whose text is a JSON object with a non-empty start_sha;
// a refusal carries none, and a result entry without a timestamp is skipped.
func (run *RunTally) recordBase(entries []shuttleengine.TranscriptEntry) {
	beginBatchCalls := map[string]bool{}
	for _, e := range entries {
		for _, call := range e.ToolCalls {
			if call.Kind == shuttleengine.ToolShell && beginBatchCommand.MatchString(call.Command) {
				beginBatchCalls[call.ID] = true
			}
		}
		if e.At.IsZero() {
			continue
		}
		for _, result := range e.ToolResults {
			if !beginBatchCalls[result.CallID] {
				continue
			}
			var envelope struct {
				StartSHA string `json:"start_sha"`
			}
			if json.Unmarshal([]byte(result.Text), &envelope) != nil || envelope.StartSHA == "" {
				continue
			}
			if run.baseAt.IsZero() || e.At.Before(run.baseAt) {
				run.BaseSHA, run.baseAt = envelope.StartSHA, e.At
			}
		}
	}
}

// readMerriamStart measures one Merriam session's start in its read transcript.
// The prompt path comes from the first user entry holding the engine's launch line;
// the start is the context of the first assistant entry with usage after the result of the first file read of that path.
func readMerriamStart(read shuttleengine.TranscriptRead) MerriamStart {
	start := MerriamStart{Session: read.ID, Started: read.Started}
	var promptPath, readCallID string
	resultSeen, measured := false, false
	for _, e := range read.Entries {
		if measured {
			break
		}
		switch e.Role {
		case shuttleengine.EntryUser:
			if promptPath == "" {
				promptPath = e.PromptPath
				continue
			}
			if readCallID == "" || resultSeen {
				continue
			}
			for _, result := range e.ToolResults {
				if result.CallID == readCallID {
					resultSeen = true
				}
			}
		case shuttleengine.EntryAssistant:
			if promptPath != "" && readCallID == "" {
				for _, call := range e.ToolCalls {
					if call.Kind == shuttleengine.ToolFileRead && call.Path == promptPath {
						readCallID = call.ID
						break
					}
				}
				continue
			}
			if resultSeen && e.Usage != nil {
				u := usageOf(*e.Usage)
				start.StartContext = u.Input + u.CacheCreate + u.CacheRead
				measured = true
			}
		}
	}
	switch {
	case promptPath == "":
		start.NoStart = "no launch line"
	case readCallID == "":
		start.NoStart = "no Read of the prompt file " + promptPath
	case !measured:
		start.NoStart = "no assistant message after the prompt Read's result"
	}
	return start
}

var trailingNumber = regexp.MustCompile(`-\d+$`)

// leadSuffix ends the role a seat table's lead carried before seats were named, `<prefix>-chair`; the lead's role is now the prefix alone.
const leadSuffix = "-chair"

// roleOf turns a strand name into its role: the last colon-separated part, without a -N suffix and then without the pre-rename lead suffix.
func roleOf(title string) string {
	if title == "" {
		return "untitled"
	}
	return strings.TrimSuffix(trailingNumber.ReplaceAllString(title[strings.LastIndex(title, ":")+1:], ""), leadSuffix)
}

// ArchiveTag is one archive tag of a finished run: its name, the commit it names and that commit's committer date.
type ArchiveTag struct {
	// Name is the tag's name, archive/<slug>/<sha>.
	Name      string
	Commit    string
	Committed time.Time
}

// finishedRuns reads the archive tags of the runs whose pairs were torn down from the weft repository at weft.
// They are keyed by slug, each slug's tags newest first.
// Archive tags are lightweight, so the object name each names is the commit.
func finishedRuns(weft string) (map[string][]ArchiveTag, error) {
	out, err := gitexec.Run([]string{"for-each-ref", "--format=%(refname)%09%(objectname)%09%(committerdate:iso-strict)", "refs/tags/archive/"}, weft)
	if err != nil {
		return nil, fmt.Errorf("reading the archive tags in %s: %w", weft, err)
	}
	return archivedSlugs(out), nil
}

// archivedSlugs parses for-each-ref output into each slug's tags ordered newest first by date, ties broken by name.
// A line holds a refs/tags/archive/<slug>/<sha> ref, a commit and an RFC 3339 date, tab-separated.
// A line of another shape is skipped.
func archivedSlugs(refs string) map[string][]ArchiveTag {
	slugs := map[string][]ArchiveTag{}
	for _, line := range strings.Split(refs, "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) != 3 {
			continue
		}
		name, ok := strings.CutPrefix(fields[0], "refs/tags/")
		if !ok || fields[1] == "" {
			continue
		}
		slug, ok := fabricengine.ArchiveTagSlug(name)
		if !ok {
			continue
		}
		committed, err := time.Parse(time.RFC3339, fields[2])
		if err != nil {
			continue
		}
		slugs[slug] = append(slugs[slug], ArchiveTag{Name: name, Commit: fields[1], Committed: committed})
	}
	for _, tags := range slugs {
		sort.Slice(tags, func(i, j int) bool {
			if !tags[i].Committed.Equal(tags[j].Committed) {
				return tags[i].Committed.After(tags[j].Committed)
			}
			return tags[i].Name < tags[j].Name
		})
	}
	return slugs
}

// recentRuns names the full loom runs of the hub whose sessions changed most recently, newest first, at most last of them, and, newest first, the runs it skipped on the way because they are not full loom runs.
// A candidate is the slug of an archive tag in finished, except the prime's own, with a session located at the worktree <hub>/<slug> through some engine;
// its recency is the newest modification time among those sessions.
// fullLoomRun decides it from the archive tags read through trees.
func recentRuns(reads ReadSet, hub, prime string, finished map[string][]ArchiveTag, trees ArchiveTrees, last int) (counted, skipped []string, err error) {
	type candidate struct {
		slug    string
		touched time.Time
	}
	slugs := make([]string, 0, len(finished))
	for slug := range finished {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	var found []candidate
	for _, slug := range slugs {
		if slug == "" || slug == prime || len(finished[slug]) == 0 {
			continue
		}
		located, err := locateSessions(reads, filepath.Join(hub, slug))
		if err != nil {
			return nil, nil, err
		}
		var touched time.Time
		for _, s := range located {
			if s.session.Modified.After(touched) {
				touched = s.session.Modified
			}
		}
		if !touched.IsZero() {
			found = append(found, candidate{slug, touched})
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].touched.After(found[j].touched) })
	counted = []string{}
	for i := 0; i < len(found) && len(counted) < last; i++ {
		full, err := fullLoomRun(trees, hub, found[i].slug, finished[found[i].slug])
		if err != nil {
			return nil, nil, err
		}
		if full {
			counted = append(counted, found[i].slug)
		} else {
			skipped = append(skipped, found[i].slug)
		}
	}
	return counted, skipped, nil
}

// runBound returns the lower bound of a slug's run from its archive tags, newest first.
// The bound is the newest tag's commit date for a run in flight and the next-older tag's for a finished run, the zero time when there is no such tag.
func runBound(tags []ArchiveTag, inFlight bool) time.Time {
	previous := 1
	if inFlight {
		previous = 0
	}
	if previous >= len(tags) {
		return time.Time{}
	}
	return tags[previous].Committed
}

// transcriptStart returns the earliest timestamp of an entry in the transcript, false when no entry carries one.
func transcriptStart(read shuttleengine.TranscriptRead) (time.Time, bool) {
	var start time.Time
	var found bool
	for _, e := range read.Entries {
		if !e.At.IsZero() && (!found || e.At.Before(start)) {
			start, found = e.At, true
		}
	}
	return start, found
}

// CountRun tallies every session the engines in reads recorded under the worktree.
// A non-zero bound leaves out, with its forks, every main-session transcript that starts before the bound or has no timestamped entry, and counts them in LeftOut.
// It fails when no engine located a session, and when an engine fails to locate or read one.
func CountRun(reads ReadSet, worktree, slug string, bound time.Time) (RunTally, error) {
	located, err := locateSessions(reads, worktree)
	if err != nil {
		return RunTally{}, err
	}
	if len(located) == 0 {
		readable, _ := reads.engines()
		return RunTally{}, fmt.Errorf("no sessions for %s at %s from engines %s", slug, worktree, strings.Join(readable, ", "))
	}

	run := RunTally{Slug: slug, Roles: map[string]*RoleTally{}, Bound: bound}
	seen := map[string]bool{}
	for _, l := range located {
		session, err := readLocated(reads, l)
		if err != nil {
			return RunTally{}, err
		}
		if !bound.IsZero() {
			start, ok := transcriptStart(session.Main)
			if !ok || start.Before(bound) {
				run.LeftOut++
				continue
			}
		}
		role := roleOf(session.Main.Name)
		pricer := reads[l.engine].Pricer
		run.countFile(l.engine, pricer, session.Main, role, seen, nil)
		if role == websterMasterRole {
			if run.WebsterEngine == "" {
				run.WebsterEngine = l.engine
			}
			run.recordBase(session.Main.Entries)
			run.Merriams = append(run.Merriams, readMerriamStart(session.Main))
		}
		for _, forkRead := range session.Forks {
			var fork *ForkTally
			if role == websterMasterRole {
				fork = &ForkTally{Slug: slug, File: forkRead.ID, Session: session.Main.ID, Started: forkRead.Started}
			}
			run.countFile(l.engine, pricer, forkRead, role+"+sub", seen, fork)
			if fork != nil {
				run.Forks = append(run.Forks, *fork)
			}
		}
	}
	sort.SliceStable(run.Forks, func(i, j int) bool { return run.Forks[i].Started.Before(run.Forks[j].Started) })
	sort.SliceStable(run.Merriams, func(i, j int) bool { return run.Merriams[i].Started.Before(run.Merriams[j].Started) })
	run.Span = Interval{run.First, run.Last}
	return run, nil
}

// countFile tallies one read transcript of an engine under role;
// a non-nil fork also receives the transcript's own tally and card attribution.
// The read's repeats count as duplicates, and so does each message already seen under the same engine and message key.
func (run *RunTally) countFile(engine string, pricer shuttleengine.MessagePricer, read shuttleengine.TranscriptRead, role string, seen map[string]bool, fork *ForkTally) {
	tally := run.Roles[role]
	if tally == nil {
		tally = &RoleTally{Role: role, Models: map[string]int{}}
		run.Roles[role] = tally
	}
	tally.Sessions++
	run.Duplicates += read.Repeats
	var points []turnPoint
	for _, e := range read.Entries {
		if fork != nil {
			fork.observe(e)
		}
		run.observeTime(e)
		if !e.At.IsZero() {
			points = append(points, turnPoint{e.At, e.EndsTurn, e.StartsTurn})
		}
		if e.Usage == nil {
			continue
		}
		id := messageKey(engine, e)
		if seen[id] {
			run.Duplicates++
			continue
		}
		seen[id] = true
		u := usageOf(*e.Usage)
		tally.Usage.add(u)
		tally.Cost.add(messageCost(pricer, e.Model, *e.Usage))
		tally.Models[e.Model]++
		if fork != nil {
			fork.Messages++
			fork.Usage.add(u)
			context := u.Input + u.CacheCreate + u.CacheRead
			if fork.Messages == 1 {
				fork.StartContext = context
			}
			fork.PeakContext = max(fork.PeakContext, context)
		}
	}
	working := workingIntervals(points)
	tally.Working = unionIntervals(append(tally.Working, working...))
	run.Working = unionIntervals(append(run.Working, working...))
}

// messageKey identifies a message across the run's transcripts: the engine that read it and the engine's key for it.
func messageKey(engine string, e shuttleengine.TranscriptEntry) string {
	return engine + "/" + e.MessageKey
}

// observe records, until a card pointer has been found, a fork entry's card attribution: the fork's prompt is the first tool result holding a card pointer line.
func (f *ForkTally) observe(e shuttleengine.TranscriptEntry) {
	if f.Cards != nil || e.Role != shuttleengine.EntryUser {
		return
	}
	for _, result := range e.ToolResults {
		if cards := cardPointers(result.Text); len(cards) > 0 {
			f.Cards = cards
			return
		}
	}
}

// Report is every run counted in one invocation.
type Report struct {
	Runs []RunTally
	// Orch is the hub orchestrator's usage charged to the runs, nil when it was not counted.
	Orch *OrchTally
}

// WriteMarkdown writes the total first, each role summed over all runs, then the orchestrator section, then one table per run.
// When the orch was counted, each role table gains an orch row of the orch usage charged to its runs.
func (r Report) WriteMarkdown(w io.Writer) error {
	total := map[string]*RoleTally{}
	duplicates := 0
	grand := 0.0
	var orchTotal *OrchCharge
	if r.Orch != nil {
		orchTotal = &OrchCharge{}
	}
	totalTimes := tableTime{Roles: map[string]time.Duration{}}
	var slept time.Duration
	for _, run := range r.Runs {
		duplicates += run.Duplicates
		totalTimes.Idle += run.Idle()
		totalTimes.Span += run.AwakeSpan()
		slept += run.Slept()
		for role := range run.Roles {
			totalTimes.Roles[role] += run.RoleTime(role)
		}
		if orchTotal != nil {
			charge := r.Orch.Charges[run.Slug]
			orchTotal.Tokens += charge.Tokens
			orchTotal.Weight += charge.Weight
			orchTotal.Cost.add(charge.Cost)
			grand += charge.Weight
		}
		for role, t := range run.Roles {
			grand += t.Usage.Weight()
			sum := total[role]
			if sum == nil {
				sum = &RoleTally{Role: role, Models: map[string]int{}}
				total[role] = sum
			}
			sum.Sessions += t.Sessions
			sum.Usage.add(t.Usage)
			sum.Cost.add(t.Cost)
			for model, n := range t.Models {
				sum.Models[model] += n
			}
		}
	}

	fmt.Fprintf(w, "## All runs\n\n")
	writeTable(w, total, totalTimes, orchTotal)
	fmt.Fprintf(w, "Total weight %.1fM; %d repeated transcript lines skipped.\n\n", grand/1e6, duplicates)
	fmt.Fprintln(w, "Time is turn time: a session works from a user or assistant line until the line that ends its turn.")
	fmt.Fprintf(w, "An interval into a line that starts a turn, or longer than the stall cap of %s, is idle.\n", stallCap)
	fmt.Fprintln(w, "Idle is the time no session of the run is in a turn, which holds parent review, PR-Gate, gate-slot waits, a parked driver and Go-only work such as verify and push.")
	fmt.Fprintln(w, "Time shares can sum past 100% where sessions overlap, a `+sub` row overlapping its parent.")
	fmt.Fprintln(w, "The union of working time plus idle is the span.")
	if slept > 0 {
		fmt.Fprintf(w, "Every time figure leaves out machine sleep, %s over these runs.\n", slept.Round(time.Second))
	}
	fmt.Fprintln(w)
	if rows := sumProducers(r.Runs); len(rows) > 0 {
		writeProducers(w, rows)
	}
	var cost Cost
	for _, t := range total {
		cost.add(t.Cost)
	}
	if orchTotal != nil {
		cost.add(orchTotal.Cost)
	}
	if len(cost.Unpriced) > 0 {
		fmt.Fprintf(w, "Unpriced models (messages), in no cost figure: %s.\n\n", models(cost.Unpriced))
	}
	if r.Orch != nil {
		r.Orch.WriteMarkdown(w, r.Runs)
	}

	for _, run := range r.Runs {
		fmt.Fprintf(w, "## %s\n\n", run.Slug)
		var charge *OrchCharge
		if r.Orch != nil {
			charge = r.Orch.Charges[run.Slug]
		}
		times := tableTime{Roles: map[string]time.Duration{}, Idle: run.Idle(), Span: run.AwakeSpan()}
		for role := range run.Roles {
			times.Roles[role] = run.RoleTime(role)
		}
		writeTable(w, run.Roles, times, charge)
		if run.LeftOut > 0 {
			fmt.Fprintf(w, "Session transcripts left out: %d; they started before the slug's previous run ended, at %s.\n\n", run.LeftOut, run.Bound.Format(time.RFC3339))
		}
		if slept := run.Slept(); slept > 0 {
			fmt.Fprintf(w, "Machine sleep left out of the span: %s.\n\n", slept.Round(time.Second))
		}
		if run.Shed != nil {
			writeProducers(w, run.Producers())
		} else {
			fmt.Fprintln(w, "No shed status was read, so the span is the transcript window and no Producers table is printed.")
			fmt.Fprintln(w)
		}
	}

	fmt.Fprintf(w, "## Webster forks\n\n")
	fmt.Fprintln(w, "| run | cards | start context | messages | peak context | weight |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|")
	for _, run := range r.Runs {
		for _, f := range run.Forks {
			cards := "unattributed"
			if len(f.Cards) > 0 {
				cards = strings.Join(f.Cards, ", ")
			}
			fmt.Fprintf(w, "| %s | %s | %d | %d | %d | %.1fM |\n", f.Slug, cards, f.StartContext, f.Messages, f.PeakContext, f.Usage.Weight()/1e6)
		}
	}
	fmt.Fprintln(w)
	return nil
}

// tableTime is one role table's time figures: a duration per role, the idle duration and the span duration.
type tableTime struct {
	Roles map[string]time.Duration
	Idle  time.Duration
	Span  time.Duration
}

// writeTable writes one role table, then an idle row, and below it an orch row of the charge orch when orch is not nil;
// the shares divide by the table's total weight, the orch row's included, and the time shares by the span in times.
func writeTable(w io.Writer, roles map[string]*RoleTally, times tableTime, orch *OrchCharge) {
	tallies := make([]*RoleTally, 0, len(roles))
	for _, t := range roles {
		tallies = append(tallies, t)
	}
	sort.Slice(tallies, func(i, j int) bool {
		if tallies[i].Usage.Weight() != tallies[j].Usage.Weight() {
			return tallies[i].Usage.Weight() > tallies[j].Usage.Weight()
		}
		return tallies[i].Role < tallies[j].Role
	})
	tableWeight := 0.0
	for _, t := range tallies {
		tableWeight += t.Usage.Weight()
	}
	if orch != nil {
		tableWeight += orch.Weight
	}
	share := func(weight float64) float64 {
		if tableWeight == 0 {
			return 0
		}
		return 100 * weight / tableWeight
	}
	timeCells := func(d time.Duration) string {
		pct := 0.0
		if times.Span > 0 {
			pct = 100 * float64(d) / float64(times.Span)
		}
		return fmt.Sprintf("%s | %.1f%%", d.Round(time.Second), pct)
	}
	fmt.Fprintln(w, "| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|---|---|---|")
	for _, t := range tallies {
		u := t.Usage
		fmt.Fprintf(w, "| %s | %d | %d | %d | %d | %d | %.1fM | %.1f%% | %s | %s | %s |\n",
			t.Role, t.Sessions, u.Output, u.CacheCreate, u.CacheRead, u.Input, u.Weight()/1e6, share(u.Weight()), timeCells(times.Roles[t.Role]), t.Cost, models(t.Models))
	}
	fmt.Fprintf(w, "| idle | | | | | | | | %s | | |\n", timeCells(times.Idle))
	if orch != nil {
		fmt.Fprintf(w, "| %s | | | | | | %.1fM | %.1f%% | | | %s | |\n", orchRole, orch.Weight/1e6, share(orch.Weight), orch.Cost)
	}
	fmt.Fprintln(w)
}

func models(counts map[string]int) string {
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s: %d", name, counts[name]))
	}
	return strings.Join(parts, ", ")
}
