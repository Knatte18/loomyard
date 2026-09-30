// commitrecords_test.go drives commitRecordsVerb over stub commitStatusDeps closures, spawning no git.

package loomcli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// stubRecordsDeps builds commitStatusDeps counting Commit and Push calls.
func stubRecordsDeps(active bool, probeErr, commitErr, pushErr error) (commitStatusDeps, *int, *int) {
	commits, pushes := 0, 0
	return commitStatusDeps{
		MergeActive: func() (bool, error) { return active, probeErr },
		Commit:      func(string) error { commits++; return commitErr },
		Push:        func() error { pushes++; return pushErr },
	}, &commits, &pushes
}

func TestCommitRecordsVerb_MidMergeSkipsAndSucceeds(t *testing.T) {
	deps, commits, pushes := stubRecordsDeps(true, nil, nil, nil)
	var out bytes.Buffer
	if code := commitRecordsVerb(&out, deps); code != 0 {
		t.Fatalf("exit = %d; want 0; output %s", code, out.String())
	}
	if *commits != 0 || *pushes != 0 {
		t.Errorf("Commit/Push calls = %d/%d; want 0/0", *commits, *pushes)
	}
	if !strings.Contains(out.String(), `"committed":false`) || !strings.Contains(out.String(), "mid-merge") {
		t.Errorf("output = %s; want committed false with a mid-merge skipped reason", out.String())
	}
}

func TestCommitRecordsVerb_ErrorEnvelopes(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name                       string
		probeErr, commitErr, pushE error
		want                       string
	}{
		{"probe", boom, nil, nil, "probe merge state"},
		{"commit", nil, boom, nil, "commit failed"},
		{"push", nil, nil, boom, "not pushed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _, _ := stubRecordsDeps(false, tt.probeErr, tt.commitErr, tt.pushE)
			var out bytes.Buffer
			if code := commitRecordsVerb(&out, deps); code == 0 {
				t.Fatalf("exit = 0; want non-zero; output %s", out.String())
			}
			if !strings.Contains(out.String(), tt.want) {
				t.Errorf("output = %s; want it to contain %q", out.String(), tt.want)
			}
		})
	}
}

func TestCommitRecordsVerb_OrdinaryPathCommitsThenPushes(t *testing.T) {
	deps, commits, pushes := stubRecordsDeps(false, nil, nil, nil)
	var out bytes.Buffer
	if code := commitRecordsVerb(&out, deps); code != 0 {
		t.Fatalf("exit = %d; want 0; output %s", code, out.String())
	}
	if *commits != 1 || *pushes != 1 {
		t.Errorf("Commit/Push calls = %d/%d; want 1/1", *commits, *pushes)
	}
}
