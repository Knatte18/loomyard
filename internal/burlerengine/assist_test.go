// assist_test.go tables the review half's assistant reviewer against the same-package fakeShuttle.
// It covers the assistant's report and end-marker paths and the assisted round end to end;
// it also covers the review step of a round with no assistant.

package burlerengine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/lensreport"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// assistRole is the strand role of the review table's assistant seat.
var assistRole = seatengine.SeatRole(ReviewRolePrefix, AssistSeatName)

// assistOpts is the RunOpts of an assisted round: a chair on claude and a codex assistant, in round 2b.
func assistOpts() RunOpts {
	return RunOpts{
		Review:        shuttleengine.ModelChoice{Engine: "claude"},
		Assist:        shuttleengine.ModelChoice{Engine: "codex", Model: "gpt-6-luna", Effort: "low"},
		AssistKey:     shuttleengine.ModelKey{Module: "loom", Key: "assist_model"},
		Timeout:       30 * time.Minute,
		AssistTimeout: 7 * time.Minute,
		Round:         "2b",
	}
}

// newAssistProfile builds a valid Profile whose review path is absolute under a fresh root.
// The root sits outside the anchor's _lym.
func newAssistProfile(t *testing.T) (string, Profile) {
	t.Helper()
	root, p := newEngineTestProfile(t)
	p.ReviewPath = filepath.Join(root, "review.md")
	return root, p
}

// newAssistEngine wires an Engine to shuttle over a geometry rooted at root whose default engine is claude.
func newAssistEngine(t *testing.T, root string, shuttle *fakeShuttle) *Engine {
	t.Helper()
	geom := Geometry{WorktreeRoot: root, AnchorPath: root, Shortname: "tst", Slug: "slug", DefaultEngine: "claude"}
	return New(shuttle, geom, lenses.Config{}, newTestStencilsDir(t), "", nil)
}

// newAssistShuttle scripts an assisted review round.
// The chair writes an approved review and is held until the assistant has written its report,
// so the assistant's own run ends first.
// assist scripts the assistant, which must end done.
func newAssistShuttle(assist halfScript) *fakeShuttle {
	released := make(chan struct{})
	assist.writes = approvedReview
	assist.beforeReturn = func() { close(released) }
	return &fakeShuttle{
		review: halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, writes: approvedReview, hold: released},
		assist: assist,
	}
}

// TestAssistPaths proves the assistant's report and end marker sit under the anchor's .lym.
// They sit at the review's subpath when the review is under the anchor's _lym, and under burler/assist otherwise.
// A retry's token gives other paths than the first attempt's.
func TestAssistPaths(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	anchor := filepath.Join(string(filepath.Separator), "anchor")
	tests := []struct {
		name       string
		reviewPath string
		token      string
		wantDir    string
		wantBase   string
	}{
		{name: "review under _lym", reviewPath: filepath.Join(anchor, "_lym", "discuss", "x", "review.md"), token: "1", wantDir: filepath.Join(anchor, ".lym", "discuss", "x"), wantBase: "review.attempt-1.assist-assist"},
		{name: "retry of the review under _lym", reviewPath: filepath.Join(anchor, "_lym", "discuss", "x", "review.md"), token: "1-retry", wantDir: filepath.Join(anchor, ".lym", "discuss", "x"), wantBase: "review.attempt-1-retry.assist-assist"},
		{name: "review directly in _lym", reviewPath: filepath.Join(anchor, "_lym", "review.md"), token: "2b", wantDir: filepath.Join(anchor, ".lym"), wantBase: "review.attempt-2b.assist-assist"},
		{name: "review outside _lym", reviewPath: filepath.Join(anchor, "elsewhere", "review.md"), token: "1", wantDir: filepath.Join(anchor, ".lym", "burler", "assist"), wantBase: "review.attempt-1.assist-assist"},
		{name: "review beside _lym with a shared prefix", reviewPath: filepath.Join(anchor, "_lym-other", "review.md"), token: "3", wantDir: filepath.Join(anchor, ".lym", "burler", "assist"), wantBase: "review.attempt-3.assist-assist"},
	}
	seen := map[string]string{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, marker := assistPaths(anchor, tt.reviewPath, tt.token, AssistSeatName)
			if want := filepath.Join(tt.wantDir, tt.wantBase+".md"); report != want {
				t.Errorf("assistPaths() report = %q; want %q", report, want)
			}
			if want := filepath.Join(tt.wantDir, tt.wantBase+".end"); marker != want {
				t.Errorf("assistPaths() marker = %q; want %q", marker, want)
			}
		})
		report, _ := assistPaths(anchor, tt.reviewPath, tt.token, AssistSeatName)
		if other, taken := seen[report]; taken {
			t.Errorf("assistPaths() report %q is the same for %q and %q; want each round and token its own", report, other, tt.name)
		}
		seen[report] = tt.name
	}
}

// TestAssistCopyPath proves the copy of an assistant's report sits beside the review file.
// The copy is named by the review's stem and the seat's name.
func TestAssistCopyPath(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	if got, want := AssistCopyPath(filepath.Join("d", "review.md"), "assist"), filepath.Join("d", "review.assist-assist.md"); got != want {
		t.Errorf("AssistCopyPath() = %q; want %q", got, want)
	}
}

// TestEngine_AssistSeat proves the table a round with an assistant builds.
// The chair uses the assisted stencil and reads the assistant's report as an input.
// The one assistant seat uses its own stencil.
// It has its model choice, key, report, end marker, review-parse gate, timeout and skip reason.
// It also proves a round with no assistant builds the one chair seat on the plain reviewer stencil.
func TestEngine_AssistSeat(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		opts       RunOpts
		wantSeats  int
		wantSkip   string
		wantChair  string
		wantAssist bool
	}{
		{name: "no assistant", opts: RunOpts{Round: "2b"}, wantSeats: 1, wantChair: "burler-template-reviewer"},
		{name: "a model choice with no model is no assistant", opts: RunOpts{Round: "2b", Assist: shuttleengine.ModelChoice{Engine: "codex"}}, wantSeats: 1, wantChair: "burler-template-reviewer"},
		{name: "an assistant", opts: assistOpts(), wantSeats: 2, wantChair: "burler-template-reviewer-assisted", wantAssist: true},
		{name: "a skipped assistant", opts: func() RunOpts { o := assistOpts(); o.AssistSkip = "no budget"; return o }(), wantSeats: 2, wantChair: "burler-template-reviewer-assisted", wantSkip: "no budget", wantAssist: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newAssistProfile(t)
			e := newAssistEngine(t, root, &fakeShuttle{})

			table := e.probeTable(&p, tt.opts)

			if len(table.Seats) != tt.wantSeats {
				t.Fatalf("probeTable() holds %d seats; want %d", len(table.Seats), tt.wantSeats)
			}
			chair := table.Seats[0]
			if chair.Stencil != tt.wantChair {
				t.Errorf("chair stencil = %q; want %q", chair.Stencil, tt.wantChair)
			}
			if !tt.wantAssist {
				if len(chair.Inputs) != 0 {
					t.Errorf("chair inputs = %v; want none", chair.Inputs)
				}
				return
			}
			report, marker := assistPaths(root, p.ReviewPath, tt.opts.Round, AssistSeatName)
			if !slices.Equal(chair.Inputs, []string{report}) {
				t.Errorf("chair inputs = %v; want the assistant's report [%s]", chair.Inputs, report)
			}
			if !slices.Equal(chair.Outputs, []string{p.ReviewPath}) {
				t.Errorf("chair outputs = %v; want the review file", chair.Outputs)
			}
			seat := table.Seats[1]
			if seat.Name != AssistSeatName || !seat.Assistant || seat.Stencil != "burler-template-assist-reviewer" {
				t.Errorf("assistant seat name %q, assistant %v, stencil %q; want %q, true, burler-template-assist-reviewer", seat.Name, seat.Assistant, seat.Stencil, AssistSeatName)
			}
			if seat.ModelChoice != tt.opts.Assist || seat.ModelKey != tt.opts.AssistKey {
				t.Errorf("assistant model choice %+v key %+v; want %+v and %+v", seat.ModelChoice, seat.ModelKey, tt.opts.Assist, tt.opts.AssistKey)
			}
			if !slices.Equal(seat.Outputs, []string{report}) || seat.EndMarker != marker {
				t.Errorf("assistant outputs %v end marker %q; want [%s] and %q", seat.Outputs, seat.EndMarker, report, marker)
			}
			if len(seat.Gate) != 1 || seat.Gate[0].Name != "review" || !seat.Gate[0].PassOnCap || seat.Gate[0].Attempts != reviewGateAttempts {
				t.Errorf("assistant gate = %+v; want the one review-parse entry", seat.Gate)
			}
			if seat.Timeout != tt.opts.AssistTimeout || seat.SkipReason != tt.wantSkip {
				t.Errorf("assistant timeout %v skip reason %q; want %v and %q", seat.Timeout, seat.SkipReason, tt.opts.AssistTimeout, tt.wantSkip)
			}
			if seat.ForkSubagents || len(seat.Inputs) != 0 {
				t.Errorf("assistant forks %v inputs %v; want it to authorize no fork and read no input", seat.ForkSubagents, seat.Inputs)
			}
			if !slices.Equal(seat.Skills, chair.Skills) {
				t.Errorf("assistant skills = %v; want the chair's %v", seat.Skills, chair.Skills)
			}
		})
	}
}

// TestEngine_Review_AssistedRound proves an assisted round end to end over the fake shuttle.
// Both the chair and the assistant start, and the assistant runs under its own spec.
// The chair's prompt and review step name the assistant and its consolidation part.
// The report of an assistant whose end marker reads done is copied beside the review;
// any other ending copies nothing, and the verdict stays the chair's.
func TestEngine_Review_AssistedRound(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		result     shuttleengine.Result
		wantEnding string
		wantCopy   bool
	}{
		{name: "a done assistant", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, wantEnding: seatengine.EndingDone, wantCopy: true},
		{name: "an assistant whose gate failed", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Reason: "spent"}}, wantEnding: seatengine.EndingGateFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newAssistProfile(t)
			opts := assistOpts()
			shuttle := newAssistShuttle(halfScript{result: tt.result})
			e := newAssistEngine(t, root, shuttle)
			report, marker := assistPaths(root, p.ReviewPath, opts.Round, AssistSeatName)

			got, err := e.Review(p, opts)

			if err != nil {
				t.Fatalf("Review() = %v; want nil error", err)
			}
			if got.Outcome != shuttleengine.OutcomeDone || got.Verdict != VerdictApproved {
				t.Errorf("Review() outcome %q verdict %q; want the chair's accepted approved review", got.Outcome, got.Verdict)
			}
			if want := []string{reviewHalfRole, assistRole}; !slices.Equal(shuttle.started, want) {
				t.Fatalf("started seats = %v; want %v", shuttle.started, want)
			}
			chair, assist := shuttle.specs[reviewHalfRole], shuttle.specs[assistRole]
			if !slices.Equal(assist.OutputFiles, []string{report}) || assist.Timeout != opts.AssistTimeout || assist.ModelChoice != opts.Assist {
				t.Errorf("assistant spec outputs %v timeout %v model %+v; want [%s], %v, %+v", assist.OutputFiles, assist.Timeout, assist.ModelChoice, report, opts.AssistTimeout, opts.Assist)
			}
			if chair.Timeout != opts.Timeout || chair.ModelChoice != opts.Review {
				t.Errorf("chair spec timeout %v model %+v; want %v and %+v", chair.Timeout, chair.ModelChoice, opts.Timeout, opts.Review)
			}
			if gate := shuttle.gates[assistRole]; len(gate) != 1 || gate[0].Name != "review" {
				t.Errorf("assistant gate = %+v; want the one review-parse entry", gate)
			}
			for _, want := range []string{report, marker} {
				if !strings.Contains(chair.Prompt, want) {
					t.Errorf("chair prompt lacks %q; want it to name the assistant's report and end marker", want)
				}
			}
			if !strings.Contains(assist.Prompt, report) {
				t.Errorf("assistant prompt lacks its report path %q", report)
			}
			step, err := os.ReadFile(firstGlob(t, filepath.Join(root, ".lym", "burler", "review-*", "instruction-2-review.md")))
			if err != nil {
				t.Fatalf("ReadFile(chair review step) = %v; want nil", err)
			}
			if !strings.Contains(string(step), "origin: assist:assist") {
				t.Errorf("chair review step lacks the consolidation part with `origin: assist:assist`")
			}
			assistStep, err := os.ReadFile(firstGlob(t, filepath.Join(root, ".lym", "burler", "assist-*", "instruction-2-review.md")))
			if err != nil {
				t.Fatalf("ReadFile(assistant review step) = %v; want nil", err)
			}
			if strings.Contains(string(assistStep), "origin: assist:assist") || !strings.Contains(string(assistStep), report) {
				t.Errorf("assistant review step holds the consolidation part or lacks its own report path %q", report)
			}

			if len(got.Assists) != 1 {
				t.Fatalf("Assists = %+v; want one outcome", got.Assists)
			}
			outcome := got.Assists[0]
			wantCopy := ""
			if tt.wantCopy {
				wantCopy = AssistCopyPath(p.ReviewPath, AssistSeatName)
			}
			if outcome.Seat != AssistSeatName || outcome.Ending != tt.wantEnding || outcome.Report != report || outcome.Copy != wantCopy {
				t.Errorf("Assists[0] = %+v; want seat %q ending %q report %q copy %q", outcome, AssistSeatName, tt.wantEnding, report, wantCopy)
			}
			copied, err := os.ReadFile(AssistCopyPath(p.ReviewPath, AssistSeatName))
			if tt.wantCopy && (err != nil || string(copied) != approvedReview) {
				t.Errorf("ReadFile(report copy) = %q, %v; want the assistant's report", copied, err)
			}
			if !tt.wantCopy && err == nil {
				t.Errorf("a %q assistant's report was copied beside the review; want nothing copied", tt.wantEnding)
			}
		})
	}
}

// TestEngine_Review_AssistantPromptFollowsItsFan proves a fanned assistant's prompt names every lens report.
// The reports are kept off the assistant's outputs.
// A solo assistant's prompt names none.
// It also proves Review names the fan's lenses that returned no report in the assistant's outcome, in fan order.
// A solo assistant names none.
func TestEngine_Review_AssistantPromptFollowsItsFan(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		fan      string
		wantLens bool
	}{
		{name: "a fanned assistant", fan: "standard", wantLens: true},
		{name: "a solo assistant"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newAssistProfile(t)
			opts := assistOpts()
			opts.AssistFan = tt.fan
			shuttle := newAssistShuttle(halfScript{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}})
			geom := Geometry{WorktreeRoot: root, AnchorPath: root, Shortname: "tst", Slug: "slug", DefaultEngine: "claude"}
			e := New(shuttle, geom, clusterTestConfig, newTestStencilsDir(t), "", nil)
			report, _ := assistPaths(root, p.ReviewPath, opts.Round, AssistSeatName)

			got, err := e.Review(p, opts)
			if err != nil {
				t.Fatalf("Review() = %v; want nil error", err)
			}

			assist := shuttle.specs[assistRole]
			if !slices.Equal(assist.OutputFiles, []string{report}) {
				t.Errorf("assistant spec outputs = %v; want the report alone, [%s]", assist.OutputFiles, report)
			}
			lensPaths := []string{lensreport.PathPattern(report), lensreport.Path(report, "style")}
			for _, path := range lensPaths {
				if holds := strings.Contains(assist.Prompt, path); holds != tt.wantLens {
					t.Errorf("assistant prompt holds %q = %v; want %v:\n%s", path, holds, tt.wantLens, assist.Prompt)
				}
			}
			var wantMissing []string
			if tt.wantLens {
				wantMissing = e.assistLensNames(tt.fan)
				if len(wantMissing) == 0 {
					t.Fatalf("assistLensNames(%q) = none; want the fan's lenses", tt.fan)
				}
			}
			if len(got.Assists) != 1 || !slices.Equal(got.Assists[0].MissingLenses, wantMissing) {
				t.Errorf("Assists = %+v; want one outcome missing lenses %v, since the shuttle wrote no lens report", got.Assists, wantMissing)
			}
		})
	}
}

// TestEngine_Review_AssistantThatDoesNotStart proves an assistant never starts, and its end marker says so.
// It covers a skipped assistant, an engine with no assist-reading block, and a fan the lens library does not define.
// The chair is started and the review half returns its accepted review.
func TestEngine_Review_AssistantThatDoesNotStart(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		engine     string
		skip       string
		fan        string
		wantEnding string
	}{
		{name: "a skipped assistant", engine: "codex", skip: "no budget", wantEnding: "not started: no budget"},
		{name: "an engine with no assist-reading block", engine: "gemini", wantEnding: "not started: "},
		{name: "a fan the lens library does not define", engine: "codex", fan: "nosuchfan", wantEnding: "not started: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newAssistProfile(t)
			opts := assistOpts()
			opts.Assist.Engine = tt.engine
			opts.AssistSkip = tt.skip
			opts.AssistFan = tt.fan
			shuttle := doneReview(approvedReview)
			e := newAssistEngine(t, root, shuttle)

			got, err := e.Review(p, opts)

			if err != nil {
				t.Fatalf("Review() = %v; want nil error", err)
			}
			if want := []string{reviewHalfRole}; !slices.Equal(shuttle.started, want) {
				t.Errorf("started seats = %v; want only the chair %v", shuttle.started, want)
			}
			if got.Outcome != shuttleengine.OutcomeDone || got.Verdict != VerdictApproved {
				t.Errorf("Review() outcome %q verdict %q; want the chair's accepted approved review", got.Outcome, got.Verdict)
			}
			if len(got.Assists) != 1 || !strings.HasPrefix(got.Assists[0].Ending, tt.wantEnding) || got.Assists[0].Copy != "" {
				t.Errorf("Assists = %+v; want one outcome whose ending starts %q and no copy", got.Assists, tt.wantEnding)
			}
		})
	}
}

// TestKeepAssistReports proves a seat whose end marker reads done has its report copied beside the review.
// Any other ending, or no marker, copies nothing.
// A seat that is no assistant is passed over.
func TestKeepAssistReports(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		marker   string
		noMarker bool
		wantCopy bool
	}{
		{name: "done", marker: seatengine.EndingDone, wantCopy: true},
		{name: "gate failed", marker: seatengine.EndingGateFailed},
		{name: "stopped", marker: seatengine.EndingStopped},
		{name: "not started", marker: "not started: no budget"},
		{name: "not found", marker: seatengine.EndingNotFound},
		{name: "no marker", noMarker: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			reviewPath := filepath.Join(dir, "review.md")
			report, marker := filepath.Join(dir, "report.md"), filepath.Join(dir, "report.end")
			if err := os.WriteFile(report, []byte("the report"), 0o644); err != nil {
				t.Fatalf("WriteFile(report) = %v; want nil", err)
			}
			if !tt.noMarker {
				if err := os.WriteFile(marker, []byte(tt.marker+"\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(marker) = %v; want nil", err)
				}
			}
			seats := []seatengine.SeatResult{
				{Name: "advisor", Outputs: []string{report}, EndMarker: marker},
				{Name: AssistSeatName, Assistant: true, Outputs: []string{report}, EndMarker: marker},
			}

			got := keepAssistReports(reviewPath, seats, nil)

			if len(got) != 1 || got[0].Seat != AssistSeatName || got[0].Report != report || got[0].Ending != tt.marker {
				t.Fatalf("keepAssistReports() = %+v; want one outcome of %q with report %q and ending %q", got, AssistSeatName, report, tt.marker)
			}
			copyPath := AssistCopyPath(reviewPath, AssistSeatName)
			content, err := os.ReadFile(copyPath)
			if tt.wantCopy {
				if got[0].Copy != copyPath || err != nil || string(content) != "the report" {
					t.Errorf("keepAssistReports() copy %q, file %q, %v; want %q holding the report", got[0].Copy, content, err, copyPath)
				}
				return
			}
			if got[0].Copy != "" || err == nil {
				t.Errorf("keepAssistReports() copy %q, copy file read error %v; want nothing copied", got[0].Copy, err)
			}
		})
	}
}

// TestKeepAssistReports_MissingLenses proves a done assistant's outcome names each fan lens that returned no report.
// The names follow fan order.
// An assistant that did not end done names none, since its ending already says so.
func TestKeepAssistReports_MissingLenses(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const reported = "```yaml\nlens: x\noutcome: reported\n```\n\nThe message.\n"
	const noReport = "```yaml\nlens: x\noutcome: no report\n```\n"
	tests := []struct {
		name    string
		ending  string
		reports map[string]string
		want    []string
	}{
		{name: "every lens returned", ending: seatengine.EndingDone, reports: map[string]string{"a": reported, "b": reported}},
		{name: "a seat that ran solo", ending: seatengine.EndingDone, want: []string{"a", "b"}},
		{name: "one lens file absent", ending: seatengine.EndingDone, reports: map[string]string{"b": reported}, want: []string{"a"}},
		{name: "one lens returned nothing", ending: seatengine.EndingDone, reports: map[string]string{"a": reported, "b": noReport}, want: []string{"b"}},
		{name: "an assistant that did not end done", ending: seatengine.EndingStopped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			report, marker := filepath.Join(dir, "report.md"), filepath.Join(dir, "report.end")
			files := map[string]string{report: "the report", marker: tt.ending + "\n"}
			for lens, content := range tt.reports {
				files[lensreport.Path(report, lens)] = content
			}
			for path, content := range files {
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile(%s) = %v; want nil", path, err)
				}
			}
			seats := []seatengine.SeatResult{{Name: AssistSeatName, Assistant: true, Outputs: []string{report}, EndMarker: marker}}

			got := keepAssistReports(filepath.Join(dir, "review.md"), seats, []string{"a", "b"})

			if len(got) != 1 || !slices.Equal(got[0].MissingLenses, tt.want) {
				t.Errorf("keepAssistReports() = %+v; want one outcome missing lenses %v", got, tt.want)
			}
		})
	}
}

// TestEngine_ProbeReview_AssistantSeat proves ProbeReview probes the assistant seat when RunOpts name an assistant.
// With no assistant named, it probes only the chair.
func TestEngine_ProbeReview_AssistantSeat(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		opts       RunOpts
		wantAssist bool
	}{
		{name: "an assistant", opts: assistOpts(), wantAssist: true},
		{name: "no assistant", opts: RunOpts{Round: "2b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, p := newAssistProfile(t)
			shuttle := &fakeShuttle{live: map[string]bool{reviewHalfRole: true}}
			e := newAssistEngine(t, root, shuttle)

			half, err := e.ProbeReview(p, tt.opts)
			if err != nil {
				t.Fatalf("ProbeReview() = %v; want nil error", err)
			}

			if half.State != HalfLive {
				t.Errorf("state = %d; want %d", half.State, HalfLive)
			}
			if len(shuttle.started) != 0 {
				t.Errorf("ProbeReview started %v; want nothing started", shuttle.started)
			}
			spec, probed := shuttle.probeSpecs[assistRole]
			if probed != tt.wantAssist {
				t.Fatalf("assistant probed = %v; want %v", probed, tt.wantAssist)
			}
			if !tt.wantAssist {
				return
			}
			report, _ := assistPaths(root, p.ReviewPath, tt.opts.Round, AssistSeatName)
			if !slices.Equal(spec.OutputFiles, []string{report}) || spec.Timeout != tt.opts.AssistTimeout {
				t.Errorf("assistant probe spec outputs %v timeout %v; want [%s] and %v", spec.OutputFiles, spec.Timeout, report, tt.opts.AssistTimeout)
			}
		})
	}
}

// TestComposeReviewPrompt_AssistRules proves a round with no assistant renders the review step unchanged.
// An assisted round renders that same step with the consolidation part added after the cluster rules.
func TestComposeReviewPrompt_AssistRules(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	stencilsDir := newTestStencilsDir(t)
	p := newComposableProfile(t)

	noAssist, err := assistRulesBlock(stencilsDir, "claude", RunOpts{})
	if err != nil || noAssist != "" {
		t.Fatalf("assistRulesBlock(no assistant) = %q, %v; want \"\" and nil error", noAssist, err)
	}
	block, err := assistRulesBlock(stencilsDir, "claude", assistOpts())
	if err != nil {
		t.Fatalf("assistRulesBlock(assistant) = %v; want nil error", err)
	}
	if !strings.HasPrefix(block, "\n\n") || !strings.Contains(block, "origin: assist:assist") {
		t.Fatalf("assistRulesBlock(assistant) = %q; want a block led by a blank line carrying `origin: assist:assist`", block)
	}

	plain, err := composeReviewPrompt(stencilsDir, testSpecsDir, "claude", &p, "", noAssist, "", testReviewFiles)
	if err != nil {
		t.Fatalf("composeReviewPrompt(no assistant) = %v; want nil error", err)
	}
	assisted, err := composeReviewPrompt(stencilsDir, testSpecsDir, "claude", &p, "", block, "", testReviewFiles)
	if err != nil {
		t.Fatalf("composeReviewPrompt(assistant) = %v; want nil error", err)
	}

	plainStep, assistedStep := plain.Files[reviewStepFile].Content, assisted.Files[reviewStepFile].Content
	if strings.Contains(plainStep, "assistant reviewer") || strings.Contains(plainStep, "origin: assist") || strings.Contains(plainStep, "{{") {
		t.Errorf("review step of a round with no assistant holds assistant text or a marker residue:\n%s", plainStep)
	}
	if !strings.Contains(assistedStep, block) {
		t.Errorf("review step of an assisted round lacks the consolidation block")
	}
	if got := strings.Replace(assistedStep, block, "", 1); got != plainStep {
		t.Errorf("assisted review step without its block differs from the plain step\n got: %q\nwant: %q", got, plainStep)
	}
	for i, file := range plain.Files {
		if i != reviewStepFile && file.Content != assisted.Files[i].Content {
			t.Errorf("instruction file %s differs between a plain and an assisted round; want only the review step to change", file.Path)
		}
	}
}
