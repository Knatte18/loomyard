// Command tokencount reads Claude Code's session transcripts for a set of task runs and
// writes a markdown report of their token use per run and per role, each role summed
// over all runs first.
//
//	go run ./tools/tokencount
//	go run ./tools/tokencount -last 10
//	go run ./tools/tokencount bugfix-sessions bugfix-webster
//	go run ./tools/tokencount -hub ~/Code/loomyard-LYXHUB -out .scratch/tokens.md test-suite-prune-core
//
// The two launchers beside this file, tokencount.sh and tokencount.cmd, are the intended
// entry points: each runs from the repository root and writes
// .scratch/token-usage-by-role.md, one fixed path overwritten per run, with the run's
// instant and slugs recorded inside the report.
//
// Each argument is a task slug, read as the worktree <hub>/<slug>; -hub defaults to the
// parent of the current directory, which is the hub when run from its prime.
// With no slugs it counts the -last finished runs whose sessions changed most recently,
// leaving out the prime, the current directory.
// A run is finished once its pair is torn down, which leaves an archive/<slug>/<sha> tag in
// the prime's fabric repository (the one repository flag, -weft, default the current worktree's weft sibling); a run still in flight,
// or parked before Webster, would count as one whose later steps cost nothing.
// Claude Code keeps a worktree's sessions in ~/.claude/projects/<encoded path>/, one
// <session>.jsonl per session, and a session's sub-agents (the Webster master's forks)
// in <session>/subagents/*.jsonl.
//
// A session's role is the part after the last colon of its latest custom-title line,
// which lyx sets to the strand name (ly:<slug>:burler), with a trailing -N dropped, so
// burler-2 counts as burler. A sub-agent counts under its parent's role with "+sub"
// appended, and a session without a title counts as "untitled".
//
// One API message can appear on several transcript lines, and a fork repeats its
// parent's context, so usage is counted once per message id within a run.
//
// One fork is one sub-agent transcript of a session whose role is webster;
// the report's "Webster forks" section lists every such fork of every run, by run as listed and then by start time, with the cards it ran, its start context, its counted messages, its peak context and its weight.
// A fork's start context is the input + cache write + cache read of its first counted message, the context Merriam held when it spawned the fork, and its peak context is the largest such sum of any counted message;
// the fork also records the file name of the webster session transcript its own transcript sits under, its Merriam session.
// A fork's cards are read from its prompt, never from commit times: the prompt is the first tool result in the fork's transcript holding a card pointer line, a "- `<path>/NN-<slug>.md`" bullet with nothing after the closing backtick (a Read result's line-number prefix is ignored), and the cards are every such line of that one result.
// A fork whose transcript has no such result is listed as unattributed.
//
// With -calibrate <profile> the report gains a "Calibration (<profile>)" section of two tables, each row a fork that names cards.
// The one repository flag, -weft, names the repository holding the archive tags of finished runs, the runs' plan commits and the runs' webster records;
// its default, the current worktree's weft sibling, applies whenever the flag is empty and either no slugs are named or the calibration is asked for, so -calibrate needs no other flag.
// -config names the directory whose batcher.yaml holds the profile, default the current directory.
// The code repository is the current directory.
//
// A fork's position is one plus the card-naming forks before it in the same Merriam session, by start time, so a resumed session restarts its positions at 1.
// The start table gives each fork's measured start context beside the run's computed Merriam base and the start the profile estimates for its position, base + orientation + (position - 1) x batch_growth.
// A run's base is websterengine.MerriamBaseOf over CLAUDE.md, the repository's copy of the Master template, and the orchestrator PATTERN directive rendered from the directive stencil and PATTERN.md, each at the run's base commit, and the run's 00-overview.md from its plan commit, priced at the profile's context_per_line.
// A run whose plan or base commit cannot be read keeps its rows, listed with the reason it has no base.
// Below it the section prints the least-squares fit of orientation and batch_growth, the measured start minus the base over position - 1 across the forks of the runs that have a base, beside the profile's values, with its residual spread: the 75th over the 25th percentile of each fork's measured start over its fitted start.
// With fewer than two distinct positions among those forks it prints "not fitted" with that reason, and a negative fitted coefficient is printed and marked unusable, since batcher.yaml refuses a negative weight.
// The fit is a report; nothing consumes it and the tool never edits batcher.yaml.
//
// The peak table gives, for every fork whose cards are all cards of the run's plan, one-card and multi-card alike, batcher.PeakContext of the fork's cards at its position beside the fork's measured peak context and their ratio, measured over estimate.
// A run's plan is read from the newest "loom: plan artifacts for <slug>" commit in the repository committed before the run's first Webster fork started.
// The estimate's tree is the run's base: the start_sha of the earliest successful begin-batch result in the run's webster session transcripts, the HEAD before the first batch forked, read from the code repository.
// The card's own text is not in the base tree, so the estimate leaves it out.
// The estimate starts from the run's computed Merriam base, so the peak table and the start table agree; a run without one is priced without it.
// Each row also gives the fork's in-fork growth: its measured peak minus its measured start, against its estimated peak minus the estimated start of its position, and the growth ratio of the two, "n/a" when the estimated growth is not positive.
// The section lists the runs, forks and cards it left out, each with its reason:
//   - a run: "no webster fork"; "no plan commit before the first fork"; "plan at <sha> does not parse: <error>"; "no base: no begin-batch result in its webster sessions"; "base <sha> is not in the repository";
//   - a fork: "names a card the plan lacks: <id>"; "estimate is 0";
//   - a card: "no fork names it".
//
// The section ends with the fit per run and overall: the number of forks, the median peak ratio and its spread, and the number of forks with a growth ratio, their median growth ratio and its spread.
// A spread is the 75th percentile of the ratios over the 25th, both interpolated linearly between the closest ranks.
//
// The calibration section also ends with a "Merriam fixed context" subsection, which re-measures websterengine's merriamFixedContext.
// A Merriam session is a webster-role session, and its start context is the input + cache write + cache read of the first assistant message after the result of its Read of the prompt file named by the launch line lyx types.
// For each run's first Merriam session by start time, the subsection gives the measured start, the line count of the texts MerriamBase counts but the plan overview (CLAUDE.md, the Master template and the rendered directive; CLAUDE.local.md is assumed absent), and the fixed context left once those lines are priced at context_per_line.
// Below the table it gives the median and the 25th and 75th percentiles of the fixed figures beside the current constant.
// A session with no measured start, a run whose texts cannot be reconstructed, and every later session of a run (a resume, a refresh or a re-launch starts with prior conversation or a compaction summary in context) are listed with their reasons.
//
// With -calibrate the report then gains three more sections, read from the same repository through gitrepo's read side: the cost of each batch, one row per run and one summary line per profile.
// A run's webster records are the commits whose subject is "loom: webster run record for <slug>", and the last one is the newest;
// its state.json, decoded leniently so an older shape still reads, gives the partition and the batch records.
// A batch's profile is its recorded partition batch's profile, and a run with no recorded partition counts as "identity", one batch per card, with its cards counted from the run's plan commit.
// A batch's weight and measured peak come from the run's forks whose transcript file name is among its record's fork transcripts;
// a batch is failed when its record's terminal status is failed, dead or stuck or its kind is recovery, and a record of kind recovery is one recovery.
// A run whose records carry more than one run guid was restarted with --fresh, and a run whose state cannot be read or decoded has no partition to name a profile;
// each keeps a row marked with its reason and enters no profile line.
// Webster-Review findings are those of the round-<n>-review.md files directly in the webster review directory at the last record, counted through the review parser;
// no such file, or one that does not parse, marks the run's findings not read.
// Per-batch figures go to each batch's own profile, so a run whose batches name several profiles contributes to each.
// A run contributes findings to a profile line only when its batches name one profile, it was not restarted with --fresh and its review files were read, and the line's findings per card is over the cards of those runs alone, with their number stated, or "n/a" when there are none.
//
// The weight column is input + 1.25 x cache writes + 0.1 x cache reads + 5 x output: a
// relative figure for ranking roles, not a price, and blind to the per-model price
// difference shown in the models column.
// The share column is a role's weight as a percentage of its table's total weight.
// When the orchestrator was counted, the "All runs" table and each run's table end with an orch row: the orch weight and cost charged to the table's runs, the "Orchestrator" section's attributed part, never its unattributed rest;
// its weight counts in the table's total, so its share is orch's percentage of all the weight spent on those runs.
//
// The est. cost column prices each message at its model's Claude API list price, from the price table in cost.go, which cites its source;
// a cache write is priced by its lifetime, five minutes or one hour, as the transcript's usage splits it, and a model with a long-prompt rate card, Haiku 5.5 above 100K tokens of input + cache write + cache read, is priced per message on the card its prompt selects.
// A message with usage whose model the table lacks is unpriced, never free: its cell reads "unpriced" or "$x + unpriced", and the "All runs" section names the unpriced models.
//
// Right after the "All runs" section, before the per-run sections, comes the "Orchestrator" section: the hub orchestrator's own session, which runs in the prime and spans many runs, charged to the counted runs.
// An orch session is a session in the prime's project directory whose role is orch (ly:orch, lyxhub:orch), and its sub-agents, the orch's review, gate and stop forks, count as orch+sub;
// usage is counted once per message id across them.
// A run's active window runs from its earliest to its latest user or assistant line over all its transcripts, and only orch messages between the earliest window start and the latest window end are counted, so the orch's older history stays out.
// An orch sub-agent whose first user prompt names exactly one counted run slug goes wholly to that run;
// every other orch message is split equally among the runs whose window holds its timestamp, and one outside every window goes to the unattributed row.
// The section gives the orch total split into main session and orch+sub, then per run the orch tokens and cost charged to it, the run's own cost and orch's percentage of the run's cost with orch included.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// repositoryFlag is the name of the one flag naming the repository that holds the archive tags, plan commits and webster records.
const repositoryFlag = "weft"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "tokencount:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("tokencount", flag.ContinueOnError)
	hub := fs.String("hub", "", "hub directory holding the task worktrees (default: parent of the current directory)")
	projects := fs.String("projects", "", "Claude Code projects directory (default: ~/.claude/projects)")
	out := fs.String("out", "", "write the report to this file instead of stdout")
	last := fs.Int("last", 6, "with no slugs named, count this many of the most recently active finished runs")
	weft := fs.String(repositoryFlag, "", "the prime's fabric repository holding the archive tags of finished runs, the runs' plan commits and webster records (default: the current worktree's weft sibling)")
	calibrate := fs.String("calibrate", "", "add the calibration section for this batcher.yaml profile")
	configDir := fs.String("config", "", "directory whose batcher.yaml holds the profile (default: the current directory)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	wd, err := lyxcwd.Getwd()
	if err != nil {
		return err
	}
	if *hub == "" {
		*hub = filepath.Dir(wd)
	}
	if *projects == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		*projects = filepath.Join(home, ".claude", "projects")
	}
	slugs := fs.Args()
	if *weft == "" && (len(slugs) == 0 || *calibrate != "") {
		loc, err := lyxcwd.Resolve(wd)
		if err != nil {
			return err
		}
		*weft = fabricengine.RecordsWorktree(loc)
	}
	if len(slugs) == 0 {
		finished, err := finishedRuns(*weft)
		if err != nil {
			return err
		}
		slugs, err = recentRuns(*projects, *hub, filepath.Base(wd), finished, *last)
		if err != nil {
			return err
		}
		if len(slugs) == 0 {
			return fmt.Errorf("no finished task runs of %s under %s", *hub, *projects)
		}
	}

	report := Report{}
	for _, slug := range slugs {
		r, err := CountRun(projectDir(*projects, filepath.Join(*hub, slug)), slug)
		if err != nil {
			return err
		}
		report.Runs = append(report.Runs, r)
	}
	orch, err := CountOrch(projectDir(*projects, wd), report.Runs)
	if err != nil {
		return err
	}
	report.Orch = &orch

	var calibration *Calibration
	var profiles *ProfileReport
	if *calibrate != "" {
		if *configDir == "" {
			*configDir = wd
		}
		fabric := gitrepo.New(*weft)
		c, err := Calibrate(report.Runs, *calibrate, *configDir, fabric, gitrepo.New(wd))
		if err != nil {
			return err
		}
		calibration = &c
		p, err := BuildProfileReport(report.Runs, fabric)
		if err != nil {
			return err
		}
		profiles = &p
	}

	w := stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	fmt.Fprintf(w, "# Token usage by role\n\nGenerated %s for %s.\n\n", time.Now().Format("2006-01-02 15:04"), strings.Join(slugs, ", "))
	if err := report.WriteMarkdown(w); err != nil {
		return err
	}
	if calibration != nil {
		calibration.WriteMarkdown(w)
	}
	if profiles != nil {
		profiles.WriteMarkdown(w)
	}
	if *out != "" {
		fmt.Fprintln(stdout, "wrote", *out)
	}
	return nil
}
