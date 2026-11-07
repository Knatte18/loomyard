// govalues_test.go pins the Go-declared values the stencils name and the host-neutral ban on the twill stencils.
// It also pins the ban on Go verify-tier commands in the planning and implementer stencils.
// The tests carry the `//lym:guard` marker,
// so a card that renames a value outside this package still fails its own card gate.

package stencils

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gatecli"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// yamlKey returns the name part of the yaml tag on field of Report, plus ":".
func yamlKey(t *testing.T, field string) string {
	t.Helper()

	f, ok := reflect.TypeOf(websterengine.Report{}).FieldByName(field)
	if !ok {
		t.Fatalf("websterengine.Report has no field %q", field)
	}
	name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
	return name + ":"
}

// TestStencils_NameGoValues asserts each stencil names the value Go declares for it.
// Renaming either side without the other fails the test.
//
//lym:guard
func TestStencils_NameGoValues(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		stencil string
		value   string
	}{
		{"webster-body-implementer", "`" + strconv.Itoa(gatecli.SlotBusyExit) + "`"},
		{"orch-role", shuttleengine.MessageTail},
		{"shed-template-driver-notify", shuttleengine.MessageTail},
		{"shed-template-driver-notify-watched", shuttleengine.MessageTail},
		{"shed-template-driver", "`" + shedrun.ParkMarkerFileName + "`"},
		{"webster-body-implementer", yamlKey(t, "Status")},
		{"webster-body-implementer", yamlKey(t, "HeadSHA")},
	}
	if len(tests) == 0 {
		t.Fatal("the Go-value table is empty")
	}
	for _, tt := range tests {
		t.Run(tt.stencil+"/"+tt.value, func(t *testing.T) {
			t.Parallel()

			text, ok := shippedStencils(t)[tt.stencil]
			if !ok {
				t.Fatalf("stencil %q is not shipped", tt.stencil)
			}
			if !strings.Contains(string(text), tt.value) {
				t.Errorf("stencil %q does not contain %q", tt.stencil, tt.value)
			}
		})
	}
}

// TestTwillStencils_HostNeutral asserts no twill stencil names a host-specific value,
// so another host can render it with its own values.
//
//lym:guard
func TestTwillStencils_HostNeutral(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	banned := []string{"decision record", "support log", "decision-record.md", "support-log.md", "_lym/discussion", "Step 5", "Step 6", "validate-discussion", "## Interview", "Discussion", "validate-plan", "00-overview.md", "lens-review.md", "Card Index"}
	var twill []string
	for _, name := range Registry().Names() {
		if strings.HasPrefix(name, "twill-") {
			twill = append(twill, name)
		}
	}
	if len(twill) == 0 {
		t.Fatal("the registry holds no twill- stencil")
	}
	for _, name := range twill {
		text, _ := Registry().Default(name)
		lower := strings.ToLower(string(text))
		for _, phrase := range banned {
			if strings.Contains(lower, strings.ToLower(phrase)) {
				t.Errorf("twill stencil %q contains the host-specific phrase %q; every host value arrives through a marker", name, phrase)
			}
		}
	}
}

// TestHubGenericStencils_NameNoGoTiers asserts the planning and implementer stencils name no Go verify-tier command,
// so a hub declaring no tier gets none; a hub with tiers gets them through the gate_tiers marker.
//
//lym:guard
func TestHubGenericStencils_NameNoGoTiers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	banned := []string{"go build", "go vet", "go test", "-tags tmux", "-tags llm", "go -C"}
	for _, name := range []string{"loom-template-plan", "loom-block-plan-glyphs", "loom-block-plan-format", "loom-block-plan-overview", "loom-block-plan-cards", "loom-rubric-plan-review", "webster-body-implementer"} {
		text, ok := Registry().Default(name)
		if !ok {
			t.Fatalf("stencil %q is not registered", name)
		}
		for _, phrase := range banned {
			if strings.Contains(string(text), phrase) {
				t.Errorf("stencil %q contains the Go tier command %q; a hub's tiers arrive through the gate_tiers marker", name, phrase)
			}
		}
	}
}
