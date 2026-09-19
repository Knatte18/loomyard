// entries_batten_test.go covers the three batten entries: worktreeCreateEntry, innerRunEntry,
// and worktreeTeardownEntry. It follows entries_simple_test.go's table shape for the shared
// Slug/ScratchDir/seam validation, plus innerRunEntry's own poll_interval_s/poll_attempts config
// coverage.
//
// Every seam the three batten entries validate -- CreateWorktree, PrimeLock.Acquire,
// Teardown.Shutdown, Teardown.Remove, InnerRun.Spawn, InnerRun.ResolveStatus, and InnerRun.ReadStatus
// -- is a concrete func type, not an interface, so there is no separate typed-nil-interface case to
// exercise beyond the plain-nil case requireSeam handles for a reflect.Func value: a nil func value
// passed as any already reports Kind() == reflect.Func with IsNil() true, the same detection path a
// typed-nil interface takes.

package shedrecipe

import (
	"strings"
	"testing"
)

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

// TestInnerRunEntry_NilNowAndSleepAreAccepted asserts innerRunEntry does not validate
// Env.InnerRun.Now or Env.InnerRun.Sleep: their nil values are legitimate and select the production
// clock and sleep inside battenshed.NewInnerRun.
func TestInnerRunEntry_NilNowAndSleepAreAccepted(t *testing.T) {
	env := newTestEnv(t)
	if env.InnerRun.Now != nil {
		t.Fatalf("newTestEnv(t).InnerRun.Now is non-nil; want nil by default")
	}
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

// TestInnerRunEntry_PollConfigKeys covers innerRunEntry's poll_interval_s and poll_attempts config
// keys: both absent resolve to defaultInnerRunPollIntervalS and defaultInnerRunPollAttempts, an
// explicit value for either builds successfully, and a negative value for either is rejected naming
// that key.
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

	t.Run("ExplicitPollAttempts", func(t *testing.T) {
		producer, err := innerRunEntry("InnerRun", Config{"poll_attempts": 3}, newTestEnv(t))
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

	t.Run("NegativePollAttemptsIsRejected", func(t *testing.T) {
		_, err := innerRunEntry("InnerRun", Config{"poll_attempts": -1}, newTestEnv(t))
		if err == nil {
			t.Fatalf("innerRunEntry() error = nil; want non-nil for a negative poll_attempts")
		}
		if !strings.Contains(err.Error(), "poll_attempts") {
			t.Errorf("innerRunEntry() error = %v; want it to name the offending key %q", err, "poll_attempts")
		}
	})
}
