package shedengine

// Routing is the told projection of a recipe's producer graph that progress is derived from.
// Only each ProducerDef's Name, OnDone, OnStuck, Segment and MaxBounces are read, so Producer
// may be nil and no engine needs building to project a recipe.
type Routing struct {
	// Entry is the name of the first row of the main line.
	Entry string
	// Producers is the recipe's producer table.
	Producers []ProducerDef
	// MaxBounces is the shed-level default bounce budget, inherited as in Shed.MaxBounces.
	MaxBounces int
}

// Progress locates the current producer on the recipe's main line.
type Progress struct {
	// Step is the 1-based index of the current producer's step, or 0 when it maps to none.
	Step int `json:"step"`
	// Steps is the number of main-line steps.
	Steps int `json:"steps"`
	// Name is the current step's name; for an unmapped producer, the producer's own name.
	Name string `json:"name"`
	// Remaining names the main-line steps after the current one.
	Remaining []string `json:"remaining"`
}

// progressStep is one main-line step: a run of consecutive rows sharing a segment, or a lone row.
type progressStep struct {
	name string
	rows []string
}

// mainLine follows OnDone from Entry until an empty OnDone or a revisited or unknown name, and
// collapses consecutive rows sharing a non-empty Segment into one step named after the segment.
func (r Routing) mainLine() []progressStep {
	byName := make(map[string]ProducerDef, len(r.Producers))
	for _, def := range r.Producers {
		byName[def.Name] = def
	}
	var steps []progressStep
	lastSegment := ""
	seen := map[string]bool{}
	for name := r.Entry; name != "" && !seen[name]; {
		def, ok := byName[name]
		if !ok {
			break
		}
		seen[name] = true
		if def.Segment != "" && len(steps) > 0 && lastSegment == def.Segment {
			last := &steps[len(steps)-1]
			last.rows = append(last.rows, def.Name)
		} else if def.Segment != "" {
			steps = append(steps, progressStep{name: def.Segment, rows: []string{def.Name}})
		} else {
			steps = append(steps, progressStep{name: def.Name, rows: []string{def.Name}})
		}
		lastSegment = def.Segment
		name = def.OnDone
	}
	return steps
}

// stepIndex returns the index into steps of the step current maps to, or -1. A main-line row maps
// to its own step; a row off the main line maps to the step named after its segment.
func (r Routing) stepIndex(steps []progressStep, current string) int {
	for i, s := range steps {
		for _, row := range s.rows {
			if row == current {
				return i
			}
		}
	}
	for _, def := range r.Producers {
		if def.Name != current || def.Segment == "" {
			continue
		}
		for i, s := range steps {
			if s.name == def.Segment {
				return i
			}
		}
	}
	return -1
}

// ProgressAt reports where current sits on the main line.
func (r Routing) ProgressAt(current string) Progress {
	steps := r.mainLine()
	names := make([]string, len(steps))
	for i, s := range steps {
		names[i] = s.name
	}
	idx := r.stepIndex(steps, current)
	if idx < 0 {
		return Progress{Step: 0, Steps: len(steps), Name: current, Remaining: names}
	}
	return Progress{Step: idx + 1, Steps: len(steps), Name: names[idx], Remaining: names[idx+1:]}
}

// Bounces reports the current segment's bounce count and budget. The count is the episode stuck
// count of the segment's main-line row, whichever of the segment's rows is current: a Burler
// returns Stuck every round and never Done, so its own episode never resets and is never counted.
// inSegment is false, with zero count and budget, when current's row has no Segment.
func (r Routing) Bounces(current string, history []HistoryEntry) (count int, budget int, inSegment bool) {
	var cur *ProducerDef
	for i := range r.Producers {
		if r.Producers[i].Name == current {
			cur = &r.Producers[i]
			break
		}
	}
	if cur == nil || cur.Segment == "" {
		return 0, 0, false
	}
	row := *cur
	steps := r.mainLine()
	if idx := r.stepIndex(steps, current); idx >= 0 {
		onLine := false
		for _, name := range steps[idx].rows {
			if name == current {
				onLine = true
			}
		}
		if !onLine {
			for _, def := range r.Producers {
				if def.Name == steps[idx].rows[0] {
					row = def
					break
				}
			}
		}
	}
	return episodeStuckCount(history, row, r.Producers), effectiveMaxBounces(row, r.MaxBounces), true
}
