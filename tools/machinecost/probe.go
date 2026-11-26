// probe.go runs one probe: every measurement in order, between two load snapshots, inside the probe's own temp folder.

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/lymtemp"
)

// trueBinary is the cheapest program to start, so its start time is the process start cost alone.
const trueBinary = "/bin/true"

// probeFolderPrefix opens the name of every folder the probe makes.
const probeFolderPrefix = "machinecost-"

// lymStartName keys the measurements of the lym starts.
const lymStartName = "lym_help"

// runProbe takes every measurement config names and returns the report, writing a line to progress as each part begins.
// It stops with an error once ctx is cancelled.
// It deletes its temp folder and every folder it made in config's dirs before it returns, on failure too.
func runProbe(ctx context.Context, config ProbeConfig, startedAt time.Time, progress io.Writer) (Report, error) {
	tempDir, err := lymtemp.MkdirTemp(probeFolderPrefix)
	if err != nil {
		return Report{}, fmt.Errorf("create temp folder: %w", err)
	}
	defer os.RemoveAll(tempDir)

	machine, err := readMachine(tempDir)
	if err != nil {
		return Report{}, err
	}
	report := Report{
		GeneratedAt: startedAt.UTC().Format(time.RFC3339),
		Machine:     machine,
		Before:      readSnapshot(),
		TempDir:     tempDir,
		Config:      config,
	}
	measurements := &collector{}

	fmt.Fprintln(progress, "machinecost: process starts")
	if err := measureStarts(ctx, "true", []string{trueBinary}, config.Starts, measurements); err != nil {
		return Report{}, err
	}
	if err := measureStarts(ctx, "git_version", []string{gitBinary, "--version"}, config.Starts, measurements); err != nil {
		return Report{}, err
	}
	if config.LymBinary == "" {
		report.Skipped = append(report.Skipped, groupStart+"/"+lymStartName+": no lym on PATH and no -lym flag")
	} else if err := measureStarts(ctx, lymStartName, []string{config.LymBinary, "--help"}, config.Starts, measurements); err != nil {
		return Report{}, err
	}

	if len(config.Dirs) == 0 {
		report.Skipped = append(report.Skipped, groupNewBinary+", "+groupFileOps+": no -dir folder given")
	}
	for _, dir := range config.Dirs {
		fmt.Fprintf(progress, "machinecost: new binaries in %s\n", dir.Path)
		if err := measureNewBinaries(ctx, tempDir, dir, config.Binaries, measurements); err != nil {
			return Report{}, err
		}
		fmt.Fprintf(progress, "machinecost: file operations in %s\n", dir.Path)
		if err := measureFileOps(ctx, dir, config.FileOps, measurements); err != nil {
			return Report{}, err
		}
	}

	fmt.Fprintln(progress, "machinecost: git workload")
	if err := measureGitWorkload(ctx, tempDir, config.GitRounds, measurements); err != nil {
		return Report{}, err
	}

	report.After = readSnapshot()
	report.Measurements = measurements.measurements
	return report, nil
}
