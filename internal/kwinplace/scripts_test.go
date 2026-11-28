// scripts_test.go pins the input filled into the apply script.

package kwinplace

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestApplyScript_AddressesWindowsByInternalID(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	script := ApplyScript("run1", ApplyOrder{
		TilingOutput: "DP-2",
		Desktop:      "desktop-3",
		Rows:         2,
		Cols:         2,
		Zones:        []string{"{a}", "{b}", "{c}", ""},
		Placed:       "{c}",
	})

	_, rest, found := strings.Cut(script, "const input = ")
	if !found {
		t.Fatal("script holds no input line")
	}
	line, _, _ := strings.Cut(rest, ";\n")
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("input line %q: %v", line, err)
	}
	want := map[string]any{
		"runId":        "run1",
		"tilingOutput": "DP-2",
		"desktop":      "desktop-3",
		"rows":         2.0,
		"cols":         2.0,
		"zones":        []any{"{a}", "{b}", "{c}", ""},
		"placed":       "{c}",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("input = %v; want %v", got, want)
	}
}
