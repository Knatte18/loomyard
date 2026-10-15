//go:build integration

// time_integration_test.go checks that run reads each finished run's archived shed status from a real git repository's archive tags,
// and that with no slugs named it counts only full loom runs and says in the header which it counted and skipped.
// It also checks that -no-cutoff counts every transcript and keeps the span from the seed's start.

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

func TestRunReadsArchivedShedStatus(t *testing.T) {
	t.Parallel()
	records, hub, projects := t.TempDir(), t.TempDir(), t.TempDir()
	gitkit.Git(t, records, "init", "-b", "main")

	// commit writes files into the records and commits them with the given committer date, returning the commit.
	commit := func(date string, files map[string]string) string {
		t.Helper()
		for rel, content := range files {
			full := filepath.Join(records, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		gitkit.Git(t, records, "add", ".")
		cmd := exec.Command("git", "commit", "--allow-empty", "-m", "archive")
		cmd.Dir = records
		cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+date, "GIT_AUTHOR_DATE="+date)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit: %v; output: %s", err, out)
		}
		return gitkit.Git(t, records, "rev-parse", "HEAD")
	}
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
		doneSeed:    `{"recipe":"loom","driver":"go","started_at":"2026-01-01T09:58:00Z"}`,
		doneStatus:  `{"history":[` + entry("Plan", "2026-01-01T10:03:00Z") + `,` + entry("Publish", "2026-01-01T10:12:00Z") + `]}`,
		liveStatus:  `{"history":[` + entry("Plan", "2026-01-01T10:03:00Z") + `]}`,
		fullSeed:    `{"recipe":"loom","driver":"go","started_at":"2026-01-01T10:58:00Z"}`,
		fullStatus:  `{"history":[` + entry("Plan", "2026-01-01T11:03:00Z") + `,` + entry("Finalize", "2026-01-01T11:12:00Z") + `]}`,
		earlySeed:   `{"recipe":"loom","driver":"go","started_at":"2026-01-01T08:30:00Z"}`,
		earlyStatus: `{"history":[` + entry("Plan", "2026-01-01T10:03:00Z") + `,` + entry("Publish", "2026-01-01T10:12:00Z") + `]}`,
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
	// The done slug's counted transcript starts after its older tag's date and the live slug's after its newest tag's date;
	// the solo slug has one tag and no worktree, so no bound.
	// An earlier transcript of each bounded slug starts before that date and is left out.
	for slug, hour := range map[string]string{"done": "10", "live": "13", "solo": "08", "full": "11"} {
		at := func(minute string) string { return "2026-01-01T" + hour + ":" + minute + ":00Z" }
		writeLines(t, filepath.Join(projectDir(projects, filepath.Join(hub, slug)), "a.jsonl"),
			`{"type":"custom-title","customTitle":"ly:`+slug+`:burler"}`,
			userAt(t, at("00"), "go"),
			stamped(at("02"), `"end_turn"`),
			userAt(t, at("08"), "again"),
			stamped(at("10"), `"end_turn"`),
		)
	}
	for slug, started := range map[string]string{"done": "2026-01-01T08:00:00Z", "live": "2026-01-01T11:00:00Z"} {
		writeLines(t, filepath.Join(projectDir(projects, filepath.Join(hub, slug)), "earlier.jsonl"),
			`{"type":"custom-title","customTitle":"ly:`+slug+`:burler"}`,
			userAt(t, started, "earlier run"),
		)
	}
	gitkit.Git(t, records, "tag", "archive/solo/eeeeeee", older)

	reportPath := filepath.Join(t.TempDir(), "report.md")
	var stdout bytes.Buffer
	// -no-sleep keeps the host's journal out of the figures.
	args := []string{"-hub", hub, "-projects", projects, "-" + repositoryFlag, records, "-out", reportPath, "-no-sleep", "done", "live", "solo"}
	if err := run(args, &stdout); err != nil {
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
		if !strings.Contains(c.part, c.want) || !strings.Contains(c.part, "| burler | 1 |") {
			t.Errorf("%s section lacks %q beside one counted session:\n%s", c.name, c.want, c.part)
		}
	}
	if strings.Contains(soloPart, "left out") || !strings.Contains(soloPart, "| burler | 1 |") {
		t.Errorf("one-tag run = %q; want its transcript counted and no left-out line", soloPart)
	}
	const sleepOffLine = "Machine sleep counts as time: -no-sleep is set.\n"
	if !strings.Contains(report, " for done, live, solo.\n"+sleepOffLine+"\n") {
		t.Errorf("report header lacks the named slugs as given:\n%s", report)
	}
	const cutoffOffLine = "The archive-tag cutoff is off: every transcript in each slug's project directory counts.\n"
	if strings.Contains(report, cutoffOffLine) {
		t.Errorf("report without -no-cutoff holds the cutoff-off line:\n%s", report)
	}

	// With no slugs named, the default walks the runs newest session first and counts only full loom runs:
	// done stopped before Finalize and live is in flight, so both are skipped ahead of full.
	for slug, age := range map[string]time.Duration{"done": time.Hour, "live": 2 * time.Hour, "full": 3 * time.Hour, "solo": 4 * time.Hour} {
		sessions, err := filepath.Glob(filepath.Join(projectDir(projects, filepath.Join(hub, slug)), "*.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		touched := time.Now().Add(-age)
		for _, s := range sessions {
			if err := os.Chtimes(s, touched, touched); err != nil {
				t.Fatal(err)
			}
		}
	}
	var defaulted bytes.Buffer
	if err := run([]string{"-hub", hub, "-projects", projects, "-" + repositoryFlag, records, "-no-sleep", "-last", "1"}, &defaulted); err != nil {
		t.Fatalf("run with no slugs: %v", err)
	}
	wantHeader := " for the 1 most recent full loom runs: full.\n" + sleepOffLine + "Skipped 2 more recent runs that are not full loom runs: done, live.\n\n"
	if !strings.Contains(defaulted.String(), wantHeader) {
		t.Errorf("default report header lacks %q:\n%s", wantHeader, defaulted.String())
	}

	emptyHub := t.TempDir()
	err = run([]string{"-hub", emptyHub, "-projects", projects, "-" + repositoryFlag, records}, &defaulted)
	wantErr := "no full loom runs of " + emptyHub + " under " + projects + "; name the slugs to count a partial run"
	if err == nil || err.Error() != wantErr {
		t.Errorf("run with no full loom run = %v; want %q", err, wantErr)
	}

	// Written after the default runs above, so early's sessions change neither their order nor their skipped header.
	// Under -no-cutoff the earlier transcripts and the untimestamped one count, and the span keeps the seed's start instead of the older tag's date.
	earlyDir := projectDir(projects, filepath.Join(hub, "early"))
	writeLines(t, filepath.Join(earlyDir, "a.jsonl"),
		`{"type":"custom-title","customTitle":"ly:early:burler"}`,
		userAt(t, "2026-01-01T10:00:00Z", "go"),
		stamped("2026-01-01T10:02:00Z", `"end_turn"`),
		userAt(t, "2026-01-01T10:08:00Z", "again"),
		stamped("2026-01-01T10:10:00Z", `"end_turn"`),
	)
	writeLines(t, filepath.Join(earlyDir, "none.jsonl"),
		`{"type":"custom-title","customTitle":"ly:early:burler"}`,
		assistant("untimed", "claude-sonnet-4-5", 1, 1),
	)
	noCutoffPath := filepath.Join(t.TempDir(), "no-cutoff.md")
	args = []string{"-hub", hub, "-projects", projects, "-" + repositoryFlag, records, "-out", noCutoffPath, "-no-cutoff", "-no-sleep", "done", "live", "early"}
	if err := run(args, &stdout); err != nil {
		t.Fatalf("run with -no-cutoff: %v", err)
	}
	data, err = os.ReadFile(noCutoffPath)
	if err != nil {
		t.Fatal(err)
	}
	report = string(data)
	if !strings.Contains(report, " for done, live, early.\n"+cutoffOffLine+sleepOffLine+"\n") {
		t.Errorf("-no-cutoff report header lacks the cutoff-off line after the counted runs:\n%s", report)
	}
	if strings.Contains(report, "left out") {
		t.Errorf("-no-cutoff report leaves transcripts out:\n%s", report)
	}
	_, afterDone, _ = strings.Cut(report, "## done\n")
	donePart, afterLive, _ = strings.Cut(afterDone, "## live\n")
	livePart, earlyPart, _ := strings.Cut(afterLive, "## early\n")
	for name, part := range map[string]string{"done": donePart, "live": livePart, "early": earlyPart} {
		if !strings.Contains(part, "| burler | 2 |") {
			t.Errorf("-no-cutoff %s section lacks both transcripts counted:\n%s", name, part)
		}
	}
	if !strings.Contains(earlyPart, "| Plan | 1 | 1h33m0s |") {
		t.Errorf("-no-cutoff early section lacks the Plan row spanning from the seed's start:\n%s", earlyPart)
	}
}
