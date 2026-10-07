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
// the report's "Webster forks" section lists every such fork of every run, by run as listed and then by start time, with the cards it ran, its counted messages, its peak context (the largest input + cache write + cache read of any counted message) and its weight.
// A fork's cards are read from its prompt, never from commit times: the prompt is the first tool result in the fork's transcript holding a card pointer line, a "- `<path>/NN-<slug>.md`" bullet with nothing after the closing backtick (a Read result's line-number prefix is ignored), and the cards are every such line of that one result.
// A fork whose transcript has no such result is listed as unattributed.
//
// With -calibrate <profile> the report gains a "Calibration (<profile>)" section: the batcher's peak-context estimate for each card beside the measured peak context of the fork that ran it, for every run counted.
// The one repository flag, -weft, names the repository holding the archive tags of finished runs, the runs' plan commits and the runs' webster records;
// its default, the current worktree's weft sibling, applies whenever the flag is empty and either no slugs are named or the calibration is asked for, so -calibrate needs no other flag.
// -config names the directory whose batcher.yaml holds the profile, default the current directory.
// The code repository is the current directory.
// A run's plan is read from the newest "loom: plan artifacts for <slug>" commit in that repository committed before the run's first Webster fork started.
// Its base tree is the start_sha of the earliest successful begin-batch result in the run's webster session transcripts, the HEAD before the first batch forked, read from the code repository.
// A card's estimate is batcher.PeakContext of the card alone over the base tree with the profile's weights;
// its measured peak is the largest PeakContext of any fork whose cards name it, and its ratio is measured over estimate.
// The card's own text is not in the base tree, so the estimate leaves it out.
// The section lists the runs and cards it left out, each with its reason:
//   - a run: "no webster fork"; "no plan commit before the first fork"; "plan at <sha> does not parse: <error>"; "no base: no begin-batch result in its webster sessions"; "base <sha> is not in the repository";
//   - a card: "no fork names it"; "ran in a multi-card fork", since only a one-card fork measures a one-card cost; "estimate is 0".
//
// The section ends with the fit per run and overall: the number of cards, the median ratio and the spread, the 75th percentile of the ratios over the 25th, both interpolated linearly between the closest ranks.
//
// The weight column is input + 1.25 x cache writes + 0.1 x cache reads + 5 x output: a
// relative figure for ranking roles, not a price, and blind to the per-model price
// difference shown in the models column.
// The share column is a role's weight as a percentage of its table's total weight.
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
		*weft = fabricengine.WeftWorktree(loc)
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

	var calibration *Calibration
	if *calibrate != "" {
		if *configDir == "" {
			*configDir = wd
		}
		c, err := Calibrate(report.Runs, *calibrate, *configDir, gitrepo.New(*weft), gitrepo.New(wd))
		if err != nil {
			return err
		}
		calibration = &c
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
	if *out != "" {
		fmt.Fprintln(stdout, "wrote", *out)
	}
	return nil
}
