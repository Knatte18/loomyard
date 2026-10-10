package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// Usage is a token tally: one message's usage, or a sum over many.
type Usage struct {
	Input       int `json:"input_tokens"`
	Output      int `json:"output_tokens"`
	CacheCreate int `json:"cache_creation_input_tokens"`
	CacheRead   int `json:"cache_read_input_tokens"`
	// CacheCreation splits CacheCreate by cache lifetime, which sets a cache write's price.
	CacheCreation CacheCreation `json:"cache_creation"`
}

// CacheCreation is the part of a tally's cache writes made with the one-hour lifetime;
// the rest of the writes have the default five-minute lifetime.
type CacheCreation struct {
	OneHour int `json:"ephemeral_1h_input_tokens"`
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
	// BaseSHA is the start_sha of the earliest successful begin-batch result in the run's webster sessions, the HEAD before its first batch forked;
	// empty when there is none.
	BaseSHA string
	// baseAt is the timestamp of the result BaseSHA came from.
	baseAt time.Time
	// First and Last bound the run's active window: the earliest and latest timestamp of a user or assistant line in any of its transcripts;
	// both are zero when no such line carries one.
	First, Last time.Time
}

// Cost is the run's estimated cost, summed over its roles.
func (run RunTally) Cost() Cost {
	var total Cost
	for _, t := range run.Roles {
		total.add(t.Cost)
	}
	return total
}

// observeTime widens the run's active window to hold a user or assistant line's timestamp.
func (run *RunTally) observeTime(l line) {
	if l.Type != "user" && l.Type != "assistant" {
		return
	}
	at, err := time.Parse(time.RFC3339Nano, l.Timestamp)
	if err != nil {
		return
	}
	if run.First.IsZero() || at.Before(run.First) {
		run.First = at
	}
	if at.After(run.Last) {
		run.Last = at
	}
}

// line is the part of a transcript line tokencount reads.
type line struct {
	Type        string `json:"type"`
	CustomTitle string `json:"customTitle"`
	UUID        string `json:"uuid"`
	Timestamp   string `json:"timestamp"`
	Message     *struct {
		ID      string          `json:"id"`
		Model   string          `json:"model"`
		Usage   *Usage          `json:"usage"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
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

// toolResultItem is one tool result of a user message: the tool use it answers and its text.
type toolResultItem struct {
	UseID string
	Text  string
}

// toolResultItems returns every tool result in a user message's content, which is a string or a list of items;
// a tool result's own content is likewise a string or a list of text items.
func toolResultItems(content json.RawMessage) []toolResultItem {
	var items []struct {
		Type      string          `json:"type"`
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
	}
	if json.Unmarshal(content, &items) != nil {
		return nil
	}
	var results []toolResultItem
	for _, item := range items {
		if item.Type != "tool_result" {
			continue
		}
		var text string
		if json.Unmarshal(item.Content, &text) == nil {
			results = append(results, toolResultItem{item.ToolUseID, text})
			continue
		}
		var parts []struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(item.Content, &parts) != nil {
			continue
		}
		var joined []string
		for _, p := range parts {
			joined = append(joined, p.Text)
		}
		results = append(results, toolResultItem{item.ToolUseID, strings.Join(joined, "\n")})
	}
	return results
}

// toolResultTexts returns the text of every tool result in a user message's content.
func toolResultTexts(content json.RawMessage) []string {
	var texts []string
	for _, item := range toolResultItems(content) {
		texts = append(texts, item.Text)
	}
	return texts
}

// beginBatchCommand matches a Bash command that invokes the webster begin-batch verb.
var beginBatchCommand = regexp.MustCompile(`\blyx\s+webster\s+begin-batch\b`)

// bashCommands returns the command of every Bash tool use in an assistant message's content, keyed by tool use id.
func bashCommands(content json.RawMessage) map[string]string {
	var items []struct {
		Type  string `json:"type"`
		ID    string `json:"id"`
		Name  string `json:"name"`
		Input struct {
			Command string `json:"command"`
		} `json:"input"`
	}
	if json.Unmarshal(content, &items) != nil {
		return nil
	}
	commands := map[string]string{}
	for _, item := range items {
		if item.Type == "tool_use" && item.Name == "Bash" {
			commands[item.ID] = item.Input.Command
		}
	}
	return commands
}

// recordBase scans one webster session for begin-batch result envelopes and keeps the start_sha of the earliest-timestamped one across every session of the run.
// An envelope is a tool result answering a begin-batch Bash call whose text is a JSON object with a non-empty start_sha;
// a refusal carries none.
func (run *RunTally) recordBase(path string) error {
	beginBatchUses := map[string]bool{}
	return eachLine(path, func(l line) {
		if l.Message == nil {
			return
		}
		switch l.Type {
		case "assistant":
			for id, command := range bashCommands(l.Message.Content) {
				if beginBatchCommand.MatchString(command) {
					beginBatchUses[id] = true
				}
			}
		case "user":
			at, err := time.Parse(time.RFC3339Nano, l.Timestamp)
			if err != nil {
				return
			}
			for _, item := range toolResultItems(l.Message.Content) {
				if !beginBatchUses[item.UseID] {
					continue
				}
				var envelope struct {
					StartSHA string `json:"start_sha"`
				}
				if json.Unmarshal([]byte(item.Text), &envelope) != nil || envelope.StartSHA == "" {
					continue
				}
				if run.baseAt.IsZero() || at.Before(run.baseAt) {
					run.BaseSHA, run.baseAt = envelope.StartSHA, at
				}
			}
		}
	})
}

// launchPointerLine matches the launch line lyx types into a Merriam session, capturing the prompt file's path.
var launchPointerLine = regexp.MustCompile(`(?m)^Read (.+) in full first; it is your complete, authoritative instructions\.\s*$`)

// userTexts returns the text of a user message's content, which is a string or a list of items, the text items of which count.
func userTexts(content json.RawMessage) []string {
	var text string
	if json.Unmarshal(content, &text) == nil {
		return []string{text}
	}
	var items []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &items) != nil {
		return nil
	}
	var texts []string
	for _, item := range items {
		if item.Type == "text" {
			texts = append(texts, item.Text)
		}
	}
	return texts
}

// readToolUseIDs returns the id of every Read tool use in an assistant message's content, keyed by the file path it reads.
func readToolUseIDs(content json.RawMessage) map[string]string {
	var items []struct {
		Type  string `json:"type"`
		ID    string `json:"id"`
		Name  string `json:"name"`
		Input struct {
			FilePath string `json:"file_path"`
		} `json:"input"`
	}
	if json.Unmarshal(content, &items) != nil {
		return nil
	}
	ids := map[string]string{}
	for _, item := range items {
		if item.Type == "tool_use" && item.Name == "Read" {
			if _, seen := ids[item.Input.FilePath]; !seen {
				ids[item.Input.FilePath] = item.ID
			}
		}
	}
	return ids
}

// readMerriamStart measures one Merriam session's start in the transcript at path.
// The prompt path comes from the first user message holding the launch line;
// the start is the context of the first assistant message with usage after the result of the first Read of that path.
func readMerriamStart(path string) (MerriamStart, error) {
	start := MerriamStart{Session: filepath.Base(path)}
	var promptPath, readUseID string
	resultSeen, measured := false, false
	err := eachLine(path, func(l line) {
		if start.Started.IsZero() && l.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, l.Timestamp); err == nil {
				start.Started = t
			}
		}
		if measured || l.Message == nil {
			return
		}
		switch l.Type {
		case "user":
			if promptPath == "" {
				for _, text := range userTexts(l.Message.Content) {
					if m := launchPointerLine.FindStringSubmatch(text); m != nil {
						promptPath = m[1]
						return
					}
				}
				return
			}
			if readUseID == "" || resultSeen {
				return
			}
			for _, item := range toolResultItems(l.Message.Content) {
				if item.UseID == readUseID {
					resultSeen = true
				}
			}
		case "assistant":
			if promptPath != "" && readUseID == "" {
				readUseID = readToolUseIDs(l.Message.Content)[promptPath]
				return
			}
			if resultSeen && l.Message.Usage != nil {
				u := l.Message.Usage
				start.StartContext = u.Input + u.CacheCreate + u.CacheRead
				measured = true
			}
		}
	})
	if err != nil {
		return MerriamStart{}, err
	}
	switch {
	case promptPath == "":
		start.NoStart = "no launch line"
	case readUseID == "":
		start.NoStart = "no Read of the prompt file " + promptPath
	case !measured:
		start.NoStart = "no assistant message after the prompt Read's result"
	}
	return start, nil
}

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

// projectDir is the directory Claude Code keeps a worktree's sessions in: the absolute path
// with every character but letters and digits turned into a hyphen.
func projectDir(projects, worktree string) string {
	return filepath.Join(projects, nonAlnum.ReplaceAllString(worktree, "-"))
}

var trailingNumber = regexp.MustCompile(`-\d+$`)

// roleOf turns a strand name into its role: the last colon-separated part, without a -N suffix.
func roleOf(title string) string {
	if title == "" {
		return "untitled"
	}
	return trailingNumber.ReplaceAllString(title[strings.LastIndex(title, ":")+1:], "")
}

// finishedRuns reads the slugs of the runs whose pairs were torn down from the archive tags
// in the weft repository at weft.
func finishedRuns(weft string) (map[string]bool, error) {
	out, err := gitexec.Run([]string{"for-each-ref", "--format=%(refname)", "refs/tags/archive/"}, weft)
	if err != nil {
		return nil, fmt.Errorf("reading the archive tags in %s: %w", weft, err)
	}
	return archivedSlugs(out), nil
}

// archivedSlugs parses for-each-ref output, one refs/tags/archive/<slug>/<sha> per line,
// into the set of slugs.
func archivedSlugs(refs string) map[string]bool {
	slugs := map[string]bool{}
	for _, line := range strings.Split(refs, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "refs/tags/archive/")
		if slug, _, found := strings.Cut(rest, "/"); ok && found && slug != "" {
			slugs[slug] = true
		}
	}
	return slugs
}

// recentRuns names the finished task runs of the hub whose sessions changed most recently,
// newest first, at most last of them: every project directory of a worktree under the hub
// whose slug is in finished, except the prime's own.
func recentRuns(projects, hub, prime string, finished map[string]bool, last int) ([]string, error) {
	prefix := filepath.Base(projectDir(projects, hub)) + "-"
	entries, err := os.ReadDir(projects)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		slug    string
		touched time.Time
	}
	var found []candidate
	for _, e := range entries {
		slug, ok := strings.CutPrefix(e.Name(), prefix)
		if !e.IsDir() || !ok || slug == "" || slug == prime || !finished[slug] {
			continue
		}
		sessions, err := filepath.Glob(filepath.Join(projects, e.Name(), "*.jsonl"))
		if err != nil {
			return nil, err
		}
		var touched time.Time
		for _, s := range sessions {
			if info, err := os.Stat(s); err == nil && info.ModTime().After(touched) {
				touched = info.ModTime()
			}
		}
		if !touched.IsZero() {
			found = append(found, candidate{slug, touched})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].touched.After(found[j].touched) })
	slugs := []string{}
	for i := 0; i < len(found) && i < last; i++ {
		slugs = append(slugs, found[i].slug)
	}
	return slugs, nil
}

// CountRun tallies every session Claude Code recorded in the project directory dir.
func CountRun(dir, slug string) (RunTally, error) {
	sessions, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return RunTally{}, err
	}
	if len(sessions) == 0 {
		return RunTally{}, fmt.Errorf("no sessions for %s under %s", slug, dir)
	}
	sort.Strings(sessions)

	run := RunTally{Slug: slug, Roles: map[string]*RoleTally{}}
	seen := map[string]bool{}
	for _, session := range sessions {
		title, err := latestTitle(session)
		if err != nil {
			return RunTally{}, err
		}
		role := roleOf(title)
		if err := run.countFile(session, role, seen, nil); err != nil {
			return RunTally{}, err
		}
		if role == websterMasterRole {
			if err := run.recordBase(session); err != nil {
				return RunTally{}, err
			}
			merriam, err := readMerriamStart(session)
			if err != nil {
				return RunTally{}, err
			}
			run.Merriams = append(run.Merriams, merriam)
		}
		subs, err := filepath.Glob(filepath.Join(strings.TrimSuffix(session, ".jsonl"), "subagents", "*.jsonl"))
		if err != nil {
			return RunTally{}, err
		}
		sort.Strings(subs)
		for _, sub := range subs {
			var fork *ForkTally
			if role == websterMasterRole {
				fork = &ForkTally{Slug: slug, File: filepath.Base(sub), Session: filepath.Base(session)}
			}
			if err := run.countFile(sub, role+"+sub", seen, fork); err != nil {
				return RunTally{}, err
			}
			if fork != nil {
				run.Forks = append(run.Forks, *fork)
			}
		}
	}
	sort.SliceStable(run.Forks, func(i, j int) bool { return run.Forks[i].Started.Before(run.Forks[j].Started) })
	sort.SliceStable(run.Merriams, func(i, j int) bool { return run.Merriams[i].Started.Before(run.Merriams[j].Started) })
	return run, nil
}

func latestTitle(path string) (string, error) {
	title := ""
	err := eachLine(path, func(l line) {
		if l.Type == "custom-title" && l.CustomTitle != "" {
			title = l.CustomTitle
		}
	})
	return title, err
}

// countFile tallies one transcript under role;
// a non-nil fork also receives the transcript's own tally and card attribution.
func (run *RunTally) countFile(path, role string, seen map[string]bool, fork *ForkTally) error {
	tally := run.Roles[role]
	if tally == nil {
		tally = &RoleTally{Role: role, Models: map[string]int{}}
		run.Roles[role] = tally
	}
	tally.Sessions++
	return eachLine(path, func(l line) {
		if fork != nil {
			fork.observe(l)
		}
		run.observeTime(l)
		if l.Type != "assistant" || l.Message == nil || l.Message.Usage == nil {
			return
		}
		id := l.Message.ID
		if id == "" {
			id = l.UUID
		}
		if seen[id] {
			run.Duplicates++
			return
		}
		seen[id] = true
		tally.Usage.add(*l.Message.Usage)
		tally.Cost.add(messageCost(l.Message.Model, *l.Message.Usage))
		tally.Models[l.Message.Model]++
		if fork != nil {
			fork.Messages++
			fork.Usage.add(*l.Message.Usage)
			u := l.Message.Usage
			context := u.Input + u.CacheCreate + u.CacheRead
			if fork.Messages == 1 {
				fork.StartContext = context
			}
			fork.PeakContext = max(fork.PeakContext, context)
		}
	})
}

// observe records a line's timestamp and, until a card pointer has been found, its card attribution: the fork's prompt is the first tool result holding a card pointer line.
func (f *ForkTally) observe(l line) {
	if f.Started.IsZero() && l.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, l.Timestamp); err == nil {
			f.Started = t
		}
	}
	if f.Cards != nil || l.Type != "user" || l.Message == nil {
		return
	}
	for _, text := range toolResultTexts(l.Message.Content) {
		if cards := cardPointers(text); len(cards) > 0 {
			f.Cards = cards
			return
		}
	}
}

// eachLine calls fn for every line of a JSONL file that parses, skipping any that do not.
func eachLine(path string, fn func(line)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for scanner.Scan() {
		var l line
		if json.Unmarshal(scanner.Bytes(), &l) == nil {
			fn(l)
		}
	}
	return scanner.Err()
}

// Report is every run counted in one invocation.
type Report struct {
	Runs []RunTally
	// Orch is the hub orchestrator's usage charged to the runs, nil when it was not counted.
	Orch *OrchTally
}

// WriteMarkdown writes the total first, each role summed over all runs, then one table per run.
func (r Report) WriteMarkdown(w io.Writer) error {
	total := map[string]*RoleTally{}
	duplicates := 0
	grand := 0.0
	for _, run := range r.Runs {
		duplicates += run.Duplicates
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
	writeTable(w, total)
	fmt.Fprintf(w, "Total weight %.1fM; %d repeated transcript lines skipped.\n\n", grand/1e6, duplicates)
	var cost Cost
	for _, t := range total {
		cost.add(t.Cost)
	}
	if len(cost.Unpriced) > 0 {
		fmt.Fprintf(w, "Unpriced models (messages), in no cost figure: %s.\n\n", models(cost.Unpriced))
	}

	for _, run := range r.Runs {
		fmt.Fprintf(w, "## %s\n\n", run.Slug)
		writeTable(w, run.Roles)
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
	if r.Orch != nil {
		r.Orch.WriteMarkdown(w, r.Runs)
	}
	return nil
}

func writeTable(w io.Writer, roles map[string]*RoleTally) {
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
	fmt.Fprintln(w, "| role | sessions | output | cache write | cache read | input | weight | share | est. cost | models (messages) |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|---|")
	for _, t := range tallies {
		u := t.Usage
		share := 0.0
		if tableWeight > 0 {
			share = 100 * u.Weight() / tableWeight
		}
		fmt.Fprintf(w, "| %s | %d | %d | %d | %d | %d | %.1fM | %.1f%% | %s | %s |\n",
			t.Role, t.Sessions, u.Output, u.CacheCreate, u.CacheRead, u.Input, u.Weight()/1e6, share, t.Cost, models(t.Models))
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
