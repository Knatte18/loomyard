//go:build tier0

// beamrecipe_test.go covers NewBeam and BeamRouting: the beam recipe's two rows, their edges and producer types, the absence of a reroute edge on Finalize, and the refusal of an unfilled description seam.

package loomrecipe

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestNewBeam(t *testing.T) {
	t.Parallel()

	t.Run("BuildsTheTwoRowsAndTheirEdges", func(t *testing.T) {
		t.Parallel()
		env, paths := testEnv(t)
		shed, err := NewBeam(env, paths)
		if err != nil {
			t.Fatalf("NewBeam() error = %v; want nil", err)
		}

		describe, err := landingshed.NewBeamDescribe("Beam-Describe", env.BeamDescribe)
		if err != nil {
			t.Fatalf("landingshed.NewBeamDescribe() error = %v; want nil", err)
		}
		want := []struct {
			name         string
			producerType reflect.Type
			onDone       string
		}{
			{"Beam-Describe", reflect.TypeOf(describe), loomshed.NameFinalize},
			{loomshed.NameFinalize, reflect.TypeOf(&landingshed.Finalize{}), ""},
		}
		if len(shed.Producers) != len(want) {
			t.Fatalf("NewBeam() built %d rows; want %d", len(shed.Producers), len(want))
		}
		for i, w := range want {
			got := shed.Producers[i]
			if got.Name != w.name {
				t.Errorf("row %d name = %q; want %q", i, got.Name, w.name)
			}
			if got.OnDone != w.onDone {
				t.Errorf("row %q OnDone = %q; want %q", w.name, got.OnDone, w.onDone)
			}
			if got.OnStuck != "" || got.OnReroute != "" {
				t.Errorf("row %q OnStuck/OnReroute = %q/%q; want neither", w.name, got.OnStuck, got.OnReroute)
			}
			if reflect.TypeOf(got.Producer) != w.producerType {
				t.Errorf("row %q producer type = %v; want %v", w.name, reflect.TypeOf(got.Producer), w.producerType)
			}
		}
	})

	t.Run("RefusesAMissingBeamDescribeSeam", func(t *testing.T) {
		t.Parallel()
		env, paths := testEnv(t)
		env.BeamDescribe.Entry = nil
		shed, err := NewBeam(env, paths)
		if err == nil || !strings.Contains(err.Error(), "BeamDescribe") {
			t.Fatalf("NewBeam() error = %v; want one naming BeamDescribe", err)
		}
		if shed != nil {
			t.Errorf("NewBeam() shed = %+v; want nil alongside a non-nil error", shed)
		}
	})

	t.Run("RefusesDivergentStatusPaths", func(t *testing.T) {
		t.Parallel()
		env, paths := testEnv(t)
		paths.StatusPath += ".other"
		if _, err := NewBeam(env, paths); err == nil || !strings.Contains(err.Error(), paths.StatusPath) {
			t.Errorf("NewBeam() error = %v; want one naming the divergent StatusPath", err)
		}
	})
}

func TestBeamRouting(t *testing.T) {
	t.Parallel()
	routing, err := BeamRouting()
	if err != nil {
		t.Fatalf("BeamRouting() error = %v; want nil", err)
	}
	if routing.Entry != "Beam-Describe" {
		t.Errorf("BeamRouting().Entry = %q; want %q", routing.Entry, "Beam-Describe")
	}
	got := map[string]shedengine.ProducerDef{}
	for _, def := range routing.Producers {
		got[def.Name] = def
	}
	if len(got) != 2 {
		t.Fatalf("BeamRouting() has rows %v; want Beam-Describe and Finalize", routing.Producers)
	}
	if got["Beam-Describe"].OnDone != loomshed.NameFinalize {
		t.Errorf("Beam-Describe OnDone = %q; want %q", got["Beam-Describe"].OnDone, loomshed.NameFinalize)
	}
	if finalize := got[loomshed.NameFinalize]; finalize.OnDone != "" || finalize.OnReroute != "" {
		t.Errorf("Finalize OnDone/OnReroute = %q/%q; want neither", finalize.OnDone, finalize.OnReroute)
	}
}
