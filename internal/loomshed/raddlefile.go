// raddlefile.go implements the Raddle-File producer, the row that runs in Go without an agent.
// It checks an approved raddle proposal once more and stages its store files and part records.
// It files the entries on the board and ends the run's entry.

package loomshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/raddleparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// raddleMaxReturns is how many times Raddle-File sends a proposal back to Raddle-Write;
// the failure after that halts the run for its parent.
const raddleMaxReturns = 2

// raddleWriteRow is the row a return sends the run to, and the row the halt's goto names.
const raddleWriteRow = "Raddle-Write"

// The files of the stage directory hold the store files the board write keeps and the entry's created stamp.
// The hash file is written last and holds the hash of the proposal the stage was made from.
const (
	stageProposalFile = "proposal.yaml"
	stageColdFile     = "cold-proposal.yaml"
	stageRecordFile   = "decision-record.md"
	stageCreatedFile  = "created"
	stageHashFile     = "hash"
)

// recordHashSuffix names the file beside a part record that holds the hash of the proposal the record was written from.
const recordHashSuffix = ".hash"

// RaddleRunEntry is the raddle run's own board entry as Raddle-File reads it.
// It holds the slug, the created stamp, the labels that are not type labels, and the project field.
type RaddleRunEntry struct {
	Slug         string
	Created      time.Time
	ModuleLabels []string
	// Project is the project the Sketch belongs to, the level its entries are filed at; empty for the top level.
	Project string
}

// RaddleFileDeps are the paths and seams the Raddle-File producer is built with.
type RaddleFileDeps struct {
	// ProposalPath, ColdPath and RecordPath are the proposal, the cold proposal and the Sketch's decision record.
	ProposalPath string
	ColdPath     string
	RecordPath   string
	// StageDir holds the staged store files, and RecordsDir the part records PartRecordPath names.
	StageDir   string
	RecordsDir string
	// PartRecordPath returns the path of the decision record of the entry slug.
	PartRecordPath func(slug string) string
	// StatePath and StateLockPath are the run's state record and its lock.
	StatePath     string
	StateLockPath string

	// Check runs the proposal's checks at filing.
	Check func() ([]raddleparser.Finding, error)
	// Recheck runs the board re-check.
	Recheck func() (raddleparser.Recheck, error)
	// Entry reads the run's own board entry.
	Entry func() (RaddleRunEntry, error)
	// StoreDone reports whether the store holds the filing: the Sketch's directory with that created stamp and hash.
	StoreDone func(created time.Time, hash string) (bool, error)
	// AlreadyFiled reports whether the board holds every slug as filed from the Sketch, a state a failed filing leaves.
	AlreadyFiled func(sketch string, slugs []string) (bool, error)
	// File writes the filing on the board in one write.
	File func(boardengine.RaddleFiling) error
	// CreateBeam makes the filed project a beam; it is passed the project's slug.
	CreateBeam func(project string) error
	// Release releases the filed tasks that are still draft.
	Release func(slugs []string) error
	// Notify tells the run's parent what was filed.
	Notify func(line string) error
	// EndEntry ends the run's own board entry.
	EndEntry func() error
	// Commit commits the raddle directory.
	Commit func() error
}

// raddleFile is the Raddle-File producer.
type raddleFile struct {
	name string
	deps RaddleFileDeps
}

var _ shedengine.ShedProducer = (*raddleFile)(nil)

// NewRaddleFile returns the producer of the Raddle-File row, identified as name.
func NewRaddleFile(name string, deps RaddleFileDeps) shedengine.ShedProducer {
	return &raddleFile{name: name, deps: deps}
}

// Call implements shedengine.ShedProducer.
// A store that already holds the filing skips the checks and steps 1 to 3.
// A board write can land while the store write does not, leaving the board with every filed entry.
// In that state, with the stage still holding this proposal's hash, the checks and the approval match are skipped:
// the filed slugs would count as taken.
// Only the entries the re-check names as landed are read, and steps 1 to 3 run so the store is written.
// Otherwise Call runs the checks, the approval match in interactive mode and the board re-check.
// A failure of one sends the proposal back, deleting the stage and the part records.
// Step 1 stages the store files, step 2 writes the part records and commits, and step 3 files the entries.
// A step whose outputs carry the hash of the proposal now approved is skipped,
// so a retry after a crash finishes the steps it missed.
// Steps 4 and 5 make the project a beam and release its parts, both for the beam shape;
// step 6 notifies the parent once, and step 7 ends the run's entry, leaving the row Done.
// A failure of steps 4 to 7 is Stuck with Halt naming the step and the resume that re-runs the row.
func (p *raddleFile) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	data, err := os.ReadFile(p.deps.ProposalPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, p.fail("read the proposal", err)
	}
	hash := raddleparser.Hash(data)
	current, err := ReadRaddleState(p.deps.StatePath, p.deps.StateLockPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, p.fail("read the run's state", err)
	}
	entry, err := p.deps.Entry()
	if err != nil {
		return "", shedengine.OutputPointer{}, p.fail("read the run's board entry", err)
	}
	filed, err := p.deps.StoreDone(entry.Created, hash)
	if err != nil {
		return "", shedengine.OutputPointer{}, p.fail("read the raddle store", err)
	}

	proposal, parseErr := raddleparser.Parse(data)
	resumed := false
	if !filed && parseErr == nil && p.stagedHashIs(hash) {
		resumed, err = p.deps.AlreadyFiled(proposal.Sketch, raddleFiledSlugs(proposal))
		if err != nil {
			return "", shedengine.OutputPointer{}, p.fail("read the board's filed entries", err)
		}
	}

	var landed []string
	if resumed {
		// The filed entries hold the slugs,
		// so the re-check's failures are ignored and only the entries it names as landed are read.
		recheck, err := p.deps.Recheck()
		if err != nil {
			return "", shedengine.OutputPointer{}, p.fail("re-check the board", err)
		}
		landed = recheck.Landed
	} else if !filed {
		reason, entries, err := p.verify(current, hash)
		if err != nil {
			return "", shedengine.OutputPointer{}, p.fail("check the proposal", err)
		}
		if reason != "" {
			return p.sendBack(ctx, current, reason)
		}
		landed = entries
	}

	if parseErr != nil {
		return "", shedengine.OutputPointer{}, p.fail("parse the proposal", parseErr)
	}
	if !filed {
		reason, err := p.stageAndFile(proposal, data, hash, entry)
		if err != nil {
			return "", shedengine.OutputPointer{}, p.fail("file the proposal", err)
		}
		if reason != "" {
			return p.sendBack(ctx, current, reason)
		}
	}
	return p.finish(ctx, current, proposal, landed)
}

// verify returns the reason the proposal is sent back, empty when it passes every check.
// It also returns the board_checked entries that landed meanwhile.
// It runs the proposal's checks.
// In interactive mode it matches the proposal's hash against the recorded approval.
// Then it runs the board re-check.
func (p *raddleFile) verify(current RaddleState, hash string) (reason string, landed []string, err error) {
	findings, err := p.deps.Check()
	if err != nil {
		return "", nil, err
	}
	if len(findings) > 0 {
		return "the proposal fails its checks: " + FormatRaddleFindings(findings), nil, nil
	}

	if current.Mode == RaddleModeInteractive {
		switch {
		case current.Approval == nil:
			return "the proposal has no recorded approval, so it is not filed until the operator approves it", nil, nil
		case current.Approval.Hash != hash:
			return fmt.Sprintf("the proposal was edited after it was approved: its hash is %s and the approved hash is %s", hash, current.Approval.Hash), nil, nil
		}
	}

	recheck, err := p.deps.Recheck()
	if err != nil {
		return "", nil, err
	}
	if len(recheck.Failures) > 0 {
		return "the board moved since the proposal was approved: " + FormatRaddleFindings(recheck.Failures), nil, nil
	}
	return "", recheck.Landed, nil
}

// stageAndFile runs steps 1 to 3.
// It stages the store files, writes the part records and commits them, and files the entries.
// It returns the reason the proposal is sent back when the board write's own re-check refuses.
// Any other failure is returned as an error.
func (p *raddleFile) stageAndFile(proposal raddleparser.Proposal, data []byte, hash string, entry RaddleRunEntry) (reason string, err error) {
	record, err := os.ReadFile(p.deps.RecordPath)
	if err != nil {
		return "", err
	}
	if err := p.stage(data, record, hash, entry); err != nil {
		return "", fmt.Errorf("stage the store files: %w", err)
	}
	if err := p.writeRecords(proposal, record, hash); err != nil {
		return "", err
	}
	if err := p.deps.Commit(); err != nil {
		return "", fmt.Errorf("commit the raddle directory: %w", err)
	}

	filing, err := p.filing(proposal, hash, entry)
	if err != nil {
		return "", err
	}
	if err := p.deps.File(filing); err != nil {
		if errors.Is(err, boardengine.ErrRaddleRecheck) {
			return "the board moved while the proposal was being filed: " + err.Error(), nil
		}
		return "", err
	}
	return "", nil
}

// stagedHashIs reports whether the stage directory holds the hash file of the proposal with that hash.
// Step 1 writes the hash file last.
func (p *raddleFile) stagedHashIs(hash string) bool {
	staged, err := os.ReadFile(filepath.Join(p.deps.StageDir, stageHashFile))
	return err == nil && string(staged) == hash
}

// stage writes the store files into the stage directory, the hash last.
// It does nothing when the stage already holds this proposal's hash.
func (p *raddleFile) stage(data, record []byte, hash string, entry RaddleRunEntry) error {
	if p.stagedHashIs(hash) {
		return nil
	}
	cold, err := os.ReadFile(p.deps.ColdPath)
	if err != nil {
		return fmt.Errorf("read the cold proposal: %w", err)
	}
	files := []struct {
		name    string
		content []byte
	}{
		{stageProposalFile, data},
		{stageColdFile, cold},
		{stageRecordFile, record},
		{stageCreatedFile, []byte(entry.Created.UTC().Format(time.RFC3339Nano))},
		{stageHashFile, []byte(hash)},
	}
	for _, file := range files {
		if err := fsx.AtomicWriteBytes(filepath.Join(p.deps.StageDir, file.name), file.content); err != nil {
			return err
		}
	}
	return nil
}

// writeRecords writes the decision record of the project and of every part.
// The project's record and, for a tapestry shape, its one part's record are the Sketch's record whole;
// every other part gets a record cut by SplicePart.
// Every record declares a mapped PATTERN change under its live entry, so the Sketch's id never reaches the plan gate;
// the store keeps the Sketch's own record.
// A record whose hash file already holds the proposal's hash is left as it is.
// A SplicePart error is returned naming the part.
func (p *raddleFile) writeRecords(proposal raddleparser.Proposal, record []byte, hash string) error {
	record, err := discussionparser.MapPatternChanges(record, raddlePatternMappings(proposal))
	if err != nil {
		return fmt.Errorf("map the PATTERN changes onto their live entries: %w", err)
	}
	if proposal.Project != nil {
		if err := p.writeRecord(proposal.Project.Slug, record, hash); err != nil {
			return err
		}
	}
	for _, part := range proposal.Parts {
		content := record
		if proposal.Shape != raddleparser.ShapeTapestry {
			spliced, err := discussionparser.SplicePart(record, discussionparser.PartSplice{
				Goal:           raddlePartGoal(part),
				Scope:          raddlePartScope(part),
				Decisions:      part.Decisions,
				OpenRisks:      part.OpenRisks,
				Criteria:       part.Criteria,
				PatternChanges: part.PatternEntryIDs(),
			})
			if err != nil {
				return fmt.Errorf("cut the decision record of the part %s: %w", part.Slug, err)
			}
			content = spliced
		}
		if err := p.writeRecord(part.Slug, content, hash); err != nil {
			return err
		}
	}
	return nil
}

// raddlePatternMappings returns every part's pattern_clusters mapping, in the proposal's order.
func raddlePatternMappings(proposal raddleparser.Proposal) []discussionparser.PatternClusterMapping {
	var mappings []discussionparser.PatternClusterMapping
	for _, part := range proposal.Parts {
		for _, cluster := range part.PatternClusters {
			mappings = append(mappings, discussionparser.PatternClusterMapping{SketchID: cluster.SketchID, Entry: cluster.Entry, Text: cluster.Text})
		}
	}
	return mappings
}

// writeRecord writes the record of the entry slug and the hash file beside it, with the hash last.
// It skips both when the hash file already holds hash and the record exists.
func (p *raddleFile) writeRecord(slug string, content []byte, hash string) error {
	path := p.deps.PartRecordPath(slug)
	_, statErr := os.Stat(path)
	if held, err := os.ReadFile(path + recordHashSuffix); err == nil && string(held) == hash && statErr == nil {
		return nil
	}
	if err := fsx.AtomicWriteBytes(path, content); err != nil {
		return fmt.Errorf("write the decision record of %s: %w", slug, err)
	}
	return fsx.AtomicWriteBytes(path+recordHashSuffix, []byte(hash))
}

// raddlePartGoal renders the Goal section of a part's record from its title and contract.
func raddlePartGoal(part raddleparser.Part) string {
	return fmt.Sprintf("%s\n\nTakes: %s\nDelivers: %s\nExports: %s", part.Title, part.Contract.Takes, part.Contract.Delivers, part.Contract.Exports)
}

// raddlePartScope renders the Scope section of a part's record from its packages.
func raddlePartScope(part raddleparser.Part) string {
	if len(part.Packages) == 0 {
		return "The part names no package."
	}
	lines := make([]string, len(part.Packages))
	for i, pkg := range part.Packages {
		lines[i] = fmt.Sprintf("- %s (about %d changed lines)", pkg.Path, pkg.Size)
	}
	return "The part changes these packages:\n\n" + strings.Join(lines, "\n")
}

// raddlePartBody renders the board body of a filed part:
// the Sketch it comes from, its contract and its packages with their sizes.
func raddlePartBody(sketch string, part raddleparser.Part) string {
	return fmt.Sprintf("%s.\n\nTakes: %s\nDelivers: %s\nExports: %s\n\n%s\n", boardengine.RaddlePartBodyPrefix(sketch), part.Contract.Takes, part.Contract.Delivers, part.Contract.Exports, raddlePartScope(part))
}

// filing builds the board filing from the proposal, the part records on disk and the staged store files.
// The parts are in dependency order, the project is the project shapes' own entry,
// and the entries are filed at the run's own project level.
// Every entry carries its type label and the Sketch's module labels, never the sketch label.
func (p *raddleFile) filing(proposal raddleparser.Proposal, hash string, entry RaddleRunEntry) (boardengine.RaddleFiling, error) {
	staged := make(map[string][]byte, 3)
	for _, name := range []string{stageProposalFile, stageColdFile, stageRecordFile} {
		content, err := os.ReadFile(filepath.Join(p.deps.StageDir, name))
		if err != nil {
			return boardengine.RaddleFiling{}, fmt.Errorf("read the staged %s: %w", name, err)
		}
		staged[name] = content
	}

	labels := func(entryType string) []string {
		return append([]string{entryType}, entry.ModuleLabels...)
	}
	filing := boardengine.RaddleFiling{
		Sketch:       proposal.Sketch,
		Created:      entry.Created,
		Project:      entry.Project,
		Hash:         hash,
		RunEntry:     entry.Slug,
		Release:      proposal.Shape != raddleparser.ShapeBeam,
		Proposal:     staged[stageProposalFile],
		ColdProposal: staged[stageColdFile],
		SketchRecord: staged[stageRecordFile],
		Check: func(tasks []boardengine.Task, store boardengine.RaddleStore) error {
			board := raddleparser.BoardContext{
				Sketch:       entry.Slug,
				Entry:        entry.Slug,
				ModuleLabels: entry.ModuleLabels,
				Entries:      raddleBoardEntries(tasks),
				Store:        store.Slugs(),
				Filing:       true,
			}
			if failures := raddleparser.RecheckBoard(proposal, board).Failures; len(failures) > 0 {
				return errors.New(FormatRaddleFindings(failures))
			}
			return nil
		},
	}

	var slugs []string
	for _, part := range dependencyOrder(proposal.Parts) {
		record, err := os.ReadFile(p.deps.PartRecordPath(part.Slug))
		if err != nil {
			return boardengine.RaddleFiling{}, fmt.Errorf("read the decision record of %s: %w", part.Slug, err)
		}
		slugs = append(slugs, part.Slug)
		filing.Parts = append(filing.Parts, boardengine.RaddleEntry{
			Slug:      part.Slug,
			Title:     part.Title,
			Kind:      boardengine.KindTask,
			Labels:    labels(part.Type),
			Recipe:    part.Recipe,
			Profile:   part.Profile,
			DependsOn: part.DependsOn,
			Body:      raddlePartBody(proposal.Sketch, part),
			Record:    record,
		})
	}
	if proposal.Project != nil {
		record, err := os.ReadFile(p.deps.PartRecordPath(proposal.Project.Slug))
		if err != nil {
			return boardengine.RaddleFiling{}, fmt.Errorf("read the decision record of %s: %w", proposal.Project.Slug, err)
		}
		filing.ProjectEntry = &boardengine.RaddleEntry{
			Slug:   proposal.Project.Slug,
			Title:  proposal.Project.Title,
			Kind:   boardengine.KindProject,
			Labels: labels(proposal.Project.Type),
			Body:   fmt.Sprintf("%s: %s.\n", boardengine.RaddleProjectBodyPrefix(proposal.Sketch), strings.Join(slugs, ", ")),
			Record: record,
		}
	}
	return filing, nil
}

// raddleBoardEntries maps the board's entries onto the entries the proposal's checks read.
func raddleBoardEntries(tasks []boardengine.Task) []raddleparser.BoardEntry {
	entries := make([]raddleparser.BoardEntry, len(tasks))
	for i, task := range tasks {
		entries[i] = raddleparser.BoardEntry{Slug: task.Slug, Kind: task.Kind, Labels: task.Labels, Open: !boardengine.IsDoneStatus(task.Status)}
		if !task.Revised.IsZero() {
			entries[i].Revised = task.Revised.UTC().Format(time.RFC3339)
		}
	}
	return entries
}

// dependencyOrder returns parts so each follows the parts it depends on, keeping the proposal's order otherwise.
// A part left over by a cycle, which the proposal's checks refuse, is appended last.
func dependencyOrder(parts []raddleparser.Part) []raddleparser.Part {
	ordered := make([]raddleparser.Part, 0, len(parts))
	placed := map[string]bool{}
	isPart := func(slug string) bool {
		return slices.ContainsFunc(parts, func(part raddleparser.Part) bool { return part.Slug == slug })
	}
	for len(ordered) < len(parts) {
		progressed := false
		for _, part := range parts {
			if placed[part.Slug] {
				continue
			}
			ready := !slices.ContainsFunc(part.DependsOn, func(dep string) bool { return isPart(dep) && !placed[dep] })
			if ready {
				ordered = append(ordered, part)
				placed[part.Slug] = true
				progressed = true
			}
		}
		if !progressed {
			for _, part := range parts {
				if !placed[part.Slug] {
					ordered = append(ordered, part)
					placed[part.Slug] = true
				}
			}
		}
	}
	return ordered
}

// finish runs steps 4 to 7 over the filed entries.
// Each is skipped when its result is there:
// the beam and the release are idempotent calls, the notice is marked in the state,
// and the entry's end is the last write.
func (p *raddleFile) finish(ctx context.Context, current RaddleState, proposal raddleparser.Proposal, landed []string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if proposal.Shape == raddleparser.ShapeBeam {
		if err := p.deps.CreateBeam(proposal.Project.Slug); err != nil {
			return p.halt(ctx, "make the project a beam", err)
		}
		slugs := make([]string, len(proposal.Parts))
		for i, part := range dependencyOrder(proposal.Parts) {
			slugs[i] = part.Slug
		}
		if err := p.deps.Release(slugs); err != nil {
			return p.halt(ctx, "release the filed parts", err)
		}
	}

	if !current.NoticeSent {
		if err := p.deps.Notify(raddleFiledNotice(proposal, landed)); err != nil {
			return p.halt(ctx, "notify the parent", err)
		}
		if _, err := UpdateRaddleState(p.deps.StatePath, p.deps.StateLockPath, func(stored *RaddleState) { stored.NoticeSent = true }); err != nil {
			return "", shedengine.OutputPointer{}, p.fail("record the notice", err)
		}
	}
	if err := p.deps.EndEntry(); err != nil {
		return p.halt(ctx, "end the run's board entry", err)
	}
	if err := cancelErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// raddleFiledSlugs returns the slug of the proposal's project, when it has one:
// then the slug of each part in the proposal's order.
func raddleFiledSlugs(proposal raddleparser.Proposal) []string {
	var slugs []string
	if proposal.Project != nil {
		slugs = append(slugs, proposal.Project.Slug)
	}
	for _, part := range proposal.Parts {
		slugs = append(slugs, part.Slug)
	}
	return slugs
}

// raddleFiledNotice renders the line that tells the parent what the run filed:
// the slugs of the filed entries, then the entries the proposal weighed that landed meanwhile, when any did.
func raddleFiledNotice(proposal raddleparser.Proposal, landed []string) string {
	slugs := raddleFiledSlugs(proposal)
	line := fmt.Sprintf("raddle: filed %d entries from the Sketch %s as the %s shape: %s", len(slugs), proposal.Sketch, proposal.Shape, strings.Join(slugs, ", "))
	if len(landed) > 0 {
		line += "; entries the proposal weighed that landed meanwhile: " + strings.Join(landed, ", ")
	}
	return line
}

// sendBack answers a failure of the proposal.
// With the run's returns spent, it is Stuck with Halt for the parent.
// The reason names the failures, the used returns and the goto that restarts the row.
// Otherwise it deletes the stage and the part records of the failed attempt.
// It raises the return count, records the reason and clears the approval in one state update.
// It is Stuck without Halt, so the row's on_stuck sends it to Raddle-Write.
func (p *raddleFile) sendBack(ctx context.Context, current RaddleState, reason string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := cancelErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if current.Returns >= raddleMaxReturns {
		return shedengine.Stuck, shedengine.OutputPointer{
			Reason: fmt.Sprintf("%s failed the filing again: %s; the run used its %d returns to %s; way forward: settle the cause, then run lym loom goto --to %s, which starts a new session with the approval cleared; the count of returns is not reset, so each further failure halts for the parent again", p.name, reason, raddleMaxReturns, raddleWriteRow, raddleWriteRow),
			Halt:   true,
		}, nil
	}

	for _, dir := range []string{p.deps.StageDir, p.deps.RecordsDir} {
		if err := os.RemoveAll(dir); err != nil {
			return "", shedengine.OutputPointer{}, p.fail("delete the outputs of the failed attempt", err)
		}
	}
	if _, err := UpdateRaddleState(p.deps.StatePath, p.deps.StateLockPath, func(stored *RaddleState) {
		stored.Returns++
		stored.ReturnReason = reason
		stored.Approval = nil
	}); err != nil {
		return "", shedengine.OutputPointer{}, p.fail("record the return", err)
	}
	return shedengine.Stuck, shedengine.OutputPointer{Reason: fmt.Sprintf("%s returned the proposal to %s: %s", p.name, raddleWriteRow, reason)}, nil
}

// halt is the Stuck verdict with Halt for a failed step after the filing.
// Its reason names the step and the resume that re-runs the row from the first step not done.
func (p *raddleFile) halt(ctx context.Context, step string, err error) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cancelled := cancelErr(ctx, p.name); cancelled != nil {
		return "", shedengine.OutputPointer{}, cancelled
	}
	return shedengine.Stuck, shedengine.OutputPointer{
		Reason: fmt.Sprintf("%s: could not %s: %v; way forward: fix what the error names, then run lym loom resume, which re-runs the row from the first step not done", p.name, step, err),
		Halt:   true,
	}, nil
}

// fail returns the error of a failed step, naming the producer and the re-step that retries it.
func (p *raddleFile) fail(step string, err error) error {
	return fmt.Errorf("loomshed: %s: %s: %w; way forward: the fault is transient, re-step the %s row", p.name, step, err, p.name)
}
