//go:build tier1

// hub_integration_test.go drives the front task over a real hub's board: filing, the draft a failed filing leaves, removing and the owner read of settling.

package fronttask_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fronttask"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
)

func TestHubDeps_FilesAndRemovesOnTheHubBoard(t *testing.T) {
	t.Parallel()
	boardkit.InstallReplay(t)

	hub := hubforge.NewHub(t, ".")
	deps, err := fronttask.HubDeps(hub.Location)
	if err != nil {
		t.Fatalf("HubDeps: %v", err)
	}
	board, err := boardengine.OpenHub(hub.Path)
	if err != nil {
		t.Fatalf("open the hub board: %v", err)
	}
	record := func() string {
		data, err := os.ReadFile(deps.RecordPath)
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read the record: %v", err)
		}
		return string(data)
	}

	filed := fronttask.Request{Slug: "front-a", Title: "front a", Body: "body", Labels: []string{"bug"}, OnLand: fronttask.ActionAdoptCheck}
	if err := fronttask.File(deps, filed, nil); err != nil {
		t.Fatalf("File: %v", err)
	}
	task, found, err := board.GetTask("front-a")
	if err != nil || !found {
		t.Fatalf("GetTask(front-a) = found %v, error %v; want the filed entry", found, err)
	}
	if !task.Front || task.OnLand != fronttask.ActionAdoptCheck || !boardengine.IsEmptyStatus(task.Status) {
		t.Errorf("filed entry = front %v, on_land %q, status %v; want a released front task of %q", task.Front, task.OnLand, task.Status, fronttask.ActionAdoptCheck)
	}
	if !strings.Contains(record(), "front-a") {
		t.Errorf("record = %q; want it to hold front-a", record())
	}

	if err := fronttask.Remove(deps, "front-a"); err != nil {
		t.Fatalf("Remove of the never-started entry: %v", err)
	}
	if _, found, err := board.GetTask("front-a"); err != nil || found {
		t.Errorf("GetTask(front-a) after Remove = found %v, error %v; want the entry gone", found, err)
	}
	if strings.Contains(record(), "front-a") {
		t.Errorf("record = %q after Remove; want front-a dropped", record())
	}

	failed := fronttask.Request{Slug: "front-b", Title: "front b", Labels: []string{"bug"}, OnLand: fronttask.ActionAdoptCheck}
	if err := fronttask.File(deps, failed, func() error { return errors.New("between failed") }); err == nil {
		t.Fatal("File with a failing between = nil; want the error")
	}
	tasks, err := deps.Tasks()
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if open, found := fronttask.OpenFront(tasks, ""); found {
		t.Errorf("OpenFront = %q after a failed filing; want none, since the draft entry is closed", open.Slug)
	}
	if draft, found, _ := board.GetTask("front-b"); !found || draft.Status == nil || *draft.Status != boardengine.StatusDraft {
		t.Errorf("front-b after a failed filing = %+v, found %v; want a draft entry", draft, found)
	}
}

func TestHubSettleDeps_OwnedHereReadsTheOwnerOnTheHubBoard(t *testing.T) {
	t.Parallel()
	boardkit.InstallReplay(t)

	here, err := machinename.Name()
	if err != nil {
		t.Fatalf("machinename.Name: %v", err)
	}
	tests := []struct {
		name string
		// beam asks for the settle of the beam project b, whose own recorded owner decides; otherwise the prime's owner decides.
		beam  bool
		owner string
		want  bool
	}{
		{"AnotherMachineOwnsThePrime", false, "not-" + here, false},
		{"ThisMachineOwnsThePrime", false, here, true},
		{"AnotherMachineOwnsTheBeam", true, "not-" + here, false},
		{"ThisMachineOwnsTheBeam", true, here, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			request := boardengine.OwnerRequest{Machine: tt.owner, Prime: true, Takeover: true}
			beam := ""
			if tt.beam {
				beam = "b"
				board, err := boardengine.OpenHub(hub.Path)
				if err != nil {
					t.Fatalf("open the hub board: %v", err)
				}
				if _, err := board.UpsertTask(map[string]any{"slug": beam, "kind": "project", "title": beam, "labels": []string{"bug"}}); err != nil {
					t.Fatalf("seed the beam project: %v", err)
				}
				request = boardengine.OwnerRequest{Machine: tt.owner, Slugs: []string{beam}, Beam: true, Takeover: true}
			}
			if _, err := boardengine.SetOwners(hub.BoardDir(), request); err != nil {
				t.Fatalf("record the owner: %v", err)
			}
			deps, err := fronttask.HubSettleDeps(hub.Location, beam)
			if err != nil {
				t.Fatalf("HubSettleDeps: %v", err)
			}

			got, err := deps.OwnedHere()
			if err != nil {
				t.Fatalf("OwnedHere() error = %v; want nil", err)
			}
			if got != tt.want {
				t.Errorf("OwnedHere() = %v with the prime owned by %q; want %v", got, tt.owner, tt.want)
			}
		})
	}
}
