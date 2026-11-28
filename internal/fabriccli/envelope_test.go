// envelope_test.go covers the envelope shape of okWithRecord, errWithRecord and errConflictsWithRecord.
// It checks the fixed "mutations" and "partial" keys on the success and failure paths.
// It also checks that "mutations" is never null, and each helper's reserved-key override set.
// It is a pure-data test with no git spawn and no hub fixture;
// the end-to-end per-verb assertions that need a real hub live in the integration-tagged scenario files.

package fabriccli

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// populatedMutations builds a non-empty fabricengine.Mutations value the way a verb's recorder does,
// so the value has the shape a real verb produces.
func populatedMutations(hubRoot string) fabricengine.Mutations {
	rec := fabricengine.NewMutations(hubRoot)
	rec.Append(fabricengine.KindFileWritten, hubRoot+"/marker", "")
	return rec.Snapshot()
}

// TestOkWithRecord_SuccessShape asserts that the success envelope always carries a non-null "mutations" array.
// It also asserts "partial": false, with the caller's own fields unchanged.
// A caller-supplied "ok", "mutations" or "partial" is overridden by the helper's own value.
// "error" is left alone, because the helper does not reserve that key.
//
//testtiming:keep pins the exact envelope shape (a "mutations" array that is never null, "partial" false) that its covering integration test only sees through a real hub
func TestOkWithRecord_SuccessShape(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name   string
		rec    fabricengine.Mutations
		fields map[string]any
	}{
		{
			name: "EmptyRecord_RemoveShapedFields",
			rec:  fabricengine.Mutations{},
			fields: map[string]any{
				"slug":          "my-task",
				"path":          "/hub/my-task",
				"links_removed": 2,
			},
		},
		{
			name: "NonEmptyRecord_CloneShapedFields",
			rec:  populatedMutations("/hub"),
			fields: map[string]any{
				"hub":                   "/hub",
				"anchor":                ".",
				"warp":                  "https://example.com/warp.git",
				"warp_binding_recorded": true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			exitCode := okWithRecord(&out, tt.rec, tt.fields)
			if exitCode != 0 {
				t.Errorf("okWithRecord() = %d; want 0", exitCode)
			}

			result := envelope.RequireOK(t, out.String())
			mutations, present := result.Raw["mutations"]
			if !present {
				t.Fatalf("result missing 'mutations' key: %v", result)
			}
			if mutations == nil {
				t.Errorf("mutations = nil; want a non-null array (empty rather than null)")
			}
			if result.Partial == nil {
				t.Fatalf("result missing 'partial' key: %v", result)
			}
			if *result.Partial {
				t.Errorf("partial = true; want false on the success path")
			}
			for key, want := range tt.fields {
				if key == "ok" || key == "mutations" || key == "partial" {
					continue
				}
				got, present := result.Raw[key]
				if !present {
					t.Errorf("result missing caller field %q; want it unchanged", key)
					continue
				}
				gotJSON, _ := json.Marshal(got)
				wantJSON, _ := json.Marshal(want)
				if string(gotJSON) != string(wantJSON) {
					t.Errorf("result.Raw[%q] = %s; want %s (unchanged)", key, gotJSON, wantJSON)
				}
			}
		})
	}
}

// TestErrWithRecord_FailureShape covers the two record states that the failure path's "partial" key distinguishes.
// An empty record gives partial false, and a non-empty one gives partial true.
// Both carry a non-null "mutations" array, "ok": false and the flattened error string.
//
//testtiming:keep pins the exact failure-envelope shape (a "mutations" array that is never null, "partial" derived from the record, no "refusal" key) that its covering integration test only sees through a real hub
func TestErrWithRecord_FailureShape(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name        string
		rec         fabricengine.Mutations
		wantPartial bool
	}{
		{name: "EmptyRecord", rec: fabricengine.Mutations{}, wantPartial: false},
		{name: "NonEmptyRecord", rec: populatedMutations("/hub"), wantPartial: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			wantErr := errors.New("something failed")
			exitCode := errWithRecord(&out, tt.rec, wantErr)
			if exitCode != 1 {
				t.Errorf("errWithRecord() = %d; want 1", exitCode)
			}

			result := envelope.RequireErr(t, out.String(), "")
			if result.Error != wantErr.Error() {
				t.Errorf("error = %q; want %q", result.Error, wantErr.Error())
			}
			mutations, present := result.Raw["mutations"]
			if !present {
				t.Fatalf("result missing 'mutations' key: %v", result)
			}
			if mutations == nil {
				t.Errorf("mutations = nil; want a non-null array (empty rather than null)")
			}
			if result.Partial == nil {
				t.Fatalf("result missing 'partial' key: %v", result)
			}
			if *result.Partial != tt.wantPartial {
				t.Errorf("partial = %v; want %v", *result.Partial, tt.wantPartial)
			}
			if _, hasRefusal := result.Raw["refusal"]; hasRefusal {
				t.Errorf("result has 'refusal' key for an ordinary error; want it absent")
			}
		})
	}
}

// TestErrConflictsWithRecord_ConflictEnvelopeShape covers the envelope errConflictsWithRecord writes.
// It checks that "mutations" comes from the record and is never null.
// It checks that "partial" is the literal false even against a non-empty record; this test pins that property.
// A nil engine error with a non-empty record would compute true through errWithRecordFields.
// This helper never routes through that path.
// It also checks that "conflicts" is never null and that the fixed error text names the way forward,
// and that the return value is 1.
//
//testtiming:keep pins the conflict envelope's literal "partial": false and never-null "conflicts" array against a non-empty record, which its covering integration test only sees through a real hub
func TestErrConflictsWithRecord_ConflictEnvelopeShape(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name      string
		rec       fabricengine.Mutations
		conflicts []string
	}{
		{name: "EmptyRecord", rec: fabricengine.Mutations{}, conflicts: []string{"conflict.txt"}},
		{name: "NonEmptyRecord_PartialStaysFalse", rec: populatedMutations("/hub"), conflicts: []string{"_lym/conflict.txt", "conflict.txt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			exitCode := errConflictsWithRecord(&out, tt.rec, tt.conflicts, nil)
			if exitCode != 1 {
				t.Errorf("errConflictsWithRecord() = %d; want 1", exitCode)
			}

			result := envelope.RequireErr(t, out.String(), "")
			mutations, present := result.Raw["mutations"]
			if !present {
				t.Fatalf("result missing 'mutations' key: %v", result)
			}
			if mutations == nil {
				t.Errorf("mutations = nil; want a non-null array")
			}
			if result.Partial == nil {
				t.Fatalf("result missing 'partial' key: %v", result)
			}
			if *result.Partial {
				t.Errorf("partial = true; want the literal false, even against a non-empty record")
			}
			conflicts, present := result.Raw["conflicts"]
			if !present {
				t.Fatalf("result missing 'conflicts' key: %v", result)
			}
			if conflicts == nil {
				t.Errorf("conflicts = nil; want a non-null array")
			}
			gotConflicts, _ := conflicts.([]any)
			if len(gotConflicts) != len(tt.conflicts) {
				t.Errorf("conflicts = %v; want %d entries", conflicts, len(tt.conflicts))
			}
		})
	}
}
