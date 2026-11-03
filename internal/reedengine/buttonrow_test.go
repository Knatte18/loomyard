//go:build !windows

// buttonrow_test.go covers buttonrow.go's render of the status-bar button row into session options.
// It is built where `sh` resolves only, and drives the render through the fake tmux with a stub button context.

package reedengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// rowFragment is the literal tmux format of one drawn button, written out independently of buttonRangeFormat.
func rowFragment(index int, label string) string {
	return fmt.Sprintf("#[range=user|lymbtn%d bg=colour236 fg=white] %s #[norange list=on default] ", index, label)
}

// buttonRowSpecSlot is the index of the spec JSON within a button row invocation.
const buttonRowSpecSlot = 16

// TestPinButtonRowLocked pins the four options one render sets, for each shape of button context.
// It reads Warns through logcapture, whose output is process-global, so it does not call t.Parallel.
func TestPinButtonRowLocked(t *testing.T) {
	tiergate.Require(t, 0)
	posix := shell.Posix()
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Fatalf("LookPath(sh) = %v, want sh on PATH", err)
	}

	tests := []struct {
		name          string
		shell         string
		nilClosure    bool
		closureErr    error
		place         ButtonPlace
		buttons       string
		machine       string
		beam          ButtonBeam
		beamErr       error
		wantBeamCalls int
		wantPlain     string
		wantAgent     string
		wantEntries   []buttonSpecEntry
		wantDir       string
		wantWarns     []string
	}{
		{
			name:       "NilClosureRendersFollowAlone",
			nilClosure: true,
			place:      ButtonPlacePrime,
			buttons:    "buttons:\n  beam: [{label: A, run: a}]\n",
		},
		{
			name:       "ClosureErrorRendersNothingAndWarns",
			closureErr: errors.New("geometry unavailable"),
			place:      ButtonPlacePrime,
			buttons:    "buttons:\n  beam: [{label: A, run: a}]\n",
			wantWarns:  []string{"geometry unavailable"},
		},
		{
			name:      "UnresolvableShellRendersNothingAndWarns",
			shell:     "no-such-shell-lym",
			place:     ButtonPlacePrime,
			buttons:   "buttons:\n  beam: [{label: A, run: a}]\n",
			wantWarns: []string{"shell"},
		},
		{
			name:          "BeamErrorForATaskRendersNothingAndWarns",
			place:         ButtonPlaceTask,
			buttons:       "buttons:\n  task: [{label: A, run: a}]\n",
			beamErr:       errors.New("beam lookup failed"),
			wantBeamCalls: 1,
			wantWarns:     []string{"beam lookup failed"},
		},
		{
			name:    "PrimeTakesReedFirstThenMachineAndNeverAsksForTheBeam",
			place:   ButtonPlacePrime,
			buttons: "buttons:\n  prime: []\n  beam: [{label: A, run: a}]\n",
			machine: "buttons:\n  beam: [{label: M, run: m}]\n",
			wantPlain: rowFragment(0, "A") +
				rowFragment(1, "M"),
			wantAgent: rowFragment(0, "A") +
				rowFragment(1, "M"),
			wantEntries: []buttonSpecEntry{
				{Index: 0, Label: "A", Run: "a"},
				{Index: 1, Label: "M", Run: "m"},
			},
			wantDir: "ANCHOR",
		},
		{
			name:  "AgentsEntriesJoinOnlyTheAgentRowAndFlagStrand",
			place: ButtonPlaceBeam,
			buttons: "buttons:\n  beam:\n" +
				"    - {label: S, run: 'echo {strand}', window: agents}\n" +
				"    - {label: P, run: 'echo p', window: agents}\n" +
				"    - {label: Q, run: q}\n",
			wantPlain: rowFragment(2, "Q"),
			wantAgent: rowFragment(0, "S") + rowFragment(1, "P") + rowFragment(2, "Q"),
			wantEntries: []buttonSpecEntry{
				{Index: 0, Label: "S", Run: "echo {strand}", Strand: true},
				{Index: 1, Label: "P", Run: "echo p"},
				{Index: 2, Label: "Q", Run: "q"},
			},
			wantDir: "ANCHOR",
		},
		{
			name:        "InvalidEntryIsSkippedAndTheValidOneIsNumberedFromZero",
			place:       ButtonPlaceBeam,
			buttons:     "buttons:\n  beam: [{label: '', run: x}, {label: V, run: v}]\n",
			wantPlain:   rowFragment(0, "V"),
			wantAgent:   rowFragment(0, "V"),
			wantEntries: []buttonSpecEntry{{Index: 0, Label: "V", Run: "v"}},
			wantDir:     "ANCHOR",
			wantWarns:   []string{"skipped an invalid button entry"},
		},
		{
			name:          "TaskRunsInTheBeamAnchorAndQuotesItIntoBeamPlaceholders",
			place:         ButtonPlaceTask,
			buttons:       "buttons:\n  task: [{label: B, run: 'ls {beam}'}]\n",
			beam:          ButtonBeam{Anchor: "/b eam", Name: "bn", PrimeAnchor: "/prime"},
			wantBeamCalls: 1,
			wantPlain:     rowFragment(0, "B"),
			wantAgent:     rowFragment(0, "B"),
			wantEntries:   []buttonSpecEntry{{Index: 0, Label: "B", Run: "ls " + posix.Quote("/b eam")}},
			wantDir:       "/b eam",
		},
		{
			name:          "TaskWithoutABeamAnchorLeavesOutBeamEntriesAndRunsInThePrime",
			place:         ButtonPlaceTask,
			buttons:       "buttons:\n  task: [{label: B, run: 'ls {beam}'}, {label: C, run: pwd}]\n",
			beam:          ButtonBeam{Name: "bn", PrimeAnchor: "/prime"},
			wantBeamCalls: 1,
			wantPlain:     rowFragment(0, "C"),
			wantAgent:     rowFragment(0, "C"),
			wantEntries:   []buttonSpecEntry{{Index: 0, Label: "C", Run: "pwd"}},
			wantDir:       "/prime",
			wantWarns:     []string{"lym adopt", "bn"},
		},
		{
			name:          "BeamPlaceholderInAPrimeEntryAsksForTheBeamOnce",
			place:         ButtonPlacePrime,
			buttons:       "buttons:\n  beam: [{label: B, run: 'ls {beam}'}, {label: C, confirm: 'on {beam}?', run: c}]\n",
			beam:          ButtonBeam{Anchor: "/beam", Name: "bn", PrimeAnchor: "/prime"},
			wantBeamCalls: 1,
			wantPlain:     rowFragment(0, "B") + rowFragment(1, "C"),
			wantAgent:     rowFragment(0, "B") + rowFragment(1, "C"),
			wantEntries: []buttonSpecEntry{
				{Index: 0, Label: "B", Run: "ls " + posix.Quote("/beam")},
				{Index: 1, Label: "C", Run: "c", Confirm: "on /beam?"},
			},
			wantDir: "ANCHOR",
		},
		{
			name:        "ConfirmDoublesHashFromAPlaceholderValue",
			place:       ButtonPlaceBeam,
			buttons:     "buttons:\n  beam: [{label: D, run: d, confirm: 'drop {slug}?'}]\n",
			wantPlain:   rowFragment(0, "D"),
			wantAgent:   rowFragment(0, "D"),
			wantEntries: []buttonSpecEntry{{Index: 0, Label: "D", Run: "d", Confirm: "drop a##b?"}},
			wantDir:     "ANCHOR",
		},
		{
			name:        "AskEntryCarriesItsFilledAskAndDefaultAndLeavesAnswerForTheClick",
			place:       ButtonPlaceBeam,
			buttons:     "buttons:\n  beam: [{label: N, run: 'open {answer}', ask: 'Name for {slug}:', ask_default: x}]\n",
			wantPlain:   rowFragment(0, "N"),
			wantAgent:   rowFragment(0, "N"),
			wantEntries: []buttonSpecEntry{{Index: 0, Label: "N", Run: "open {answer}", Ask: "Name for a##b:", AskDefault: "x"}},
			wantDir:     "ANCHOR",
		},
		{
			name:        "AskNamingStrandFlagsStrandForTheClick",
			place:       ButtonPlaceBeam,
			buttons:     "buttons:\n  beam: [{label: S, run: 'echo hi', ask: 'For {strand}:', window: agents}]\n",
			wantAgent:   rowFragment(0, "S"),
			wantEntries: []buttonSpecEntry{{Index: 0, Label: "S", Run: "echo hi", Ask: "For {strand}:", Strand: true}},
			wantDir:     "ANCHOR",
		},
		{
			name:          "AskNamingAnAbsentBeamIsLeftOutWithTheBeamWarn",
			place:         ButtonPlaceTask,
			buttons:       "buttons:\n  task: [{label: B, run: b, ask: 'On {beam}:'}, {label: C, run: pwd}]\n",
			beam:          ButtonBeam{Name: "bn", PrimeAnchor: "/prime"},
			wantBeamCalls: 1,
			wantPlain:     rowFragment(0, "C"),
			wantAgent:     rowFragment(0, "C"),
			wantEntries:   []buttonSpecEntry{{Index: 0, Label: "C", Run: "pwd"}},
			wantDir:       "/prime",
			wantWarns:     []string{"lym adopt", "bn"},
		},
		{
			name:          "AskNamingAPresentBeamIsKeptWithTheBeamFilled",
			place:         ButtonPlacePrime,
			buttons:       "buttons:\n  beam: [{label: B, run: b, ask: 'On {beam}:'}]\n",
			beam:          ButtonBeam{Anchor: "/beam", Name: "bn", PrimeAnchor: "/prime"},
			wantBeamCalls: 1,
			wantPlain:     rowFragment(0, "B"),
			wantAgent:     rowFragment(0, "B"),
			wantEntries:   []buttonSpecEntry{{Index: 0, Label: "B", Run: "b", Ask: "On /beam:"}},
			wantDir:       "ANCHOR",
		},
		{
			name:          "AskHoldingACommaFromAPlaceholderValueIsLeftOutWithAWarn",
			place:         ButtonPlacePrime,
			buttons:       "buttons:\n  beam: [{label: B, run: b, ask: 'On {beam}:'}, {label: C, run: pwd}]\n",
			beam:          ButtonBeam{Anchor: "/b,eam", Name: "bn", PrimeAnchor: "/prime"},
			wantBeamCalls: 1,
			wantPlain:     rowFragment(0, "C"),
			wantAgent:     rowFragment(0, "C"),
			wantEntries:   []buttonSpecEntry{{Index: 0, Label: "C", Run: "pwd"}},
			wantDir:       "ANCHOR",
			wantWarns:     []string{"holds a ','", "/b,eam"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			e.cfg.Shell = "sh"
			if tt.shell != "" {
				e.cfg.Shell = tt.shell
			}
			e.cfg.Buttons = buttonsNode(t, tt.buttons)
			fake := installFakeTmux(t, e)
			logs := logcapture.CaptureVerbose(t)

			beamCalls := 0
			if !tt.nilClosure {
				var machine = buttonsNode(t, "buttons:\n")
				if tt.machine != "" {
					machine = buttonsNode(t, tt.machine)
				}
				e.geom.Buttons = func() (ButtonContext, error) {
					if tt.closureErr != nil {
						return ButtonContext{}, tt.closureErr
					}
					return ButtonContext{
						Place:          tt.place,
						Slug:           "a#b",
						Worktree:       "/wt",
						Session:        "sess",
						MachineButtons: machine,
						Beam: func() (ButtonBeam, error) {
							beamCalls++
							return tt.beam, tt.beamErr
						},
					}, nil
				}
			}

			target := exactSessionWindowTarget(e.SessionName())
			e.pinButtonRowLocked(target)

			wantSpec := buttonSpec{}
			wantSpecJSON, wantDirWord := "", ""
			if len(tt.wantEntries) > 0 {
				dir := tt.wantDir
				if dir == "ANCHOR" {
					dir = e.geom.AnchorPath
				}
				wantSpec = buttonSpec{Shell: shPath, Flag: posix.CommandFlag(), Dir: dir, Entries: tt.wantEntries}
				wantSpecJSON = "decoded below"
				wantDirWord = posix.Quote(dir)
			}
			want := wantButtonRowArgv(target, tt.wantPlain, tt.wantAgent, wantSpecJSON, wantDirWord)

			calls := fake.ArgvFor("set-option")
			if len(calls) != 1 {
				t.Fatalf("pinButtonRowLocked issued %d set-option invocations, want one: %v", len(calls), calls)
			}
			got := slices.Clone(calls[0])
			gotSpecJSON := got[buttonRowSpecSlot]
			if len(tt.wantEntries) > 0 {
				var gotSpec buttonSpec
				if err := json.Unmarshal([]byte(gotSpecJSON), &gotSpec); err != nil {
					t.Fatalf("spec option %q is not JSON: %v", gotSpecJSON, err)
				}
				if !slices.Equal(gotSpec.Entries, wantSpec.Entries) || gotSpec.Shell != wantSpec.Shell || gotSpec.Flag != wantSpec.Flag || gotSpec.Dir != wantSpec.Dir {
					t.Errorf("spec = %+v, want %+v", gotSpec, wantSpec)
				}
				got[buttonRowSpecSlot] = "decoded below"
			}
			if !slices.Equal(got, want) {
				t.Errorf("set-option invocation = %q, want %q", got, want)
			}
			if tt.wantBeamCalls != beamCalls {
				t.Errorf("Beam called %d times, want %d", beamCalls, tt.wantBeamCalls)
			}
			for _, warn := range tt.wantWarns {
				if !strings.Contains(logs.String(), warn) {
					t.Errorf("log = %q, want a Warn containing %q", logs.String(), warn)
				}
			}
			if len(tt.wantWarns) == 0 && logs.String() != "" {
				t.Errorf("log = %q, want nothing logged", logs.String())
			}
		})
	}

	t.Run("ASecondRenderTakesTheChangedConfig", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Shell = "sh"
		e.cfg.Buttons = buttonsNode(t, "buttons:\n  beam: [{label: A, run: a}]\n")
		e.geom.Buttons = func() (ButtonContext, error) { return ButtonContext{Place: ButtonPlaceBeam}, nil }
		fake := installFakeTmux(t, e)
		target := exactSessionWindowTarget(e.SessionName())

		e.pinButtonRowLocked(target)
		e.cfg.Buttons = buttonsNode(t, "buttons:\n  beam: [{label: Z, run: z}]\n")
		e.pinButtonRowLocked(target)

		calls := fake.ArgvFor("set-option")
		if len(calls) != 2 {
			t.Fatalf("set-option invocations = %d, want 2: %v", len(calls), calls)
		}
		if calls[0][4] != rowFragment(0, "A") || calls[1][4] != rowFragment(0, "Z") {
			t.Errorf("plain rows = %q then %q, want the old then the new button", calls[0][4], calls[1][4])
		}
	})

	t.Run("AFailingInvocationWarnsAndDoesNotPanic", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Shell = "sh"
		e.cfg.Buttons = buttonsNode(t, "buttons:\n  beam: [{label: A, run: a}]\n")
		e.geom.Buttons = func() (ButtonContext, error) { return ButtonContext{Place: ButtonPlaceBeam}, nil }
		fake := installFakeTmux(t, e)
		fake.answer("set-option", "", errors.New("boom"))
		logs := logcapture.CaptureVerbose(t)

		e.pinButtonRowLocked(exactSessionWindowTarget(e.SessionName()))

		if !strings.Contains(logs.String(), "failed to pin the button row") {
			t.Errorf("log = %q, want a Warn for the failed invocation", logs.String())
		}
	})
}
