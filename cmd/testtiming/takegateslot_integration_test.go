//go:build tier1

// takegateslot_integration_test.go pins the one slot every testtiming mode holds, over a hubforge hub whose pool it reads.
// It is Tier 2: the hub fixture spawns git, so it needs the integration build tag.

package main

import (
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
)

func TestTakeGateSlot(t *testing.T) {
	t.Parallel()

	t.Run("a mode in a hub holds one testtiming slot until it returns and gets tier0's parallel", func(t *testing.T) {
		t.Parallel()
		const tier0Parallel = 3
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		gateConfig := "slots: 2\ntiers: {tier0: {command: x, timeout_min: 5, counter: vstest, parallel: " + strconv.Itoa(tier0Parallel) + "}}\n"
		if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(gateConfig), 0o644); err != nil {
			t.Fatal(err)
		}
		pool := hubgeom.GateSlots(h.Location)

		ran := false
		err := withGateSlot(h.PrimeWorktree(), "testtiming", func(env []string, parallel int) error {
			ran = true
			holders, err := pool.Holders()
			if err != nil || len(holders) != 1 || holders[0].Site != "testtiming" {
				t.Errorf("Holders() during the mode = (%+v, %v); want one holder under the testtiming site", holders, err)
			}
			if want := "-p=" + strconv.Itoa(tier0Parallel) + " -trimpath"; !hasGoFlagsSuffix(env, want) {
				t.Errorf("mode env GOFLAGS does not end in %q; want the pool's tier0 parallel through HolderEnv", want)
			}
			if parallel != tier0Parallel {
				t.Errorf("mode parallel = %d; want the pool's tier0 parallel %d", parallel, tier0Parallel)
			}
			return nil
		})
		if err != nil || !ran {
			t.Fatalf("withGateSlot() = %v, mode ran %v; want the mode run and no error", err, ran)
		}
		if holders, err := pool.Holders(); err != nil || len(holders) != 0 {
			t.Errorf("Holders() after the mode = (%+v, %v); want no slot held", holders, err)
		}
	})

	t.Run("a mode outside every hub holds no slot and runs under the CPU count", func(t *testing.T) {
		t.Parallel()
		err := withGateSlot(t.TempDir(), "testtiming", func(env []string, parallel int) error {
			if parallel != runtime.NumCPU() {
				t.Errorf("mode parallel = %d; want the CPU count %d", parallel, runtime.NumCPU())
			}
			if !hasGoFlagsSuffix(env, "-trimpath") {
				t.Errorf("mode env GOFLAGS does not end in -trimpath; want it set outside a hub")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("withGateSlot() outside a hub = %v; want none", err)
		}
	})
}

// hasGoFlagsSuffix reports whether env holds a GOFLAGS entry whose value ends in suffix.
func hasGoFlagsSuffix(env []string, suffix string) bool {
	return slices.ContainsFunc(env, func(entry string) bool {
		value, ok := strings.CutPrefix(entry, "GOFLAGS=")
		return ok && strings.HasSuffix(value, suffix)
	})
}
