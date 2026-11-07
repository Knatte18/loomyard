// fixtures_test.go holds the in-memory fakes and fixture builders that the calibration, count and profile tests share.
// The git-backed tests share them too.
// It carries no build tag, so the tests of both tiers compile it.

package main

import (
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// fakeHistory is an in-memory PlanHistory: commits by subject and file contents by "<sha>:<path>".
type fakeHistory struct {
	commits map[string][]gitrepo.SubjectCommit
	files   map[string]string
}

func (f fakeHistory) CommitsWithSubject(subject string) ([]gitrepo.SubjectCommit, error) {
	return f.commits[subject], nil
}

func (f fakeHistory) FileAtRevision(rev, relPath string) ([]byte, error) {
	data, ok := f.files[rev+":"+relPath]
	if !ok {
		return nil, gitrepo.ErrPathNotAtRevision
	}
	return []byte(data), nil
}

// fakeBase is an in-memory BaseTrees: file contents by "<sha>:<path>" for each known sha.
type fakeBase struct {
	shas  map[string]bool
	files map[string]string
}

func (f fakeBase) SHAExists(sha string) bool { return f.shas[sha] }

func (f fakeBase) FileAtRevision(rev, relPath string) ([]byte, error) {
	data, ok := f.files[rev+":"+relPath]
	if !ok {
		return nil, gitrepo.ErrPathNotAtRevision
	}
	return []byte(data), nil
}

func (f fakeBase) FilesInDirAtRevision(rev, dir string) ([]string, error) {
	var names []string
	for key := range f.files {
		prefix := rev + ":" + dir + "/"
		if rest, ok := strings.CutPrefix(key, prefix); ok && !strings.Contains(rest, "/") {
			names = append(names, rest)
		}
	}
	return names, nil
}

// planWith renders a plan whose cards each edit one file, keyed by card number.
func planWith(targets map[int]string) map[string]string {
	p := plankit.Plan{Approved: true, Language: "go"}
	for number := 1; number <= len(targets); number++ {
		p.Cards = append(p.Cards, plankit.Card{
			Number: number,
			Slug:   fmt.Sprintf("c%d", number),
			Groups: []plankit.Group{{Label: "Edit", Targets: []string{targets[number]}}},
		})
	}
	files := map[string]string{}
	for name, data := range plankit.Render(p) {
		files[name] = string(data)
	}
	return files
}

// lines returns content of n lines, each the text x.
func lines(n int) string { return strings.Repeat("x\n", n) }

// The base files of a based run hold these many lines each.
const (
	instructionLines = 3
	patternLines     = 4
	templateLines    = 5
)

// directiveStencil is the orchestrator directive stencil of a based run:
// a banner, then a line before and after the PATTERN overview marker.
const directiveStencil = "<!-- banner -->\n\nhead\n{{.pattern_overview}}\ntail\n"

// renderedDirective is directiveStencil filled with the PATTERN.md of a based run:
// the wrapper's lines around the overview.
func renderedDirective() string { return "head\n" + lines(patternLines) + "\ntail\n" }

// withBaseFiles adds the files a run's Merriam base is computed from to the base tree at sha.
// They are INSTRUCTIONS.md, PATTERN.md, the orchestrator directive stencil and the Master template.
// INSTRUCTIONS.md is the fake engine's first instruction file.
// LOCAL.md, the fake engine's second instruction file, is untracked and so never at a base commit.
func withBaseFiles(files map[string]string, sha string) {
	files[sha+":INSTRUCTIONS.md"] = lines(instructionLines)
	files[sha+":PATTERN.md"] = lines(patternLines)
	files[sha+":"+directiveStencilPath] = directiveStencil
	files[sha+":"+masterTemplatePath] = lines(templateLines)
}

// knownBaseContext is the context the seeded profiles price for a based run whose plan renders overview.
// It counts one context unit per line of the overview, INSTRUCTIONS.md, the rendered directive and the Master template.
// The fixed system-prompt context is added on top.
func knownBaseContext(overview string) float64 {
	base := websterengine.MerriamBaseOf(overview, lines(instructionLines), renderedDirective(), lines(templateLines))
	return float64(base.Lines) + base.Fixed
}

// basedRunFixture returns a plan history and a base tree for run slug, plus the base context they give.
// The plan commit precedes the forks of the tests; the base commit, named "base" + slug, holds the base files.
func basedRunFixture(slug string, planTargets map[int]string, forksStarted time.Time) (fakeHistory, fakeBase, float64) {
	plan := planWith(planTargets)
	history := fakeHistory{
		commits: map[string][]gitrepo.SubjectCommit{planCommitSubjectPrefix + slug: {{SHA: slug + "plan", Committed: forksStarted.Add(-time.Hour)}}},
		files:   map[string]string{},
	}
	for name, data := range plan {
		history.files[slug+"plan:_lym/plan/"+name] = data
	}
	base := fakeBase{shas: map[string]bool{"base" + slug: true}, files: map[string]string{}}
	withBaseFiles(base.files, "base"+slug)
	return history, base, knownBaseContext(plan["00-overview.md"])
}

// seedProfile writes a batcher.yaml with a "fit" profile and a "grow" profile.
// In "fit", one card's peak is its orientation plus the lines of its file, and every other coefficient is 0.
// "grow" is "fit" with a batch_growth of 5.
func seedProfile(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(configengine.ConfigDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	const config = `profiles:
  fit:
    weights:
      orientation: 10
      batch_growth: 0
      fork_messages: 0
      message_context: 0
      write_per_card_line: 0
      target_messages: 1
      test_file_messages: 0
      uses_messages: 0
      context_per_line: 1
      package_context: 0
  grow:
    weights:
      orientation: 10
      batch_growth: 5
      fork_messages: 0
      message_context: 0
      write_per_card_line: 0
      target_messages: 1
      test_file_messages: 0
      uses_messages: 0
      context_per_line: 1
      package_context: 0
`
	if err := os.WriteFile(configengine.ConfigFile(dir, "batcher"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fork returns a fork in the session "" that ran cards.
// Its first message held start tokens of context, and its largest held peak.
func fork(started time.Time, start, peak int, cards ...string) ForkTally {
	return forkIn("", started, start, peak, cards...)
}

// forkIn returns a fork in the webster session transcript named session, with the given start and peak tokens.
func forkIn(session string, started time.Time, start, peak int, cards ...string) ForkTally {
	return ForkTally{Cards: cards, Started: started, Session: session, Usage: Usage{Input: peak}, StartContext: start, PeakContext: peak}
}

// fakeEngine is the name the fake engine is registered under in a read set.
const fakeEngine = "fake"

// fakeRates are the fake engine's USD per million tokens of every kind, by model.
var fakeRates = fakePricer{"model-a": 20, "model-b": 10}

// fakeTranscripts is an in-memory transcript reader.
// It keys located sessions by worktree and session reads by session Ref.
// A locateErr fails every locate, and a readErrs entry fails the read of its Ref.
type fakeTranscripts struct {
	located   map[string][]shuttleengine.LocatedSession
	reads     map[string]shuttleengine.SessionRead
	locateErr error
	readErrs  map[string]error
}

func newFakeTranscripts() fakeTranscripts {
	return fakeTranscripts{
		located:  map[string][]shuttleengine.LocatedSession{},
		reads:    map[string]shuttleengine.SessionRead{},
		readErrs: map[string]error{},
	}
}

// add locates read under worktree with the given modification time.
// Its Ref is the worktree joined with the main read's ID.
func (f fakeTranscripts) add(worktree string, modified time.Time, read shuttleengine.SessionRead) {
	located := shuttleengine.LocatedSession{
		TranscriptRef: shuttleengine.TranscriptRef{ID: read.Main.ID, Ref: worktree + "/" + read.Main.ID},
		Modified:      modified,
	}
	for _, fork := range read.Forks {
		located.Forks = append(located.Forks, shuttleengine.TranscriptRef{ID: fork.ID, Ref: located.Ref + "/" + fork.ID})
	}
	f.located[worktree] = append(f.located[worktree], located)
	f.reads[located.Ref] = read
}

func (f fakeTranscripts) LocateSessions(worktree string) ([]shuttleengine.LocatedSession, error) {
	if f.locateErr != nil {
		return nil, f.locateErr
	}
	return slices.Clone(f.located[worktree]), nil
}

func (f fakeTranscripts) ReadSession(s shuttleengine.LocatedSession) (shuttleengine.SessionRead, error) {
	if err := f.readErrs[s.Ref]; err != nil {
		return shuttleengine.SessionRead{}, err
	}
	read, ok := f.reads[s.Ref]
	if !ok {
		return shuttleengine.SessionRead{}, fs.ErrNotExist
	}
	return read, nil
}

// fakePricer prices a message at its model's USD per million tokens of every kind, and has no price for another model.
type fakePricer map[string]float64

func (p fakePricer) PriceMessage(model string, u shuttleengine.MessageUsage) (float64, bool) {
	rate, ok := p[model]
	if !ok {
		return 0, false
	}
	return rate * float64(u.Input+u.CacheWrite+u.CacheRead+u.Output) / 1e6, true
}

// fakeInstructions is an in-memory instruction file lister.
type fakeInstructions []string

func (f fakeInstructions) InstructionFiles() []string { return f }

// fakeInstructionFiles are the instruction file names the fake engine lists, the second of which no base commit holds.
var fakeInstructionFiles = fakeInstructions{"INSTRUCTIONS.md", "LOCAL.md"}

// fakeReadSet returns a read set of one engine named "fake", reading through the given transcripts.
// The engine prices at the fake rates and lists the fake instruction files.
func fakeReadSet(fake fakeTranscripts) ReadSet {
	return ReadSet{fakeEngine: {Transcripts: fake, Pricer: fakeRates, Instructions: fakeInstructionFiles}}
}

// oneEngine is a one-engine read set whose engine located every given session under worktree.
func oneEngine(worktree string, sessions ...shuttleengine.SessionRead) ReadSet {
	fake := newFakeTranscripts()
	for _, session := range sessions {
		fake.add(worktree, time.Time{}, session)
	}
	return fakeReadSet(fake)
}

// stampOf parses an RFC 3339 test timestamp.
func stampOf(ts string) time.Time {
	at, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		panic(err)
	}
	return at
}

// readOf is a transcript read named name whose Started is its first timestamped entry.
func readOf(id, name string, entries ...shuttleengine.TranscriptEntry) shuttleengine.TranscriptRead {
	read := shuttleengine.TranscriptRead{ID: id, Name: name, Entries: entries}
	for _, e := range entries {
		if !e.At.IsZero() {
			read.Started = e.At
			break
		}
	}
	return read
}

// sessionOf is a session read of a main transcript and its forks.
func sessionOf(main shuttleengine.TranscriptRead, forks ...shuttleengine.TranscriptRead) shuttleengine.SessionRead {
	return shuttleengine.SessionRead{Main: main, Forks: forks}
}

// assistant is an assistant entry of message id with one input token, out output tokens and read cache-read tokens.
func assistant(id, model string, out, read int) shuttleengine.TranscriptEntry {
	return shuttleengine.TranscriptEntry{
		Role: shuttleengine.EntryAssistant, MessageKey: id, Model: model,
		Usage: &shuttleengine.MessageUsage{Input: 1, Output: out, CacheRead: read},
	}
}

// toolResult is a user entry at ts holding one tool result with the given text.
func toolResult(ts, text string) shuttleengine.TranscriptEntry {
	return toolResultFor("t", ts, text)
}

// toolResultFor returns a user entry at ts that answers the tool call with the given id.
// The entry holds one tool result with the given text.
func toolResultFor(useID, ts, text string) shuttleengine.TranscriptEntry {
	return shuttleengine.TranscriptEntry{
		At: stampOf(ts), Role: shuttleengine.EntryUser,
		ToolResults: []shuttleengine.ToolResult{{CallID: useID, Text: text}},
	}
}

// bashUse is an assistant entry holding one shell call of command.
func bashUse(id, command string) shuttleengine.TranscriptEntry {
	return shuttleengine.TranscriptEntry{
		Role: shuttleengine.EntryAssistant, MessageKey: "use-" + id,
		ToolCalls: []shuttleengine.ToolCall{{ID: id, Kind: shuttleengine.ToolShell, Command: command}},
	}
}

// stamped is an assistant entry at ts, ending its turn when endsTurn is set.
func stamped(ts string, endsTurn bool) shuttleengine.TranscriptEntry {
	return shuttleengine.TranscriptEntry{
		At: stampOf(ts), Role: shuttleengine.EntryAssistant, MessageKey: ts, EndsTurn: endsTurn,
		Usage: &shuttleengine.MessageUsage{Input: 1, Output: 1},
	}
}

// userAt is a user entry at ts holding text, which starts a turn.
func userAt(ts, text string) shuttleengine.TranscriptEntry {
	return shuttleengine.TranscriptEntry{At: stampOf(ts), Role: shuttleengine.EntryUser, StartsTurn: true, UserTexts: []string{text}}
}

// fakeRecords is an in-memory store of run records.
// It holds the plan history and the files directly in a directory at a revision.
type fakeRecords struct{ fakeHistory }

func (f fakeRecords) FilesInDirAtRevision(rev, dir string) ([]string, error) {
	var names []string
	for key := range f.files {
		if rest, ok := strings.CutPrefix(key, rev+":"+dir+"/"); ok && !strings.Contains(rest, "/") {
			names = append(names, rest)
		}
	}
	return names, nil
}

// reviewFile renders a review file that parses and holds the given number of findings.
func reviewFile(findings int) string {
	if findings == 0 {
		return "---\nverdict: APPROVED\nfindings: []\n---\n"
	}
	var b strings.Builder
	b.WriteString("---\nverdict: APPROVED\nfindings:\n")
	for i := 1; i <= findings; i++ {
		fmt.Fprintf(&b, "  - id: f%d\n    severity: MEDIUM\n    class: scope\n    location: internal/a/a.go\n    summary: a finding\n", i)
	}
	b.WriteString("---\n")
	return b.String()
}
