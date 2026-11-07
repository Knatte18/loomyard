// gatefixer_test.go covers the row gate fixer wiring.
// It checks the visit directory a fixer's records live in and the seam that wraps a gate with a fixer.
// It also checks the launch spec each gate entry's fixer gets.

package loomcli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// testGateFixerInputs returns the inputs of a pair run over a gate config declaring tier0 and the darn-write key.
// The registry holds one alias, and the stencils directory is seeded.
func testGateFixerInputs(t *testing.T) gateFixerInputs {
	t.Helper()
	gate := gateslot.Config{
		FixerTimeoutMin: 7,
		Tiers:           map[string]gateslot.Tier{"tier0": {About: "unit"}},
		Gates:           map[string]gateslot.GateKey{darnWriteGateKey: {Tiers: []int{0}, Scope: "diff"}},
	}
	dir := t.TempDir()
	return gateFixerInputs{
		pairSlug:           "pair",
		registry:           modelspec.Registry{"opus": {Engine: "claude", Model: "claude-opus-test"}},
		gate:               gate,
		stencilsDir:        stencilkit.Seed(t),
		scratchDir:         filepath.Join(dir, "scratch"),
		statusPath:         filepath.Join(dir, "status.json"),
		statusLockPath:     filepath.Join(dir, "status.json.lock"),
		worktreeRoot:       filepath.Join(dir, "worktree"),
		decisionRecordPath: filepath.Join(dir, "decision-record.md"),
		supportLogPath:     filepath.Join(dir, "support-log.md"),
		relevantPath:       filepath.Join(dir, "relevant.md"),
		planDir:            filepath.Join(dir, "plan"),
		lensReviewPath:     filepath.Join(dir, "plan", "lens-review.md"),
	}
}

// TestWriterFixerOn covers when a writer's prompt drops its validator loop.
// It sets a pair slug, a gate element that names a fixer setting, and a non-empty value in that setting.
// A failed lookup is returned.
func TestWriterFixerOn(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	settings := func(value any, err error) func(module, key string) (any, error) {
		return func(module, key string) (any, error) {
			if module != "tapestry" || key != "plan_gate_fixer" {
				t.Errorf("setting(%q, %q); want the plan gate's own setting tapestry.plan_gate_fixer", module, key)
			}
			return value, err
		}
	}
	tests := []struct {
		name    string
		engine  string
		gate    string
		setting func(module, key string) (any, error)
		slug    string
		want    bool
		wantErr bool
	}{
		{name: "on with a slug, a named setting and a value", engine: planWriteEngine, gate: planGateEntry, setting: settings("sonnet[medium]", nil), slug: "pair", want: true},
		{name: "off with an empty slug whatever the setting", engine: planWriteEngine, gate: planGateEntry, setting: settings("sonnet[medium]", nil), slug: ""},
		{name: "off with an empty value", engine: planWriteEngine, gate: planGateEntry, setting: settings("", nil), slug: "pair"},
		{name: "off for an element with no fixer", engine: planWriteEngine, gate: "no-such-gate", setting: settings("sonnet[medium]", nil), slug: "pair"},
		{name: "a lookup error is returned", engine: planWriteEngine, gate: planGateEntry, setting: settings(nil, errors.New("unreadable")), slug: "pair", wantErr: true},
		{name: "a non-string value is returned as an error", engine: planWriteEngine, gate: planGateEntry, setting: settings(7, nil), slug: "pair", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := writerFixerOn("", tt.engine, tt.gate, tt.setting, tt.slug)
			if (err != nil) != tt.wantErr {
				t.Fatalf("writerFixerOn(%s, %s) error = %v; want error %v", tt.engine, tt.gate, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("writerFixerOn(%s, %s) = %v; want %v", tt.engine, tt.gate, got, tt.want)
			}
		})
	}
}

func TestRowVisitDir(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	in := testGateFixerInputs(t)
	writeStatus := func(history ...shedengine.HistoryEntry) {
		t.Helper()
		if err := state.WriteJSON(in.statusPath, in.statusLockPath, shedengine.Status{History: history}); err != nil {
			t.Fatalf("write the status: %v", err)
		}
	}
	visitDir := func() string {
		t.Helper()
		dir, err := rowVisitDir(in.scratchDir, in.statusPath, in.statusLockPath, "Darn-Write")
		if err != nil {
			t.Fatalf("rowVisitDir() error = %v; want nil", err)
		}
		return dir
	}

	if _, err := rowVisitDir(in.scratchDir, in.statusPath, in.statusLockPath, "Darn-Write"); err == nil || !strings.Contains(err.Error(), in.statusPath) {
		t.Errorf("rowVisitDir() over a missing status error = %v; want one naming %s", err, in.statusPath)
	}

	writeStatus(shedengine.HistoryEntry{Producer: "Darn-Write"})
	first := visitDir()
	if want := filepath.Join(in.scratchDir, "gatefix", "Darn-Write-1-0"); first != want {
		t.Errorf("rowVisitDir() = %q; want %q", first, want)
	}
	if again := visitDir(); again != first {
		t.Errorf("rowVisitDir() over an unchanged history = %q; want %q, so a crash re-step finds its fixer", again, first)
	}

	writeStatus(shedengine.HistoryEntry{Producer: "Darn-Write"}, shedengine.HistoryEntry{Producer: "Darn-Write"})
	appended := visitDir()
	if appended == first {
		t.Errorf("rowVisitDir() after an appended history entry = %q; want a new directory", appended)
	}

	writeStatus(shedengine.HistoryEntry{Producer: "Darn-Write"}, shedengine.HistoryEntry{Producer: "Darn-Write", Repeats: 1})
	if folded := visitDir(); folded == appended {
		t.Errorf("rowVisitDir() after a fold into the newest entry = %q; want a new directory", folded)
	}
}

func TestGateFixerSeam(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	model, err := modelspec.Parse("opus")
	if err != nil {
		t.Fatalf("modelspec.Parse() error = %v; want nil", err)
	}
	passing := func() (shuttleengine.GateResult, error) { return shuttleengine.GateResult{Passed: true}, nil }
	request := func(model modelspec.Spec) shedrecipe.GateFixerRequest {
		return shedrecipe.GateFixerRequest{
			Row:   "Darn-Write",
			Entry: "verify",
			Model: model,
			Spec: shuttleengine.GateSpec{
				{Name: "clean", Gate: passing, Attempts: 1},
				{Name: "verify", Gate: passing, Attempts: 2},
			},
		}
	}

	t.Run("EmptySlugReturnsTheSpecUnchanged", func(t *testing.T) {
		t.Parallel()
		in := testGateFixerInputs(t)
		in.pairSlug = ""
		got, err := gateFixerSeam(in)(request(model))
		if err != nil {
			t.Fatalf("seam error = %v; want nil", err)
		}
		if len(got) != 2 || got[1].MayHold || got[1].Final != nil {
			t.Errorf("seam = %+v; want the spec as it came", got)
		}
	})

	t.Run("PairRunWrapsTheFixerEntry", func(t *testing.T) {
		t.Parallel()
		got, err := gateFixerSeam(testGateFixerInputs(t))(request(model))
		if err != nil {
			t.Fatalf("seam error = %v; want nil", err)
		}
		if len(got) != 2 || got[0].Name != "clean" || got[1].Name != "verify" || !got[1].MayHold || got[1].Final == nil {
			t.Errorf("seam = %+v; want the verify entry carrying MayHold and Final", got)
		}
	})

	t.Run("UnknownAliasIsRefusedWithTheWayForward", func(t *testing.T) {
		t.Parallel()
		_, err := gateFixerSeam(testGateFixerInputs(t))(request(modelspec.Spec{Alias: "nope"}))
		if err == nil || !strings.Contains(err.Error(), "nope") || !strings.Contains(err.Error(), "way forward:") {
			t.Errorf("seam error = %v; want one naming the alias and a way forward", err)
		}
	})
}

func TestFixerSpec(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	in := testGateFixerInputs(t)
	darnCall, err := in.gate.AgentGateCall(darnWriteGateKey)
	if err != nil {
		t.Fatalf("AgentGateCall() error = %v; want nil", err)
	}
	choice := shuttleengine.ModelChoice{Engine: "claude", Model: "claude-opus-test"}
	tests := []struct {
		name    string
		entry   string
		forked  bool
		segment segmentcolor.Segment
		want    []string
		notWant []string
	}{
		{"DarnVerify", "verify", false, segmentcolor.Darn, []string{darnCall, in.worktreeRoot, in.decisionRecordPath}, nil},
		{"PlanUnforked", "plan", false, segmentcolor.Plan, []string{planCheckCommand, in.planDir, in.lensReviewPath}, []string{"--lens-review "}},
		{"PlanForked", "plan", true, segmentcolor.Plan, []string{planLensReviewCheckCommand, in.lensReviewPath}, nil},
		{"Discussion", "discussion", false, segmentcolor.Discussion, []string{discussionCheckCommand, in.supportLogPath, in.relevantPath}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := shedrecipe.GateFixerRequest{Row: "Row", Entry: tt.entry, Forked: tt.forked}
			build, err := fixerSpec(in, req, choice)
			if err != nil {
				t.Fatalf("fixerSpec() error = %v; want nil", err)
			}
			spec, err := build("/lym/report.md", "/lym/reply.md")
			if err != nil {
				t.Fatalf("spec builder error = %v; want nil", err)
			}
			if spec.Role != gatefixer.RoleGateFixer || spec.Timeout != in.gate.FixerTimeout() || spec.ModelChoice != choice || spec.Segment != tt.segment {
				t.Errorf("spec = role %q, timeout %v, choice %+v, segment %q; want role %q, timeout %v, choice %+v, segment %q",
					spec.Role, spec.Timeout, spec.ModelChoice, spec.Segment, gatefixer.RoleGateFixer, in.gate.FixerTimeout(), choice, tt.segment)
			}
			if spec.ReportPath != "/lym/report.md" || spec.ReplyPath != "/lym/reply.md" {
				t.Errorf("spec paths = %q, %q; want the told report and reply", spec.ReportPath, spec.ReplyPath)
			}
			for _, want := range append([]string{"/lym/report.md", "/lym/reply.md"}, tt.want...) {
				if !strings.Contains(string(spec.Prompt), want) {
					t.Errorf("prompt lacks %q", want)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(string(spec.Prompt), notWant) {
					t.Errorf("prompt holds %q; want it absent", notWant)
				}
			}
		})
	}

	t.Run("EntryOutsideTheTableIsRefused", func(t *testing.T) {
		t.Parallel()
		_, err := fixerSpec(in, shedrecipe.GateFixerRequest{Row: "Row", Entry: "clean"}, choice)
		if err == nil || !strings.Contains(err.Error(), `"clean"`) {
			t.Errorf("fixerSpec() error = %v; want one naming the entry", err)
		}
	})
}
