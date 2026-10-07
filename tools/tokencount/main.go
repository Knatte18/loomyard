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
// With no slugs it counts the -last runs whose sessions changed most recently, leaving out
// the prime (the current directory) and the weft.
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
)

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
	last := fs.Int("last", 6, "with no slugs named, count this many of the most recently active runs")
	if err := fs.Parse(args); err != nil {
		return err
	}
	wd, err := os.Getwd()
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
	if len(slugs) == 0 {
		slugs, err = recentRuns(*projects, *hub, filepath.Base(wd), *last)
		if err != nil {
			return err
		}
		if len(slugs) == 0 {
			return fmt.Errorf("no task runs of %s under %s", *hub, *projects)
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
	if *out != "" {
		fmt.Fprintln(stdout, "wrote", *out)
	}
	return nil
}
