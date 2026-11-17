// retro.go chooses the runs a retro covers, finds the last retro, and renders the retro task's board fields.

package retroengine

import (
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

const (
	// ReportDirRel is the worktree-relative directory holding one report directory per retro, named by its slug.
	ReportDirRel = "docs/research/retro"

	// ResearchIndexRel is the worktree-relative index that lists every research report, the retro reports included.
	ResearchIndexRel = "docs/research/README.md"

	// ToolsDirRel is the worktree-relative directory of the retro's own scripts.
	ToolsDirRel = "tools/retro"

	// SlugPrefix opens the slug of every retro task and the name of every retro report directory.
	SlugPrefix = "retro-"

	// stampLayout is the form of the UTC time that follows SlugPrefix: the end of the window a retro covers.
	stampLayout = "20060102-150405"

	// StencilName names the stencil the retro task's body renders from.
	StencilName = "retro-template-task"

	// TaskProfile is the run profile of the retro task, the darn profile for prose work.
	TaskProfile = "docs"

	// TaskLabel is the type label of the retro task.
	TaskLabel = "research"

	// DefaultMaxRuns is the most runs one retro covers when the caller sets no cap.
	DefaultMaxRuns = 10

	// ReadCap is the most transcript spans, stencils and instruction passages the retro's writer reads.
	ReadCap = 12

	// DefaultWindow is how far back a retro reaches when it has no earlier retro and no start of its own.
	DefaultWindow = 7 * 24 * time.Hour

	// NamedLookback is how far back a named run may have landed.
	NamedLookback = 60 * 24 * time.Hour
)

// Run is one landed run: its slug and the time of the commit that landed it on the parent branch.
type Run struct {
	Slug   string
	Landed time.Time
}

// Window says which runs a retro covers: the named runs, or the runs landed in a span of time.
type Window struct {
	// Slugs names the runs to cover; when it holds any, Since and Until are zero.
	Slugs []string

	// Since is the time after which a landed run is covered; zero means after the last retro.
	Since time.Time

	// Until is the time up to which a landed run is covered; zero means Now.
	Until time.Time

	// Now is the time the retro is filed.
	Now time.Time

	// MaxRuns is the most runs the retro covers, at least one.
	MaxRuns int
}

// Plan is one retro: its slug, the window it covers and the runs in it, oldest first.
type Plan struct {
	Slug  string
	From  time.Time
	Until time.Time
	Runs  []Run
}

// LastRetro returns the newest stamp among names that carry the retro form, SlugPrefix and then a stamp.
// The bool is false when no name does.
func LastRetro(names []string) (time.Time, bool) {
	var newest time.Time
	found := false
	for _, name := range names {
		rest, ok := strings.CutPrefix(name, SlugPrefix)
		if !ok {
			continue
		}
		stamp, err := time.ParseInLocation(stampLayout, rest, time.UTC)
		if err != nil {
			continue
		}
		if !found || stamp.After(newest) {
			newest, found = stamp, true
		}
	}
	return newest, found
}

// Start returns the earliest landing time the window can cover, given the last retro's stamp when hasLast holds.
// A caller reads the landings from this time on.
func (w Window) Start(last time.Time, hasLast bool) time.Time {
	switch {
	case len(w.Slugs) > 0:
		return w.Now.Add(-NamedLookback)
	case !w.Since.IsZero():
		return w.Since
	case hasLast:
		return last
	default:
		return w.end().Add(-DefaultWindow)
	}
}

// end returns the time up to which the window covers runs.
func (w Window) end() time.Time {
	if w.Until.IsZero() {
		return w.Now
	}
	return w.Until
}

// Select chooses the runs a retro covers from landed, the runs landed on the parent branch, and names the retro.
// last is the stamp of the newest earlier retro, and hasLast reports whether one exists.
// Named slugs are covered exactly, and the retro's stamp is the newest one's landing;
// its window starts a second before the oldest one's landing, so every named run landed after it.
// A name with no landing within NamedLookback is refused, and so are more names than MaxRuns.
// Otherwise the retro covers the runs landed after the window's start and up to its end.
// Past MaxRuns it keeps the oldest and ends its window at the last kept landing,
// so the next retro starts with the first run left out;
// runs landed in the same second as the first one left out are left out too, so no second is split across retros,
// and a cap that falls inside the oldest landing's second keeps that whole second.
// A window with no run in it is refused.
func Select(landed []Run, last time.Time, hasLast bool, w Window) (Plan, error) {
	newest := map[string]time.Time{}
	for _, run := range landed {
		if at, ok := newest[run.Slug]; !ok || run.Landed.After(at) {
			newest[run.Slug] = run.Landed
		}
	}

	if len(w.Slugs) > 0 {
		var runs []Run
		for _, slug := range w.Slugs {
			at, ok := newest[slug]
			if !ok {
				return Plan{}, fmt.Errorf("retro: run %q has no landing on the parent branch within the last %d days, since %s: "+
					"name only runs landed within them (a misspelt slug and an unlanded run have none), or cover an older run with --since and --until",
					slug, int(NamedLookback.Hours()/24), w.Start(last, hasLast).UTC().Format(time.RFC3339))
			}
			if !slices.ContainsFunc(runs, func(r Run) bool { return r.Slug == slug }) {
				runs = append(runs, Run{Slug: slug, Landed: at})
			}
		}
		if len(runs) > w.MaxRuns {
			return Plan{}, fmt.Errorf("retro: %d runs named, more than the cap of %d: name fewer runs, or raise --max-runs", len(runs), w.MaxRuns)
		}
		sortByLanding(runs)
		return newPlan(runs[0].Landed.Add(-time.Second), runs[len(runs)-1].Landed, runs), nil
	}

	from, until := w.Start(last, hasLast), w.end()
	var runs []Run
	for slug, at := range newest {
		if at.After(from) && !at.After(until) {
			runs = append(runs, Run{Slug: slug, Landed: at})
		}
	}
	if len(runs) == 0 {
		return Plan{}, fmt.Errorf("retro: no run landed after %s and up to %s: name the runs to cover, or pass an earlier --since",
			from.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339))
	}
	sortByLanding(runs)
	if len(runs) > w.MaxRuns {
		// The next retro starts after the last kept landing.
		// A run landed in the first left-out run's second must not be split from it, so those runs are all kept or all left out.
		firstLeftOut := runs[w.MaxRuns].Landed
		cut := w.MaxRuns
		for cut > 0 && runs[cut-1].Landed.Equal(firstLeftOut) {
			cut--
		}
		if cut == 0 {
			// The cap falls inside the oldest landing's second, so that whole second is kept, past the cap.
			for cut < len(runs) && runs[cut].Landed.Equal(firstLeftOut) {
				cut++
			}
		}
		runs = runs[:cut]
		until = runs[len(runs)-1].Landed
	}
	return newPlan(from, until, runs), nil
}

// sortByLanding orders runs oldest landing first, and by slug between runs landed at the same time.
func sortByLanding(runs []Run) {
	slices.SortFunc(runs, func(a, b Run) int {
		if c := a.Landed.Compare(b.Landed); c != 0 {
			return c
		}
		return strings.Compare(a.Slug, b.Slug)
	})
}

// newPlan names the retro over runs by the end of its window.
func newPlan(from, until time.Time, runs []Run) Plan {
	return Plan{Slug: SlugPrefix + until.UTC().Format(stampLayout), From: from, Until: until, Runs: runs}
}

// ReportDir returns the worktree-relative directory the retro's report goes in.
func (p Plan) ReportDir() string {
	return path.Join(ReportDirRel, p.Slug)
}

// Slugs returns the covered runs' slugs, oldest landing first.
func (p Plan) Slugs() []string {
	slugs := make([]string, len(p.Runs))
	for i, run := range p.Runs {
		slugs[i] = run.Slug
	}
	return slugs
}

// Body renders the board body of the retro task from the retro stencil under stencilsDir.
func (p Plan) Body(stencilsDir string) (string, error) {
	template, err := stencilstore.Read(stencilsDir, StencilName)
	if err != nil {
		return "", err
	}
	lines := make([]string, len(p.Runs))
	for i, run := range p.Runs {
		lines[i] = fmt.Sprintf("- `%s`, landed %s", run.Slug, run.Landed.UTC().Format(time.RFC3339))
	}
	slugs := p.Slugs()
	body, err := stencil.Fill(template, map[string]string{
		"runs":           strings.Join(lines, "\n"),
		"run_slugs":      strings.Join(slugs, " "),
		"run_list":       strings.Join(slugs, ","),
		"from":           p.From.UTC().Format(time.RFC3339),
		"until":          p.Until.UTC().Format(time.RFC3339),
		"report_dir":     p.ReportDir(),
		"retro_dir":      ReportDirRel,
		"research_index": ResearchIndexRel,
		"tools_dir":      ToolsDirRel,
		"read_cap":       strconv.Itoa(ReadCap),
	})
	if err != nil {
		return "", fmt.Errorf("retro: render stencil %q: %w", StencilName, err)
	}
	return strings.TrimSpace(stencil.StripLeadingComment(string(body))) + "\n", nil
}

// TaskFields returns the board fields of the retro task with body: a darn task on the docs profile.
func (p Plan) TaskFields(body string) map[string]any {
	return map[string]any{
		"slug":    p.Slug,
		"title":   fmt.Sprintf("Retro of %d landed runs up to %s", len(p.Runs), p.Until.UTC().Format(time.RFC3339)),
		"brief":   "Audit the runs from their records and transcripts, and propose fixes, most severe first",
		"kind":    boardengine.KindTask,
		"recipe":  shedrun.RecipeDarn,
		"profile": TaskProfile,
		"labels":  []string{TaskLabel},
		"body":    body,
	}
}
