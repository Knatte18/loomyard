// time_integration_test.go checks that run reads each finished run's archived shed status from a real git repository's archive tags.
// It checks that with no slugs named it counts only finished runs and says in the header which it counted and skipped.
// It checks that the Recipes section counts the lines of the commit a run landed on a real code repository, root commits included,
// and that -since counts every finished run active since a date.
// It also checks that -no-cutoff counts every located session and keeps the span from the seed's start,
// and that the header names an engine that has no transcript reader.

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// burlerSessions returns the sessions cell of the burler part row in a -detail segment table, empty when the row is absent.
func burlerSessions(section string) string {
	for _, line := range strings.Split(section, "\n") {
		if cells := strings.Split(line, "|"); strings.HasPrefix(line, "| └ burler |") && len(cells) > 7 {
			return strings.TrimSpace(cells[7])
		}
	}
	return ""
}

// tapestryRow matches the Recipes row of one tapestry run that landed a commit of 3 changed lines.
var tapestryRow = regexp.MustCompile(`(?m)^\| tapestry \| 1 \| [^|]+ \| [^|]+ \| 1 \| 3 \| [^|]+ \|$`)

func TestCodeRepoChangedLines(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	dir := t.TempDir()
	gitkit.Git(t, dir, "init", "-b", "main")
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		gitkit.Git(t, dir, "add", ".")
		gitkit.Git(t, dir, "commit", "-m", "change")
	}
	write("one\ntwo\n")
	root := gitkit.Git(t, dir, "rev-parse", "HEAD")
	write("one\ntwo\nthree\n")
	child := gitkit.Git(t, dir, "rev-parse", "HEAD")
	repo := codeRepo{dir: dir}
	for _, c := range []struct {
		name, sha string
		want      int
	}{
		{"a root commit changes every line it holds", root, 2},
		{"a commit changes the lines against its parent", child, 1},
	} {
		got, err := repo.ChangedLines(c.sha)
		if err != nil || got != c.want {
			t.Errorf("%s: ChangedLines = %d, %v; want %d", c.name, got, err, c.want)
		}
	}
}

func TestRunReadsArchivedShedStatus(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	records, hub, code := t.TempDir(), t.TempDir(), t.TempDir()
	gitkit.Git(t, records, "init", "-b", "main")
	gitkit.Git(t, code, "init", "-b", "main")

	// commitIn writes files into the repository at dir and commits them under message with the given committer date, returning the commit.
	commitIn := func(dir, message, date string, files map[string]string) string {
		t.Helper()
		for rel, content := range files {
			full := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		gitkit.Git(t, dir, "add", ".")
		cmd := exec.Command("git", "commit", "--allow-empty", "-m", message)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+date, "GIT_AUTHOR_DATE="+date)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit: %v; output: %s", err, out)
		}
		return gitkit.Git(t, dir, "rev-parse", "HEAD")
	}
	commit := func(date string, files map[string]string) string {
		t.Helper()
		return commitIn(records, "archive", date, files)
	}
	// The full run landed one commit of three new lines inside its span, after a base commit.
	const landedTitle = "Land the full run"
	commitIn(code, "base", "2026-01-01T09:00:00Z", map[string]string{"base.txt": "base\n"})
	commitIn(code, landedTitle, "2026-01-01T11:10:00Z", map[string]string{"landed.txt": "one\ntwo\nthree\n"})
	paths := func(slug string) (seedRel, statusRel string) {
		t.Helper()
		seedRel, statusRel, err := shedrun.TreeRunFileRels(slug)
		if err != nil {
			t.Fatal(err)
		}
		return seedRel, statusRel
	}
	entry := func(producer, at string) string {
		return `{"producer":"` + producer + `","outcome":"done","output":"","at":"` + at + `"}`
	}
	doneSeed, doneStatus := paths("done")
	_, liveStatus := paths("live")
	fullSeed, fullStatus := paths("full")
	earlySeed, earlyStatus := paths("early")

	older := commit("2026-01-01T09:00:00Z", map[string]string{
		doneStatus: `{"history":[` + entry("OldProducer", "2026-01-01T09:30:00Z") + `]}`,
		liveStatus: `{"history":[` + entry("OldLive", "2026-01-01T09:30:00Z") + `]}`,
	})
	newest := commit("2026-01-01T12:00:00Z", map[string]string{
		doneSeed:    `{"recipe":"tapestry","driver":"go","started_at":"2026-01-01T09:58:00Z"}`,
		doneStatus:  `{"history":[` + entry("Plan", "2026-01-01T10:03:00Z") + `,` + entry("Publish", "2026-01-01T10:12:00Z") + `]}`,
		liveStatus:  `{"history":[` + entry("Plan", "2026-01-01T10:03:00Z") + `]}`,
		fullSeed:    `{"recipe":"tapestry","driver":"go","started_at":"2026-01-01T10:58:00Z"}`,
		fullStatus:  `{"history":[` + entry("Plan", "2026-01-01T11:03:00Z") + `,` + entry("Finalize", "2026-01-01T11:12:00Z") + `]}`,
		earlySeed:   `{"recipe":"tapestry","driver":"go","started_at":"2026-01-01T08:30:00Z"}`,
		earlyStatus: `{"history":[` + entry("Plan", "2026-01-01T10:03:00Z") + `,` + entry("Publish", "2026-01-01T10:12:00Z") + `]}`,
		// Every slug tagged at this commit shares its change description; only the full run's span holds the landed commit.
		summaryRel: "# " + landedTitle + "\n\nWhat changed.\n",
	})
	// Archive tags are lightweight.
	gitkit.Git(t, records, "tag", "archive/done/aaaaaaa", older)
	gitkit.Git(t, records, "tag", "archive/done/bbbbbbb", newest)
	gitkit.Git(t, records, "tag", "archive/live/ccccccc", older)
	gitkit.Git(t, records, "tag", "archive/live/ddddddd", newest)
	gitkit.Git(t, records, "tag", "archive/full/fffffff", newest)
	gitkit.Git(t, records, "tag", "archive/early/1111111", older)
	gitkit.Git(t, records, "tag", "archive/early/2222222", newest)

	// The in-flight run's worktree still exists.
	if err := os.MkdirAll(filepath.Join(hub, "live"), 0o755); err != nil {
		t.Fatal(err)
	}
	// readsFor builds a read set of the fake engine over the slugs' sessions.
	// The done slug's counted session starts after its older tag's date and the live slug's after its newest tag's date;
	// the solo slug has one tag and no worktree, so no bound.
	// An earlier session of each bounded slug starts before that date and is left out.
	// The slugs are touched done, live, full, solo from newest to oldest, which orders the default selection.
	// The early slug's sessions are written only when early is set: its older tag and newest tag bound nothing under -no-cutoff,
	// and the default selection must not see it.
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	readsFor := func(early bool) ReadSet {
		fake := newFakeTranscripts()
		ages := map[string]time.Duration{"done": time.Hour, "live": 2 * time.Hour, "full": 3 * time.Hour, "solo": 4 * time.Hour}
		for slug, hour := range map[string]string{"done": "10", "live": "13", "solo": "08", "full": "11"} {
			at := func(minute string) string { return "2026-01-01T" + hour + ":" + minute + ":00Z" }
			fake.add(filepath.Join(hub, slug), now.Add(-ages[slug]), sessionOf(readOf("a.jsonl", "ly:"+slug+":burler",
				userAt(at("00"), "go"),
				stamped(at("02"), true),
				userAt(at("08"), "again"),
				stamped(at("10"), true),
			)))
		}
		for slug, started := range map[string]string{"done": "2026-01-01T08:00:00Z", "live": "2026-01-01T11:00:00Z"} {
			fake.add(filepath.Join(hub, slug), now.Add(-ages[slug]), sessionOf(readOf("earlier.jsonl", "ly:"+slug+":burler",
				userAt(started, "earlier run"),
			)))
		}
		if early {
			fake.add(filepath.Join(hub, "early"), now, sessionOf(readOf("a.jsonl", "ly:early:burler",
				userAt("2026-01-01T10:00:00Z", "go"),
				stamped("2026-01-01T10:02:00Z", true),
				userAt("2026-01-01T10:08:00Z", "again"),
				stamped("2026-01-01T10:10:00Z", true),
			)))
			fake.add(filepath.Join(hub, "early"), now, sessionOf(readOf("none.jsonl", "ly:early:burler",
				assistant("untimed", "model-a", 1, 1),
			)))
		}
		return fakeReadSet(fake)
	}
	gitkit.Git(t, records, "tag", "archive/solo/eeeeeee", older)

	reportPath := filepath.Join(t.TempDir(), "report.md")
	var stdout bytes.Buffer
	// An engine with no transcript reader sits beside the fake one.
	// -no-sleep keeps the host's journal out of the figures.
	withBlind := readsFor(false)
	withBlind["blind"] = shuttleengine.RecordReads{}
	// -detail shows the sessions column the counted sessions are read from.
	args := []string{"-hub", hub, "-" + repositoryFlag, records, "-code", code, "-out", reportPath, "-no-sleep", "-detail", "done", "live", "solo"}
	if err := run(args, &stdout, withBlind); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	report := string(data)
	if !strings.Contains(report, "# Token usage and time by role\n") {
		t.Errorf("report lacks the title:\n%s", report)
	}
	_, afterDone, _ := strings.Cut(report, "## done\n")
	donePart, afterLive, _ := strings.Cut(afterDone, "## live\n")
	livePart, soloPart, _ := strings.Cut(afterLive, "## solo\n")
	if !strings.Contains(donePart, "| Publish | 1 | 9m0s | 7m0s | 2m0s | 50.0% |") || strings.Contains(donePart, "OldProducer") {
		t.Errorf("finished run lacks the newest tag's Producers table:\n%s", donePart)
	}
	if !strings.Contains(livePart, "No shed status was read") || strings.Contains(livePart, "### Producers") {
		t.Errorf("in-flight run = %q; want the no-status line and no Producers table", livePart)
	}
	// The finished run's bound is its older tag's date, the in-flight run's its newest tag's, and a one-tag slug has none.
	for _, c := range []struct {
		name, part, want string
	}{
		{"finished run", donePart, "Session transcripts left out: 1; they started before the slug's previous run ended, at 2026-01-01T09:00:00Z.\n"},
		{"in-flight run", livePart, "Session transcripts left out: 1; they started before the slug's previous run ended, at 2026-01-01T12:00:00Z.\n"},
	} {
		if !strings.Contains(c.part, c.want) || burlerSessions(c.part) != "1" {
			t.Errorf("%s section lacks %q beside one counted session:\n%s", c.name, c.want, c.part)
		}
	}
	if strings.Contains(soloPart, "left out") || burlerSessions(soloPart) != "1" {
		t.Errorf("one-tag run = %q; want its transcript counted and no left-out line", soloPart)
	}
	const sleepOffLine = "Machine sleep counts as time: -no-sleep is set.\n"
	const blindLine = "Not counted, no transcript reader: blind.\n"
	if !strings.Contains(report, " for done, live, solo.\n"+sleepOffLine+blindLine+"\n") {
		t.Errorf("report header lacks the named slugs as given and the engine without a reader:\n%s", report)
	}
	const cutoffOffLine = "The archive-tag cutoff is off: every located session of each slug counts.\n"
	if strings.Contains(report, cutoffOffLine) {
		t.Errorf("report without -no-cutoff holds the cutoff-off line:\n%s", report)
	}
	// The done run's span ends before the titled commit, and the other two runs have no shed record.
	wantMissing := "No landed commit: done (" + noTitledCommit + "), live (" + noShedRecord + "), solo (" + noShedRecord + ").\n"
	if !strings.Contains(report, wantMissing) {
		t.Errorf("report lacks %q:\n%s", wantMissing, report)
	}

	// With no slugs named, the default walks the runs newest session first and counts only finished runs:
	// done stopped before Finalize and live is in flight, so both are skipped ahead of full.
	var defaulted bytes.Buffer
	if err := run([]string{"-hub", hub, "-" + repositoryFlag, records, "-code", code, "-no-sleep", "-last", "1"}, &defaulted, readsFor(false)); err != nil {
		t.Fatalf("run with no slugs: %v", err)
	}
	wantHeader := " for the 1 most recent finished tapestry, darn and arras runs: full.\n" + sleepOffLine + "Skipped 2 runs that are not finished tapestry, darn or arras runs: done, live.\n\n"
	if !strings.Contains(defaulted.String(), wantHeader) {
		t.Errorf("default report header lacks %q:\n%s", wantHeader, defaulted.String())
	}
	// The full run landed one commit of three changed lines.
	if !tapestryRow.MatchString(defaulted.String()) {
		t.Errorf("default report's Recipes section lacks the full run's landed commit of 3 lines:\n%s", defaulted.String())
	}

	// -since counts every finished run active since the day, whatever -last says, and heads the report with the date.
	var sinceReport bytes.Buffer
	if err := run([]string{"-hub", hub, "-" + repositoryFlag, records, "-code", code, "-no-sleep", "-last", "1", "-since", "2026-01-01"}, &sinceReport, readsFor(false)); err != nil {
		t.Fatalf("run with -since: %v", err)
	}
	wantSinceHeader := " for the 1 finished tapestry, darn and arras runs active since 2026-01-01: full.\n" + sleepOffLine + "Skipped 3 runs that are not finished tapestry, darn or arras runs: done, live, solo.\n\n"
	if !strings.Contains(sinceReport.String(), wantSinceHeader) {
		t.Errorf("-since report header lacks %q:\n%s", wantSinceHeader, sinceReport.String())
	}

	emptyHub := t.TempDir()
	err = run([]string{"-hub", emptyHub, "-" + repositoryFlag, records}, &defaulted, readsFor(false))
	wantErr := "no finished tapestry, darn or arras runs of " + emptyHub + " from engines " + fakeEngine + "; name the slugs to count a partial run"
	if err == nil || err.Error() != wantErr {
		t.Errorf("run with no finished run = %v; want %q", err, wantErr)
	}

	// Under -no-cutoff the earlier sessions and the untimestamped one count, and the span keeps the seed's start instead of the older tag's date.
	noCutoffPath := filepath.Join(t.TempDir(), "no-cutoff.md")
	args = []string{"-hub", hub, "-" + repositoryFlag, records, "-code", code, "-out", noCutoffPath, "-no-cutoff", "-no-sleep", "-detail", "done", "live", "early"}
	if err := run(args, &stdout, readsFor(true)); err != nil {
		t.Fatalf("run with -no-cutoff: %v", err)
	}
	data, err = os.ReadFile(noCutoffPath)
	if err != nil {
		t.Fatal(err)
	}
	report = string(data)
	if !strings.Contains(report, " for done, live, early.\n"+cutoffOffLine+sleepOffLine+"\n") {
		t.Errorf("-no-cutoff report header lacks the cutoff-off line after the counted runs and any no-reader line:\n%s", report)
	}
	if strings.Contains(report, "Not counted, no transcript reader:") {
		t.Errorf("report of an all-readable engine set names an engine without a reader:\n%s", report)
	}
	if strings.Contains(report, "left out") {
		t.Errorf("-no-cutoff report leaves transcripts out:\n%s", report)
	}
	_, afterDone, _ = strings.Cut(report, "## done\n")
	donePart, afterLive, _ = strings.Cut(afterDone, "## live\n")
	livePart, earlyPart, _ := strings.Cut(afterLive, "## early\n")
	for name, part := range map[string]string{"done": donePart, "live": livePart, "early": earlyPart} {
		if burlerSessions(part) != "2" {
			t.Errorf("-no-cutoff %s section lacks both transcripts counted:\n%s", name, part)
		}
	}
	if !strings.Contains(earlyPart, "| Plan | 1 | 1h33m0s |") {
		t.Errorf("-no-cutoff early section lacks the Plan row spanning from the seed's start:\n%s", earlyPart)
	}
}
