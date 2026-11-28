// Command tokencount reports the cost and time of task runs, read from their session transcripts.
// It writes a markdown report of each run's cost and time.
// Each segment's cost and time are split into its parts and summed over all runs first.
//
//	go run ./tools/tokencount
//	go run ./tools/tokencount -last 10
//	go run ./tools/tokencount -since 2026-10-31
//	go run ./tools/tokencount -detail bugfix-sessions bugfix-webster
//	go run ./tools/tokencount -hub ~/Code/loomyard-LYMHUB -out .scratch/tokens.md test-suite-prune-core
//
// The launchers beside this file, tokencount.sh and tokencount.cmd, are the intended entry points.
// Each runs from the repository root and writes .scratch/token-usage-by-role.md, a fixed path overwritten on every run.
// The report records the run's instant and slugs.
//
// Each argument is a task slug, read as the worktree <hub>/<slug>.
// -hub defaults to the parent of the current directory, which is the hub when run from its prime.
// With no slugs it counts the -last finished runs whose sessions changed most recently.
// The current directory, the prime, is left out.
// -last is at least 1.
// -since <YYYY-MM-DD> counts every finished run whose sessions changed on or after the start of that local day;
// whatever -last says, it counts every such run instead.
// The runs of each counted recipe over one period then stand side by side.
// A run's pair is torn down when it ends, which leaves an archive/<slug>/<sha> tag in the prime's fabric repository.
// -weft names that repository and defaults to the current worktree's weft sibling.
// A torn-down run is finished when the seed.json and status.json at its newest tag name the tapestry or the darn recipe,
// and its status.json holds a Finalize history entry.
// A run stopped partway, such as a bench pair parked after Discussion-Write, or a run of another recipe, is unfinished:
// counted as one, its later steps would show no cost.
// The report's header names the runs counted.
// When the default skipped unfinished runs, the header names them on one line.
// Slugs named on the command line are counted as given, finished or not.
// Each registered shuttle engine that can locate a worktree's sessions finds them;
// the engine that found a session reads it.
// A session's forks, the Webster master's sub-agents, are read with it.
// An engine that cannot read transcripts is named in the report's header line "Not counted, no transcript reader:".
// A session that is absent contributes nothing.
// An engine's failure to locate or read a session fails the count, naming the session and the engine.
//
// A slug's earlier runs share its worktree path, so they share its located sessions.
// A run counts only the sessions that started after the slug's previous run.
// A run in flight, whose worktree <hub>/<slug> exists, is bounded by the commit date of the slug's newest archive tag.
// For a finished run it is the commit date of the next-older tag.
// Without such a tag the bound is absent, and every transcript counts.
// A main-session transcript is judged by the earliest timestamp of its user and assistant entries.
// One with no such entry is left out when a bound exists, and counted under -no-cutoff.
// A left-out transcript adds nothing to the run's tokens, cost, working intervals or session count;
// nor to the Webster forks, Merriam starts, begin-batch base or transcript window, and its forks add nothing either.
// The run's section reports how many were left out and the bound's date.
// -no-cutoff drops the bound for every run.
// A slug's earlier runs that share its worktree path then feed every figure the bound gates:
// tokens, cost and working intervals; the session count, Webster forks, Merriam starts;
// the begin-batch base, transcript window, and the orch charge with it.
// It also drops the next-older tag's clamp on the span start.
// With -calibrate the base and Merriam rows may come from an earlier run.
// The plan is read from the newest plan commit before the run's first Webster fork, and may be an earlier run's.
// With it come the peak table, the start table's base and the identity card counts.
// Or the run is listed with "no plan commit before the first fork".
// A span start taken from the earliest transcript when the seed has no started_at may be an earlier run's.
//
// A session's role is the part after the last colon of its strand name, as its engine read it:
// ly:<slug>:burler, with a trailing -N dropped.
// So burler-2 counts as burler.
// A fork counts under its parent's role with "+sub" appended, and a session without a name counts as "untitled".
// A role ending in -chair, the role of a seat table's lead, counts under the role without that suffix:
// burler-review-chair counts as burler-review.
// A report spanning both names therefore sums each role once.
// A seat named chair counts under its table's lead role too.
//
// The report groups roles into segments by prefix.
// Each segment holds the role equal to its key and every role leading with the key and "-".
// The keys are discussion, plan, webster, merge, alignment, darn, drift and code,
// and the segments are Discussion, Plan, Webster, Merge, Alignment, Darn, Drift and Code-Review.
// The Landing segment holds the roles of the landing rows' sessions:
// describe, pr-reviewer, fixer and fix-reviewer, each a part labelled by its own name.
// The webster recovery strand's role is a Webster part, and every other role an Other part labelled by its own name.
// Within a segment each role is a part.
// The key role is the writer, labelled "writer"; Webster's is labelled "Master" and Alignment's "session".
// Suffixes -burler-review, -burler-fix, -bouncer-judge and -bouncer-seed are labelled reviewer, fixer, judge and seed;
// any other suffix is labelled by itself.
// A fork role is its parent's part with " forks" appended; Webster's Master forks are labelled "forks" alone.
// So "+sub" shows nowhere in the report.
// When the orchestrator is counted, an Orch segment holds the orch usage charged to the table's runs:
// its session part and its forks part.
// In "All runs" it also holds the orch usage charged to no run, as its unattributed part,
// so it carries the whole orch cost of the counted period.
// Its row names that cost averaged per run.
//
// The report opens with "Most expensive parts": the costliest parts of "All runs".
// Each is named by segment and label ("Webster forks", "Plan reviewer");
// its row gives its cost, its share of the total cost and its cost per run.
// "Cost per run" follows, with one row per run of its recipe:
// its segment costs, the Orch segment's without the unattributed part, and its total; then an average row.
// Its segment columns are the segments some counted run holds.
//
// "Recipes" follows, one row per recipe the counted runs ran.
// A run whose archived seed was not read, such as a run in flight, is listed under "unknown".
// A row gives the runs, their cost per run and each run's total with its orch charge;
// their awake span per run, the runs that landed a commit, and the changed lines per landed run.
// The cost per changed line is the cost of the runs that landed a commit over the lines those commits changed.
// A finished run's landed commit is the newest commit reachable from a branch or tag of the code repository.
// Its subject is the title of the change description archived at the run's tag, _lym/landing/summary.md,
// and its committer date lies within the run's span, widened by a minute each way.
// Its changed lines are the lines it added and deleted against its first parent, binary files left out.
// The match assumes the landing default, one squashed commit titled after the change description.
// The section names each run without a landed commit and why:
// among them a run whose archived change description does not parse.
//
// Each segment table, "All runs" and each run's, gives a total row per segment with its parts indented below.
// Segments and parts are sorted by cost, most expensive first, and an idle row follows.
// Its columns are cost, share, time, time share and models;
// -detail adds the sessions, output, cache write, cache read, input and weight columns after them;
// it adds the total weight to the "All runs" total line too.
//
// A message can be recorded in several transcripts, so usage is counted once per engine and message key within a run.
// An engine collapses a message it recorded over several records into one; a repeat it collapsed counts as a duplicate.
//
// One fork is one fork transcript of a session whose role is webster.
// The "Webster forks" section lists every such fork of every run, by run as listed and then by start time.
// Each row gives the cards the fork ran, its start context, its counted messages, its peak context and its weight.
// A fork's start context is the input + cache write + cache read of its first counted message:
// the context Merriam held when it spawned the fork.
// Its peak context is the largest such sum of any counted message.
// The fork also records the name of the webster session transcript its own transcript sits under, its Merriam session.
// A fork's cards are read from its prompt, never from commit times.
// The prompt is the first tool result in the fork's transcript holding a card pointer line:
// a "- `<path>/NN-<slug>.md`" bullet with nothing after the closing backtick.
// A file read's line-number prefix is ignored, and the cards are every such line of that one result.
// A fork whose transcript has no such result is listed as unattributed.
//
// With -calibrate <profile> the report gains a "Calibration (<profile>)" section of two tables:
// each row a fork that names cards.
// -weft names the repository holding the archive tags of finished runs, their plan commits and their webster records.
// Its default, the current worktree's weft sibling, applies whenever the flag is empty;
// slugs named or not, so -calibrate needs no other flag.
// A -hub naming a directory other than the current worktree's parent needs -weft;
// the default would read another hub's records.
// -config names the directory whose batcher.yaml holds the profile; it defaults to the current directory.
// -code names the code repository, read for each finished run's landed commit and for the calibration's base trees.
// It defaults to the current directory.
//
// A fork's position is one plus the card-naming forks before it in the same Merriam session, by start time,
// so a resumed session restarts its positions at 1.
// The start table gives each fork's measured start context beside the run's computed Merriam base.
// It also gives the profile's estimated start for its position: base + orientation + (position - 1) x batch_growth.
// A run's base is websterengine.MerriamBaseOf over the run's texts, priced at the profile's context_per_line.
// The texts are the instruction files of the engine that located the run's Webster session;
// the repository's copy of the Master template;
// and the orchestrator PATTERN directive, rendered from the directive stencil and PATTERN.md.
// They are read at the run's base commit, except the run's 00-overview.md, which is read at its plan commit.
// A run whose plan or base commit cannot be read keeps its rows, listed with the reason it has no base.
// Below it the section prints the least-squares fit of orientation and batch_growth beside the profile's values.
// The fit uses each fork's measured start minus the base, divided by position - 1, over the forks of runs with a base.
// Its residual spread is the 75th over the 25th percentile of each fork's measured start over its fitted start.
// With fewer than two distinct positions among those forks it prints "not fitted" with that reason.
// A negative fitted coefficient is printed and marked unusable, since batcher.yaml refuses a negative weight.
// The fit is a report; nothing consumes it and the tool never edits batcher.yaml.
//
// The peak table covers every fork whose cards all belong to the run's plan, one-card and multi-card alike.
// For each fork it gives batcher.PeakContext of its cards at its position, beside the measured peak context;
// the ratio is measured over estimate.
// A run's plan is the newest "loom: plan artifacts for <slug>" commit before the run's first Webster fork started.
// The estimate's tree is the run's base, read from the code repository.
// The base is the start_sha of the earliest successful begin-batch result in the run's webster session transcripts;
// it is the HEAD before the first batch forked.
// The card's own text is not in the base tree, so the estimate leaves it out.
// The estimate starts from the run's computed Merriam base, so the peak table and the start table agree;
// a run without one is priced without it.
// Each row also gives the fork's in-fork growth: its measured peak minus its measured start.
// The estimated growth is the estimated peak minus the estimated start of its position.
// The growth ratio is the measured growth over the estimated growth, "n/a" when the estimated growth is not positive.
// The section lists the runs, forks and cards it left out, each with its reason:
//   - a run: "no webster fork"; "no plan commit before the first fork";
//     "plan at <sha> does not parse: <error>"; "no base: no begin-batch result in its webster sessions";
//     "base <sha> is not in the repository";
//   - a fork: "names a card the plan lacks: <id>"; "estimate is 0";
//   - a card: "no fork names it".
//
// The section ends with the fit per run and overall.
// Each fit gives the number of forks, the median peak ratio and its spread;
// it also gives the number of forks with a growth ratio, their median growth ratio and its spread.
// A spread is the 75th percentile of the ratios over the 25th, both interpolated linearly between the closest ranks.
//
// The calibration section also ends with a "Merriam fixed context" subsection.
// It re-measures websterengine's merriamFixedContext.
// A Merriam session is a webster-role session.
// Its start context is the input + cache write + cache read of the first assistant message after the prompt's result.
// The prompt is the file named by the launch line lym types.
// For each run's first Merriam session by start time, the subsection gives the measured start.
// It also gives the line count of the texts MerriamBase counts, other than the plan overview.
// Those texts are the Webster session engine's instruction files, the Master template and the rendered directive.
// The engine is the one that located the run's first webster-role session;
// every webster-role session of a run is assumed to have run under that engine;
// an untracked instruction file is assumed absent.
// The fixed context is what is left once those lines are priced at context_per_line.
// Below the table it gives the median, 25th and 75th percentiles of the fixed figures beside the current constant.
// A session with no measured start, and a run whose texts cannot be reconstructed, are listed with their reasons.
// So is every later session of a run;
// since a resume, a refresh or a re-launch starts with prior conversation or a compaction summary in context.
//
// With -calibrate the report then gains three more sections, read from the same repository through gitrepo's read side:
// the cost of each batch, one row per run and one summary line per profile.
// A run's webster records are the commits whose subject is "loom: webster run record for <slug>".
// The last one is the newest;
// its state.json, decoded leniently so an older shape still reads, gives the partition and the batch records.
// A batch's profile is its recorded partition batch's profile;
// a run with no recorded partition counts as "identity", one batch per card;
// its cards are counted from the run's plan commit.
// A batch's weight and measured peak come from the run's forks named in its record's fork transcripts;
// a batch is failed when its record's terminal status is failed, dead or stuck, or its kind is recovery;
// a record of kind recovery is one recovery.
// A run whose records carry more than one run guid was restarted with --fresh;
// a run whose state cannot be read or decoded has no partition to name a profile;
// each keeps a row marked with its reason and enters no profile line.
// Code-Review findings are those of the round-<n>-review.md files in the code review directory at the last record.
// For an older run they are read from the webster one, counted through the review parser;
// no such file, or one that does not parse, marks the run's findings not read.
// Per-batch figures go to each batch's own profile, so a run whose batches name several profiles contributes to each.
// A run contributes findings to a profile line only when its batches name one profile;
// it was not restarted with --fresh, and its review files were read.
// The line's findings per card use the cards of those runs alone, stating their number, or "n/a" when there are none.
//
// The -detail weight column is input + 1.25 x cache writes + 0.1 x cache reads + 5 x output.
// It is a relative figure for ranking parts, not a price;
// it is blind to the per-model price difference shown in the models column.
// The share column is a row's cost as a percentage of its table's total cost, the Orch segment's included.
//
// The cost column prices each message through the pricing capability of the engine that read it.
// The engine owns the price table of its provider,
// and a message from an engine without one is unpriced.
// A message with usage that no price was found for is unpriced, never free:
// its cell reads "unpriced" or "$x + unpriced", and the "All runs" section names the unpriced models.
//
// The time and time share columns give a row's working time and its percentage of the table's span.
// Time is turn time: a session works from a user or assistant entry until the entry that ends its turn.
// Which entries end or start a turn is the engine's reading.
// An entry that ends a turn is a final assistant answer, an interrupt or a local command's output;
// an entry that starts one is the user's own prompt.
// The interval into a turn-start entry is idle, since a crash and --resume leave the gap before the resume's prompt.
// So is an interval longer than the 30-minute stall cap;
// that cap is three times the harness's ten-minute cap on a foreground Bash call.
// Either makes the whole interval idle, never a clipped part of it.
// A part's time is the union of its roles' transcripts' working intervals clipped to the span;
// a sub-agent's transcripts count under its forks part.
// A segment's time is the union over its parts.
// The idle row is the span minus the union of every working interval of the run, orch sessions left out,
// so it holds parent review, gate-slot waits, a parked driver and Go-only work.
// The Orch rows' time and models cells are blank;
// the "All runs" table sums each run's figures per segment and part, idle and span, with no union across runs.
//
// Every time figure leaves out machine sleep: a suspend or hibernate is no time at all, neither working nor idle.
// The sleeps are read once from the systemd journal (`journalctl -t systemd-sleep`) and parsed by internal/sleepjournal.
// Each sleep runs from a "Performing sleep operation" entry to the next "System returned from sleep operation" entry;
// the two entries must share an operation and a boot.
// A working interval loses the part a sleep covers;
// the span, the idle row and the Producers table's elapsed and idle count awake time only;
// each run's section names the sleep left out of its span, and "All runs" the sum.
// With journalctl absent or failing, a journal the user may not read, or no sleep in its journal, as on Windows;
// nothing is left out, and the report's header says why.
// -no-sleep reads no journal and counts sleep as time.
//
// The span is the window the time figures are measured over.
// It runs from the archived seed's started_at, or from the run's earliest transcript line without one.
// That start is bounded below by the commit date of the slug's next-older archive tag, unless -no-cutoff is set;
// it is bounded above by the end of the first history entry.
// It ends at the latest history entry's end.
// Both files are read from the slug's newest archive/<slug>/<sha> tag.
// A run whose worktree still exists, or with no tag or no status at its tag, uses its transcript window instead;
// it prints no Producers table.
//
// The Producers table splits the span by shed history entry:
// each entry owns the interval from the latest end before it to its own end.
// So the elapsed figures sum to the awake span, and a row's elapsed holds any wait before the row started.
// A row's idle is the part of its elapsed in which no session of the run was in a turn,
// and its active time is elapsed minus idle.
// The share is a row's active time over the table's summed active time;
// rows are ordered by active time, so a parked row cannot top the table.
// "All runs" sums the table per producer over the runs that have one.
//
// Right after the "All runs" section, before the per-run sections, comes the "Orchestrator" section: the hub orchestrator's own session, which runs in the prime and spans many runs, charged to the counted runs.
// An orch session is a session in the prime's project directory whose role is orch (ly:orch, lymhub:orch), and its sub-agents, the orch's review, gate and stop forks, count as its forks;
// usage is counted once per message id across them.
// A run's active window runs from its earliest to its latest user or assistant line over all its transcripts, and only orch messages between the earliest window start and the latest window end are counted, so the orch's older history stays out.
// An orch sub-agent whose first user prompt names exactly one counted run slug goes wholly to that run;
// every other orch message is split equally among the runs whose window holds its timestamp, and one outside every window goes to the unattributed row.
// Each charge keeps its session and forks sides apart, which the Orch segment's parts show.
// The section gives the orch total split into session and forks, then per run the orch tokens and cost charged to it, the run's own cost and orch's percentage of the run's cost with orch included.
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
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// repositoryFlag is the name of the flag that names the repository holding the archive tags;
// the runs' plan commits and webster records are there too.
const repositoryFlag = "weft"

// resolveHub makes the hub flag's value absolute and clean.
// An empty value is the current worktree's parent, and a relative path joins onto wd.
// It refuses a hub other than that parent when weft is empty;
// the default weft would then read another hub's records.
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

// parseSince parses the -since flag's value, a YYYY-MM-DD date, into the start of that day in local time;
// empty is the zero time.
func parseSince(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	day, err := time.ParseInLocation(time.DateOnly, value, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("-since %q is not a YYYY-MM-DD date: %w", value, err)
	}
	return day, nil
}

func main() {
	if err := run(os.Args[1:], os.Stdout, shuttleengines.RecordReaders()); err != nil {
		fmt.Fprintln(os.Stderr, "tokencount:", err)
		os.Exit(1)
	}
}

// run counts the runs args name, or the most recent finished runs, through the engines in reads.
// It writes the report to stdout or to the -out file.
func run(args []string, stdout io.Writer, reads ReadSet) error {
	fs := flag.NewFlagSet("tokencount", flag.ContinueOnError)
	hub := fs.String("hub", "", "hub directory holding the task worktrees (default: parent of the current directory)")
	out := fs.String("out", "", "write the report to this file instead of stdout")
	last := fs.Int("last", 6, "with no slugs named, count this many of the most recently active finished runs")
	since := fs.String("since", "", "with no slugs named, count every finished run whose sessions changed on or after this local date, YYYY-MM-DD, instead of the -last most recent")
	code := fs.String("code", "", "the code repository the runs landed on (default: the current directory)")
	weft := fs.String(repositoryFlag, "", "the prime's fabric repository holding the archive tags of finished runs, the runs' plan commits and webster records (default: the current worktree's weft sibling)")
	calibrate := fs.String("calibrate", "", "add the calibration section for this batcher.yaml profile")
	configDir := fs.String("config", "", "directory whose batcher.yaml holds the profile (default: the current directory)")
	noCutoff := fs.Bool("no-cutoff", false, "turn off the archive-tag cutoff, so every located session of each slug counts")
	noSleep := fs.Bool("no-sleep", false, "read no journal and count machine sleep as time")
	detail := fs.Bool("detail", false, "add the sessions and token columns (output, cache write, cache read, input, weight) to every segment table")
	if err := fs.Parse(args); err != nil {
		return err
	}
	sinceDay, err := parseSince(*since)
	if err != nil {
		return err
	}
	if len(fs.Args()) == 0 && sinceDay.IsZero() && *last < 1 {
		return fmt.Errorf("-last %d counts no run; name at least 1, or give -since", *last)
	}
	wd, err := lymcwd.Getwd()
	if err != nil {
		return err
	}
	if *code == "" {
		*code = wd
	}
	landings := codeRepo{Repo: gitrepo.New(*code), dir: *code}
	*hub, err = resolveHub(*hub, wd, *weft)
	if err != nil {
		return err
	}
	readable, unreadable := reads.engines()
	slugs := fs.Args()
	if *weft == "" {
		loc, err := lymcwd.Resolve(wd)
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
		limit := *last
		if !sinceDay.IsZero() {
			limit = 0
		}
		slugs, skipped, err = recentRuns(reads, *hub, filepath.Base(wd), finished, fabric, limit, sinceDay)
		if err != nil {
			return err
		}
		if len(slugs) == 0 {
			return fmt.Errorf("no finished %s runs of %s from engines %s; name the slugs to count a partial run", recipeList("or"), *hub, strings.Join(readable, ", "))
		}
		counted = fmt.Sprintf("the %d most recent finished %s runs: %s", len(slugs), recipeList("and"), strings.Join(slugs, ", "))
		if !sinceDay.IsZero() {
			counted = fmt.Sprintf("the %d finished %s runs active since %s: %s", len(slugs), recipeList("and"), *since, strings.Join(slugs, ", "))
		}
		if len(skipped) > 0 {
			skippedNote = fmt.Sprintf("Skipped %d runs that are not finished %s runs: %s.\n", len(skipped), recipeList("or"), strings.Join(skipped, ", "))
		}
	}

	var sleeps []Interval
	sleepNote := "Machine sleep counts as time: -no-sleep is set.\n"
	if !*noSleep {
		sleeps, sleepNote = journalSleeps()
	}
	report := Report{Detail: *detail}
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
				landing, err := findLanding(fabric, landings, slug, *record, r.Span)
				if err != nil {
					return err
				}
				r.Landing = &landing
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
		c, err := Calibrate(report.Runs, *calibrate, *configDir, fabric, landings.Repo, reads)
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
