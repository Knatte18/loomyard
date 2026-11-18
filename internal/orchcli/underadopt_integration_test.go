// underadopt_integration_test.go pins that UnderOwnHold judges the hub's Bolt holds.
// It judges them against this machine and the worktree's subject.
// It covers the prime, a beam named by a hold, a beam outside the hold's subjects, and the boards that cover nothing.

package orchcli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestUnderOwnHold(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const foreign = "other-machine"
	here, err := machinename.ReadIdentity()
	if err != nil {
		t.Fatalf("ReadIdentity() = %v; want nil", err)
	}
	tests := []struct {
		name     string
		machine  string
		subjects []string
		corrupt  bool
		slug     string
		want     bool
	}{
		{name: "AHoldOfThisMachineCoversThePrime", machine: here.Name, subjects: []string{boardengine.RootSubject}, slug: "", want: true},
		{name: "AHoldOfThisMachineCoversABeamSubject", machine: here.Name, subjects: []string{"b"}, slug: "b", want: true},
		{name: "ABeamOutsideTheHoldsSubjectsIsNotCovered", machine: here.Name, subjects: []string{boardengine.RootSubject}, slug: "b", want: false},
		{name: "AnotherMachinesHoldCoversNothing", machine: foreign, subjects: []string{boardengine.RootSubject}, slug: "", want: false},
		{name: "NoHoldCoversNothing", slug: "", want: false},
		{name: "AnUndecodableBoardCoversNothing", machine: here.Name, subjects: []string{boardengine.RootSubject}, corrupt: true, slug: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			boardkit.InstallReplay(t)
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
			cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
			if err != nil {
				t.Fatalf("load board config: %v", err)
			}
			cfg.Path = h.BoardDir()
			if _, err := boardengine.New(cfg).UpsertTask(map[string]any{"slug": "b", "kind": "project", "title": "b", "labels": []string{"bug"}}); err != nil {
				t.Fatalf("seed the board entry b: %v", err)
			}
			if _, err := boardengine.SetOwners(h.BoardDir(), boardengine.OwnerRequest{Machine: here.Name, Slugs: []string{"b"}, Beam: true}); err != nil {
				t.Fatalf("flag b as a beam: %v", err)
			}
			if tt.machine != "" {
				hold := boardengine.Hold{Kind: boardengine.HoldKindAdopt, Machine: tt.machine, At: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)}
				for _, subject := range tt.subjects {
					hold.Subjects = append(hold.Subjects, boardengine.HoldSubject{Subject: subject})
				}
				if written, err := boardengine.WriteHold(h.BoardDir(), hold); err != nil || !written.Wrote {
					t.Fatalf("WriteHold() = %+v, %v; want Wrote", written, err)
				}
			}
			if tt.corrupt {
				if err := os.WriteFile(filepath.Join(h.BoardDir(), "board.json"), []byte("not json"), 0o644); err != nil {
					t.Fatalf("corrupt board.json: %v", err)
				}
			}

			if got := UnderOwnHold(h.Location)(tt.slug); got != tt.want {
				t.Errorf("UnderOwnHold()(%q) = %v; want %v", tt.slug, got, tt.want)
			}
		})
	}
}
