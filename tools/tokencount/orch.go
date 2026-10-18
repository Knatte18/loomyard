// orch.go counts the hub orchestrator's sessions in the prime and charges their usage to the counted runs by time.

package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
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

// OrchShare is an amount of charged orch usage.
// Tokens and Weight are fractional because a message in several runs' windows is split equally among them.
type OrchShare struct {
	Tokens float64
	// Weight is the charged messages' Usage.Weight, the unit of the segment tables' weight column.
	Weight float64
	Cost   Cost
}

// OrchCharge is the orch usage charged to one run, or left unattributed, split into the orch sessions' own messages and their sub-agents'.
type OrchCharge struct {
	Session, Forks OrchShare
}

// Total is the whole charge, session and forks together.
func (c OrchCharge) Total() OrchShare {
	var total OrchShare
	total.add(c.Session)
	total.add(c.Forks)
	return total
}

// side is the session or forks share of the charge.
func (c *OrchCharge) side(fork bool) *OrchShare {
	if fork {
		return &c.Forks
	}
	return &c.Session
}

func (s *OrchShare) add(o OrchShare) {
	s.Tokens += o.Tokens
	s.Weight += o.Weight
	s.Cost.add(o.Cost)
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

// CountOrch tallies the orch sessions the engines in reads recorded under the prime worktree, each session whose strand name has role orch, and their forks, charging every message to runs.
// Usage is counted once per engine and message key, and only messages timestamped within the span of the runs' active windows count.
// It fails when an engine fails to locate or read a session.
func CountOrch(reads ReadSet, prime string, runs []RunTally) (OrchTally, error) {
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

	located, err := locateSessions(reads, prime)
	if err != nil {
		return OrchTally{}, err
	}
	seen := map[string]bool{}
	for _, l := range located {
		session, err := readLocated(reads, l)
		if err != nil {
			return OrchTally{}, err
		}
		if roleOf(session.Main.Name) != orchRole {
			continue
		}
		pricer := reads[l.engine].Pricer
		_, messages := tally.readTranscript(l.engine, pricer, session.Main, seen)
		tally.Main.add(messages)
		for _, m := range messages {
			tally.chargeByTime(m, runs, false)
		}

		for _, forkRead := range session.Forks {
			prompt, messages := tally.readTranscript(l.engine, pricer, forkRead, seen)
			tally.Sub.add(messages)
			var named []string
			for slug, pattern := range slugPatterns {
				if pattern.MatchString(prompt) {
					named = append(named, slug)
				}
			}
			for _, m := range messages {
				if len(named) == 1 {
					tally.Charges[named[0]].Forks.charge(m, 1)
				} else {
					tally.chargeByTime(m, runs, true)
				}
			}
		}
	}
	return tally, nil
}

// readTranscript returns the text of an orch transcript's first user entry that holds text, and its messages not already in seen and timestamped within the tally's span.
// Each message is priced through the pricer of the engine that read it.
func (tally *OrchTally) readTranscript(engine string, pricer shuttleengine.MessagePricer, read shuttleengine.TranscriptRead, seen map[string]bool) (string, []orchMessage) {
	prompt := ""
	var messages []orchMessage
	for _, e := range read.Entries {
		if prompt == "" && e.Role == shuttleengine.EntryUser {
			prompt = strings.Join(e.UserTexts, "\n")
		}
		if e.Usage == nil {
			continue
		}
		id := messageKey(engine, e)
		if seen[id] {
			continue
		}
		seen[id] = true
		if e.At.IsZero() || e.At.Before(tally.From) || e.At.After(tally.To) {
			continue
		}
		messages = append(messages, orchMessage{At: e.At, Usage: usageOf(*e.Usage), Cost: messageCost(pricer, e.Model, *e.Usage)})
	}
	return prompt, messages
}

// chargeByTime splits a message equally among the runs whose active window holds its timestamp, or leaves it unattributed when none does;
// fork says whether it charges the forks side or the session side.
func (tally *OrchTally) chargeByTime(m orchMessage, runs []RunTally, fork bool) {
	var holders []string
	for _, run := range runs {
		if !run.First.IsZero() && !m.At.Before(run.First) && !m.At.After(run.Last) {
			holders = append(holders, run.Slug)
		}
	}
	if len(holders) == 0 {
		tally.Unattributed.side(fork).charge(m, 1)
		return
	}
	share := 1 / float64(len(holders))
	for _, slug := range holders {
		tally.Charges[slug].side(fork).charge(m, share)
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

// charge adds the fraction f of message m.
func (s *OrchShare) charge(m orchMessage, f float64) {
	s.Tokens += float64(m.Usage.Total()) * f
	s.Weight += m.Usage.Weight() * f
	s.Cost.add(m.Cost.share(f))
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
	}{{orchSessionLabel, tally.Main}, {orchForksLabel, tally.Sub}, {"total", total}} {
		fmt.Fprintf(w, "| %s | %d | %d | %.1fM | %s |\n", row.name, row.part.Sessions, row.part.Messages, float64(row.part.Usage.Total())/1e6, row.part.Cost)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "| run | orch tokens | orch cost | run cost | orch share of run cost |")
	fmt.Fprintln(w, "|---|---|---|---|---|")
	for _, run := range runs {
		charge := tally.Charges[run.Slug].Total()
		own := run.Cost()
		share := "n/a"
		if whole := own.USD + charge.Cost.USD; whole > 0 {
			share = fmt.Sprintf("%.1f%%", 100*charge.Cost.USD/whole)
		}
		fmt.Fprintf(w, "| %s | %.1fM | %s | %s | %s |\n", run.Slug, charge.Tokens/1e6, charge.Cost, own, share)
	}
	unattributed := tally.Unattributed.Total()
	fmt.Fprintf(w, "| unattributed | %.1fM | %s | | |\n\n", unattributed.Tokens/1e6, unattributed.Cost)
	fmt.Fprintf(w, "%s\n\n", orchAttributionRule)
}
