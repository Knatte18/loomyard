// Command tokencount reads the session transcripts of a set of task runs through the shuttle engines that recorded them.
// It writes a markdown report of their token use and time per run and per role, each role summed over all runs first.
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
// With no slugs it counts the -last full loom runs whose sessions changed most recently,
// leaving out the prime, the current directory.
// A run is finished once its pair is torn down, which leaves an archive/<slug>/<sha> tag in
// the prime's fabric repository (the one repository flag, -weft, default the current worktree's weft sibling).
// A finished run is a full loom run when the seed.json and status.json at its newest tag name the loom recipe and hold a Finalize history entry;
// a run stopped partway, such as a bench pair parked after Discussion-Write, or a run of another recipe, would count as one whose later steps cost nothing.
// The report's header names the runs counted and, when the default skipped more recent runs that are not full loom runs, names those on one line.
// Slugs named on the command line are counted as given, full loom runs or not.
// Sessions are found per worktree by every registered shuttle engine that can locate them, and read through the engine that found them;
// a session's forks (the Webster master's sub-agents) are read with it.
// An engine that cannot read transcripts is named in the report's header line "Not counted, no transcript reader:".
// Absence contributes nothing, while an engine's failure to locate or read a session fails the count, naming the session and the engine.
//
// A slug's earlier runs share its worktree path, so they share its located sessions, and a run counts only the sessions that started after the slug's previous run.
// The run's lower bound is the commit date of the slug's newest archive tag for a run in flight, whose worktree <hub>/<slug> exists.
// For a finished run it is the commit date of the next-older tag, and the bound is absent when the slug has no such tag, which counts every transcript.
// A main-session transcript is judged by the earliest timestamp of its user and assistant entries, and one with no such entry is left out when a bound exists and counted under -no-cutoff.
// A left-out transcript adds nothing to tokens, cost, working intervals, the session count, the Webster forks, the Merriam starts, the run's begin-batch base or its transcript window, and neither do its forks.
// The run's section reports how many were left out and the bound's date.
// -no-cutoff drops the bound for every run, so a slug's earlier runs that share its worktree path feed every figure the bound gates (tokens, cost, working intervals, the session count, the Webster forks, the Merriam starts, the begin-batch base, the transcript window and with it the orch charge), and it drops the next-older tag's clamp on the span start.
// With -calibrate the base and Merriam rows may come from an earlier run.
// The plan, read from the newest plan commit before the run's first Webster fork, may be an earlier run's, and with it the peak table, the start table's base and the identity card counts, or the run may be listed with "no plan commit before the first fork".
// A span start taken from the earliest transcript when the seed has no started_at may be an earlier run's.
//
// A session's role is the part after the last colon of its name, the strand name its engine read (ly:<slug>:burler), with a trailing -N dropped, so burler-2 counts as burler.
// A fork counts under its parent's role with "+sub" appended, and a session without a name counts as "untitled".
// A role ending in -chair, the role a seat table's lead carried before seats were named, counts under the role without that suffix, so burler-review-chair counts as burler-review and a report spanning the rename sums each role once.
// A seat named chair after the rename counts under its table's lead role too.
//
// A message can be recorded in several transcripts, so usage is counted once per engine and message key within a run;
// the engine collapses a message it recorded over several records into one, and a repeat it collapsed counts as a duplicate.
//
// One fork is one fork transcript of a session whose role is webster;
// the report's "Webster forks" section lists every such fork of every run, by run as listed and then by start time, with the cards it ran, its start context, its counted messages, its peak context and its weight.
// A fork's start context is the input + cache write + cache read of its first counted message, the context Merriam held when it spawned the fork, and its peak context is the largest such sum of any counted message;
// the fork also records the name of the webster session transcript its own transcript sits under, its Merriam session.
// A fork's cards are read from its prompt, never from commit times: the prompt is the first tool result in the fork's transcript holding a card pointer line, a "- `<path>/NN-<slug>.md`" bullet with nothing after the closing backtick (a file read's line-number prefix is ignored), and the cards are every such line of that one result.
// A fork whose transcript has no such result is listed as unattributed.
//
// With -calibrate <profile> the report gains a "Calibration (<profile>)" section of two tables, each row a fork that names cards.
// The one repository flag, -weft, names the repository holding the archive tags of finished runs, the runs' plan commits and the runs' webster records;
// its default, the current worktree's weft sibling, applies whenever the flag is empty, slugs named or not, so -calibrate needs no other flag.
// A -hub naming a directory other than the current worktree's parent needs -weft, because the default would read another hub's records.
// -config names the directory whose batcher.yaml holds the profile, default the current directory.
// The code repository is the current directory.
//
// A fork's position is one plus the card-naming forks before it in the same Merriam session, by start time, so a resumed session restarts its positions at 1.
// The start table gives each fork's measured start context beside the run's computed Merriam base and the start the profile estimates for its position, base + orientation + (position - 1) x batch_growth.
// A run's base is websterengine.MerriamBaseOf over the instruction files of the engine that located the run's Webster session, the repository's copy of the Master template, and the orchestrator PATTERN directive rendered from the directive stencil and PATTERN.md, each at the run's base commit, and the run's 00-overview.md from its plan commit, priced at the profile's context_per_line.
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
// A Merriam session is a webster-role session, and its start context is the input + cache write + cache read of the first assistant message after the result of its read of the prompt file named by the launch line lyx types.
// For each run's first Merriam session by start time, the subsection gives the measured start, the line count of the texts MerriamBase counts but the plan overview (the Webster session engine's instruction files, the Master template and the rendered directive; an untracked instruction file is assumed absent), and the fixed context left once those lines are priced at context_per_line.
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
// The est. cost column prices each message through the pricing capability of the engine that read it, the engine owning its provider's price table;
// a message from an engine without one is unpriced.
// A message with usage that no price was found for is unpriced, never free: its cell reads "unpriced" or "$x + unpriced", and the "All runs" section names the unpriced models.
//
// The time and time share columns give a role's working time and its percentage of the table's span.
// Time is turn time: a session works from a user or assistant entry until the entry that ends its turn.
// Which entries end or start a turn is the engine's reading: an entry that ends a turn is a final assistant answer, an interrupt or a local command's output, and an entry that starts one is the user's own prompt.
// The interval into a turn-start entry is idle, since a crash and --resume leave the gap before the resume's prompt, and so is an interval longer than the 30-minute stall cap, three times the harness's ten-minute cap on a foreground Bash call.
// Either makes the whole interval idle, never a clipped part of it.
// A role's time is the union of its transcripts' working intervals clipped to the span, a sub-agent's transcripts counting under <role>+sub.
// The idle row is the span minus the union of every working interval of the run, orch sessions left out, so it holds parent review, gate-slot waits, a parked driver and Go-only work.
// The orch row's time cells are blank, and the "All runs" table sums each run's figures per role, idle and span, with no union across runs.
//
// Every time figure leaves out machine sleep: a suspend or hibernate is no time at all, neither working nor idle.
// The sleeps are read once from the systemd journal, `journalctl -t systemd-sleep`, and parsed by internal/sleepjournal, each running from systemd-sleep's "Performing sleep operation" entry to the next "System returned from sleep operation" entry of the same operation and boot.
// A working interval loses the part a sleep covers, and the span, the idle row and the Producers table's elapsed and idle count awake time only;
// each run's section names the sleep left out of its span, and "All runs" the sum.
// With journalctl absent or failing, or no sleep in its journal, as on Windows, nothing is left out and the report's header says why.
// -no-sleep reads no journal and counts sleep as time.
//
// The span is the window the time figures are measured over.
// It runs from the archived seed's started_at, or from the run's earliest transcript line without one.
// That start is bounded below by the commit date of the slug's next-older archive tag, unless -no-cutoff is set, and above by the end of the first history entry.
// It ends at the latest history entry's end.
// Both files are read from the slug's newest archive/<slug>/<sha> tag.
// A run whose worktree still exists, or with no tag or no status at its tag, uses its transcript window instead and prints no Producers table.
//
// The Producers table splits the span by shed history entry: each entry owns the interval from the latest end before it to its own end.
// So the elapsed figures sum to the awake span, and a row's elapsed holds any wait before the row started.
// A row's idle is the part of its elapsed in which no session of the run was in a turn, and its active time is elapsed minus idle.
// The share is a row's active time over the table's summed active time, and rows are ordered by active time, so a parked row cannot top the table.
// "All runs" sums the table per producer over the runs that have one.
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
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// repositoryFlag is the name of the one flag naming the repository that holds the archive tags, plan commits and webster records.
const repositoryFlag = "weft"

// resolveHub makes the hub flag's value absolute and clean: empty is the current worktree's parent, and a relative path joins onto wd.
// It refuses a hub other than that parent when weft is empty, because the default weft would then read another hub's records.
func resolveHub(hub, wd, weft string) (string, error) {
	parent := filepath.Clean(filepath.Dir(wd))
	switch {
	case hub == "":
		hub = parent
	case !filepath.IsAbs(hub):
		hub = filepath.Join(wd, hub)
	}
	hub = filepath.Clean(hub)
	if hub != parent && weft == "" {
		return "", fmt.Errorf("-hub %s is not the current directory's parent %s, which is the hub this run assumes; the default weft would read another hub's records, so -%s must name that hub's weft repository", hub, parent, repositoryFlag)
	}
	return hub, nil
}

func main() {
	if err := run(os.Args[1:], os.Stdout, shuttleengines.RecordReaders()); err != nil {
		fmt.Fprintln(os.Stderr, "tokencount:", err)
		os.Exit(1)
	}
}

// run counts the runs args name, or the most recent full loom runs, through the engines in reads, and writes the report to stdout or to the -out file.
func run(args []string, stdout io.Writer, reads ReadSet) error {
	fs := flag.NewFlagSet("tokencount", flag.ContinueOnError)
	hub := fs.String("hub", "", "hub directory holding the task worktrees (default: parent of the current directory)")
	out := fs.String("out", "", "write the report to this file instead of stdout")
	last := fs.Int("last", 6, "with no slugs named, count this many of the most recently active full loom runs")
	weft := fs.String(repositoryFlag, "", "the prime's fabric repository holding the archive tags of finished runs, the runs' plan commits and webster records (default: the current worktree's weft sibling)")
	calibrate := fs.String("calibrate", "", "add the calibration section for this batcher.yaml profile")
	configDir := fs.String("config", "", "directory whose batcher.yaml holds the profile (default: the current directory)")
	noCutoff := fs.Bool("no-cutoff", false, "turn off the archive-tag cutoff, so every located session of each slug counts")
	noSleep := fs.Bool("no-sleep", false, "read no journal and count machine sleep as time")
	if err := fs.Parse(args); err != nil {
		return err
	}
	wd, err := lyxcwd.Getwd()
	if err != nil {
		return err
	}
	*hub, err = resolveHub(*hub, wd, *weft)
	if err != nil {
		return err
	}
	readable, unreadable := reads.engines()
	slugs := fs.Args()
	if *weft == "" {
		loc, err := lyxcwd.Resolve(wd)
		if err != nil {
			return err
		}
		*weft = fabricengine.RecordsWorktree(loc)
	}
	fabric := gitrepo.New(*weft)
	finished, err := finishedRuns(*weft)
	if err != nil {
		return err
	}
	counted := strings.Join(slugs, ", ")
	var skippedNote string
	if len(slugs) == 0 {
		var skipped []string
		slugs, skipped, err = recentRuns(reads, *hub, filepath.Base(wd), finished, fabric, *last)
		if err != nil {
			return err
		}
		if len(slugs) == 0 {
			return fmt.Errorf("no full loom runs of %s from engines %s; name the slugs to count a partial run", *hub, strings.Join(readable, ", "))
		}
		counted = fmt.Sprintf("the %d most recent full loom runs: %s", len(slugs), strings.Join(slugs, ", "))
		if len(skipped) > 0 {
			skippedNote = fmt.Sprintf("Skipped %d more recent runs that are not full loom runs: %s.\n", len(skipped), strings.Join(skipped, ", "))
		}
	}

	var sleeps []Interval
	sleepNote := "Machine sleep counts as time: -no-sleep is set.\n"
	if !*noSleep {
		sleeps, sleepNote = journalSleeps()
	}
	report := Report{}
	for _, slug := range slugs {
		running, err := inFlight(*hub, slug)
		if err != nil {
			return err
		}
		var bound time.Time
		if !*noCutoff {
			bound = runBound(finished[slug], running)
		}
		r, err := CountRun(reads, filepath.Join(*hub, slug), slug, bound)
		if err != nil {
			return err
		}
		if !running {
			record, err := readShedRecord(fabric, slug, finished[slug])
			if err != nil {
				return err
			}
			if record != nil {
				if *noCutoff {
					record.Older = time.Time{}
				}
				r.useShedRecord(*record)
			}
		}
		r.subtractSleeps(sleeps)
		report.Runs = append(report.Runs, r)
	}
	orch, err := CountOrch(reads, wd, report.Runs)
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
		c, err := Calibrate(report.Runs, *calibrate, *configDir, fabric, gitrepo.New(wd), reads)
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
	var cutoffNote string
	if *noCutoff {
		cutoffNote = "The archive-tag cutoff is off: every located session of each slug counts.\n"
	}
	var unreadableNote string
	if len(unreadable) > 0 {
		unreadableNote = fmt.Sprintf("Not counted, no transcript reader: %s.\n", strings.Join(unreadable, ", "))
	}
	fmt.Fprintf(w, "# Token usage and time by role\n\nGenerated %s for %s.\n%s%s%s%s\n", time.Now().Format("2006-01-02 15:04"), counted, cutoffNote, sleepNote, unreadableNote, skippedNote)
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
