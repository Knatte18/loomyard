// profile.go reports each batcher profile's cost and quality from finished runs: a per-batch cost table, one row per run and one summary line per profile.
// Everything but the transcripts' tokens comes from the one repository the tool is pointed at: the runs' webster records and the Webster-Review files committed beside them, read through gitrepo's read side only.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// RunRecords reads the repository holding the runs' plan commits, webster records and review files.
type RunRecords interface {
	PlanHistory
	FilesInDirAtRevision(rev, dir string) ([]string, error)
}

// websterReviewSubdir is the Webster-Review segments' run subdirectory under the reviews directory, the loom recipe's run_subdir value.
const websterReviewSubdir = "webster"

// identityProfile is the profile of a run that recorded no partition: one batch per card.
const identityProfile = "identity"

// BatchProfile is one batch of a run with the cost its forks measured.
type BatchProfile struct {
	Run     string
	Batch   int
	Profile string
	Cards   []string
	// Peak is the largest peak context of the batch's forks and Weight their summed weight.
	Peak   int
	Weight float64
	// Failed reports a terminal failed, dead or stuck record, or a recovery.
	Failed bool
	// Recovery reports a record of kind recovery.
	Recovery bool
}

// RunProfile is one run's row: its batches' counts, or the Mark saying why they are missing.
type RunProfile struct {
	Run string
	// Mark is why the run's figures are missing and it enters no profile line, empty when it is counted.
	Mark string
	// Batches are the run's batches, empty when Mark is set.
	Batches []BatchProfile
	// Findings counts the findings of the run's Webster-Review files, valid when FindingsMark is empty.
	Findings int
	// FindingsMark is why the findings were not read, empty when they were.
	FindingsMark string
}

// Profiles lists the distinct profiles of the run's batches, sorted.
func (r RunProfile) Profiles() []string {
	seen := map[string]bool{}
	var names []string
	for _, batch := range r.Batches {
		if !seen[batch.Profile] {
			seen[batch.Profile] = true
			names = append(names, batch.Profile)
		}
	}
	sort.Strings(names)
	return names
}

// Cards counts the run's cards over its batches.
func (r RunProfile) Cards() int {
	total := 0
	for _, batch := range r.Batches {
		total += len(batch.Cards)
	}
	return total
}

func (r RunProfile) failed() int {
	total := 0
	for _, batch := range r.Batches {
		if batch.Failed {
			total++
		}
	}
	return total
}

func (r RunProfile) recoveries() int {
	total := 0
	for _, batch := range r.Batches {
		if batch.Recovery {
			total++
		}
	}
	return total
}

// ProfileLine sums one profile over the counted runs, each batch going to its own profile.
type ProfileLine struct {
	Profile                 string
	Runs, Batches, Cards    int
	Weight                  float64
	Failed, Recoveries      int
	FindingsRuns            int
	Findings, FindingsCards int
}

// ProfileReport is the cost and quality of the batcher profiles over a set of runs.
type ProfileReport struct {
	Runs     []RunProfile
	Profiles []ProfileLine
}

// BuildProfileReport reads every run's webster records and review files from records and sums them per profile.
// A run that cannot be read is a marked row, never an error;
// an error is a failed read of the plan history.
func BuildProfileReport(runs []RunTally, records RunRecords) (ProfileReport, error) {
	var report ProfileReport
	lines := map[string]*ProfileLine{}
	for _, run := range runs {
		profile, err := readRunProfile(run, records)
		if err != nil {
			return ProfileReport{}, err
		}
		report.Runs = append(report.Runs, profile)
		if profile.Mark != "" {
			continue
		}
		profiles := profile.Profiles()
		inRun := map[string]bool{}
		for _, batch := range profile.Batches {
			line := lines[batch.Profile]
			if line == nil {
				line = &ProfileLine{Profile: batch.Profile}
				lines[batch.Profile] = line
			}
			if !inRun[batch.Profile] {
				inRun[batch.Profile] = true
				line.Runs++
			}
			line.Batches++
			line.Cards += len(batch.Cards)
			line.Weight += batch.Weight
			if batch.Failed {
				line.Failed++
			}
			if batch.Recovery {
				line.Recoveries++
			}
		}
		// Findings belong to the run as a whole, so only a run whose batches name one profile contributes them.
		if len(profiles) == 1 && profile.FindingsMark == "" {
			line := lines[profiles[0]]
			line.FindingsRuns++
			line.Findings += profile.Findings
			line.FindingsCards += profile.Cards()
		}
	}
	for _, line := range lines {
		report.Profiles = append(report.Profiles, *line)
	}
	sort.Slice(report.Profiles, func(i, j int) bool { return report.Profiles[i].Profile < report.Profiles[j].Profile })
	return report, nil
}

// plannedBatch is one batch of a run as its last recorded state, or its plan, partitions it.
type plannedBatch struct {
	number  int
	profile string
	cards   []string
}

// readRunProfile reads one run's records: its last state's partition and batch records, the forks that ran each batch and the findings of its review files.
func readRunProfile(run RunTally, records RunRecords) (RunProfile, error) {
	row := RunProfile{Run: run.Slug}
	commits, err := records.CommitsWithSubject(websterRecordSubjectPrefix + run.Slug)
	if err != nil {
		return row, fmt.Errorf("find webster records of %s: %w", run.Slug, err)
	}
	if len(commits) == 0 {
		row.Mark = "no webster run record"
		return row, nil
	}

	guids := map[string]bool{}
	var last *websterengine.State
	for i, commit := range commits {
		state, err := readRecordedState(records, commit.SHA)
		if err != nil {
			row.Mark = fmt.Sprintf("state at %s unreadable: %v", commit.SHA, err)
			return row, nil
		}
		guids[state.RunGUID] = true
		if i == 0 {
			last = state
		}
	}
	if len(guids) > 1 {
		row.Mark = fmt.Sprintf("restarted with --fresh: its records carry %d run guids", len(guids))
		return row, nil
	}

	var planned []plannedBatch
	if len(last.Partition) > 0 {
		for _, batch := range last.Partition {
			profile := batch.Profile
			if profile == "" {
				profile = identityProfile
			}
			planned = append(planned, plannedBatch{number: cardNumber(batch.Cards[0]), profile: profile, cards: batch.Cards})
		}
	} else {
		plan, reason, err := readRunPlan(run, records)
		if err != nil {
			return row, err
		}
		if plan == nil {
			row.Mark = "no partition recorded and its plan is unreadable: " + reason
			return row, nil
		}
		for _, card := range plan.Cards {
			planned = append(planned, plannedBatch{number: card.Number, profile: identityProfile, cards: []string{fmt.Sprintf("%02d-%s", card.Number, card.Slug)}})
		}
	}

	for _, batch := range planned {
		measured := BatchProfile{Run: run.Slug, Batch: batch.number, Profile: batch.profile, Cards: batch.cards}
		if record := last.Batches[batch.number]; record != nil {
			transcripts := map[string]bool{}
			for _, name := range record.ForkTranscripts {
				transcripts[path.Base(filepath.ToSlash(name))] = true
			}
			for _, fork := range run.Forks {
				if transcripts[fork.File] {
					measured.Weight += fork.Usage.Weight()
					measured.Peak = max(measured.Peak, fork.PeakContext)
				}
			}
			measured.Recovery = record.Kind == "recovery"
			measured.Failed = measured.Recovery || record.Status == "failed" || record.Status == "dead" || record.Status == "stuck"
		}
		row.Batches = append(row.Batches, measured)
	}

	row.Findings, row.FindingsMark = countFindings(records, commits[0].SHA)
	return row, nil
}

// readRecordedState decodes state.json as it stood at the record commit rev, leniently, so a state with an older shape still reads.
func readRecordedState(records RunRecords, rev string) (*websterengine.State, error) {
	data, err := records.FileAtRevision(rev, path.Join(websterengine.DirRel(), "state.json"))
	if err != nil {
		return nil, err
	}
	var state websterengine.State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// countFindings sums the findings of every round-<n>-review.md file directly in the webster review directory at rev.
// It returns a mark instead when there is no such file or one cannot be read or parsed.
func countFindings(records RunRecords, rev string) (int, string) {
	dir := path.Join(filepath.ToSlash(loomengine.LoomReviewsDirRel()), websterReviewSubdir)
	names, err := records.FilesInDirAtRevision(rev, dir)
	if err != nil {
		return 0, fmt.Sprintf("review directory unreadable: %v", err)
	}
	total, files := 0, 0
	for _, name := range names {
		if _, ok := shedadapters.ParseRoundReviewName(name); !ok {
			continue
		}
		files++
		data, err := records.FileAtRevision(rev, path.Join(dir, name))
		if err != nil {
			return 0, fmt.Sprintf("%s unreadable: %v", name, err)
		}
		_, findings, err := burlerengine.ParseReview(data)
		if err != nil {
			return 0, fmt.Sprintf("%s does not parse: %v", name, err)
		}
		total += len(findings)
	}
	if files == 0 {
		return 0, "no review files"
	}
	return total, ""
}

// cardNumber returns the number of a NN-<slug> card id, or 0 when it has none.
func cardNumber(id string) int {
	number, _, _ := strings.Cut(id, "-")
	n, _ := strconv.Atoi(number)
	return n
}

// WriteMarkdown writes the per-batch cost table, the row of each run and the summary line of each profile.
func (r ProfileReport) WriteMarkdown(w io.Writer) {
	fmt.Fprintf(w, "## Batch cost\n\n")
	fmt.Fprintln(w, "| run | batch | profile | cards | measured peak | weight |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|")
	for _, run := range r.Runs {
		for _, batch := range run.Batches {
			fmt.Fprintf(w, "| %s | %d | %s | %s | %d | %.1fM |\n", batch.Run, batch.Batch, batch.Profile, strings.Join(batch.Cards, ", "), batch.Peak, batch.Weight/1e6)
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "## Runs by profile\n\n")
	fmt.Fprintln(w, "| run | profiles | batches | cards | failed batches | recoveries | findings | findings per card | note |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|")
	for _, run := range r.Runs {
		if run.Mark != "" {
			fmt.Fprintf(w, "| %s | n/a | n/a | n/a | n/a | n/a | n/a | n/a | marked: %s |\n", run.Run, run.Mark)
			continue
		}
		findings, perCard, note := "not read", "n/a", "findings not read: "+run.FindingsMark
		if run.FindingsMark == "" {
			findings, note = strconv.Itoa(run.Findings), ""
			if cards := run.Cards(); cards > 0 {
				perCard = fmt.Sprintf("%.3f", float64(run.Findings)/float64(cards))
			}
		}
		fmt.Fprintf(w, "| %s | %s | %d | %d | %d | %d | %s | %s | %s |\n", run.Run, strings.Join(run.Profiles(), ", "), len(run.Batches), run.Cards(), run.failed(), run.recoveries(), findings, perCard, note)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "## Profiles\n\n")
	fmt.Fprintln(w, "| profile | runs | batches | cards | weight per card | failed batches | recoveries | findings per card |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|")
	for _, line := range r.Profiles {
		weightPerCard := "n/a"
		if line.Cards > 0 {
			weightPerCard = fmt.Sprintf("%.3fM", line.Weight/float64(line.Cards)/1e6)
		}
		findings := "n/a"
		if line.FindingsRuns > 0 && line.FindingsCards > 0 {
			findings = fmt.Sprintf("%.3f (%d runs)", float64(line.Findings)/float64(line.FindingsCards), line.FindingsRuns)
		}
		fmt.Fprintf(w, "| %s | %d | %d | %d | %s | %d | %d | %s |\n", line.Profile, line.Runs, line.Batches, line.Cards, weightPerCard, line.Failed, line.Recoveries, findings)
	}
	fmt.Fprintln(w)
}
