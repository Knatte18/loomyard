// proposal_test.go covers Parse over a valid proposal, an unknown key at two depths, a malformed file and a value of the wrong type, and Hash.

package raddleparser

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestParse covers a valid proposal and each refusal, which must name the offending key or form and a way forward.
func TestParse(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	valid, err := json.Marshal(validProposal())
	if err != nil {
		t.Fatalf("marshal the valid proposal: %v", err)
	}
	tests := []struct {
		name    string
		data    string
		wantErr []string
	}{
		{"ValidProposal", string(valid), nil},
		{"UnknownTopLevelKey", `{"sketch":"s","colour":"red"}`, []string{`"colour"`, "way forward"}},
		{"UnknownKeyInAPart", `{"parts":[{"slug":"a","priority":1}]}`, []string{`"priority"`, "parts[0]", "way forward"}},
		{"MalformedFile", `{"sketch":`, []string{"not valid JSON", "way forward"}},
		{"ValueOfTheWrongType", `{"sketch":3}`, []string{"does not fit", "way forward"}},
		{"NotAnObject", `[]`, []string{"does not fit", "way forward"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse([]byte(tt.data))
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Parse() error = %v; want nil", err)
				}
				if want := validProposal(); !reflect.DeepEqual(got, want) {
					t.Errorf("Parse() = %+v; want %+v", got, want)
				}
				return
			}
			if err == nil {
				t.Fatalf("Parse() = %+v, nil; want an error", got)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Parse() error = %q; want it to contain %q", err, want)
				}
			}
		})
	}
}

// TestHash pins the approval hash to the hex SHA-256 of the bytes.
func TestHash(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := Hash([]byte("abc")); got != want {
		t.Errorf("Hash(%q) = %q; want %q", "abc", got, want)
	}
}
