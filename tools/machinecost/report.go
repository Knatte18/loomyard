// report.go holds the probe's report: its JSON shape, reading and writing it,
// and the text renderings of one report and of two compared.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"text/tabwriter"
	"time"
)

// Units a measurement is reported in.
const (
	unitMicroseconds = "us"
	unitMilliseconds = "ms"
	unitCount        = "count"
)

// The groups a measurement name opens with.
const (
	groupStart     = "start"
	groupNewBinary = "newbinary"
	groupFileOps   = "fileops"
	groupGit       = "git"
)

// NamedDir is one folder the probe measures in, under the name that keys its measurements.
type NamedDir struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ProbeConfig is what a probe run measures and how many times.
// Two reports compare only when their counts match, so a report carries it.
type ProbeConfig struct {
	Dirs      []NamedDir `json:"dirs"`
	LymBinary string     `json:"lym_binary"`
	Starts    int        `json:"starts"`
	FileOps   int        `json:"file_ops"`
	Binaries  int        `json:"binaries"`
	GitRounds int        `json:"git_rounds"`
}

// Measurement is one figure of a report, keyed by a slash-separated name such as `start/true/median_us`.
type Measurement struct {
	Name  string  `json:"name"`
	Unit  string  `json:"unit"`
	Value float64 `json:"value"`
}

// Report is one probe run's result, written as JSON so another machine's report can be compared with it.
type Report struct {
	GeneratedAt string      `json:"generated_at"`
	Machine     Machine     `json:"machine"`
	Before      Snapshot    `json:"before"`
	After       Snapshot    `json:"after"`
	TempDir     string      `json:"temp_dir"`
	Config      ProbeConfig `json:"config"`
	// Skipped names each measurement the run could not take, with the reason.
	Skipped      []string      `json:"skipped,omitempty"`
	Measurements []Measurement `json:"measurements"`
}

// collector accumulates a run's measurements in the order they are taken.
type collector struct {
	measurements []Measurement
}

func (c *collector) add(name, unit string, value float64) {
	c.measurements = append(c.measurements, Measurement{Name: name, Unit: unit, Value: value})
}

// median returns the middle duration of samples, the mean of the two middle ones for an even count.
// samples is never empty: every count the probe takes is at least one.
func median(samples []time.Duration) time.Duration {
	sorted := slices.Clone(samples)
	slices.Sort(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

func microseconds(d time.Duration) float64 {
	return float64(d) / float64(time.Microsecond)
}

func milliseconds(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

// writeReport writes report as indented JSON to path.
func writeReport(path string, report Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// readReport reads a report a probe run wrote.
func readReport(path string) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, fmt.Errorf("read report: %w", err)
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return Report{}, fmt.Errorf("decode report %s: %w", path, err)
	}
	return report, nil
}

// ComparisonRow is one measurement name in two reports.
// A side the name is missing from holds nil; Ratio is other over base, nil when either side is missing or base is zero.
type ComparisonRow struct {
	Name  string
	Unit  string
	Base  *float64
	Other *float64
	Ratio *float64
}

// compareMeasurements pairs the measurements of two reports by name:
// base's names in base's order, then the names only other holds in other's order.
func compareMeasurements(base, other []Measurement) []ComparisonRow {
	otherByName := make(map[string]Measurement, len(other))
	for _, m := range other {
		otherByName[m.Name] = m
	}
	baseNames := make(map[string]bool, len(base))
	rows := make([]ComparisonRow, 0, len(base)+len(other))
	for _, m := range base {
		baseNames[m.Name] = true
		row := ComparisonRow{Name: m.Name, Unit: m.Unit, Base: &m.Value}
		if match, found := otherByName[m.Name]; found {
			row.Other = &match.Value
			if m.Value != 0 {
				ratio := match.Value / m.Value
				row.Ratio = &ratio
			}
		}
		rows = append(rows, row)
	}
	for _, m := range other {
		if !baseNames[m.Name] {
			rows = append(rows, ComparisonRow{Name: m.Name, Unit: m.Unit, Other: &m.Value})
		}
	}
	return rows
}

// renderReport prints one report as text: the machine, the load around the run, and every measurement.
func renderReport(w io.Writer, report Report) {
	fmt.Fprintf(w, "Generated: %s\n", report.GeneratedAt)
	renderMachine(w, report.Machine)
	fmt.Fprintf(w, "Load before: %s\n", formatSnapshot(report.Before))
	fmt.Fprintf(w, "Load after:  %s\n", formatSnapshot(report.After))
	fmt.Fprintf(w, "Temp folder: %s\n", report.TempDir)
	for _, skipped := range report.Skipped {
		fmt.Fprintf(w, "Skipped: %s\n", skipped)
	}
	fmt.Fprintln(w)
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "Measurement\tValue\tUnit")
	for _, m := range report.Measurements {
		fmt.Fprintf(table, "%s\t%.1f\t%s\n", m.Name, m.Value, m.Unit)
	}
	table.Flush()
}

// renderComparison prints both reports' machines and loads, then each measurement with the ratio of other to base.
func renderComparison(w io.Writer, base, other Report) {
	for _, side := range []struct {
		label  string
		report Report
	}{{"Base", base}, {"Other", other}} {
		fmt.Fprintf(w, "%s (%s):\n", side.label, side.report.GeneratedAt)
		renderMachine(w, side.report.Machine)
		fmt.Fprintf(w, "Load before: %s\n", formatSnapshot(side.report.Before))
		fmt.Fprintf(w, "Load after:  %s\n", formatSnapshot(side.report.After))
		fmt.Fprintln(w)
	}
	if base.Config.Starts != other.Config.Starts || base.Config.FileOps != other.Config.FileOps ||
		base.Config.Binaries != other.Config.Binaries || base.Config.GitRounds != other.Config.GitRounds {
		fmt.Fprintln(w, "Warning: the two runs used different counts.")
		fmt.Fprintln(w)
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "Measurement\tUnit\tBase\tOther\tOther/Base")
	for _, row := range compareMeasurements(base.Measurements, other.Measurements) {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n",
			row.Name, row.Unit, formatOptional(row.Base, "%.1f"), formatOptional(row.Other, "%.1f"), formatOptional(row.Ratio, "%.2f"))
	}
	table.Flush()
}

func renderMachine(w io.Writer, machine Machine) {
	fmt.Fprintf(w, "Machine: %s, %s, %d cores, kernel %s, governor %s\n",
		machine.Hostname, machine.CPUModel, machine.Cores, machine.Kernel, machine.Governor)
	fmt.Fprintf(w, "Tools: %s, %s, %s present: %t\n", machine.GoVersion, machine.GitVersion, trapsPath, machine.TrapsPresent)
}

func formatSnapshot(s Snapshot) string {
	return fmt.Sprintf("load %s %s %s, cpu pressure avg10 %s avg60 %s, mean %s MHz, throttle count %s",
		formatOptional(s.Load1, "%.2f"), formatOptional(s.Load5, "%.2f"), formatOptional(s.Load15, "%.2f"),
		formatOptional(s.CPUPressureSomeAvg10, "%.2f"), formatOptional(s.CPUPressureSomeAvg60, "%.2f"),
		formatOptional(s.MeanMHz, "%.0f"), formatOptional(s.ThrottleCount, "%.0f"))
}

// formatOptional formats a figure the machine may not expose, printing n/a when it is absent.
func formatOptional(value *float64, format string) string {
	if value == nil {
		return "n/a"
	}
	return fmt.Sprintf(format, *value)
}
