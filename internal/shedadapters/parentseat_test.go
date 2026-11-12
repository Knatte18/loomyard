// parentseat_test.go pins the parent seat of BurlerReviewProducer over shedfake.BurlerHalfRunner.
// It covers which rounds ask the run's parent orch and how an ask that cannot go out ends.
// It also covers the ask's creel line and how an accepted review settles its round's ask.
// Every test runs against a real ask record in a temp directory and fake closures for the seat's collaborators.

package shedadapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	parentSlug       = "disc-slug"
	parentRecordPath = "/worktree/_lym/discussion/decision-record.md"
	// parentWait is the time from an ask to its deadline that the fixture's Wait answers.
	parentWait = 20 * time.Minute
	// resolvedStrand is the strand the fixture's Resolve answers; storedStrand is the one a prior ask already records.
	resolvedStrand = "orch-strand"
	storedStrand   = "stored-strand"
	// parentSeed is the RFC 3339 form of the round-1 focus file's write time the fixture sets.
	parentSeed      = "2026-08-16T14:00:00Z"
	otherParentSeed = "2026-08-16T13:00:00Z"
)

var (
	parentSeedTime = time.Date(2026, 8, 16, 14, 0, 0, 0, time.UTC)
	// priorAskedAt and priorDeadline are the times every prior ask the fixture records carries.
	priorAskedAt  = archiveTestInstant.Add(-time.Hour)
	priorDeadline = archiveTestInstant.Add(30 * time.Minute)
)

// parentPost is one line the fixture's Post received.
type parentPost struct {
	strand string
	line   string
}

// priorAsk is an ask recorded before the call under test.
// An ask in state done is answered with findings, a satisfied report when there are none.
// An empty Seed takes the fixture's seed.
type priorAsk struct {
	ask      burlerengine.ParentAsk
	findings []burlerengine.ParentFinding
}

// parentFixture is a run directory seeded with a round-1 focus file, and a parent seat whose collaborators it scripts.
type parentFixture struct {
	runDir string
	files  burlerengine.ParentReviewFiles
	seat   *ParentReviewSeat

	keyOn      bool
	skipReason string
	writtenAt  time.Time
	writtenErr error
	unrecorded bool
	resolveErr error
	postErr    error

	posts        []parentPost
	resolveCalls int
}

// newParentFixture builds a fixture whose seed is parentSeed and whose key is off.
func newParentFixture(t *testing.T) *parentFixture {
	t.Helper()
	f := &parentFixture{runDir: t.TempDir(), files: burlerengine.ParentReviewFilesOf(t.TempDir())}
	focus := writeFocusFile(t, f.runDir, 1, focusFile{Round: 1, Focus: []string{"look at the seam"}})
	if err := os.Chtimes(focus, parentSeedTime, parentSeedTime); err != nil {
		t.Fatalf("Chtimes(%s): %v", focus, err)
	}
	f.seat = &ParentReviewSeat{
		Files:           f.files,
		Slug:            parentSlug,
		RecordPath:      parentRecordPath,
		AskEnabled:      func() bool { return f.keyOn },
		Wait:            func() (time.Duration, string) { return parentWait, f.skipReason },
		RecordWrittenAt: func() (time.Time, error) { return f.writtenAt, f.writtenErr },
		Resolve: func() (string, bool, error) {
			f.resolveCalls++
			return resolvedStrand, !f.unrecorded, f.resolveErr
		},
		Post: func(strand, line string) error {
			f.posts = append(f.posts, parentPost{strand: strand, line: line})
			return f.postErr
		},
	}
	return f
}

// producer returns a review producer over the fixture's run directory and seat.
func (f *parentFixture) producer(t *testing.T, runner *shedfake.BurlerHalfRunner) *BurlerReviewProducer {
	t.Helper()
	spec := newHalfSpec()
	spec.parent = f.seat
	return newReviewProducer(t, f.runDir, runner, spec)
}

// judgeRounds writes rounds 1 through n as judged, so the producer's next round is n+1.
func (f *parentFixture) judgeRounds(t *testing.T, n int) {
	t.Helper()
	for round := 1; round <= n; round++ {
		writeJudgedRound(t, f.runDir, round)
	}
}

// recordAsk records prior in the fixture's ask record, answering it when its state is done.
func (f *parentFixture) recordAsk(t *testing.T, prior priorAsk) {
	t.Helper()
	ask := prior.ask
	if ask.Seed == "" {
		ask.Seed = parentSeed
	}
	ask.AskedAt, ask.Deadline = priorAskedAt, priorDeadline
	answered := ask.State == burlerengine.ParentAskDone
	if answered {
		ask.State = burlerengine.ParentAskOpen
	}
	recorded, err := burlerengine.RecordParentAsk(f.files, ask)
	if err != nil {
		t.Fatalf("RecordParentAsk() error = %v; want nil", err)
	}
	if answered {
		if _, err := burlerengine.AnswerParentAsk(f.files, recorded.Number, priorAskedAt.Add(time.Minute), prior.findings); err != nil {
			t.Fatalf("AnswerParentAsk() error = %v; want nil", err)
		}
	}
}

// readAsks returns the fixture's ask record, failing the test when it cannot be read.
func (f *parentFixture) readAsks(t *testing.T) burlerengine.ParentAskRecord {
	t.Helper()
	record, _, err := burlerengine.ReadParentAsks(f.files)
	if err != nil {
		t.Fatalf("ReadParentAsks() error = %v; want nil", err)
	}
	return record
}

// setNanosecondWriteTime sets path's write time to at.
// It skips the test when the file system rounds away the nanoseconds.
func setNanosecondWriteTime(t *testing.T, path string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatalf("Chtimes(%s): %v", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}
	if !info.ModTime().Equal(at) {
		t.Skipf("the file system stores write times as %v; want the exact %v", info.ModTime(), at)
	}
}

// TestBurlerReviewProducer_Call_ParentAskingRule table-drives which rounds send the parent an ask.
// Each row sets the run's prior ask, the key and the history.
// It then reads the post, the recorded ask and the number the round's review table is told.
// The review fails after its table starts, so a posted ask is still open when the record is read.
func TestBurlerReviewProducer_Call_ParentAskingRule(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	findings := []burlerengine.ParentFinding{{Change: "tighten the seam"}}
	tests := []struct {
		name string
		// judged is the number of judged rounds before the call, so the call's round is judged+1.
		judged int
		keyOn  bool
		prior  *priorAsk
		// rewritten sets the history's latest Discussion-Write call after the prior ask.
		rewritten bool
		// writtenErr is the error the history read fails with.
		writtenErr error
		// focusOffset moves the round-1 focus file's write time past the fixture's seed.
		focusOffset time.Duration
		// dropReport removes the prior ask's report file before the call.
		dropReport bool
		// held marks a round whose ask was recorded before the call.
		held       bool
		wantPosts  int
		wantAsk    int
		wantState  burlerengine.ParentAskState
		wantAsksOn int
	}{
		{name: "no ask yet and the key on asks", keyOn: true, wantPosts: 1, wantAsk: 1, wantState: burlerengine.ParentAskOpen, wantAsksOn: 1},
		{name: "no ask yet and the key off asks nothing"},
		{
			name: "a previous round done with findings asks", judged: 1, keyOn: true,
			prior:     &priorAsk{ask: burlerengine.ParentAsk{Round: 1, State: burlerengine.ParentAskDone}, findings: findings},
			wantPosts: 1, wantAsk: 2, wantState: burlerengine.ParentAskOpen, wantAsksOn: 2,
		},
		{
			name: "a previous round done satisfied asks nothing", judged: 1, keyOn: true,
			prior: &priorAsk{ask: burlerengine.ParentAsk{Round: 1, State: burlerengine.ParentAskDone}}, wantAsksOn: 1,
		},
		{
			name: "a previous round that timed out asks nothing", judged: 1, keyOn: true,
			prior: &priorAsk{ask: burlerengine.ParentAsk{Round: 1, State: burlerengine.ParentAskTimeout}}, wantAsksOn: 1,
		},
		{
			name: "a previous round with no parent asks nothing", judged: 1, keyOn: true,
			prior: &priorAsk{ask: burlerengine.ParentAsk{Round: 1, State: burlerengine.ParentAskNoParent}}, wantAsksOn: 1,
		},
		{
			name: "an earlier round not sent asks", judged: 2, keyOn: true,
			prior:     &priorAsk{ask: burlerengine.ParentAsk{Round: 1, State: burlerengine.ParentAskNotSent, Reason: "no room"}},
			wantPosts: 1, wantAsk: 2, wantState: burlerengine.ParentAskOpen, wantAsksOn: 2,
		},
		{
			name: "another seed asks whatever its answer was: satisfied", keyOn: true,
			prior:     &priorAsk{ask: burlerengine.ParentAsk{Seed: otherParentSeed, Round: 1, State: burlerengine.ParentAskDone}},
			wantPosts: 1, wantAsk: 2, wantState: burlerengine.ParentAskOpen, wantAsksOn: 2,
		},
		{
			name: "another seed asks whatever its answer was: timeout", keyOn: true,
			prior:     &priorAsk{ask: burlerengine.ParentAsk{Seed: otherParentSeed, Round: 1, State: burlerengine.ParentAskTimeout}},
			wantPosts: 1, wantAsk: 2, wantState: burlerengine.ParentAskOpen, wantAsksOn: 2,
		},
		{
			name: "a record rewritten after the latest ask asks", judged: 1, keyOn: true, rewritten: true,
			prior:     &priorAsk{ask: burlerengine.ParentAsk{Round: 1, State: burlerengine.ParentAskTimeout}},
			wantPosts: 1, wantAsk: 2, wantState: burlerengine.ParentAskOpen, wantAsksOn: 2,
		},
		{
			name: "a seed one nanosecond past the prior ask's asks whatever its answer was", keyOn: true, focusOffset: time.Nanosecond,
			prior:     &priorAsk{ask: burlerengine.ParentAsk{Round: 1, State: burlerengine.ParentAskDone}},
			wantPosts: 1, wantAsk: 2, wantState: burlerengine.ParentAskOpen, wantAsksOn: 2,
		},
		{
			name: "an unreadable history is judged as a record not rewritten", judged: 1, keyOn: true, writtenErr: errors.New("boom"),
			prior: &priorAsk{ask: burlerengine.ParentAsk{Round: 1, State: burlerengine.ParentAskTimeout}}, wantAsksOn: 1,
		},
		{
			name: "a previous round done whose report is missing asks", judged: 1, keyOn: true, dropReport: true,
			prior:     &priorAsk{ask: burlerengine.ParentAsk{Round: 1, State: burlerengine.ParentAskDone}, findings: findings},
			wantPosts: 1, wantAsk: 2, wantState: burlerengine.ParentAskOpen, wantAsksOn: 2,
		},
		{
			name: "an ask still pending-post is posted again to its stored strand with the key off", held: true,
			prior:     &priorAsk{ask: burlerengine.ParentAsk{Round: 1, State: burlerengine.ParentAskPendingPost, Strand: storedStrand}},
			wantPosts: 1, wantAsk: 1, wantState: burlerengine.ParentAskOpen, wantAsksOn: 1,
		},
		{
			name: "an open ask is neither posted again nor given a new deadline", keyOn: true, held: true,
			prior:   &priorAsk{ask: burlerengine.ParentAsk{Round: 1, State: burlerengine.ParentAskOpen, Strand: storedStrand}},
			wantAsk: 1, wantState: burlerengine.ParentAskOpen, wantAsksOn: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newParentFixture(t)
			f.keyOn = tt.keyOn
			f.judgeRounds(t, tt.judged)
			if tt.prior != nil {
				f.recordAsk(t, *tt.prior)
			}
			if tt.dropReport {
				if err := os.Remove(f.files.ReportPath(1)); err != nil {
					t.Fatalf("remove the prior ask's report: %v", err)
				}
			}
			if tt.focusOffset != 0 {
				setNanosecondWriteTime(t, focusPath(f.runDir, 1), parentSeedTime.Add(tt.focusOffset))
			}
			seed := parentSeedTime.Add(tt.focusOffset).Format(time.RFC3339Nano)
			if tt.rewritten {
				f.writtenAt = priorAskedAt.Add(time.Minute)
			}
			f.writtenErr = tt.writtenErr
			round := tt.judged + 1
			runner := &shedfake.BurlerHalfRunner{ReviewErrs: []error{errors.New("review stopped")}}

			_, _, err := f.producer(t, runner).Call(context.Background())

			if err == nil || !strings.Contains(err.Error(), "review stopped") || runner.ReviewCalls != 1 {
				t.Fatalf("Call() error = %v with %d Review calls; want the review table started and stopped", err, runner.ReviewCalls)
			}
			if got := runner.GotReviewOpts[0].ParentAsk; got != tt.wantAsk {
				t.Errorf("review opts ParentAsk = %d; want %d", got, tt.wantAsk)
			}
			record := f.readAsks(t)
			if len(record.Asks) != tt.wantAsksOn {
				t.Errorf("recorded asks = %d; want %d", len(record.Asks), tt.wantAsksOn)
			}
			ask, found := record.RoundAsk(seed, round)
			if tt.wantAsk == 0 {
				if found {
					t.Errorf("round %d holds ask %+v; want none", round, ask)
				}
				if len(f.posts) != 0 {
					t.Errorf("posts = %+v; want none", f.posts)
				}
				return
			}
			wantStrand, wantAskedAt, wantDeadline, wantResolves := resolvedStrand, archiveTestInstant, archiveTestInstant.Add(parentWait), 1
			if tt.held {
				wantStrand, wantAskedAt, wantDeadline, wantResolves = storedStrand, priorAskedAt, priorDeadline, 0
			}
			if !found || ask.Number != tt.wantAsk || ask.State != tt.wantState || ask.Round != round || ask.Strand != wantStrand {
				t.Errorf("round %d ask = %+v, found %v; want number %d, state %s, strand %q", round, ask, found, tt.wantAsk, tt.wantState, wantStrand)
			}
			if !ask.AskedAt.Equal(wantAskedAt) || !ask.Deadline.Equal(wantDeadline) {
				t.Errorf("ask times = %v, %v; want asked at %v with deadline %v", ask.AskedAt, ask.Deadline, wantAskedAt, wantDeadline)
			}
			if f.resolveCalls != wantResolves {
				t.Errorf("Resolve calls = %d; want %d", f.resolveCalls, wantResolves)
			}
			var wantPosts []parentPost
			if tt.wantPosts > 0 {
				wantPosts = []parentPost{{strand: wantStrand, line: ParentAskLine(parentSlug, parentRecordPath, ask)}}
			}
			if !reflect.DeepEqual(f.posts, wantPosts) {
				t.Errorf("posts = %+v; want %+v", f.posts, wantPosts)
			}
		})
	}
}

// TestBurlerReviewProducer_Call_ParentRecordOrSeedUnreadable table-drives the reads the asking rule cannot make.
// A missing round-1 focus file leaves no seed and a corrupt ask record leaves no history;
// either way the round goes on told no ask, and nothing is resolved or posted.
func TestBurlerReviewProducer_Call_ParentRecordOrSeedUnreadable(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		corrupt func(t *testing.T, f *parentFixture)
	}{
		{"a missing round-1 focus file", func(t *testing.T, f *parentFixture) {
			t.Helper()
			if err := os.Remove(focusPath(f.runDir, 1)); err != nil {
				t.Fatalf("remove the focus file: %v", err)
			}
		}},
		{"a corrupt ask record", func(t *testing.T, f *parentFixture) {
			t.Helper()
			if err := os.MkdirAll(f.files.Dir, 0o755); err != nil {
				t.Fatalf("create the ask record directory: %v", err)
			}
			if err := os.WriteFile(f.files.Record, []byte("{not json"), 0o644); err != nil {
				t.Fatalf("write the corrupt ask record: %v", err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newParentFixture(t)
			f.keyOn = true
			tt.corrupt(t, f)
			runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{reviewDone("d")}}

			if _, _, err := f.producer(t, runner).Call(context.Background()); err != nil {
				t.Fatalf("Call() error = %v; want the round to go on without the parent", err)
			}

			if runner.ReviewCalls != 1 || runner.GotReviewOpts[0].ParentAsk != 0 {
				t.Errorf("Review calls = %d with opts %+v; want one call told no ask", runner.ReviewCalls, runner.GotReviewOpts)
			}
			if f.resolveCalls != 0 || len(f.posts) != 0 {
				t.Errorf("Resolve calls %d, posts %+v; want none", f.resolveCalls, f.posts)
			}
		})
	}
}

// TestRoundSeed pins the seed as the focus file's write time in RFC 3339 at nanosecond precision.
// A run directory with no round-1 focus file has no seed.
func TestRoundSeed(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	runDir := t.TempDir()
	if _, err := roundSeed(runDir); err == nil {
		t.Error("roundSeed(no focus file) error = nil; want an error")
	}
	focus := writeFocusFile(t, runDir, 1, focusFile{Round: 1, Focus: []string{"look at the seam"}})
	setNanosecondWriteTime(t, focus, time.Date(2026, 8, 16, 14, 0, 0, 123456789, time.UTC))

	got, err := roundSeed(runDir)

	if want := "2026-08-16T14:00:00.123456789Z"; err != nil || got != want {
		t.Errorf("roundSeed() = %q, %v; want %q", got, err, want)
	}
}

// TestBurlerReviewProducer_Call_ParentAskThatDoesNotGoOut table-drives the ways a new ask never reaches the parent.
// Each ends in its own state with its reason, and the review round goes on with the ask's number.
func TestBurlerReviewProducer_Call_ParentAskThatDoesNotGoOut(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name        string
		skipReason  string
		unrecorded  bool
		resolveErr  error
		postErr     error
		wantResolve int
		wantPosts   int
		wantState   burlerengine.ParentAskState
		wantReason  string
		wantStrand  string
	}{
		{
			name: "no orch strand recorded", unrecorded: true, wantResolve: 1,
			wantState: burlerengine.ParentAskNoParent, wantReason: "no orch strand is recorded at the run's parent",
		},
		{
			name: "a resolve error", resolveErr: errors.New("boom"), wantResolve: 1,
			wantState: burlerengine.ParentAskNoParent, wantReason: "the run's parent orch could not be resolved: boom",
		},
		{
			name: "a post error", postErr: errors.New("boom"), wantResolve: 1, wantPosts: 1,
			wantState: burlerengine.ParentAskNoParent, wantReason: "the request was not delivered: boom", wantStrand: resolvedStrand,
		},
		{
			name: "a wait the chair's timeout leaves no room for", skipReason: "the chair's timeout leaves no room",
			wantState: burlerengine.ParentAskNotSent, wantReason: "the chair's timeout leaves no room",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newParentFixture(t)
			f.keyOn = true
			f.skipReason, f.unrecorded, f.resolveErr, f.postErr = tt.skipReason, tt.unrecorded, tt.resolveErr, tt.postErr
			runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{reviewDone("d")}}

			if _, _, err := f.producer(t, runner).Call(context.Background()); err != nil {
				t.Fatalf("Call() error = %v; want the round to go on without the parent", err)
			}

			if runner.ReviewCalls != 1 || runner.GotReviewOpts[0].ParentAsk != 1 {
				t.Errorf("Review calls = %d with opts %+v; want one call told ask 1", runner.ReviewCalls, runner.GotReviewOpts)
			}
			ask, found := f.readAsks(t).RoundAsk(parentSeed, 1)
			if !found || ask.Number != 1 || ask.State != tt.wantState || ask.Reason != tt.wantReason || ask.Strand != tt.wantStrand {
				t.Errorf("round 1 ask = %+v, found %v; want number 1, state %s, reason %q, strand %q", ask, found, tt.wantState, tt.wantReason, tt.wantStrand)
			}
			if f.resolveCalls != tt.wantResolve || len(f.posts) != tt.wantPosts {
				t.Errorf("Resolve calls %d, posts %d; want %d and %d", f.resolveCalls, len(f.posts), tt.wantResolve, tt.wantPosts)
			}
		})
	}
}

// TestParentAskLine pins the line's exact text: the tag, the slug, the round, the ask number and the record path.
// It also pins the deadline as a local wall-clock time and the answer verb with its --ask flag in both forms.
func TestParentAskLine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	deadline := time.Date(2026, 8, 16, 15, 33, 26, 0, time.UTC)
	ask := burlerengine.ParentAsk{Number: 3, Round: 2, Deadline: deadline}

	got := ParentAskLine("my-slug", "/wt/decision-record.md", ask)

	answer := "lym loom parent-review answer my-slug --ask 3"
	want := fmt.Sprintf(`[discussion-review request] my-slug round 2 ask 3: review the decision record /wt/decision-record.md and answer by %s with "%s --satisfied" or "%s --findings-file <path>"`,
		deadline.Local().Format("2006-01-02 15:04:05 MST"), answer, answer)
	if got != want {
		t.Errorf("ParentAskLine() = %q; want %q", got, want)
	}
	if !strings.HasPrefix(got, ParentAskLineTag) {
		t.Errorf("ParentAskLine() = %q; want it to open with %q", got, ParentAskLineTag)
	}
}

// TestBurlerReviewProducer_Call_SettlesParentAskOnAcceptance table-drives the settle of a round's ask.
// The settle runs once the round's review is accepted.
// An accepted review is one this call spawned or one a restart finds already accepted.
// Either way a done ask's report is copied beside the review, and an open ask is set timeout with no copy.
// A restart that finds the review accepted spawns and probes nothing.
func TestBurlerReviewProducer_Call_SettlesParentAskOnAcceptance(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	findings := []burlerengine.ParentFinding{{Change: "tighten the seam"}}
	tests := []struct {
		name string
		// alreadyAccepted writes the round's review and acceptance record before the call.
		alreadyAccepted bool
		prior           priorAsk
		wantState       burlerengine.ParentAskState
		wantCopy        bool
	}{
		{
			name:  "a spawned review copies a done report beside the review",
			prior: priorAsk{ask: burlerengine.ParentAsk{Round: 1, State: burlerengine.ParentAskDone}, findings: findings}, wantState: burlerengine.ParentAskDone, wantCopy: true,
		},
		{
			name:  "a spawned review sets an open ask timeout",
			prior: priorAsk{ask: burlerengine.ParentAsk{Round: 1, State: burlerengine.ParentAskOpen, Strand: storedStrand}}, wantState: burlerengine.ParentAskTimeout,
		},
		{
			name: "an already accepted review copies a done report not yet copied", alreadyAccepted: true,
			prior: priorAsk{ask: burlerengine.ParentAsk{Round: 1, State: burlerengine.ParentAskDone}, findings: findings}, wantState: burlerengine.ParentAskDone, wantCopy: true,
		},
		{
			name: "an already accepted review sets an open ask timeout", alreadyAccepted: true,
			prior: priorAsk{ask: burlerengine.ParentAsk{Round: 1, State: burlerengine.ParentAskOpen, Strand: storedStrand}}, wantState: burlerengine.ParentAskTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newParentFixture(t)
			if tt.alreadyAccepted {
				writeAcceptedRound(t, f.runDir, 1, "d")
			}
			f.recordAsk(t, tt.prior)
			runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{reviewDone("d")}}

			if _, _, err := f.producer(t, runner).Call(context.Background()); err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}

			if tt.alreadyAccepted {
				if runner.ReviewCalls != 0 || runner.ProbeReviewCalls != 0 {
					t.Errorf("Review calls %d, ProbeReview calls %d; want a hand-on that spawns and probes nothing", runner.ReviewCalls, runner.ProbeReviewCalls)
				}
			} else if runner.ReviewCalls != 1 || runner.GotReviewOpts[0].ParentAsk != 1 {
				t.Errorf("Review calls = %d with opts %+v; want one call told ask 1", runner.ReviewCalls, runner.GotReviewOpts)
			}
			ask, found := f.readAsks(t).RoundAsk(parentSeed, 1)
			if !found || ask.State != tt.wantState {
				t.Fatalf("round 1 ask = %+v, found %v; want state %s", ask, found, tt.wantState)
			}
			copied, readErr := os.ReadFile(burlerengine.ParentCopyPath(roundReviewPath(f.runDir, 1)))
			if !tt.wantCopy {
				if !errors.Is(readErr, os.ErrNotExist) {
					t.Errorf("read of the report copy = %v; want it absent", readErr)
				}
				return
			}
			report, err := os.ReadFile(ask.Report)
			if err != nil || readErr != nil {
				t.Fatalf("reading the report %v and its copy %v; want both nil", err, readErr)
			}
			if string(copied) != string(report) || !strings.Contains(string(copied), "tighten the seam") {
				t.Errorf("report copy = %q; want the report %q", copied, report)
			}
		})
	}
}

// TestBurlerReviewProducer_Call_WithoutParentSeatAsksNothing proves a row with no parent seat asks nothing.
// Its review table is told no ask.
func TestBurlerReviewProducer_Call_WithoutParentSeatAsksNothing(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	runDir := t.TempDir()
	writeFocusFile(t, runDir, 1, focusFile{Round: 1, Focus: []string{"look at the seam"}})
	runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{reviewDone("d")}}

	if _, _, err := newReviewProducer(t, runDir, runner, newHalfSpec()).Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}

	if runner.ReviewCalls != 1 || runner.GotReviewOpts[0].ParentAsk != 0 {
		t.Errorf("Review calls = %d with opts %+v; want one call told no ask", runner.ReviewCalls, runner.GotReviewOpts)
	}
}
