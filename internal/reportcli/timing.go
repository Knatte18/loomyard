// timing.go is the `report timing` verb: it validates its flags, resolves the prime and the hub's live pairs, reads the machine's sleep from the systemd journal and hands them to the report engine.

package reportcli

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reportengine"
	"github.com/Knatte18/loomyard/internal/sleepjournal"
	"github.com/spf13/cobra"
)

// defaultTimingDays is how many days before today's UTC date the timing report opens when --since is not given.
const defaultTimingDays = 7

// timingCommand returns the `report timing` verb.
func timingCommand() *cobra.Command {
	var since, by, format string
	cmd := &cobra.Command{
		Use:         "timing",
		Short:       "report where run time went, per run, per phase and per day or week, with machine sleep and waits apart",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `timing reads the hub's runs, the finished ones and the ones in flight, and
reports where their time went: per run, per phase, and per day or week, with
machine sleep and the time spent waiting on someone else counted apart, and who
settled each stop. A phase a run holds no row of is reported absent, never as zero.

  --since <date>      UTC date (YYYY-MM-DD) the window opens at
                      (default: seven days ago)
  --by <width>        day (default) or week, the bucket width
  --format <form>     json (default) or markdown

It runs from the hub's prime worktree only, reads git history and the systemd
journal, and writes nothing.

Example:
  lyx report timing --by week --format markdown`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			fail := func(err error) error {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			options, err := parseTimingOptions(since, by, format, time.Now().UTC())
			if err != nil {
				return fail(err)
			}
			location, err := resolvePrime(ctx, "timing")
			if err != nil {
				return fail(err)
			}
			geometry, err := primeGeometry(location)
			if err != nil {
				return fail(err)
			}
			if options.Live, err = livePairs(location); err != nil {
				return fail(err)
			}
			sources, err := reportengine.Open(geometry)
			if err != nil {
				return fail(err)
			}
			options.Sleeps, options.SleepNote = journalSleeps(ctx)
			report, err := reportengine.ComputeTiming(sources, options)
			if err != nil {
				return fail(err)
			}

			if format == formatMarkdown {
				fmt.Fprint(out, reportengine.RenderTimingMarkdown(report))
				return nil
			}
			clihelp.SetExit(ctx, output.Ok(out, map[string]any{
				"selection":  report.Selection,
				"runs":       report.Runs,
				"phases":     report.Phases,
				"buckets":    report.Buckets,
				"sleep_note": report.SleepNote,
				"findings":   report.Findings,
			}))
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "UTC date (YYYY-MM-DD) the window opens at (default: seven days ago)")
	cmd.Flags().StringVar(&by, "by", reportengine.BucketDay, "bucket width: day or week")
	cmd.Flags().StringVar(&format, "format", formatJSON, "output form: json or markdown")
	return cmd
}

// parseTimingOptions validates the flags before anything is read.
// Each refusal names the flag, the value given and the form accepted.
// A since date after today is accepted and gives an empty report.
func parseTimingOptions(since, by, format string, now time.Time) (reportengine.TimingOptions, error) {
	options := reportengine.TimingOptions{By: by}
	if since == "" {
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		options.Since = today.AddDate(0, 0, -defaultTimingDays)
	} else {
		parsed, err := time.ParseInLocation(sinceLayout, since, time.UTC)
		if err != nil {
			return reportengine.TimingOptions{}, fmt.Errorf("report: --since %q is not a date: expected YYYY-MM-DD in UTC", since)
		}
		options.Since = parsed
	}
	if by != reportengine.BucketDay && by != reportengine.BucketWeek {
		return reportengine.TimingOptions{}, fmt.Errorf("report: --by %q is not a bucket: expected %s or %s", by, reportengine.BucketDay, reportengine.BucketWeek)
	}
	if format != formatJSON && format != formatMarkdown {
		return reportengine.TimingOptions{}, fmt.Errorf("report: --format %q is not a report form: expected %s or %s", format, formatJSON, formatMarkdown)
	}
	return options, nil
}

// livePairs returns the slugs whose pair is live: the base names of the hub's code worktrees other than the main one.
// The engine reads a pair records branch whose slug is not among them as abandoned.
func livePairs(location *lyxcwd.Location) ([]string, error) {
	worktrees, err := fabricengine.CodeWorktrees(location.WorktreePath())
	if err != nil {
		return nil, fmt.Errorf("report: list the hub's code worktrees: %w", err)
	}
	var slugs []string
	for _, worktree := range worktrees {
		if !worktree.Main {
			slugs = append(slugs, filepath.Base(worktree.Path))
		}
	}
	return slugs, nil
}

// journalSleeps runs journalctl once for the machine's sleeps and returns them with the note sleepsFromJournal words.
// A journalctl that is not on PATH is handed on as the run error without a spawn.
func journalSleeps(ctx context.Context) ([]reportengine.Interval, string) {
	path, err := exec.LookPath("journalctl")
	if err != nil {
		return sleepsFromJournal(nil, err)
	}
	logger.Info("report: spawning journalctl for the machine's sleep", "path", path)
	out, err := exec.CommandContext(ctx, path, sleepjournal.Args()...).Output()
	logger.Info("report: journalctl finished", "bytes", len(out), "error", err)
	return sleepsFromJournal(out, err)
}

// sleepsFromJournal maps the output and exit error of one journalctl run to the sleeps and the note the report's sleep_note carries.
// With sleeps the note says they are left out of every interval, and without them it says why no machine sleep is subtracted.
func sleepsFromJournal(out []byte, runErr error) ([]reportengine.Interval, string) {
	read, clause := sleepjournal.Read(out, runErr)
	if clause != "" {
		return nil, "No machine sleep is subtracted: " + clause + "."
	}
	sleeps := make([]reportengine.Interval, len(read))
	for i, sleep := range read {
		sleeps[i] = reportengine.Interval{From: sleep.From.UTC(), To: sleep.To.UTC()}
	}
	return sleeps, "Machine sleep, read from the systemd journal, is left out of every interval."
}
