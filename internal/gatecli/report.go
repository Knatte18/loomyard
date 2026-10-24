// report.go implements `lyx gate report`, the read-only totals of this machine's tier run records per slug and gate key.

package gatecli

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
)

// sinceLayout is the date form `lyx gate report --since` takes.
const sinceLayout = "2006-01-02"

// reportCmd builds `lyx gate report`.
func reportCmd() *cobra.Command {
	var slug, since string
	cmd := &cobra.Command{
		Use:         "report",
		Short:       "total this machine's tier runs per slug and gate key",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `report reads this machine's tier run records, from the board's gate run directory inside a hub and the worktree's own outside every hub, and prints one row per slug and gate key.
A row holds the tier runs, the carried tiers, the summed slot wait, the summed wall time, and the launch time with its share of the package time.
The build step's records count apart, in build_runs and build_seconds, and add nothing to tier_runs or wall_seconds.
A record from before --gate has no key and groups under its tier list and scope form, such as "tiers 0,1 all".
--slug keeps one slug's records, and --since YYYY-MM-DD drops the records started before that day.
It takes no slot and changes nothing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			var sinceTime time.Time
			if since != "" {
				parsed, err := time.Parse(sinceLayout, since)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(cmd.OutOrStdout(), fmt.Sprintf("gate report: --since %q is not a date; way forward: pass it as YYYY-MM-DD, such as 2026-10-24, then re-run the same command", since)))
					return nil
				}
				sinceTime = parsed
			}
			clihelp.SetExit(cmd.Context(), runReportVerb(cmd.Context(), cmd.OutOrStdout(), slug, sinceTime))
			return nil
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "keep only the records of this task slug")
	cmd.Flags().StringVar(&since, "since", "", "drop the records started before this date, as YYYY-MM-DD")
	return cmd
}

// runReportVerb reads the run records of the worktree that contains the seam cwd and reports them.
func runReportVerb(ctx context.Context, out io.Writer, slug string, since time.Time) int {
	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate report: cannot read the working directory: %v; way forward: re-run the same command", err))
	}
	location, inHub, err := resolveHub(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate report: %v; way forward: re-run the same command", err))
	}
	if !inHub {
		if location, err = lyxcwd.ResolveWorktree(cwd); err != nil {
			return output.Err(out, fmt.Sprintf("gate report: resolve the worktree of %s: %v; way forward: run the same command from a git worktree", cwd, err))
		}
	}
	records, err := gateslot.ReadRunRecords(runLogDirOf(location, inHub))
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate report: %v; way forward: re-run the same command", err))
	}
	return runReport(out, records, slug, since)
}

// runLogDirOf returns the directory the tier run records of location go in: the board's gate directory inside a hub, the worktree's own outside every hub.
func runLogDirOf(location *lyxcwd.Location, inHub bool) string {
	if inHub {
		return gateslot.RunLogDir(gateslot.Dir(fabricengine.BoardDir(location.HubPath)))
	}
	return gateslot.RunLogDir(gateslot.WaitDir(location.WorktreePath()))
}

// reportGroup is the running totals of the records of one slug and gate key.
type reportGroup struct {
	slug, label string
	runs        int
	carried     int
	waitSeconds float64
	wallSeconds float64
	// buildRuns and buildSeconds total the build step's records apart from the tier runs.
	buildRuns    int
	buildSeconds float64
	// launchSeconds and packageSeconds sum the launch gaps and the elapsed times of the records' launch lists; hasLaunches is set once a record holds a list.
	launchSeconds  float64
	packageSeconds float64
	hasLaunches    bool
}

// recordLabel returns the key a record groups under: its gate key, else its tier list and scope form.
func recordLabel(record gateslot.RunRecord) string {
	if record.GateKey != "" {
		return record.GateKey
	}
	tiers := make([]string, len(record.TierList))
	for i, tier := range record.TierList {
		tiers[i] = strconv.Itoa(tier)
	}
	return fmt.Sprintf("tiers %s %s", strings.Join(tiers, ","), record.ScopeForm)
}

// runReport prints an ok envelope holding one row per slug and gate key of records, in slug then key order.
// A non-empty slug keeps that slug's records only, and a record started before since is dropped when since is set.
// A row carries the tier runs and the carried tiers, the summed slot wait and wall time in seconds, the build step's runs and seconds counted apart, and, when a record holds a launch list, the launch time and its share of the packages' summed time.
func runReport(out io.Writer, records []gateslot.RunRecord, slug string, since time.Time) int {
	var groups []*reportGroup
	for _, record := range records {
		if (slug != "" && record.Slug != slug) || record.Start.Before(since) {
			continue
		}
		label := recordLabel(record)
		index := slices.IndexFunc(groups, func(group *reportGroup) bool { return group.slug == record.Slug && group.label == label })
		if index < 0 {
			groups = append(groups, &reportGroup{slug: record.Slug, label: label})
			index = len(groups) - 1
		}
		group := groups[index]
		if record.Build {
			group.buildRuns++
			group.buildSeconds += record.WallSec
			continue
		}
		if record.Carried {
			group.carried++
		} else {
			group.runs++
		}
		if !record.WaitStart.IsZero() {
			group.waitSeconds += record.Start.Sub(record.WaitStart).Seconds()
		}
		group.wallSeconds += record.WallSec
		for _, launch := range record.Launches {
			group.hasLaunches = true
			group.launchSeconds += launch.LaunchSeconds
			group.packageSeconds += launch.ElapsedSeconds
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].slug != groups[j].slug {
			return groups[i].slug < groups[j].slug
		}
		return groups[i].label < groups[j].label
	})

	rows := make([]map[string]any, len(groups))
	for i, group := range groups {
		row := map[string]any{
			"slug": group.slug, "gate_key": group.label,
			"tier_runs": group.runs, "carried": group.carried,
			"wait_seconds": group.waitSeconds, "wall_seconds": group.wallSeconds,
			"build_runs": group.buildRuns, "build_seconds": group.buildSeconds,
		}
		if group.hasLaunches {
			row["launch_seconds"] = group.launchSeconds
			if group.packageSeconds > 0 {
				row["launch_share"] = group.launchSeconds / group.packageSeconds
			}
		}
		rows[i] = row
	}
	return output.Ok(out, map[string]any{"rows": rows})
}
