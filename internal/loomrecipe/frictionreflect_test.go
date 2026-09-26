package loomrecipe

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/state"
)

// funcProducer is a shedengine.ShedProducer fake that counts its calls, runs an optional hook, and
// reports Done.
type funcProducer struct {
	calls int
	hook  func()
}

func (p *funcProducer) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	p.calls++
	if p.hook != nil {
		p.hook()
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// buildFrictionShed builds loom's Shed over env with the Finalize row's Producer replaced by
// finalize, so the real landing producer never runs.
func buildFrictionShed(t *testing.T, env shedrecipe.Env, paths shedbuild.ShedPaths, finalize shedengine.ShedProducer) *shedengine.Shed {
	t.Helper()
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	for i := range shed.Producers {
		if shed.Producers[i].Name == loomshed.NameFinalize {
			shed.Producers[i].Producer = finalize
			return shed
		}
	}
	t.Fatalf("New() producer list has no row named %q", loomshed.NameFinalize)
	return nil
}

func readStatus(t *testing.T, paths shedbuild.ShedPaths) shedengine.Status {
	t.Helper()
	st, found, err := state.ReadJSONStrict[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
	if err != nil || !found {
		t.Fatalf("ReadJSONStrict(status) found = %v, error = %v; want found and nil", found, err)
	}
	return st
}

func TestFrictionReflect_DonePersistsOnlyAfterReflection(t *testing.T) {
	_, env, paths := buildSequenceFixture(t)
	var observed shedengine.Status
	reflectCalls := 0
	env.ReflectFriction = func() string {
		reflectCalls++
		observed = readStatus(t, paths)
		return "reflected"
	}
	finalize := &funcProducer{}
	shed := buildFrictionShed(t, env, paths, finalize)
	resetCurrentProducer(t, paths.StatusPath, paths.StatusLockPath, loomshed.NameFinalize, false)

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != shedengine.RunDone {
		t.Fatalf("Run() outcome = %q; want %q (reason: %s)", result.Outcome, shedengine.RunDone, result.Reason)
	}
	if observed.CurrentProducer != loomshed.NameFrictionReflect || observed.State != shedengine.StateRunning {
		t.Errorf("status during reflection = %q/%q; want %q/%q", observed.CurrentProducer, observed.State, loomshed.NameFrictionReflect, shedengine.StateRunning)
	}
	final := readStatus(t, paths)
	if final.State != shedengine.StateDone || final.CurrentProducer != loomshed.NameFrictionReflect {
		t.Errorf("final status = %q/%q; want %q/%q", final.CurrentProducer, final.State, loomshed.NameFrictionReflect, shedengine.StateDone)
	}
	if reflectCalls != 1 {
		t.Errorf("reflect closure calls = %d; want 1", reflectCalls)
	}
	n := len(final.History)
	if n < 2 || final.History[n-2].Producer != loomshed.NameFinalize || final.History[n-1].Producer != loomshed.NameFrictionReflect {
		t.Errorf("history tail = %+v; want Finalize then Friction-Reflect", final.History)
	}
}

func TestFrictionReflect_PauseDuringFinalizeHaltsAtTheRow(t *testing.T) {
	_, env, paths := buildSequenceFixture(t)
	reflectCalls := 0
	env.ReflectFriction = func() string {
		reflectCalls++
		return "reflected"
	}
	finalize := &funcProducer{}
	finalize.hook = func() {
		err := state.UpdateJSON(paths.StatusPath, paths.StatusLockPath, func(cur shedengine.Status, _ bool) (shedengine.Status, error) {
			cur.PauseRequested = true
			return cur, nil
		})
		if err != nil {
			t.Errorf("state.UpdateJSON(pause) error = %v", err)
		}
	}
	shed := buildFrictionShed(t, env, paths, finalize)
	resetCurrentProducer(t, paths.StatusPath, paths.StatusLockPath, loomshed.NameFinalize, false)

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("first Run() error = %v; want nil", err)
	}
	if result.Outcome != shedengine.RunPaused || result.HaltedProducer != loomshed.NameFrictionReflect {
		t.Fatalf("first Run() = %q at %q; want %q at %q", result.Outcome, result.HaltedProducer, shedengine.RunPaused, loomshed.NameFrictionReflect)
	}
	st := readStatus(t, paths)
	if st.State != shedengine.StatePaused || st.CurrentProducer != loomshed.NameFrictionReflect {
		t.Errorf("status after pause = %q/%q; want %q/%q", st.CurrentProducer, st.State, loomshed.NameFrictionReflect, shedengine.StatePaused)
	}
	if last := st.History[len(st.History)-1].Producer; last != loomshed.NameFinalize {
		t.Errorf("history last producer = %q; want %q", last, loomshed.NameFinalize)
	}
	if reflectCalls != 0 {
		t.Errorf("reflect closure calls after pause = %d; want 0", reflectCalls)
	}

	shed2 := buildFrictionShed(t, env, paths, finalize)
	result2, err := shed2.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run() error = %v; want nil", err)
	}
	if result2.Outcome != shedengine.RunDone {
		t.Fatalf("second Run() outcome = %q; want %q (reason: %s)", result2.Outcome, shedengine.RunDone, result2.Reason)
	}
	if reflectCalls != 1 {
		t.Errorf("reflect closure calls = %d; want 1", reflectCalls)
	}
	if finalize.calls != 1 {
		t.Errorf("Finalize fake calls = %d; want 1 in total", finalize.calls)
	}
}

func TestFrictionReflect_ResumeAtTheRowCallsOnlyTheRow(t *testing.T) {
	_, env, paths := buildSequenceFixture(t)
	reflectCalls := 0
	env.ReflectFriction = func() string {
		reflectCalls++
		return "reflected"
	}
	finalize := &funcProducer{}
	shed := buildFrictionShed(t, env, paths, finalize)
	resetCurrentProducer(t, paths.StatusPath, paths.StatusLockPath, loomshed.NameFrictionReflect, false)

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != shedengine.RunDone {
		t.Fatalf("Run() outcome = %q; want %q (reason: %s)", result.Outcome, shedengine.RunDone, result.Reason)
	}
	if reflectCalls != 1 {
		t.Errorf("reflect closure calls = %d; want 1", reflectCalls)
	}
	if finalize.calls != 0 {
		t.Errorf("Finalize fake calls = %d; want 0", finalize.calls)
	}
}

func TestFrictionReflect_PreChangeDoneAtFinalizeShortCircuits(t *testing.T) {
	_, env, paths := buildSequenceFixture(t)
	reflectCalls := 0
	env.ReflectFriction = func() string {
		reflectCalls++
		return "reflected"
	}
	finalize := &funcProducer{}
	shed := buildFrictionShed(t, env, paths, finalize)
	err := state.UpdateJSON(paths.StatusPath, paths.StatusLockPath, func(cur shedengine.Status, _ bool) (shedengine.Status, error) {
		cur.CurrentProducer = loomshed.NameFinalize
		cur.State = shedengine.StateDone
		return cur, nil
	})
	if err != nil {
		t.Fatalf("state.UpdateJSON: %v", err)
	}

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != shedengine.RunDone {
		t.Fatalf("Run() outcome = %q; want %q (reason: %s)", result.Outcome, shedengine.RunDone, result.Reason)
	}
	if finalize.calls != 0 || reflectCalls != 0 {
		t.Errorf("Finalize calls = %d, reflect calls = %d; want 0 and 0", finalize.calls, reflectCalls)
	}
}
