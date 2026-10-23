// trace.go matches the located failures of a verify log to the live plan's cards and their Webster batches, and builds the tracer a landing producer is told.

package failuretrace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/Knatte18/quarry/quarry"
	"golang.org/x/mod/modfile"
)

// Input is everything Trace reads.
type Input struct {
	// Log is the failed verify's combined output.
	Log string
	// Head is the commit the verify ran at.
	Head string
	// Placement places the log's locations in the repository.
	Placement Placement
	// Plan is the parsed live plan.
	Plan *planparser.Plan
	// State is Webster's run state.
	State *websterengine.State
	// Enclose answers path:line targets against the files committed at rev.
	Enclose func(rev string, targets []string) ([]quarry.EncloseResult, error)
	// Resolve resolves glyph targets against the verified tree.
	Resolve func(targets []string) ([]quarry.ResolveResult, error)
	// Delta compares the symbols committed at from with those committed at to.
	Delta func(from, to string) (quarry.GitDeltaAnswer, error)
}

// Trace maps the failing locations of in.Log to the plan cards whose targets hold them.
// Cards come out in card-number order, each with its matched glyphs and locations, its batch and the batch's commit range.
// LaterRange runs from the last card commit of the highest-numbered matched batch that has one to in.Head.
// Untraced lists the locations no card holds, the opaque identities' locations first.
// A seam error is returned, and so is a nil plan or a nil state.
func Trace(in Input) (verifytree.FailureTrace, error) {
	if in.Plan == nil {
		return verifytree.FailureTrace{}, errors.New("failuretrace: no plan to trace against")
	}
	if in.State == nil {
		return verifytree.FailureTrace{}, errors.New("failuretrace: no webster state to trace against")
	}
	placed, untraced := Locations(websterengine.ParseVerifyFailures(in.Log), in.Placement)
	var answers []quarry.EncloseResult
	if len(placed) > 0 {
		var err error
		if answers, err = in.Enclose(in.Head, placed); err != nil {
			return verifytree.FailureTrace{}, fmt.Errorf("failuretrace: enclose %d location(s) at %s: %w", len(placed), in.Head, err)
		}
		if len(answers) != len(placed) {
			return verifytree.FailureTrace{}, fmt.Errorf("failuretrace: enclose answered %d of %d location(s)", len(answers), len(placed))
		}
	}

	m := &matcher{in: in, matched: map[int]*cardMatch{}, deltas: map[string]map[string][]string{}}
	for i, answer := range answers {
		held, err := m.match(answer, placed[i])
		if err != nil {
			return verifytree.FailureTrace{}, err
		}
		if !held {
			untraced = append(untraced, placed[i])
		}
	}
	return m.result(untraced), nil
}

// cardMatch collects what one card was matched by.
type cardMatch struct {
	card      planparser.Card
	glyphs    []string
	locations []string
}

// matcher holds one Trace call's lazily built answers.
type matcher struct {
	in      Input
	matched map[int]*cardMatch
	// resolved maps each member glyph the plan targets to the files its declarations sit in at the head; nil until first needed.
	resolved map[string][]string
	// deltas maps a batch start to the files each symbol of the delta from it to the head sat in before; keyed by glyph.
	deltas map[string]map[string][]string
	// batchOf maps a live card id to its Webster batch number; nil until first needed.
	batchOf map[string]int
}

// match matches one enclose answer to the cards that hold it and reports whether any card does.
func (m *matcher) match(answer quarry.EncloseResult, location string) (bool, error) {
	switch answer.Status {
	case quarry.StatusFound:
		held := false
		for _, card := range m.in.Plan.Cards {
			for _, target := range card.Targets {
				for _, symbol := range answer.Symbols {
					if planparser.TargetHolds(m.in.Plan, target, symbol.ID, answer.File) {
						m.add(card, target, location)
						held = true
					}
				}
			}
		}
		return held, nil
	case quarry.StatusNotFound:
		return m.matchFile(answer.File, location)
	}
	return false, nil
}

// matchFile matches a location inside no member of file to every card with a target that holds the file or its self glyph, or a member target that sits in file at the head or sat there at its batch's start.
func (m *matcher) matchFile(file, location string) (bool, error) {
	held := false
	for _, card := range m.in.Plan.Cards {
		for _, target := range card.Targets {
			if planparser.TargetHolds(m.in.Plan, target, "", file) {
				m.add(card, target, location)
				held = true
				continue
			}
			member, ok := planparser.RefMember(m.in.Plan, target)
			if !ok {
				continue
			}
			inFile, err := m.memberInFile(card, member, file)
			if err != nil {
				return false, err
			}
			if inFile {
				m.add(card, target, location)
				held = true
			}
		}
	}
	return held, nil
}

// memberInFile reports whether member resolves into file at the head, or, when it resolves nowhere there, sat in file at the start of card's batch.
func (m *matcher) memberInFile(card planparser.Card, member, file string) (bool, error) {
	if m.resolved == nil {
		if err := m.resolveMembers(); err != nil {
			return false, err
		}
	}
	if files := m.resolved[member]; len(files) > 0 {
		return slices.Contains(files, file), nil
	}
	number, ok := m.batchNumber(card)
	if !ok {
		return false, nil
	}
	start := m.in.State.Batches[number].StartSHA
	if start == "" {
		return false, nil
	}
	before, err := m.deltaFiles(start)
	if err != nil {
		return false, err
	}
	return slices.Contains(before[member], file), nil
}

// resolveMembers resolves every member glyph a live card targets against the verified tree in one call.
func (m *matcher) resolveMembers() error {
	var members []string
	for _, card := range m.in.Plan.Cards {
		for _, target := range card.Targets {
			if member, ok := planparser.RefMember(m.in.Plan, target); ok && !slices.Contains(members, member) {
				members = append(members, member)
			}
		}
	}
	m.resolved = map[string][]string{}
	if len(members) == 0 {
		return nil
	}
	results, err := m.in.Resolve(members)
	if err != nil {
		return fmt.Errorf("failuretrace: resolve %d member target(s): %w", len(members), err)
	}
	if len(results) != len(members) {
		return fmt.Errorf("failuretrace: resolve answered %d of %d member target(s)", len(results), len(members))
	}
	for i, result := range results {
		for _, symbol := range append(slices.Clone(result.Symbols), result.Candidates...) {
			m.resolved[members[i]] = append(m.resolved[members[i]], symbol.File)
		}
	}
	return nil
}

// deltaFiles returns the files each deleted, renamed or modified symbol of the delta from start to the head sat in before it, keyed by glyph.
func (m *matcher) deltaFiles(start string) (map[string][]string, error) {
	if files, ok := m.deltas[start]; ok {
		return files, nil
	}
	delta, err := m.in.Delta(start, m.in.Head)
	if err != nil {
		return nil, fmt.Errorf("failuretrace: delta %s..%s: %w", start, m.in.Head, err)
	}
	files := map[string][]string{}
	for _, symbol := range delta.Deleted {
		files[symbol.ID] = append(files[symbol.ID], symbol.File)
	}
	for _, pair := range delta.Renamed {
		files[pair.From.ID] = append(files[pair.From.ID], pair.From.File)
	}
	for _, symbol := range delta.Modified {
		for _, before := range symbol.Before {
			files[symbol.ID] = append(files[symbol.ID], before.File)
		}
	}
	m.deltas[start] = files
	return files, nil
}

// add records that target of card holds location.
func (m *matcher) add(card planparser.Card, target, location string) {
	match := m.matched[card.Number]
	if match == nil {
		match = &cardMatch{card: card}
		m.matched[card.Number] = match
	}
	if glyph, ok := m.targetGlyph(target); ok && !slices.Contains(match.glyphs, glyph) {
		match.glyphs = append(match.glyphs, glyph)
	}
	if !slices.Contains(match.locations, location) {
		match.locations = append(match.locations, location)
	}
}

// targetGlyph returns the glyph a card target names, and false for a target that is a path.
func (m *matcher) targetGlyph(target string) (string, bool) {
	body := target
	if stripped, handle := planparser.HandleBody(target); handle {
		body = stripped
	}
	parsed, err := planparser.ParseRef(m.in.Plan, body)
	if err != nil {
		return "", false
	}
	return parsed.String(), true
}

// batchNumber returns the Webster batch that ran card, read by the card id `%02d-%s` begin-batch records.
// A batch record without Cards reads as the single card of its own number and slug.
func (m *matcher) batchNumber(card planparser.Card) (int, bool) {
	if m.batchOf == nil {
		m.batchOf = map[string]int{}
		for number, batch := range m.in.State.Batches {
			cards := batch.Cards
			if len(cards) == 0 {
				cards = []string{fmt.Sprintf("%02d-%s", number, batch.Slug)}
			}
			for _, id := range cards {
				m.batchOf[id] = number
			}
		}
	}
	number, ok := m.batchOf[fmt.Sprintf("%02d-%s", card.Number, card.Slug)]
	return number, ok
}

// result assembles the trace from the matched cards.
func (m *matcher) result(untraced []string) verifytree.FailureTrace {
	numbers := make([]int, 0, len(m.matched))
	for number := range m.matched {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)

	trace := verifytree.FailureTrace{Untraced: untraced}
	laterBatch := 0
	for _, number := range numbers {
		match := m.matched[number]
		traced := verifytree.TracedCard{Number: number, Glyphs: match.glyphs, Locations: match.locations}
		if batch, ok := m.batchNumber(match.card); ok {
			record := m.in.State.Batches[batch]
			traced.Batch = batch
			if last, hasCommit := lastCardSHA(record); hasCommit {
				traced.Range = commitRange(record.StartSHA, last)
				if batch > laterBatch {
					laterBatch = batch
					trace.LaterRange = commitRange(last, m.in.Head)
				}
			}
		}
		trace.Cards = append(trace.Cards, traced)
	}
	return trace
}

// lastCardSHA returns the last commit of batch's card trail, and false when the trail is empty.
func lastCardSHA(batch *websterengine.BatchState) (string, bool) {
	if len(batch.CardSHAs) == 0 {
		return "", false
	}
	return batch.CardSHAs[len(batch.CardSHAs)-1], true
}

// commitRange returns the range from..to, or the empty range when either end is empty or both are the same commit.
func commitRange(from, to string) verifytree.CommitRange {
	if from == "" || to == "" || from == to {
		return verifytree.CommitRange{}
	}
	return verifytree.CommitRange{From: from, To: to}
}

// NewTracer returns the seam a landing producer is told, tracing a failed verify log at head over the worktree at worktreeRoot.
// Each call parses the plan under anchorPath, loads Webster's state from the anchor's directories and reads the module path from the worktree's go.mod.
// The worktree is the clean tree the verify ran on at head, so a path exists at head when it exists there.
// A missing plan or state is an error.
func NewTracer(worktreeRoot, anchorPath string) func(log, head string) (verifytree.FailureTrace, error) {
	return func(log, head string) (verifytree.FailureTrace, error) {
		plan, err := planparser.ParsePlan(planparser.PlanDir(anchorPath))
		if err != nil {
			return verifytree.FailureTrace{}, fmt.Errorf("failuretrace: read the plan: %w", err)
		}
		state, err := websterengine.LoadState(websterengine.Dir(anchorPath), websterengine.ScratchDir(anchorPath))
		if err != nil {
			return verifytree.FailureTrace{}, fmt.Errorf("failuretrace: load webster state: %w", err)
		}
		goMod, _ := os.ReadFile(filepath.Join(worktreeRoot, "go.mod"))
		return Trace(Input{
			Log:  log,
			Head: head,
			Placement: Placement{
				WorktreeRoot: worktreeRoot,
				ModulePath:   modfile.ModulePath(goMod),
				ExistsAtHead: func(rel string) bool {
					_, statErr := os.Stat(filepath.Join(worktreeRoot, filepath.FromSlash(rel)))
					return statErr == nil
				},
			},
			Plan:  plan,
			State: state,
			Enclose: func(rev string, targets []string) ([]quarry.EncloseResult, error) {
				return planglyph.EncloseAt(worktreeRoot, rev, targets)
			},
			Resolve: func(targets []string) ([]quarry.ResolveResult, error) {
				return planglyph.Resolve(worktreeRoot, targets)
			},
			Delta: func(from, to string) (quarry.GitDeltaAnswer, error) {
				return planglyph.Delta(worktreeRoot, from, to)
			},
		})
	}
}
