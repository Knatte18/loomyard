// peers_test.go — untagged Tier-1 tests for the peers verbs over fake peersDeps.

package loomcli

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// peersFakeTasks holds one task per status shape the peer set must tell apart.
func peersFakeTasks() []boardengine.Task {
	status := func(s string) *string { return &s }
	return []boardengine.Task{
		{Slug: "running-peer", Title: "Running peer", Status: status("running · Webster-Write")},
		{Slug: "awaiting-peer", Title: "Awaiting peer", Status: status("awaiting · PR-Gate")},
		{Slug: "blocked-peer", Title: "Blocked peer", Status: status("blocked · Plan-Write")},
		{Slug: "idle", Title: "Idle", Status: nil},
		{Slug: "finished", Title: "Finished", Status: status("done")},
		{Slug: "hand-set", Title: "Hand set", Status: status("someday")},
		{Slug: "me", Title: "Me", Status: status("running · Plan-Write")},
	}
}

// peersFakeDeps is a peersDeps over peersFakeTasks whose caller, board and records the test sets.
func peersFakeDeps(caller string) peersDeps {
	return peersDeps{
		board:  func() ([]boardengine.Task, bool, error) { return peersFakeTasks(), true, nil },
		caller: func() (string, error) { return caller, nil },
		readPeer: func(slug string) peerLine {
			if slug == "blocked-peer" {
				return peerLine{Unreadable: []string{"pair: no worktree", "branch: no origin record", "decision: unreadable"}}
			}
			return peerLine{ParentBranch: "main", ChangedPaths: []string{"a.go"}, DecisionRecord: true}
		},
		parentCommits: func() ([]string, error) { return []string{"bbbbbbb second", "aaaaaaa first"}, nil },
		readRecord: func(slug, record string) (string, bool, error) {
			switch {
			case slug == "running-peer" && record == peersRecordDecision:
				return "# Decision record\n", true, nil
			case slug == "running-peer" && record == peersRecordPlan:
				return "# Plan\n", true, nil
			default:
				return "", false, nil
			}
		},
	}
}

// TestPeersVerb verifies the peer set is every run-status task but the caller's own, that a peer with unread parts is kept with the reasons, that the parent's commits appear only from a task pair, and that a hub with no board refuses.
func TestPeersVerb(t *testing.T) {
	t.Parallel()

	t.Run("a task pair lists the run-status peers and its parent's commits in order", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		if code := peersVerb(&out, peersFakeDeps("me")); code != 0 {
			t.Fatalf("exit = %d, out %s; want 0", code, out.String())
		}
		raw := envelope.RequireOK(t, out.String()).Raw

		var slugs []string
		for _, peer := range raw["peers"].([]any) {
			slugs = append(slugs, peer.(map[string]any)["slug"].(string))
		}
		if want := []string{"running-peer", "awaiting-peer", "blocked-peer"}; !reflect.DeepEqual(slugs, want) {
			t.Errorf("peers = %v; want %v", slugs, want)
		}
		first := raw["peers"].([]any)[0].(map[string]any)
		if first["title"] != "Running peer" || first["status"] != "running · Webster-Write" || first["parent_branch"] != "main" || first["decision_record"] != true || first["plan"] != false {
			t.Errorf("first peer = %v; want its title, status, parent branch and record flags", first)
		}
		if want := []any{"a.go"}; !reflect.DeepEqual(first["changed_paths"], want) {
			t.Errorf("first peer changed_paths = %v; want %v", first["changed_paths"], want)
		}
		if want := []any{}; !reflect.DeepEqual(first["unreadable"], want) {
			t.Errorf("first peer unreadable = %v; want an empty list", first["unreadable"])
		}
		blocked := raw["peers"].([]any)[2].(map[string]any)
		if want := []any{"pair: no worktree", "branch: no origin record", "decision: unreadable"}; !reflect.DeepEqual(blocked["unreadable"], want) {
			t.Errorf("blocked peer unreadable = %v; want %v", blocked["unreadable"], want)
		}
		if want := []any{"bbbbbbb second", "aaaaaaa first"}; !reflect.DeepEqual(raw["parent_commits"], want) {
			t.Errorf("parent_commits = %v; want %v", raw["parent_commits"], want)
		}
	})

	t.Run("the prime lists every run-status task and no parent commits", func(t *testing.T) {
		t.Parallel()
		deps := peersFakeDeps("")
		deps.parentCommits = func() ([]string, error) { t.Fatal("parentCommits called from the prime"); return nil, nil }
		var out bytes.Buffer
		if code := peersVerb(&out, deps); code != 0 {
			t.Fatalf("exit = %d, out %s; want 0", code, out.String())
		}
		raw := envelope.RequireOK(t, out.String()).Raw
		if got := len(raw["peers"].([]any)); got != 4 {
			t.Errorf("peers = %d entries; want the four run-status tasks", got)
		}
		if _, present := raw["parent_commits"]; present {
			t.Errorf("parent_commits present from the prime; want it absent")
		}
	})

	t.Run("a hub with no board refuses with the no-board message", func(t *testing.T) {
		t.Parallel()
		deps := peersFakeDeps("me")
		deps.board = func() ([]boardengine.Task, bool, error) { return nil, false, nil }
		var out bytes.Buffer
		if code := peersVerb(&out, deps); code == 0 {
			t.Fatalf("exit = 0, out %s; want a refusal", out.String())
		}
		envelope.RequireErr(t, out.String(), "this hub has no board, so no run is in flight beside this one; way forward: nothing to compare, and a standalone run's review stays with its parent")
	})

	t.Run("a parent-commit read failure refuses naming a way forward", func(t *testing.T) {
		t.Parallel()
		deps := peersFakeDeps("me")
		deps.parentCommits = func() ([]string, error) { return nil, errors.New("boom") }
		var out bytes.Buffer
		if code := peersVerb(&out, deps); code == 0 {
			t.Fatalf("exit = 0, out %s; want a refusal", out.String())
		}
		envelope.RequireErr(t, out.String(), "way forward")
	})
}

// TestPeersShowVerb verifies a peer's decision record and plan are printed, and that each refusal reaches its row with its way forward.
func TestPeersShowVerb(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		slug    string
		record  string
		noBoard bool
		want    string
		wantErr []string
	}{
		{name: "a decision record is printed", slug: "running-peer", record: "decision", want: "# Decision record\n"},
		{name: "a plan is printed", slug: "running-peer", record: "plan", want: "# Plan\n"},
		{name: "an unknown slug", slug: "ghost", record: "plan", wantErr: []string{`unknown slug "ghost"`, `"lyx loom peers" lists the in-flight peers`}},
		{name: "a slug holding no run status", slug: "finished", record: "plan", wantErr: []string{`"finished" holds no run status`, `"lyx loom peers" lists the in-flight peers`}},
		{name: "the caller's own slug", slug: "me", record: "plan", wantErr: []string{"this worktree's own run", "read your own records in this worktree"}},
		{name: "an unknown record name", slug: "running-peer", record: "notes", wantErr: []string{`unknown record "notes"`, "pass --record decision or --record plan"}},
		{name: "a record the peer does not have yet", slug: "awaiting-peer", record: "plan", wantErr: []string{`"awaiting-peer" has no plan yet`, `"lyx loom peers" shows which records exist`}},
		{name: "a hub with no board", slug: "running-peer", record: "plan", noBoard: true, wantErr: []string{"this hub has no board", "nothing to compare"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deps := peersFakeDeps("me")
			if tt.noBoard {
				deps.board = func() ([]boardengine.Task, bool, error) { return nil, false, nil }
			}
			var out bytes.Buffer
			code := peersShowVerb(&out, deps, tt.slug, tt.record)
			if tt.wantErr != nil {
				if code == 0 {
					t.Fatalf("exit = 0, out %s; want a refusal", out.String())
				}
				for _, want := range tt.wantErr {
					envelope.RequireErr(t, out.String(), want)
				}
				return
			}
			if code != 0 {
				t.Fatalf("exit = %d, out %s; want 0", code, out.String())
			}
			raw := envelope.RequireOK(t, out.String()).Raw
			if raw["content"] != tt.want || !strings.EqualFold(raw["record"].(string), tt.record) {
				t.Errorf("envelope = %v; want content %q for record %q", raw, tt.want, tt.record)
			}
		})
	}
}
