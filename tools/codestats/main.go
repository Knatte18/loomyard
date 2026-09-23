// Command codestats scans a source tree and writes a markdown report of how its lines
// divide: code, comment and blank per language, tests against production, and markdown's
// share of the whole.
//
//	go run ./tools/codestats
//	go run ./tools/codestats -root ../some-other-repo
//	go run ./tools/codestats -out .scratch/codestats.md
//	go run ./tools/codestats -json -out .scratch/codestats.json
//	go run ./tools/codestats -exclude crucible -exclude '*.pb.go'
//
// The two launchers beside this file, codestats.sh and codestats.cmd, are the intended
// entry points: each writes .scratch/codestats.md, one fixed path overwritten per run,
// with the run's instant recorded inside the report rather than in its name.
//
// -out writes UTF-8 whatever the shell is, which a redirect does not: PowerShell's own >
// still produces UTF-16 on the Windows machines this repo is worked from.
//
// A line carrying both code and a trailing comment counts as code, so the three buckets
// partition each file rather than overlapping, and the percentages add up.
//
// Comment counting knows two families. The C family (Go, C#, and the other // and /* */
// languages) honours string literals and Go's backtick raw strings, so a // inside a
// string is code. The # family (Python, shell, YAML, TOML) is line-local and
// string-unaware, so a # inside a quoted string overcounts. C#'s @"" verbatim strings
// are not tracked across line breaks, which can misread a // inside one as a comment --
// accepted because the alternative is a per-language lexer, and the error is bounded by
// how rarely those strings span lines at all. Every other extension is counted for lines
// only, markdown among them, which is why the markdown section reports files and lines
// rather than a comment share.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// excludeList collects a repeatable -exclude flag, the one flag that must accumulate
// rather than overwrite.
type excludeList []string

func (e *excludeList) String() string { return strings.Join(*e, ",") }

func (e *excludeList) Set(v string) error {
	*e = append(*e, v)
	return nil
}

// Bucket is an aggregate over some set of files -- one language, one side of the
// test/production split, or the whole tree.
type Bucket struct {
	Files   int `json:"files"`
	Lines   int `json:"lines"`
	Code    int `json:"code"`
	Comment int `json:"comment"`
	Blank   int `json:"blank"`
}

func (b *Bucket) add(f FileStats) {
	b.Files++
	b.Lines += f.Lines
	b.Code += f.Code
	b.Comment += f.Comment
	b.Blank += f.Blank
}

// Report is one scan's full result, shaped so -json emits it directly.
type Report struct {
	Root string `json:"root"`
	// GeneratedAt is when the scan ran, carried in the report rather than only in a file
	// name so a report stays self-describing once it is copied, pasted, or committed.
	GeneratedAt string `json:"generated_at"`
	// DurationMS is how long the walk and the tally took, in milliseconds. It measures
	// this process only: `go run`'s own compile of the tool happens before main starts and
	// is outside it, so the launcher's wall-clock time is the larger of the two on a cold
	// build cache.
	// One numeric field rather than a number beside a pre-rendered string: render formats
	// it for a reader, and a consumer comparing two runs gets a value it can subtract.
	DurationMS int64             `json:"duration_ms"`
	ByExt      map[string]Bucket `json:"by_ext"`
	Source     Bucket            `json:"source"`
	Production Bucket            `json:"production"`
	Test       Bucket            `json:"test"`
	Markdown   Bucket            `json:"markdown"`
	Total      Bucket            `json:"total"`
}

// build aggregates files into a Report.
// Source counts only the extensions with a comment family, because the test/production
// split and the comment share are both meaningless for the rest; Total counts everything
// scanned, which is what markdown's share is taken against.
func build(root string, at time.Time, files []FileStats) Report {
	report := Report{
		Root:        root,
		GeneratedAt: at.Format(time.RFC3339),
		ByExt:       map[string]Bucket{},
	}
	for _, f := range files {
		bucket := report.ByExt[f.Ext]
		bucket.add(f)
		report.ByExt[f.Ext] = bucket

		report.Total.add(f)
		if f.Ext == ".md" {
			report.Markdown.add(f)
		}
		if familyFor(f.Ext) == familyNone {
			continue
		}
		report.Source.add(f)
		if f.IsTest {
			report.Test.add(f)
		} else {
			report.Production.add(f)
		}
	}
	return report
}

// pct renders part of whole as a percentage, or "-" when whole is zero, so a language
// with no lines prints a dash rather than a misleading 0.0%.
func pct(part, whole int) string {
	if whole == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", float64(part)/float64(whole)*100)
}

func main() {
	var excludes excludeList
	root := flag.String("root", ".", "directory to scan")
	hidden := flag.Bool("hidden", false, "descend into dotted files and directories")
	asJSON := flag.Bool("json", false, "emit the report as JSON instead of a table")
	out := flag.String("out", "", "write the report to this file instead of stdout (parent directories are created)")
	flag.Var(&excludes, "exclude", "path, base name, or glob to skip (repeatable)")
	flag.Parse()

	abs, err := filepath.Abs(*root)
	if err != nil {
		fail(err.Error())
	}

	started := time.Now()
	files, err := Scan(abs, *hidden, excludes)
	if err != nil {
		fail(err.Error())
	}
	report := build(abs, started, files)
	report.DurationMS = time.Since(started).Milliseconds()

	sink, closeSink, err := openSink(*out)
	if err != nil {
		fail(err.Error())
	}
	if err := emit(sink, report, *asJSON); err != nil {
		fail(err.Error())
	}
	if err := closeSink(); err != nil {
		fail(err.Error())
	}
	if *out != "" {
		fmt.Fprintf(os.Stderr, "codestats: wrote %s\n", *out)
	}
}

// openSink returns the writer the report goes to and the close that must run after it,
// which is a no-op for stdout: closing os.Stdout would break a caller that pipes this
// command into another.
// It creates the parent directories of path, so -out .scratch/stats.txt works in a tree
// that has no .scratch yet.
func openSink(path string) (io.Writer, func() error, error) {
	if path == "" {
		return os.Stdout, func() error { return nil }, nil
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, err
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return file, file.Close, nil
}

// emit writes report to out in whichever of the two shapes was asked for.
// The two paths are joined here rather than at each call site so -out and -json compose:
// every combination of the two goes through one writer.
func emit(out io.Writer, r Report, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	}
	render(out, r)
	return nil
}

// fail prints msg and exits non-zero, the one error path every argument and I/O fault
// takes.
func fail(msg string) {
	fmt.Fprintln(os.Stderr, "codestats:", msg)
	os.Exit(1)
}

// render writes the report as markdown.
// Each section answers one question, in the order a reader asks them: how big is this
// tree, how much of it is test, and how much of it is prose.
// Markdown rather than an aligned text table because the report is read in an editor and
// on GitHub, where a fixed-width table only lines up in one of the two; numeric columns
// are right-aligned through the separator row, which is where markdown puts that.
func render(out io.Writer, r Report) {
	fmt.Fprintln(out, "# codestats")
	fmt.Fprintln(out)
	// A list, not two bare lines: consecutive lines are one paragraph in markdown, so a
	// renderer would run the two onto the same line.
	fmt.Fprintf(out, "- **Root:** `%s`\n", r.Root)
	fmt.Fprintf(out, "- **Generated:** %s\n", r.GeneratedAt)
	fmt.Fprintf(out, "- **Scan took:** %s\n", time.Duration(r.DurationMS)*time.Millisecond)

	fmt.Fprintln(out, "\n## By extension")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "| ext | files | lines | code | comment | blank | comment% |")
	fmt.Fprintln(out, "|---|---:|---:|---:|---:|---:|---:|")
	for _, ext := range sortedExts(r) {
		b := r.ByExt[ext]
		name := "`" + ext + "`"
		if ext == "" {
			name = "*(no extension)*"
		}
		commentShare := "—"
		if familyFor(ext) != familyNone {
			commentShare = pct(b.Comment, b.Code+b.Comment)
		}
		fmt.Fprintf(out, "| %s | %d | %d | %d | %d | %d | %s |\n",
			name, b.Files, b.Lines, b.Code, b.Comment, b.Blank, commentShare)
	}
	fmt.Fprintf(out, "| **total** | **%d** | **%d** | **%d** | **%d** | **%d** | **%s** |\n",
		r.Total.Files, r.Total.Lines, r.Total.Code, r.Total.Comment, r.Total.Blank,
		pct(r.Total.Comment, r.Total.Code+r.Total.Comment))

	fmt.Fprintln(out, "\n## Test vs production")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Source files only — the extensions with a comment syntax this tool reads.")
	fmt.Fprintln(out, "Markdown and the other prose formats are excluded, so the shares below are shares of code rather than of the tree.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "| | files | code | comment | comment% |")
	fmt.Fprintln(out, "|---|---:|---:|---:|---:|")
	fmt.Fprintf(out, "| production | %d | %d | %d | %s |\n",
		r.Production.Files, r.Production.Code, r.Production.Comment,
		pct(r.Production.Comment, r.Production.Code+r.Production.Comment))
	fmt.Fprintf(out, "| test | %d | %d | %d | %s |\n",
		r.Test.Files, r.Test.Code, r.Test.Comment,
		pct(r.Test.Comment, r.Test.Code+r.Test.Comment))
	fmt.Fprintf(out, "| **all source** | **%d** | **%d** | **%d** | **%s** |\n",
		r.Source.Files, r.Source.Code, r.Source.Comment,
		pct(r.Source.Comment, r.Source.Code+r.Source.Comment))
	fmt.Fprintln(out)
	fmt.Fprintf(out, "- Test share of source **code lines**: **%s**\n", pct(r.Test.Code, r.Source.Code))
	fmt.Fprintf(out, "- Test share of source **files**: **%s**\n", pct(r.Test.Files, r.Source.Files))

	fmt.Fprintln(out, "\n## Markdown")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "| | markdown | everything scanned | share |")
	fmt.Fprintln(out, "|---|---:|---:|---:|")
	fmt.Fprintf(out, "| files | %d | %d | %s |\n",
		r.Markdown.Files, r.Total.Files, pct(r.Markdown.Files, r.Total.Files))
	fmt.Fprintf(out, "| lines | %d | %d | %s |\n",
		r.Markdown.Lines, r.Total.Lines, pct(r.Markdown.Lines, r.Total.Lines))
}

// sortedExts orders the extension rows by size, biggest first, with ties broken by name
// so two runs over an unchanged tree produce byte-identical reports.
func sortedExts(r Report) []string {
	exts := make([]string, 0, len(r.ByExt))
	for ext := range r.ByExt {
		exts = append(exts, ext)
	}
	sort.Slice(exts, func(i, j int) bool {
		a, b := r.ByExt[exts[i]], r.ByExt[exts[j]]
		if a.Lines != b.Lines {
			return a.Lines > b.Lines
		}
		return exts[i] < exts[j]
	})
	return exts
}
