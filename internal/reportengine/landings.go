// landings.go reads the landings of the parent branch: it maps each first-parent commit to the run that landed it, classifies its kind from the board labels, and judges whether a fix repaired freshly landed code.

package reportengine

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// The kinds of a Landing.
const (
	LandingKindFix      = "fix"
	LandingKindFeature  = "feature"
	LandingKindOther    = "other"
	LandingKindUnmapped = "unmapped"
)

// The joins that map a landing to a run.
const (
	JoinExact = "exact"
	JoinTime  = "time"
)

// The verdicts of the fresh-code rule on a fix.
const (
	FreshnessFresh      = "fresh"
	FreshnessAged       = "aged"
	FreshnessNoPreImage = "no-pre-image"
)

// LLMStepsLanding is the RunRow.LLMSteps source counting a run that was landed by hand.
const LLMStepsLanding = "landing"

const (
	// firstTrailerPage is the page size of the first walk down a tag's history for a task-commit trailer; each later page doubles it.
	firstTrailerPage = 64

	// boardStatusDone is the status a board entry carries once its task is done.
	boardStatusDone = "done"

	labelBug         = "bug"
	labelFeature     = "feature"
	labelEnhancement = "enhancement"
)

// Landing is one first-parent commit of the parent branch.
// Slug and Tag name the run that landed it, Join says how it was mapped and is empty for an unmapped commit.
// Hand marks a commit landed without a finished loom run.
// LandIncomplete is nil unless a tagged full run's Finalize is done and its batten record is present, and then says the record never reaches Land.
// Fresh is the fresh-code verdict of a fix or unmapped landing, and empty for any other kind.
type Landing struct {
	Commit         string    `json:"commit"`
	Committed      time.Time `json:"committed"`
	Week           string    `json:"week"`
	Kind           string    `json:"kind"`
	Slug           string    `json:"slug,omitempty"`
	Tag            string    `json:"tag,omitempty"`
	Join           string    `json:"join,omitempty"`
	Hand           bool      `json:"hand"`
	LandIncomplete *bool     `json:"land_incomplete"`
	Fresh          string    `json:"fresh,omitempty"`
}

// landingClaim is a run's claim on one parent-branch commit.
// selected indexes landingInputs.selected, and is -1 for a slug with no archive tag to claim for.
type landingClaim struct {
	slug     string
	tag      string
	selected int
	join     string
}

// landingJoiner holds the state of one landing read.
type landingJoiner struct {
	src    Sources
	opts   Options
	report *Report

	// chain is the first-parent walk, oldest first, and position maps a commit to its index in it.
	chain    []gitrepo.LogCommit
	position map[string]int

	// absorbed marks the chain indexes of a fast-forwarded branch's own commits, and owner maps each to the landing commit it belongs to.
	absorbed map[int]bool
	owner    map[string]string

	// claimed maps a chain index to the claims on it.
	claimed map[int][]landingClaim

	board      []gitrepo.LogCommit
	boardTasks map[string][]boardengine.Task
}

// readLandings fills Report.Landings and each run's Landing and landing LLM step.
func readLandings(src Sources, opts Options, report *Report) error {
	j := &landingJoiner{
		src:        src,
		opts:       opts,
		report:     report,
		position:   map[string]int{},
		absorbed:   map[int]bool{},
		owner:      map[string]string{},
		claimed:    map[int][]landingClaim{},
		boardTasks: map[string][]boardengine.Task{},
	}
	for i := range report.Runs {
		report.Runs[i].LLMSteps[LLMStepsLanding] = 0
	}

	since := opts.Since
	if !since.IsZero() {
		since = since.AddDate(0, 0, -opts.FreshDays)
	}
	newestFirst, err := src.Code.Log(src.ParentTip, gitrepo.LogQuery{FirstParent: true, Since: since})
	if err != nil {
		return fmt.Errorf("reportengine: walk the parent branch: %w", err)
	}
	for i := len(newestFirst) - 1; i >= 0; i-- {
		j.position[newestFirst[i].SHA] = len(j.chain)
		j.chain = append(j.chain, newestFirst[i])
	}

	board, err := src.Board.Log(src.BoardTip, gitrepo.LogQuery{})
	if err != nil {
		return fmt.Errorf("reportengine: walk the board branch: %w", err)
	}
	j.board = board

	attempted, err := j.joinExact()
	if err != nil {
		return err
	}
	if err := j.joinTime(attempted); err != nil {
		return err
	}
	if err := j.assemble(); err != nil {
		return err
	}
	return j.judgeFreshness()
}

// taskCommitBelow returns the task commit named by the newest trailer in the first-parent history of tagCommit.
// It walks in growing pages, re-reading from the tag, until a trailer is found or a page comes back short.
func (j *landingJoiner) taskCommitBelow(tagCommit string) (string, bool, error) {
	for limit := firstTrailerPage; ; limit *= 2 {
		commits, err := j.src.Records.Log(tagCommit, gitrepo.LogQuery{FirstParent: true, Limit: limit})
		if err != nil {
			return "", false, fmt.Errorf("reportengine: walk the history of %s: %w", tagCommit, err)
		}
		for _, commit := range commits {
			if sha, ok := fabricengine.TaskCommitOf(commit.Message); ok {
				return sha, true, nil
			}
		}
		if len(commits) < limit {
			return "", false, nil
		}
	}
}

// landingOfTask returns the chain index of the first-parent commit that landed task: the task commit itself, a merge having it as a parent, or a squash holding its tree.
func (j *landingJoiner) landingOfTask(task string) int {
	if index, ok := j.position[task]; ok {
		return index
	}
	for index, commit := range j.chain {
		if slices.Contains(commit.Parents, task) {
			return index
		}
	}
	taskCommits, err := j.src.Code.Log(task, gitrepo.LogQuery{Limit: 1})
	if err != nil || len(taskCommits) == 0 {
		return -1
	}
	for index, commit := range j.chain {
		if commit.Tree != "" && commit.Tree == taskCommits[0].Tree {
			return index
		}
	}
	return -1
}

// joinExact maps each selected tag to the commit that landed its task commit.
// Two tags claiming one commit are both left unmapped and the clash is a finding.
// The returned set holds the indexes of the tags whose exact join was attempted, whether or not it mapped.
func (j *landingJoiner) joinExact() (map[int]bool, error) {
	attempted := map[int]bool{}
	taskOf := map[int]string{}
	claims := map[int][]int{}
	var order []int
	for i, selected := range j.report.inputs.selected {
		task, found, err := j.taskCommitBelow(selected.tag.Commit)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		attempted[i] = true
		taskOf[i] = task
		index := j.landingOfTask(task)
		if index < 0 {
			continue
		}
		if len(claims[index]) == 0 {
			order = append(order, index)
		}
		claims[index] = append(claims[index], i)
	}
	sort.Ints(order)

	for _, index := range order {
		tags := claims[index]
		if len(tags) > 1 {
			names := make([]string, len(tags))
			for k, i := range tags {
				names[k] = j.report.inputs.selected[i].tag.Name
			}
			j.report.Findings = append(j.report.Findings, Finding{Tag: names[0], Error: fmt.Sprintf("the tags %s claim one landing commit, %s", strings.Join(names, ", "), j.chain[index].SHA)})
		}
		for _, i := range tags {
			selected := j.report.inputs.selected[i]
			j.claimed[index] = append(j.claimed[index], landingClaim{slug: selected.slug, tag: selected.tag.Name, selected: i, join: JoinExact})
		}
	}

	j.absorbBranches(order, claims, taskOf)
	return attempted, nil
}

// absorbBranches marks the first-parent commits a fast-forwarded task branch brought before its task commit as that landing's own.
// A branch reaches back to the nearest older merge commit or exact-joined landing.
func (j *landingJoiner) absorbBranches(order []int, claims map[int][]int, taskOf map[int]string) {
	for _, index := range order {
		if len(claims[index]) != 1 || j.chain[index].SHA != taskOf[claims[index][0]] {
			continue
		}
		for k := index - 1; k >= 0 && len(j.chain[k].Parents) <= 1 && len(claims[k]) == 0; k-- {
			j.absorbed[k] = true
			j.owner[j.chain[k].SHA] = j.chain[index].SHA
		}
	}
}

// boardDoneAfter returns the time of the first board commit after open whose board shows slug's entry done.
func (j *landingJoiner) boardDoneAfter(slug string, open time.Time) (time.Time, bool, error) {
	for k := len(j.board) - 1; k >= 0; k-- {
		commit := j.board[k]
		if !commit.Committed.After(open) {
			continue
		}
		tasks, err := j.tasksAt(commit.SHA)
		if err != nil {
			return time.Time{}, false, err
		}
		for _, task := range tasks {
			if task.Slug == slug && task.Status != nil && *task.Status == boardStatusDone {
				return commit.Committed, true, nil
			}
		}
	}
	return time.Time{}, false, nil
}

// tasksAt returns the board entries at a board commit; a commit holding no board, or one that does not parse, holds none.
func (j *landingJoiner) tasksAt(sha string) ([]boardengine.Task, error) {
	if tasks, ok := j.boardTasks[sha]; ok {
		return tasks, nil
	}
	data, err := j.src.Board.FileAtRevision(sha, boardengine.BoardFileRel())
	var tasks []boardengine.Task
	switch {
	case errors.Is(err, gitrepo.ErrPathNotAtRevision):
	case err != nil:
		return nil, fmt.Errorf("reportengine: read the board at %s: %w", sha, err)
	default:
		if parsed, parseErr := boardengine.ParseBoardTasks(data); parseErr == nil {
			tasks = parsed
		}
	}
	j.boardTasks[sha] = tasks
	return tasks, nil
}

// newestCheckpoint returns the newest checkpoint time of times that falls after floor, when set, and at or before ceiling, when set.
func newestCheckpoint(times []time.Time, floor, ceiling *time.Time) (time.Time, bool) {
	for k := len(times) - 1; k >= 0; k-- {
		if (floor == nil || times[k].After(*floor)) && (ceiling == nil || !times[k].After(*ceiling)) {
			return times[k], true
		}
	}
	return time.Time{}, false
}

// joinTime maps the tags the exact join left unmapped, and the slugs whose pair has checkpoints after their newest tag, through their landing checkpoint and the board's done mark.
// A tag whose exact join was attempted is not retried; two runs claiming one commit are both left unmapped and the clash is a finding.
func (j *landingJoiner) joinTime(attempted map[int]bool) error {
	type windowOwner struct {
		slug     string
		tag      string
		selected int
		open     time.Time
	}
	var owners []windowOwner
	taggedSlugs := map[string]bool{}
	for i, selected := range j.report.inputs.selected {
		taggedSlugs[selected.slug] = true
		if attempted[i] {
			continue
		}
		ceiling := selected.tag.Committed
		if open, ok := newestCheckpoint(j.report.inputs.checkpoints[selected.slug], selected.previous, &ceiling); ok {
			owners = append(owners, windowOwner{slug: selected.slug, tag: selected.tag.Name, selected: i, open: open})
		}
	}
	var untagged []string
	for slug := range j.report.inputs.checkpoints {
		untagged = append(untagged, slug)
	}
	sort.Strings(untagged)
	for _, slug := range untagged {
		var floor *time.Time
		if newest, ok := j.report.inputs.newestTag[slug]; ok {
			floor = &newest
		}
		if open, ok := newestCheckpoint(j.report.inputs.checkpoints[slug], floor, nil); ok {
			owners = append(owners, windowOwner{slug: slug, selected: -1, open: open})
		}
	}

	claims := map[int][]windowOwner{}
	var order []int
	for _, owner := range owners {
		closed, found, err := j.boardDoneAfter(owner.slug, owner.open)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		for index, commit := range j.chain {
			if j.absorbed[index] || len(j.claimed[index]) > 0 || commit.Committed.Before(owner.open) || commit.Committed.After(closed) {
				continue
			}
			if len(claims[index]) == 0 {
				order = append(order, index)
			}
			claims[index] = append(claims[index], owner)
			break
		}
	}
	sort.Ints(order)
	for _, index := range order {
		holders := claims[index]
		if len(holders) > 1 {
			names := make([]string, len(holders))
			for k, holder := range holders {
				names[k] = holder.slug
			}
			j.report.Findings = append(j.report.Findings, Finding{Tag: holders[0].tag, Error: fmt.Sprintf("the slugs %s claim one landing commit, %s, through the time join", strings.Join(names, ", "), j.chain[index].SHA)})
			continue
		}
		holder := holders[0]
		j.claimed[index] = append(j.claimed[index], landingClaim{slug: holder.slug, tag: holder.tag, selected: holder.selected, join: JoinTime})
	}
	return nil
}

// labelsOf returns the labels of slug's entry in the newest board commit at or before at that holds it.
func (j *landingJoiner) labelsOf(slug string, at time.Time) ([]string, bool, error) {
	for _, commit := range j.board {
		if commit.Committed.After(at) {
			continue
		}
		tasks, err := j.tasksAt(commit.SHA)
		if err != nil {
			return nil, false, err
		}
		for _, task := range tasks {
			if task.Slug == slug {
				return task.Labels, true, nil
			}
		}
	}
	return nil, false, nil
}

// kindOfLabels maps board labels to a landing kind.
func kindOfLabels(labels []string) string {
	switch {
	case slices.Contains(labels, labelBug):
		return LandingKindFix
	case slices.Contains(labels, labelFeature), slices.Contains(labels, labelEnhancement):
		return LandingKindFeature
	}
	return LandingKindOther
}

// assemble builds Report.Landings from the walk and the claims, and points each mapped full run at its landing.
func (j *landingJoiner) assemble() error {
	for index, commit := range j.chain {
		if j.absorbed[index] || commit.Committed.Before(j.opts.Since) {
			continue
		}
		landing := Landing{Commit: commit.SHA, Committed: commit.Committed.UTC(), Week: isoWeekLabel(commit.Committed), Kind: LandingKindUnmapped}

		held := j.claimed[index]
		if len(held) == 1 {
			claim := held[0]
			landing.Slug, landing.Tag, landing.Join = claim.slug, claim.tag, claim.join
			labels, found, err := j.labelsOf(claim.slug, commit.Committed)
			if err != nil {
				return err
			}
			landing.Kind = kindOfLabels(labels)
			if !found {
				j.report.Findings = append(j.report.Findings, Finding{Tag: claim.tag, Error: fmt.Sprintf("no board commit holds an entry for the slug %q", claim.slug)})
			}
			j.applyRun(&landing, claim)
		}
		j.report.Landings = append(j.report.Landings, landing)
	}
	return nil
}

// applyRun sets what the claiming tag says about its landing: whether it was landed by hand, and whether the batten record never reached Land.
// A full run also points at the landing and counts a hand landing as an LLM step.
func (j *landingJoiner) applyRun(landing *Landing, claim landingClaim) {
	if claim.selected < 0 {
		return
	}
	selected := j.report.inputs.selected[claim.selected]
	if claim.join == JoinExact {
		landing.Hand = selected.noFinalize || (selected.run >= 0 && !selected.finalizeDone)
	}
	if selected.run < 0 {
		return
	}
	row := &j.report.Runs[selected.run]
	if selected.finalizeDone && len(row.battenSnapshots) > 0 {
		reachedLand := slices.ContainsFunc(row.battenSnapshots[len(row.battenSnapshots)-1].History, func(entry shedengine.HistoryEntry) bool {
			return entry.Producer == battenrecipe.NameLand && entry.Outcome == shedengine.Done
		})
		incomplete := !reachedLand
		landing.LandIncomplete = &incomplete
	}
}

// judgeFreshness decides, for each fix and unmapped landing, whether its modified lines mostly came from freshly landed code.
// Runs after assemble, so it also points each full run at its finished landing.
func (j *landingJoiner) judgeFreshness() error {
	for i := range j.report.Landings {
		landing := &j.report.Landings[i]
		if landing.Kind == LandingKindFix || landing.Kind == LandingKindUnmapped {
			fresh, err := j.freshnessOf(*landing)
			if err != nil {
				return err
			}
			landing.Fresh = fresh
		}
	}

	for i := range j.report.Landings {
		landing := &j.report.Landings[i]
		held := j.claimed[j.position[landing.Commit]]
		if len(held) != 1 || held[0].selected < 0 || j.report.inputs.selected[held[0].selected].run < 0 {
			continue
		}
		row := &j.report.Runs[j.report.inputs.selected[held[0].selected].run]
		row.Landing = landing
		if landing.Hand {
			row.LLMSteps[LLMStepsLanding] = 1
		}
	}
	return nil
}

// freshnessOf blames the lines landing modifies or deletes in its first parent and rates the share that came from recent landings.
func (j *landingJoiner) freshnessOf(landing Landing) (string, error) {
	preImage, err := j.src.Code.PreImageLines(landing.Commit)
	if err != nil {
		return "", fmt.Errorf("reportengine: read the pre-image of %s: %w", landing.Commit, err)
	}
	var paths []string
	for name, lines := range preImage {
		if len(lines) > 0 && !strings.HasSuffix(name, "_test.go") && !strings.HasPrefix(name, "docs/") && path.Ext(name) != ".md" {
			paths = append(paths, name)
		}
	}
	commit := j.chain[j.position[landing.Commit]]
	if len(paths) == 0 || len(commit.Parents) == 0 {
		return FreshnessNoPreImage, nil
	}
	sort.Strings(paths)

	window := time.Duration(j.opts.FreshDays) * 24 * time.Hour
	total, fresh := 0, 0
	for _, name := range paths {
		lines := preImage[name]
		attributed, err := j.src.Code.BlameLines(commit.Parents[0], name, lines)
		if err != nil {
			return "", fmt.Errorf("reportengine: blame %s at %s: %w", name, commit.Parents[0], err)
		}
		for _, line := range lines {
			total++
			owner, ok := j.landingCommitted(attributed[line])
			if ok && !owner.After(landing.Committed) && landing.Committed.Sub(owner) <= window {
				fresh++
			}
		}
	}
	if fresh*2 > total {
		return FreshnessFresh, nil
	}
	return FreshnessAged, nil
}

// landingCommitted returns the commit time of the landing that brought the blamed commit sha: the commit itself, or the fast-forward landing its task-branch commit belongs to.
// A commit the walk did not reach has no owner.
func (j *landingJoiner) landingCommitted(sha string) (time.Time, bool) {
	if landing, ok := j.owner[sha]; ok {
		sha = landing
	}
	index, ok := j.position[sha]
	if !ok {
		return time.Time{}, false
	}
	return j.chain[index].Committed, true
}
