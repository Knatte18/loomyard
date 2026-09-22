// entries_batten_test.go covers the four batten entries: worktreeCreateEntry, innerRunEntry,
// seedChildEntry, and worktreeTeardownEntry. It follows entries_simple_test.go's table shape for
// the shared Slug/ScratchDir/seam validation, plus innerRunEntry's own poll_interval_s config
// coverage and seedChildEntry's own dedicated table below.
//
// Every seam the four batten entries validate -- CreateWorktree, PrimeLock.Acquire,
// Teardown.Shutdown, Teardown.Remove, InnerRun.Spawn, InnerRun.ResolveStatus, InnerRun.ReadStatus,
// and the five SeedChild closures -- is a concrete func type, not an interface, so there is no
// separate typed-nil-interface case to exercise beyond the plain-nil case requireSeam handles for a
// reflect.Func value: a nil func value passed as any already reports Kind() == reflect.Func with
// IsNil() true, the same detection path a typed-nil interface takes.

package shedrecipe

import (
	"context"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenshed"
)

// newTestEnvWithSeedChild returns newTestEnv(t) with Env.SeedChild filled with five no-op fakes,
// kept local to this file rather than folded into fixture_test.go's own newTestEnv: no other test
// file in this package needs a filled SeedChild, so widening the shared fixture would cost every
// other entry's test a field it never reads.
func newTestEnvWithSeedChild(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	env.SeedChild = battenshed.SeedChildDeps{
		ReadBoardType: func(context.Context) (string, error) { return "loom", nil },
		ChildDriver:   func() (string, error) { return "claude", nil },
		WriteSeed:     func(context.Context, string, string) error { return nil },
		CommitSeed:    func(context.Context) error { return nil },
		PushSeed:      func(context.Context) error { return nil },
	}
	return env
}

// lifecycleEntryCase is one row of the table shared by the happy-path, Slug, ScratchDir, and
// unrecognised-config-key tests below.
type lifecycleEntryCase struct {
	// registryKey is the name this entry is registered under in registry.go.
	registryKey string
	// entry is the Constructor under test.
	entry Constructor
	// zeroSeams lists, per seam this entry validates, a closure returning a copy of env with that
	// seam nilled out and the field name it is validated under -- used to drive the per-seam
	// nil-rejection subtests.
	zeroSeams []lifecycleSeamCase
}

// lifecycleSeamCase names one nil-able seam a lifecycle entry validates.
type lifecycleSeamCase struct {
	field string
	zero  func(env Env) Env
}

func lifecycleEntryCases() []lifecycleEntryCase {
	return []lifecycleEntryCase{
		{
			registryKey: "WorktreeCreate",
			entry:       worktreeCreateEntry,
			zeroSeams: []lifecycleSeamCase{
				{"CreateWorktree", func(env Env) Env { env.CreateWorktree = nil; return env }},
				{"PrimeLock.Acquire", func(env Env) Env { env.PrimeLock.Acquire = nil; return env }},
			},
		},
		{
			registryKey: "WorktreeTeardown",
			entry:       worktreeTeardownEntry,
			zeroSeams: []lifecycleSeamCase{
				{"Teardown.Shutdown", func(env Env) Env { env.Teardown.Shutdown = nil; return env }},
				{"Teardown.Remove", func(env Env) Env { env.Teardown.Remove = nil; return env }},
				{"PrimeLock.Acquire", func(env Env) Env { env.PrimeLock.Acquire = nil; return env }},
			},
		},
		{
			registryKey: "InnerRun",
			entry:       innerRunEntry,
			zeroSeams: []lifecycleSeamCase{
				{"InnerRun.Spawn", func(env Env) Env { env.InnerRun.Spawn = nil; return env }},
				{"InnerRun.ResolveStatus", func(env Env) Env { env.InnerRun.ResolveStatus = nil; return env }},
				{"InnerRun.ReadStatus", func(env Env) Env { env.InnerRun.ReadStatus = nil; return env }},
			},
		},
	}
}

func TestLifecycleEntries_HappyPath(t *testing.T) {
	for _, tt := range lifecycleEntryCases() {
		t.Run(tt.registryKey, func(t *testing.T) {
			producer, err := tt.entry("row-name", Config{}, newTestEnv(t))
			if err != nil {
				t.Fatalf("%s() error = %v; want nil", tt.registryKey, err)
			}
			if producer == nil {
				t.Fatalf("%s() = nil producer; want non-nil", tt.registryKey)
			}
		})
	}
}

func TestLifecycleEntries_RejectsEmptySlug(t *testing.T) {
	for _, tt := range lifecycleEntryCases() {
		t.Run(tt.registryKey, func(t *testing.T) {
			env := newTestEnv(t)
			env.Slug = ""
			_, err := tt.entry("row-name", Config{}, env)
			if err == nil {
				t.Fatalf("%s() error = nil; want non-nil for an empty Env.Slug", tt.registryKey)
			}
			if !strings.Contains(err.Error(), "Slug") {
				t.Errorf("%s() error = %v; want it to name field %q", tt.registryKey, err, "Slug")
			}
		})
	}
}

func TestLifecycleEntries_RejectsBadScratchDir(t *testing.T) {
	for _, tt := range lifecycleEntryCases() {
		t.Run(tt.registryKey+"_Empty", func(t *testing.T) {
			env := newTestEnv(t)
			env.ScratchDir = ""
			_, err := tt.entry("row-name", Config{}, env)
			if err == nil {
				t.Fatalf("%s() error = nil; want non-nil for an empty Env.ScratchDir", tt.registryKey)
			}
			if !strings.Contains(err.Error(), "ScratchDir") {
				t.Errorf("%s() error = %v; want it to name field %q", tt.registryKey, err, "ScratchDir")
			}
		})

		t.Run(tt.registryKey+"_Relative", func(t *testing.T) {
			env := newTestEnv(t)
			env.ScratchDir = "relative/scratch"
			_, err := tt.entry("row-name", Config{}, env)
			if err == nil {
				t.Fatalf("%s() error = nil; want non-nil for a relative Env.ScratchDir", tt.registryKey)
			}
			if !strings.Contains(err.Error(), "ScratchDir") {
				t.Errorf("%s() error = %v; want it to name field %q", tt.registryKey, err, "ScratchDir")
			}
		})
	}
}

func TestLifecycleEntries_RejectsNilSeams(t *testing.T) {
	for _, tt := range lifecycleEntryCases() {
		t.Run(tt.registryKey, func(t *testing.T) {
			for _, seam := range tt.zeroSeams {
				t.Run("Nil"+strings.ReplaceAll(seam.field, ".", "_"), func(t *testing.T) {
					env := seam.zero(newTestEnv(t))
					_, err := tt.entry("row-name", Config{}, env)
					if err == nil {
						t.Fatalf("%s() error = nil; want non-nil when Env.%s is nil", tt.registryKey, seam.field)
					}
					if !strings.Contains(err.Error(), seam.field) {
						t.Errorf("%s() error = %v; want it to name field %q", tt.registryKey, err, seam.field)
					}
				})
			}
		})
	}
}

func TestLifecycleEntries_RejectsUnrecognisedConfigKey(t *testing.T) {
	for _, tt := range lifecycleEntryCases() {
		t.Run(tt.registryKey, func(t *testing.T) {
			_, err := tt.entry("row-name", Config{"bogus_key": "x"}, newTestEnv(t))
			if err == nil {
				t.Fatalf("%s() error = nil; want non-nil for an unrecognised config key", tt.registryKey)
			}
			if !strings.Contains(err.Error(), "bogus_key") {
				t.Errorf("%s() error = %v; want it to name the offending key %q", tt.registryKey, err, "bogus_key")
			}
		})
	}
}

// TestInnerRunEntry_NilSleepIsAccepted asserts innerRunEntry does not validate Env.InnerRun.Sleep:
// its nil value is legitimate and selects the production sleep inside battenshed.NewInnerRun.
func TestInnerRunEntry_NilSleepIsAccepted(t *testing.T) {
	env := newTestEnv(t)
	if env.InnerRun.Sleep != nil {
		t.Fatalf("newTestEnv(t).InnerRun.Sleep is non-nil; want nil by default")
	}
	producer, err := innerRunEntry("InnerRun", Config{}, env)
	if err != nil {
		t.Fatalf("innerRunEntry() error = %v; want nil", err)
	}
	if producer == nil {
		t.Fatalf("innerRunEntry() = nil producer; want non-nil")
	}
}

// TestInnerRunEntry_PollConfigKeys covers innerRunEntry's poll_interval_s config key: absent
// resolves to defaultInnerRunPollIntervalS, an explicit value builds successfully, a negative value
// is rejected naming the key, and the retired poll_attempts key is now rejected as unrecognised
// rather than silently read.
func TestInnerRunEntry_PollConfigKeys(t *testing.T) {
	t.Run("AbsentBuildsSuccessfully", func(t *testing.T) {
		producer, err := innerRunEntry("InnerRun", Config{}, newTestEnv(t))
		if err != nil {
			t.Fatalf("innerRunEntry() error = %v; want nil", err)
		}
		if producer == nil {
			t.Fatalf("innerRunEntry() = nil producer; want non-nil")
		}
	})

	t.Run("ExplicitPollIntervalS", func(t *testing.T) {
		producer, err := innerRunEntry("InnerRun", Config{"poll_interval_s": 30}, newTestEnv(t))
		if err != nil {
			t.Fatalf("innerRunEntry() error = %v; want nil", err)
		}
		if producer == nil {
			t.Fatalf("innerRunEntry() = nil producer; want non-nil")
		}
	})

	t.Run("NegativePollIntervalSIsRejected", func(t *testing.T) {
		_, err := innerRunEntry("InnerRun", Config{"poll_interval_s": -1}, newTestEnv(t))
		if err == nil {
			t.Fatalf("innerRunEntry() error = nil; want non-nil for a negative poll_interval_s")
		}
		if !strings.Contains(err.Error(), "poll_interval_s") {
			t.Errorf("innerRunEntry() error = %v; want it to name the offending key %q", err, "poll_interval_s")
		}
	})

	t.Run("PollAttemptsIsRejectedAsUnrecognised", func(t *testing.T) {
		_, err := innerRunEntry("InnerRun", Config{"poll_attempts": 3}, newTestEnv(t))
		if err == nil {
			t.Fatalf("innerRunEntry() error = nil; want non-nil now that poll_attempts is retired")
		}
		if !strings.Contains(err.Error(), "poll_attempts") {
			t.Errorf("innerRunEntry() error = %v; want it to name the offending key %q", err, "poll_attempts")
		}
	})
}

// seedChildLifecycleCase, unlike lifecycleEntryCases' shared table, cannot reuse newTestEnv
// directly -- seedChildEntry needs Env.SeedChild filled, which newTestEnv leaves zero on purpose --
// so seedChildEntry gets its own dedicated table here, matching the shared table's shape field for
// field.
func TestSeedChildEntry_HappyPath(t *testing.T) {
	producer, err := seedChildEntry("SeedChild", Config{}, newTestEnvWithSeedChild(t))
	if err != nil {
		t.Fatalf("seedChildEntry() error = %v; want nil", err)
	}
	if producer == nil {
		t.Fatalf("seedChildEntry() = nil producer; want non-nil")
	}
}

func TestSeedChildEntry_RejectsEmptySlug(t *testing.T) {
	env := newTestEnvWithSeedChild(t)
	env.Slug = ""
	_, err := seedChildEntry("SeedChild", Config{}, env)
	if err == nil {
		t.Fatalf("seedChildEntry() error = nil; want non-nil for an empty Env.Slug")
	}
	if !strings.Contains(err.Error(), "Slug") {
		t.Errorf("seedChildEntry() error = %v; want it to name field %q", err, "Slug")
	}
}

func TestSeedChildEntry_RejectsBadScratchDir(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		env := newTestEnvWithSeedChild(t)
		env.ScratchDir = ""
		_, err := seedChildEntry("SeedChild", Config{}, env)
		if err == nil {
			t.Fatalf("seedChildEntry() error = nil; want non-nil for an empty Env.ScratchDir")
		}
		if !strings.Contains(err.Error(), "ScratchDir") {
			t.Errorf("seedChildEntry() error = %v; want it to name field %q", err, "ScratchDir")
		}
	})

	t.Run("Relative", func(t *testing.T) {
		env := newTestEnvWithSeedChild(t)
		env.ScratchDir = "relative/scratch"
		_, err := seedChildEntry("SeedChild", Config{}, env)
		if err == nil {
			t.Fatalf("seedChildEntry() error = nil; want non-nil for a relative Env.ScratchDir")
		}
		if !strings.Contains(err.Error(), "ScratchDir") {
			t.Errorf("seedChildEntry() error = %v; want it to name field %q", err, "ScratchDir")
		}
	})
}

func TestSeedChildEntry_RejectsNilSeams(t *testing.T) {
	seams := []struct {
		field string
		zero  func(env Env) Env
	}{
		{"SeedChild.ReadBoardType", func(env Env) Env { env.SeedChild.ReadBoardType = nil; return env }},
		{"SeedChild.ChildDriver", func(env Env) Env { env.SeedChild.ChildDriver = nil; return env }},
		{"SeedChild.WriteSeed", func(env Env) Env { env.SeedChild.WriteSeed = nil; return env }},
		{"SeedChild.CommitSeed", func(env Env) Env { env.SeedChild.CommitSeed = nil; return env }},
		{"SeedChild.PushSeed", func(env Env) Env { env.SeedChild.PushSeed = nil; return env }},
	}
	for _, seam := range seams {
		t.Run("Nil"+strings.ReplaceAll(seam.field, ".", "_"), func(t *testing.T) {
			env := seam.zero(newTestEnvWithSeedChild(t))
			_, err := seedChildEntry("SeedChild", Config{}, env)
			if err == nil {
				t.Fatalf("seedChildEntry() error = nil; want non-nil when Env.%s is nil", seam.field)
			}
			if !strings.Contains(err.Error(), seam.field) {
				t.Errorf("seedChildEntry() error = %v; want it to name field %q", err, seam.field)
			}
		})
	}
}

func TestSeedChildEntry_RejectsUnrecognisedConfigKey(t *testing.T) {
	_, err := seedChildEntry("SeedChild", Config{"bogus_key": "x"}, newTestEnvWithSeedChild(t))
	if err == nil {
		t.Fatalf("seedChildEntry() error = nil; want non-nil for an unrecognised config key")
	}
	if !strings.Contains(err.Error(), "bogus_key") {
		t.Errorf("seedChildEntry() error = %v; want it to name the offending key %q", err, "bogus_key")
	}
}
