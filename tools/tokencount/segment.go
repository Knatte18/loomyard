// segment.go groups a run's roles and its orch charge into the report's segments, each split into parts.
// It writes the segment tables, the most expensive parts list and the cost per run table.

package main

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/websterengine"
)

// forkSuffix ends the role a session's forks count under.
const forkSuffix = "+sub"

// segmentDef is one segment a run's roles fall into by prefix.
type segmentDef struct {
	Name string
	// Key is the role of the segment's writer and, followed by "-", the prefix of its other roles.
	Key string
	// Writer and WriterForks label the parts of the writer's own sessions and of their forks.
	Writer, WriterForks string
}

// segmentDefs lists the segments in report order, each placed by its role prefix.
var segmentDefs = []segmentDef{
	{Name: "Discussion", Key: "discussion", Writer: "writer", WriterForks: "writer forks"},
	{Name: "Plan", Key: "plan", Writer: "writer", WriterForks: "writer forks"},
	{Name: "Webster", Key: websterMasterRole, Writer: "Master", WriterForks: "forks"},
	{Name: "Merge", Key: "merge", Writer: "writer", WriterForks: "writer forks"},
	{Name: "Alignment", Key: "alignment", Writer: "session", WriterForks: "session forks"},
	{Name: "Darn", Key: "darn", Writer: "writer", WriterForks: "writer forks"},
	{Name: "Drift", Key: "drift", Writer: "writer", WriterForks: "writer forks"},
	{Name: "Code-Review", Key: "code", Writer: "writer", WriterForks: "writer forks"},
}

// landingRoles lists the roles whose sessions the Landing segment holds:
// `describe`, `pr-reviewer`, `fixer` and `fix-reviewer`.
var landingRoles = []string{"describe", "pr-reviewer", "fixer", "fix-reviewer"}

const (
	// websterSegment holds the webster recovery strand beside its prefixed roles.
	websterSegment = "Webster"
	// landingSegment holds the landing roles, each a part labelled by its own name.
	landingSegment = "Landing"
	orchSegment    = "Orch"
	// otherSegment holds every role no segment's prefix places.
	otherSegment = "Other"
)

// Labels of the orch segment's parts.
const (
	orchSessionLabel      = "session"
	orchForksLabel        = "forks"
	orchUnattributedLabel = "unattributed"
)

// roleSuffixLabels label a segment role by what follows the segment key and its "-".
var roleSuffixLabels = map[string]string{
	"burler-review": "reviewer",
	"burler-fix":    "fixer",
	"bouncer-judge": "judge",
	"bouncer-seed":  "seed",
}

// segmentNames returns the segment names in the column order of the cost per run table.
func segmentNames() []string {
	names := make([]string, 0, len(segmentDefs)+3)
	for _, def := range segmentDefs {
		names = append(names, def.Name)
	}
	return append(names, landingSegment, orchSegment, otherSegment)
}

// partOf places a role in its segment and labels its part there.
// The segment's key role is its writer's part, and a role starting with the key and "-" is labelled by the rest.
// A fork role is labelled by its parent's label with " forks" appended.
// The webster recovery role goes to the Webster segment.
// A landing role goes to the Landing segment under its own name,
// and any other role goes to the Other segment under its own name.
func partOf(role string) (segment, label string) {
	base, forked := strings.CutSuffix(role, forkSuffix)
	withForks := func(label string) string {
		if forked {
			return label + " forks"
		}
		return label
	}
	if base == string(websterengine.RoleRecovery) {
		return websterSegment, withForks(base)
	}
	if slices.Contains(landingRoles, base) {
		return landingSegment, withForks(base)
	}
	for _, def := range segmentDefs {
		if base == def.Key {
			if forked {
				return def.Name, def.WriterForks
			}
			return def.Name, def.Writer
		}
		if suffix, ok := strings.CutPrefix(base, def.Key+"-"); ok {
			if named, ok := roleSuffixLabels[suffix]; ok {
				suffix = named
			}
			return def.Name, withForks(suffix)
		}
	}
	return otherSegment, withForks(base)
}

// Part is one sub-row of a segment: the roles placed under one label, or one side of an orch charge.
type Part struct {
	Label    string
	Sessions int
	Usage    Usage
	// Weight is the usage's weight for a role part and the charged weight for an orch part.
	Weight float64
	Cost   Cost
	Models map[string]int
	Time   time.Duration
	// Charged marks an orch part, which carries no sessions, tokens, time or models.
	Charged bool
}

// Segment is one of a run's report segments: its parts, most expensive first, and its working time.
// The working time is the union of its roles' working intervals.
type Segment struct {
	Name  string
	Parts []*Part
	Time  time.Duration
}

// Cost is the sum of the segment's parts' costs.
func (s Segment) Cost() Cost {
	var total Cost
	for _, p := range s.Parts {
		total.add(p.Cost)
	}
	return total
}

// Charged reports whether the segment is the orch charge, which carries no time.
func (s Segment) Charged() bool {
	return len(s.Parts) > 0 && s.Parts[0].Charged
}

// Segments groups the run's roles into segments, each sorted by cost.
// A non-nil orch charge becomes an Orch segment.
func (run RunTally) Segments(orch *OrchCharge) []Segment {
	parts := map[string]map[string]*Part{}
	segmentWork := map[string][]Interval{}
	partWork := map[string]map[string][]Interval{}
	for role, t := range run.Roles {
		name, label := partOf(role)
		if parts[name] == nil {
			parts[name] = map[string]*Part{}
			partWork[name] = map[string][]Interval{}
		}
		p := parts[name][label]
		if p == nil {
			p = &Part{Label: label, Models: map[string]int{}}
			parts[name][label] = p
		}
		p.Sessions += t.Sessions
		p.Usage.add(t.Usage)
		p.Weight += t.Usage.Weight()
		p.Cost.add(t.Cost)
		for model, n := range t.Models {
			p.Models[model] += n
		}
		partWork[name][label] = append(partWork[name][label], t.Working...)
		segmentWork[name] = append(segmentWork[name], t.Working...)
	}
	var segments []Segment
	for name, byLabel := range parts {
		segment := Segment{Name: name, Time: covered(segmentWork[name], run.Span)}
		for label, p := range byLabel {
			p.Time = covered(partWork[name][label], run.Span)
			segment.Parts = append(segment.Parts, p)
		}
		segments = append(segments, segment)
	}
	if orch != nil {
		segments = append(segments, Segment{Name: orchSegment, Parts: []*Part{
			{Label: orchSessionLabel, Charged: true, Weight: orch.Session.Weight, Cost: orch.Session.Cost},
			{Label: orchForksLabel, Charged: true, Weight: orch.Forks.Weight, Cost: orch.Forks.Cost},
		}})
	}
	sortSegments(segments)
	return segments
}

// mergeSegments sums the segments of several runs by segment name and part label, times included.
// It sorts the result by cost.
func mergeSegments(perRun [][]Segment) []Segment {
	var merged []Segment
	index := map[string]int{}
	for _, segments := range perRun {
		for _, s := range segments {
			i, ok := index[s.Name]
			if !ok {
				i = len(merged)
				index[s.Name] = i
				merged = append(merged, Segment{Name: s.Name})
			}
			merged[i].Time += s.Time
			for _, p := range s.Parts {
				j := slices.IndexFunc(merged[i].Parts, func(q *Part) bool { return q.Label == p.Label })
				if j < 0 {
					merged[i].Parts = append(merged[i].Parts, &Part{Label: p.Label, Models: map[string]int{}, Charged: p.Charged})
					j = len(merged[i].Parts) - 1
				}
				sum := merged[i].Parts[j]
				sum.Sessions += p.Sessions
				sum.Usage.add(p.Usage)
				sum.Weight += p.Weight
				sum.Cost.add(p.Cost)
				sum.Time += p.Time
				for model, n := range p.Models {
					sum.Models[model] += n
				}
			}
		}
	}
	sortSegments(merged)
	return merged
}

// sortSegments orders the parts within each segment and the segments themselves by cost, most expensive first.
// Ties are broken by name.
func sortSegments(segments []Segment) {
	for _, s := range segments {
		sort.Slice(s.Parts, func(i, j int) bool {
			if s.Parts[i].Cost.USD != s.Parts[j].Cost.USD {
				return s.Parts[i].Cost.USD > s.Parts[j].Cost.USD
			}
			return s.Parts[i].Label < s.Parts[j].Label
		})
	}
	sort.SliceStable(segments, func(i, j int) bool {
		ci, cj := segments[i].Cost().USD, segments[j].Cost().USD
		if ci != cj {
			return ci > cj
		}
		return segments[i].Name < segments[j].Name
	})
}

// totalCost is the sum of every segment's cost.
func totalCost(segments []Segment) Cost {
	var total Cost
	for _, s := range segments {
		total.add(s.Cost())
	}
	return total
}

// costShare renders part as a percentage of whole's USD, 0% when whole has none.
func costShare(part, whole Cost) string {
	if whole.USD == 0 {
		return "0.0%"
	}
	return fmt.Sprintf("%.1f%%", 100*part.USD/whole.USD)
}

// mostExpensiveParts caps how many rows the most expensive parts list holds.
const mostExpensiveParts = 10

// writeMostExpensiveParts writes the costliest parts of segments, each with its share of their total cost.
// A part's cost per run is its cost divided by runs.
func writeMostExpensiveParts(w io.Writer, segments []Segment, runs int) {
	type named struct {
		name string
		cost Cost
	}
	var all []named
	for _, s := range segments {
		for _, p := range s.Parts {
			all = append(all, named{s.Name + " " + p.Label, p.Cost})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].cost.USD != all[j].cost.USD {
			return all[i].cost.USD > all[j].cost.USD
		}
		return all[i].name < all[j].name
	})
	total := totalCost(segments)
	fmt.Fprintf(w, "## Most expensive parts\n\n")
	fmt.Fprintln(w, "| part | cost | share | per run |")
	fmt.Fprintln(w, "|---|---|---|---|")
	for _, part := range all[:min(len(all), mostExpensiveParts)] {
		fmt.Fprintf(w, "| %s | %s | %s | %s |\n", part.name, part.cost, costShare(part.cost, total), part.cost.share(1/float64(runs)))
	}
	fmt.Fprintln(w)
}

// writeCostPerRun writes one row per run with its segments' costs and its total, then their average.
// perRun holds each run's segments; the segment columns are the segments some run holds, in segmentNames order.
func writeCostPerRun(w io.Writer, runs []RunTally, perRun [][]Segment) {
	var names []string
	for _, name := range segmentNames() {
		held := slices.ContainsFunc(perRun, func(segments []Segment) bool {
			return slices.ContainsFunc(segments, func(s Segment) bool { return s.Name == name })
		})
		if held {
			names = append(names, name)
		}
	}
	fmt.Fprintf(w, "## Cost per run\n\n")
	fmt.Fprintf(w, "| run | recipe | %s | Total |\n", strings.Join(names, " | "))
	fmt.Fprintf(w, "|---|---|%s---|\n", strings.Repeat("---|", len(names)))
	sums := make([]Cost, len(names)+1)
	for i, segments := range perRun {
		cells := make([]Cost, len(names)+1)
		for _, s := range segments {
			cells[slices.Index(names, s.Name)] = s.Cost()
		}
		cells[len(names)] = totalCost(segments)
		writeCostRow(w, runs[i].Slug, runs[i].Recipe(), cells)
		for j, c := range cells {
			sums[j].add(c)
		}
	}
	for j := range sums {
		sums[j] = sums[j].share(1 / float64(len(perRun)))
	}
	writeCostRow(w, "average", "", sums)
	fmt.Fprintln(w)
}

func writeCostRow(w io.Writer, name, recipe string, cells []Cost) {
	rendered := make([]string, len(cells))
	for i, c := range cells {
		rendered[i] = c.String()
	}
	fmt.Fprintf(w, "| %s | %s | %s |\n", name, recipe, strings.Join(rendered, " | "))
}

// segmentTable holds the settings one segment table needs beside its segments.
type segmentTable struct {
	// Idle is the table's idle duration, and Span is the duration its time shares divide by.
	Idle, Span time.Duration
	// Runs is the number of runs the table sums, and 0 omits the per-run average the `orchSegment` row shows.
	Runs int
	// Detail adds the token columns.
	Detail bool
}

// detailColumns are the token columns a detailed table appends after the models column.
const detailColumns = " sessions | output | cache write | cache read | input | weight |"

// write writes the segment tree: each segment's total row, then its parts indented below it, then an idle row.
// Cost shares divide by the total cost of the segments, and time shares divide by the span.
func (table segmentTable) write(w io.Writer, segments []Segment) {
	total := totalCost(segments)
	timeCells := func(d time.Duration) string {
		pct := 0.0
		if table.Span > 0 {
			pct = 100 * float64(d) / float64(table.Span)
		}
		return fmt.Sprintf("%s | %.1f%%", d.Round(time.Second), pct)
	}
	header, rule := "| segment | cost | share | time | time share | models (messages) |", "|---|---|---|---|---|---|"
	if table.Detail {
		header += detailColumns
		rule += strings.Repeat("---|", strings.Count(detailColumns, "|"))
	}
	fmt.Fprintln(w, header)
	fmt.Fprintln(w, rule)
	row := func(name string, p Part, charged bool) {
		times, modelCell := " | ", ""
		if !charged {
			times = timeCells(p.Time)
			modelCell = models(p.Models)
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s |", name, p.Cost, costShare(p.Cost, total), times, modelCell)
		if table.Detail {
			if charged {
				fmt.Fprintf(w, " | | | | | %.1fM |", p.Weight/1e6)
			} else {
				u := p.Usage
				fmt.Fprintf(w, " %d | %d | %d | %d | %d | %.1fM |", p.Sessions, u.Output, u.CacheCreate, u.CacheRead, u.Input, p.Weight/1e6)
			}
		}
		fmt.Fprintln(w)
	}
	for _, s := range segments {
		sum := Part{Models: map[string]int{}, Time: s.Time, Cost: s.Cost()}
		for _, p := range s.Parts {
			sum.Sessions += p.Sessions
			sum.Usage.add(p.Usage)
			sum.Weight += p.Weight
			for model, n := range p.Models {
				sum.Models[model] += n
			}
		}
		name := "**" + s.Name + "**"
		if s.Name == orchSegment && table.Runs > 0 {
			name += fmt.Sprintf(", %s per run", sum.Cost.share(1/float64(table.Runs)))
		}
		row(name, sum, s.Charged())
		for _, p := range s.Parts {
			row("└ "+p.Label, *p, p.Charged)
		}
	}
	idle := "| idle | | | " + timeCells(table.Idle) + " | |"
	if table.Detail {
		idle += strings.Repeat(" |", strings.Count(detailColumns, "|"))
	}
	fmt.Fprintln(w, idle)
	fmt.Fprintln(w)
}
