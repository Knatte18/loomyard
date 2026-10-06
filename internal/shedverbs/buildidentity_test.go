// buildidentity_test.go covers binaryChanged's table: it is true only for two known builds that differ in revision or modified flag.

package shedverbs

import "testing"

//testtiming:keep pins the flipped modified flag and the empty-revision rows that never report a change, which its covering test does not
func TestBinaryChanged(t *testing.T) {
	tests := []struct {
		name              string
		running, recorded BuildIdentity
		want              bool
	}{
		{"equal pairs", BuildIdentity{"abc", false}, BuildIdentity{"abc", false}, false},
		{"differing revisions", BuildIdentity{"abc", false}, BuildIdentity{"def", false}, true},
		{"flipped modified flag", BuildIdentity{"abc", true}, BuildIdentity{"abc", false}, true},
		{"empty recorded revision", BuildIdentity{"abc", false}, BuildIdentity{"", true}, false},
		{"empty running revision", BuildIdentity{"", false}, BuildIdentity{"abc", true}, false},
		{"both empty", BuildIdentity{"", true}, BuildIdentity{"", false}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := binaryChanged(tt.running, tt.recorded); got != tt.want {
				t.Errorf("binaryChanged(%+v, %+v) = %v; want %v", tt.running, tt.recorded, got, tt.want)
			}
		})
	}
}
