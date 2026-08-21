// envelope_test.go covers okWithRecord/errWithRecord's shape directly against an io.Writer buffer:
// the fixed mutations/partial key pair on both the success and the failure path, the never-null
// "mutations" array, and each helper's own reserved-key override set. It is a pure-data test with no
// git spawn and no hub fixture, per the Test Tier Purity Shared Decision — the end-to-end per-verb
// assertions that need a real hub live in the integration-tagged cli_test.go instead.

package fabriccli

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// decodeEnvelope parses one line of JSON envelope output into a generic map.
func decodeEnvelope(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON output: %v\noutput: %s", err, buf.String())
	}
	return result
}

// populatedMutations builds a non-empty fabricengine.Mutations value the way a verb's own recorder
// would: NewMutations, one Append, then Snapshot to freeze it into the value form the helpers take.
func populatedMutations(hubRoot string) fabricengine.Mutations {
	rec := fabricengine.NewMutations(hubRoot)
	rec.Append(fabricengine.KindFileWritten, hubRoot+"/marker", "")
	return rec.Snapshot()
}

// TestOkWithRecord_SuccessShape asserts the success envelope always carries a non-null "mutations"
// array and "partial": false, alongside the caller's own fields unchanged — backward compatibility of
// the success envelope is an explicit assertion, not an assumption.
func TestOkWithRecord_SuccessShape(t *testing.T) {
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

			result := decodeEnvelope(t, &out)
			if ok, _ := result["ok"].(bool); !ok {
				t.Errorf("ok = %v; want true", result["ok"])
			}
			mutations, present := result["mutations"]
			if !present {
				t.Fatalf("result missing 'mutations' key: %v", result)
			}
			if mutations == nil {
				t.Errorf("mutations = nil; want a non-null array (empty rather than null)")
			}
			partial, present := result["partial"]
			if !present {
				t.Fatalf("result missing 'partial' key: %v", result)
			}
			if partial != false {
				t.Errorf("partial = %v; want false on the success path", partial)
			}
			for key, want := range tt.fields {
				if key == "mutations" || key == "partial" {
					continue
				}
				got, present := result[key]
				if !present {
					t.Errorf("result missing caller field %q; want it unchanged", key)
					continue
				}
				gotJSON, _ := json.Marshal(got)
				wantJSON, _ := json.Marshal(want)
				if string(gotJSON) != string(wantJSON) {
					t.Errorf("result[%q] = %s; want %s (unchanged)", key, gotJSON, wantJSON)
				}
			}
		})
	}
}

// TestErrWithRecord_FailureShape covers the two record states the failure path's "partial" derivation
// distinguishes: an empty record (partial false) and a non-empty one (partial true), both carrying a
// non-null "mutations" array, "ok": false and the flattened error string.
func TestErrWithRecord_FailureShape(t *testing.T) {
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

			result := decodeEnvelope(t, &out)
			if ok, _ := result["ok"].(bool); ok {
				t.Errorf("ok = true; want false")
			}
			errMsg, present := result["error"].(string)
			if !present || errMsg != wantErr.Error() {
				t.Errorf("error = %v; want %q present", result["error"], wantErr.Error())
			}
			mutations, present := result["mutations"]
			if !present {
				t.Fatalf("result missing 'mutations' key: %v", result)
			}
			if mutations == nil {
				t.Errorf("mutations = nil; want a non-null array (empty rather than null)")
			}
			partial, present := result["partial"]
			if !present {
				t.Fatalf("result missing 'partial' key: %v", result)
			}
			if partial != tt.wantPartial {
				t.Errorf("partial = %v; want %v", partial, tt.wantPartial)
			}
			if _, hasRefusal := result["refusal"]; hasRefusal {
				t.Errorf("result has 'refusal' key for an ordinary error; want it absent")
			}
		})
	}
}

// TestOkWithRecord_ReservedKeysOverrideCallerFields asserts okWithRecord overrides a caller-supplied
// "ok", "mutations" or "partial" key, but leaves "error" alone — output.Ok never touches it, so
// okWithRecord does not reserve it either.
func TestOkWithRecord_ReservedKeysOverrideCallerFields(t *testing.T) {
	var out bytes.Buffer
	fields := map[string]any{
		"ok":        "bogus",
		"mutations": "bogus",
		"partial":   "bogus",
		"error":     "caller-supplied, left alone",
	}
	okWithRecord(&out, fabricengine.Mutations{}, fields)

	result := decodeEnvelope(t, &out)
	if ok, _ := result["ok"].(bool); !ok {
		t.Errorf("ok = %v; want the reserved true, not the caller's bogus value", result["ok"])
	}
	if _, isString := result["mutations"].(string); isString {
		t.Errorf("mutations = %v; want the reserved array, not the caller's bogus value", result["mutations"])
	}
	if partial, _ := result["partial"].(bool); partial != false {
		t.Errorf("partial = %v; want the reserved false, not the caller's bogus value", result["partial"])
	}
	if errMsg, _ := result["error"].(string); errMsg != "caller-supplied, left alone" {
		t.Errorf("error = %q; want the caller's own value untouched, since okWithRecord never reserves it", errMsg)
	}
}

// TestErrWithRecord_KeysAreAlwaysTheReservedValues asserts errWithRecord's four keys — "ok", "error",
// "mutations" and "partial" — are always exactly its own computed values. Unlike okWithRecord,
// errWithRecord takes no caller fields map at all (see this file's helpers, and envelope.go's
// signature), so there is no caller-supplied value for it to override; the "reserved" property here is
// that these four keys are the only ones present and are never anything but the helper's own
// derivation, regardless of what rec or err carry.
func TestErrWithRecord_KeysAreAlwaysTheReservedValues(t *testing.T) {
	var out bytes.Buffer
	wantErr := errors.New("the real failure")
	errWithRecord(&out, fabricengine.Mutations{}, wantErr)

	result := decodeEnvelope(t, &out)
	if ok, _ := result["ok"].(bool); ok {
		t.Errorf("ok = %v; want false", result["ok"])
	}
	if errMsg, _ := result["error"].(string); errMsg != wantErr.Error() {
		t.Errorf("error = %q; want %q", errMsg, wantErr.Error())
	}
	if _, isArray := result["mutations"].([]any); !isArray {
		t.Errorf("mutations = %v; want a JSON array", result["mutations"])
	}
	if partial, _ := result["partial"].(bool); partial != false {
		t.Errorf("partial = %v; want false for an empty record", partial)
	}
}

// TestErrConflictsWithRecord_ConflictEnvelopeShape covers the dedicated conflict-envelope helper's
// contract: "mutations" from the record and never null, "partial" the literal false even against a
// non-empty record (the property the Shared Decision exists to pin — a nil engine error with a
// non-empty record would compute true through errWithRecordFields, which this helper never routes
// through), "conflicts" never null, reserved keys winning over any collision, and return value 1.
func TestErrConflictsWithRecord_ConflictEnvelopeShape(t *testing.T) {
	tests := []struct {
		name      string
		rec       fabricengine.Mutations
		conflicts []string
	}{
		{name: "EmptyRecord", rec: fabricengine.Mutations{}, conflicts: []string{"conflict.txt"}},
		{name: "NonEmptyRecord_PartialStaysFalse", rec: populatedMutations("/hub"), conflicts: []string{"_lyx/conflict.txt", "conflict.txt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			exitCode := errConflictsWithRecord(&out, tt.rec, tt.conflicts)
			if exitCode != 1 {
				t.Errorf("errConflictsWithRecord() = %d; want 1", exitCode)
			}

			result := decodeEnvelope(t, &out)
			if ok, _ := result["ok"].(bool); ok {
				t.Errorf("ok = true; want false")
			}
			mutations, present := result["mutations"]
			if !present {
				t.Fatalf("result missing 'mutations' key: %v", result)
			}
			if mutations == nil {
				t.Errorf("mutations = nil; want a non-null array")
			}
			partial, present := result["partial"]
			if !present {
				t.Fatalf("result missing 'partial' key: %v", result)
			}
			if partial != false {
				t.Errorf("partial = %v; want the literal false, even against a non-empty record", partial)
			}
			conflicts, present := result["conflicts"]
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

// TestErrConflictsWithRecord_ReservedKeysAreAlwaysTheHelperOwnValues asserts errConflictsWithRecord
// takes no caller fields map at all, so its four keys — "ok", "error", "mutations", "partial" — plus
// "conflicts" are always exactly its own derivation.
func TestErrConflictsWithRecord_ReservedKeysAreAlwaysTheHelperOwnValues(t *testing.T) {
	var out bytes.Buffer
	exitCode := errConflictsWithRecord(&out, populatedMutations("/hub"), []string{"conflict.txt"})
	if exitCode != 1 {
		t.Errorf("errConflictsWithRecord() = %d; want 1", exitCode)
	}

	result := decodeEnvelope(t, &out)
	wantErr := `merge produced conflicts; resolve each listed path, mark it resolved with "lyx fabric merge-stage <path>...", then run "lyx fabric merge --continue"`
	if errMsg, _ := result["error"].(string); errMsg != wantErr {
		t.Errorf("error = %q; want %q", errMsg, wantErr)
	}
}
