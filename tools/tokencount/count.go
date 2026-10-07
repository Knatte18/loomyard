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
}

func (u *Usage) add(o Usage) {
	u.Input += o.Input
	u.Output += o.Output
	u.CacheCreate += o.CacheCreate
	u.CacheRead += o.CacheRead
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
	Models   map[string]int // assistant messages per model
}

// RunTally is one task run's usage, split by role.
type RunTally struct {
	Slug       string
	Roles      map[string]*RoleTally
	Duplicates int // transcript lines skipped as a repeat of a message already counted
}

// line is the part of a transcript line tokencount reads.
type line struct {
	Type        string `json:"type"`
	CustomTitle string `json:"customTitle"`
	UUID        string `json:"uuid"`
	Message     *struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *Usage `json:"usage"`
	} `json:"message"`
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
		if err := run.countFile(session, role, seen); err != nil {
			return RunTally{}, err
		}
		subs, err := filepath.Glob(filepath.Join(strings.TrimSuffix(session, ".jsonl"), "subagents", "*.jsonl"))
		if err != nil {
			return RunTally{}, err
		}
		sort.Strings(subs)
		for _, sub := range subs {
			if err := run.countFile(sub, role+"+sub", seen); err != nil {
				return RunTally{}, err
			}
		}
	}
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

func (run *RunTally) countFile(path, role string, seen map[string]bool) error {
	tally := run.Roles[role]
	if tally == nil {
		tally = &RoleTally{Role: role, Models: map[string]int{}}
		run.Roles[role] = tally
	}
	tally.Sessions++
	return eachLine(path, func(l line) {
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
		tally.Models[l.Message.Model]++
	})
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
			for model, n := range t.Models {
				sum.Models[model] += n
			}
		}
	}

	fmt.Fprintf(w, "## All runs\n\n")
	writeTable(w, total)
	fmt.Fprintf(w, "Total weight %.1fM; %d repeated transcript lines skipped.\n\n", grand/1e6, duplicates)

	for _, run := range r.Runs {
		fmt.Fprintf(w, "## %s\n\n", run.Slug)
		writeTable(w, run.Roles)
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
	fmt.Fprintln(w, "| role | sessions | output | cache write | cache read | input | weight | share | models (messages) |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|")
	for _, t := range tallies {
		u := t.Usage
		share := 0.0
		if tableWeight > 0 {
			share = 100 * u.Weight() / tableWeight
		}
		fmt.Fprintf(w, "| %s | %d | %d | %d | %d | %d | %.1fM | %.1f%% | %s |\n",
			t.Role, t.Sessions, u.Output, u.CacheCreate, u.CacheRead, u.Input, u.Weight()/1e6, share, models(t.Models))
	}
	fmt.Fprintln(w)
}

func models(counts map[string]int) string {
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return counts[names[i]] > counts[names[j]] })
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s: %d", name, counts[name]))
	}
	return strings.Join(parts, ", ")
}
