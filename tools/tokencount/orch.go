// orch.go counts the hub orchestrator's sessions in the prime and charges their usage to the counted runs by time.

package main

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// orchRole is the role of the hub orchestrator's sessions.
const orchRole = "orch"

// orchAttributionRule is the report's statement of how orch usage is charged to runs.
const orchAttributionRule = "Attribution is an estimate: an orch sub-agent whose first prompt names exactly one counted run goes wholly to that run; " +
	"every other orch message is split equally among the counted runs whose active window, first to last message, holds its timestamp, " +
	"and one outside every window is unattributed."

// OrchPart is the counted orch usage of one kind of transcript: the main sessions, or their sub-agents.
type OrchPart struct {
	// Sessions counts the transcripts with at least one counted message.
	Sessions int
	Messages int
	Usage    Usage
	Cost     Cost
}

// OrchCharge is the orch usage charged to one run, or left unattributed.
// Tokens is fractional because a message in several runs' windows is split equally among them.
type OrchCharge struct {
	Tokens float64
	Cost   Cost
}

// OrchTally is the hub orchestrator's usage between the earliest start and the latest end of the counted runs' active windows, charged to those runs.
type OrchTally struct {
	// From and To bound the counted orch messages; both are zero when no counted run has a window, and then nothing is counted.
	From, To time.Time
	// Main is the orch sessions' own usage and Sub their sub-agents'.
	Main, Sub OrchPart
	// Charges holds every counted run's charge, keyed by slug.
	Charges      map[string]*OrchCharge
	Unattributed OrchCharge
}

// orchMessage is one counted orch assistant message.
type orchMessage struct {
	At    time.Time
	Usage Usage
	Cost  Cost
}

// CountOrch tallies the orch sessions in the prime's project directory dir, each session whose latest custom title has role orch, and their sub-agents, charging every message to runs.
// Usage is counted once per message id, and only messages timestamped within the span of the runs' active windows count.
func CountOrch(dir string, runs []RunTally) (OrchTally, error) {
	tally := OrchTally{Charges: map[string]*OrchCharge{}}
	slugPatterns := map[string]*regexp.Regexp{}
	for _, run := range runs {
		tally.Charges[run.Slug] = &OrchCharge{}
		slugPatterns[run.Slug] = regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(run.Slug) + `(?:$|[^A-Za-z0-9_-])`)
		if run.First.IsZero() {
			continue
		}
		if tally.From.IsZero() || run.First.Before(tally.From) {
			tally.From = run.First
		}
		if run.Last.After(tally.To) {
			tally.To = run.Last
		}
	}
	if tally.From.IsZero() {
		return tally, nil
	}

	sessions, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return OrchTally{}, err
	}
	sort.Strings(sessions)
	seen := map[string]bool{}
	for _, session := range sessions {
		title, err := latestTitle(session)
		if err != nil {
			return OrchTally{}, err
		}
		if roleOf(title) != orchRole {
			continue
		}
		_, messages, err := tally.readTranscript(session, seen)
		if err != nil {
			return OrchTally{}, err
		}
		tally.Main.add(messages)
		for _, m := range messages {
			tally.chargeByTime(m, runs)
		}

		subs, err := filepath.Glob(filepath.Join(strings.TrimSuffix(session, ".jsonl"), "subagents", "*.jsonl"))
		if err != nil {
			return OrchTally{}, err
		}
		sort.Strings(subs)
		for _, sub := range subs {
			prompt, messages, err := tally.readTranscript(sub, seen)
			if err != nil {
				return OrchTally{}, err
			}
			tally.Sub.add(messages)
			var named []string
			for slug, pattern := range slugPatterns {
				if pattern.MatchString(prompt) {
					named = append(named, slug)
				}
			}
			for _, m := range messages {
				if len(named) == 1 {
					tally.Charges[named[0]].add(float64(m.Usage.Total()), m.Cost)
				} else {
					tally.chargeByTime(m, runs)
				}
			}
		}
	}
	return tally, nil
}

// readTranscript returns the text of an orch transcript's first user message that holds text, and its assistant messages not already in seen and timestamped within the tally's span.
func (tally *OrchTally) readTranscript(path string, seen map[string]bool) (string, []orchMessage, error) {
	prompt := ""
	var messages []orchMessage
	err := eachLine(path, func(l line) {
		if l.Message == nil {
			return
		}
		if prompt == "" && l.Type == "user" {
			prompt = strings.Join(userTexts(l.Message.Content), "\n")
		}
		if l.Type != "assistant" || l.Message.Usage == nil {
			return
		}
		id := l.Message.ID
		if id == "" {
			id = l.UUID
		}
		if seen[id] {
			return
		}
		seen[id] = true
		at, err := time.Parse(time.RFC3339Nano, l.Timestamp)
		if err != nil || at.Before(tally.From) || at.After(tally.To) {
			return
		}
		messages = append(messages, orchMessage{At: at, Usage: *l.Message.Usage, Cost: messageCost(l.Message.Model, *l.Message.Usage)})
	})
	return prompt, messages, err
}

// chargeByTime splits a message equally among the runs whose active window holds its timestamp, or leaves it unattributed when none does.
func (tally *OrchTally) chargeByTime(m orchMessage, runs []RunTally) {
	var holders []string
	for _, run := range runs {
		if !run.First.IsZero() && !m.At.Before(run.First) && !m.At.After(run.Last) {
			holders = append(holders, run.Slug)
		}
	}
	if len(holders) == 0 {
		tally.Unattributed.add(float64(m.Usage.Total()), m.Cost)
		return
	}
	share := 1 / float64(len(holders))
	for _, slug := range holders {
		tally.Charges[slug].add(float64(m.Usage.Total())*share, m.Cost.share(share))
	}
}

func (p *OrchPart) add(messages []orchMessage) {
	if len(messages) > 0 {
		p.Sessions++
	}
	for _, m := range messages {
		p.Messages++
		p.Usage.add(m.Usage)
		p.Cost.add(m.Cost)
	}
}

func (c *OrchCharge) add(tokens float64, cost Cost) {
	c.Tokens += tokens
	c.Cost.add(cost)
}

// WriteMarkdown writes the "Orchestrator" section: the counted orch usage split into main sessions and sub-agents, then each run's charge beside the run's own cost, the unattributed rest and the attribution rule.
func (tally OrchTally) WriteMarkdown(w io.Writer, runs []RunTally) {
	fmt.Fprintf(w, "## Orchestrator\n\n")
	if tally.From.IsZero() {
		fmt.Fprintf(w, "No counted run has a timestamped message, so no orch message was counted.\n\n")
		return
	}
	fmt.Fprintf(w, "Orch sessions of the prime from %s to %s, the span of the counted runs.\n\n",
		tally.From.Local().Format("2006-01-02 15:04"), tally.To.Local().Format("2006-01-02 15:04"))

	fmt.Fprintln(w, "| part | sessions | messages | tokens | est. cost |")
	fmt.Fprintln(w, "|---|---|---|---|---|")
	total := OrchPart{Sessions: tally.Main.Sessions + tally.Sub.Sessions, Messages: tally.Main.Messages + tally.Sub.Messages}
	for _, part := range []OrchPart{tally.Main, tally.Sub} {
		total.Usage.add(part.Usage)
		total.Cost.add(part.Cost)
	}
	for _, row := range []struct {
		name string
		part OrchPart
	}{{orchRole, tally.Main}, {orchRole + "+sub", tally.Sub}, {"total", total}} {
		fmt.Fprintf(w, "| %s | %d | %d | %.1fM | %s |\n", row.name, row.part.Sessions, row.part.Messages, float64(row.part.Usage.Total())/1e6, row.part.Cost)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "| run | orch tokens | orch cost | run cost | orch share of run cost |")
	fmt.Fprintln(w, "|---|---|---|---|---|")
	for _, run := range runs {
		charge := tally.Charges[run.Slug]
		own := run.Cost()
		share := "n/a"
		if whole := own.USD + charge.Cost.USD; whole > 0 {
			share = fmt.Sprintf("%.1f%%", 100*charge.Cost.USD/whole)
		}
		fmt.Fprintf(w, "| %s | %.1fM | %s | %s | %s |\n", run.Slug, charge.Tokens/1e6, charge.Cost, own, share)
	}
	fmt.Fprintf(w, "| unattributed | %.1fM | %s | | |\n\n", tally.Unattributed.Tokens/1e6, tally.Unattributed.Cost)
	fmt.Fprintf(w, "%s\n\n", orchAttributionRule)
}
