// entries_simple_test.go is table-driven over the value-only entries simpleEntryCases lists: a happy-path table and an under-filled-Env table, plus the entry-specific subtests entries_simple.go's own doc comments call out.

package shedrecipe

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// nilFabricOpener is a landingshed pair-opener closure fake. It returns a typed-nil
// *fabricengine.Fabric and a nil error, which legally satisfies NewPublish/NewFinalize -- both
// construct their resolver from the interface value the fabric handle is stored behind, and a nil
// check on an interface holding a typed-nil pointer still passes -- without ever dereferencing the
// handle.
func nilFabricOpener() (*fabricengine.Fabric, error) {
	return nil, nil
}

// validLandingDeps returns a landingshed.Deps filled with a synthetic-but-valid told value for
// every field NewPublish/NewFinalize require, deriving every path from t.TempDir().
func validLandingDeps(t *testing.T) landingshed.Deps {
	t.Helper()
	dir := t.TempDir()
	return landingshed.Deps{
		WorktreeRoot:      dir,
		TaskBranch:        "task-branch",
		ParentBranch:      "fixture-parent",
		DescriptionPath:   summaryparser.Path(dir),
		StencilsDir:       dir,
		ScratchDir:        dir,
		OriginURL:         "https://example.invalid/fixture/fixture.git",
		PushBranch:        func() error { return nil },
		RemoteOnlyCommits: func() (string, []string, error) { return "", nil, nil },
		OpenFabric:        nilFabricOpener,
		OpenParentFabric:  nilFabricOpener,
		Shuttle:           &shedfake.MergeShuttle{},
	}
}

// armPRGate adds the three fields landingshed.NewPRGate requires on top of validLandingDeps: both decision-record paths and a TaskHead closure.
func armPRGate(deps *landingshed.Deps) {
	deps.ApprovalPath = filepath.Join(deps.ScratchDir, "approval.json")
	deps.RejectionPath = filepath.Join(deps.ScratchDir, "rejection.json")
	deps.TaskHead = func() (string, error) { return "head", nil }
}

// zeroEnvField returns a copy of env with the named field set to its Go zero value.
// field is either a top-level Env field name, or a dotted WebsterDeps.<Field> name for one of the
// four seams websterEntry validates.
func zeroEnvField(env Env, field string) Env {
	switch field {
	case "Cwd":
		env.Cwd = ""
	case "AnchorPath":
		env.AnchorPath = ""
	case "WorktreeRoot":
		env.WorktreeRoot = ""
	case "StatusPath":
		env.StatusPath = ""
	case "StatusLockPath":
		env.StatusLockPath = ""
	case "StencilsDir":
		env.StencilsDir = ""
	case "RunRoot":
		env.RunRoot = ""
	case "DecisionRecordPath":
		env.DecisionRecordPath = ""
	case "SupportLogPath":
		env.SupportLogPath = ""
	case "WebsterRun":
		env.WebsterRun = nil
	case "CommitWebster":
		env.CommitWebster = nil
	case "WebsterDeps.Starter":
		env.WebsterDeps.Starter = nil
	case "WebsterDeps.Stopper":
		env.WebsterDeps.Stopper = nil
	case "WebsterDeps.Engine":
		env.WebsterDeps.Engine = nil
	case "WebsterDeps.RefMatcher":
		env.WebsterDeps.RefMatcher = nil
	default:
		panic(fmt.Sprintf("zeroEnvField: unknown field %q", field))
	}
	return env
}

// simpleEntryCase is one row of the table entries_simple_test.go's three shared tables drive.
type simpleEntryCase struct {
	// registryKey is the name this entry is registered under in registry.go.
	registryKey string
	// entry is the Constructor under test.
	entry Constructor
	// buildEnv returns a fully valid Env for this entry. It defaults to newTestEnv(t); Publish and
	// Finalize override it because newTestEnv(t) leaves Env.Landing zero, which their underlying
	// constructors reject.
	buildEnv func(t *testing.T) Env
	// validatedFields lists every Env field (or WebsterDeps.<Field> seam) this entry validates, in
	// the same spelling zeroEnvField and the entries' own requireAbsRoot/requireSeam calls use.
	validatedFields []string
	// unreadField names one Env field this entry does not read, used to prove that blanking it
	// still lets construction succeed.
	unreadField string
}

func simpleEntryCases() []simpleEntryCase {
	return []simpleEntryCase{
		{
			registryKey:     "Preflight",
			entry:           preflightEntry,
			buildEnv:        newTestEnv,
			validatedFields: []string{"Cwd"},
			unreadField:     "AnchorPath",
		},
		{
			registryKey: "Publish",
			entry:       publishEntry,
			buildEnv: func(t *testing.T) Env {
				env := newTestEnv(t)
				env.Landing = validLandingDeps(t)
				return env
			},
			validatedFields: nil,
			unreadField:     "Cwd",
		},
		{
			registryKey: "PRGate",
			entry:       prGateEntry,
			buildEnv: func(t *testing.T) Env {
				env := newTestEnv(t)
				env.Landing = validLandingDeps(t)
				armPRGate(&env.Landing)
				return env
			},
			validatedFields: nil,
			unreadField:     "Cwd",
		},
		{
			registryKey: "Finalize",
			entry:       finalizeEntry,
			buildEnv: func(t *testing.T) Env {
				env := newTestEnv(t)
				env.Landing = validLandingDeps(t)
				return env
			},
			validatedFields: nil,
			unreadField:     "Cwd",
		},
		{
			registryKey:     "FrictionReflect",
			entry:           frictionReflectEntry,
			buildEnv:        newTestEnv,
			validatedFields: nil,
			unreadField:     "Cwd",
		},
		{
			registryKey:     "LoomPreflight",
			entry:           loomPreflightEntry,
			buildEnv:        newTestEnv,
			validatedFields: []string{"StatusPath", "StatusLockPath", "AnchorPath"},
			unreadField:     "Cwd",
		},
		{
			registryKey:     "Batchifier",
			entry:           batchifierEntry,
			buildEnv:        newTestEnv,
			validatedFields: []string{"AnchorPath"},
			unreadField:     "Cwd",
		},
		{
			registryKey:     "Stub",
			entry:           stubEntry,
			buildEnv:        newTestEnv,
			validatedFields: nil,
			unreadField:     "Cwd",
		},
		{
			registryKey:     "Webster",
			entry:           websterEntry,
			buildEnv:        newTestEnv,
			validatedFields: []string{"AnchorPath", "WebsterRun", "CommitWebster", "WebsterDeps.Starter", "WebsterDeps.Stopper", "WebsterDeps.Engine", "WebsterDeps.RefMatcher"},
			unreadField:     "Cwd",
		},
	}
}

func TestSimpleEntries_HappyPath(t *testing.T) {
	for _, tt := range simpleEntryCases() {
		t.Run(tt.registryKey, func(t *testing.T) {
			producer, err := tt.entry("row-name", Config{}, tt.buildEnv(t))
			if err != nil {
				t.Fatalf("%s() error = %v; want nil", tt.registryKey, err)
			}
			if producer == nil {
				t.Fatalf("%s() = nil producer; want non-nil", tt.registryKey)
			}
		})
	}
}

func TestSimpleEntries_UnderfilledEnv(t *testing.T) {
	for _, tt := range simpleEntryCases() {
		t.Run(tt.registryKey, func(t *testing.T) {
			for _, field := range tt.validatedFields {
				t.Run("Blank"+strings.ReplaceAll(field, ".", "_"), func(t *testing.T) {
					env := zeroEnvField(tt.buildEnv(t), field)
					_, err := tt.entry("row-name", Config{}, env)
					if err == nil {
						t.Fatalf("%s() error = nil; want non-nil when Env.%s is blank", tt.registryKey, field)
					}
					if !strings.Contains(err.Error(), field) {
						t.Errorf("%s() error = %v; want it to name field %q", tt.registryKey, err, field)
					}
				})
			}

			t.Run("BlankUnreadField_"+tt.unreadField, func(t *testing.T) {
				env := zeroEnvField(tt.buildEnv(t), tt.unreadField)
				producer, err := tt.entry("row-name", Config{}, env)
				if err != nil {
					t.Fatalf("%s() error = %v; want nil when the blanked field (%s) is not one this entry reads", tt.registryKey, err, tt.unreadField)
				}
				if producer == nil {
					t.Fatalf("%s() = nil producer; want non-nil", tt.registryKey)
				}
			})
		})
	}
}

// TestWebsterEntry_SeamFields covers websterEntry's four required WebsterDeps seams beyond the
// shared under-filled-Env table: each nil in turn fails naming that field, and a WebsterDeps with
// Batcher nil (newTestEnv's own default) still constructs successfully.
func TestWebsterEntry_SeamFields(t *testing.T) {
	seamFields := []string{"WebsterDeps.Starter", "WebsterDeps.Stopper", "WebsterDeps.Engine", "WebsterDeps.RefMatcher"}
	for _, field := range seamFields {
		t.Run("Nil"+strings.ReplaceAll(field, ".", "_"), func(t *testing.T) {
			env := zeroEnvField(newTestEnv(t), field)
			_, err := websterEntry("Webster", Config{}, env)
			if err == nil {
				t.Fatalf("websterEntry() error = nil; want non-nil when %s is nil", field)
			}
			if !strings.Contains(err.Error(), field) {
				t.Errorf("websterEntry() error = %v; want it to name field %q", err, field)
			}
		})
	}

	t.Run("BatcherNil", func(t *testing.T) {
		env := newTestEnv(t)
		if env.WebsterDeps.Batcher != nil {
			t.Fatalf("newTestEnv(t).WebsterDeps.Batcher = %v; want nil by default", env.WebsterDeps.Batcher)
		}
		producer, err := websterEntry("Webster", Config{}, env)
		if err != nil {
			t.Fatalf("websterEntry() error = %v; want nil", err)
		}
		if producer == nil {
			t.Fatalf("websterEntry() = nil producer; want non-nil")
		}
	})
}

// TestWebsterEntry_GateKeys covers the "gates" key websterEntry resolves through resolveGateSpec:
// an absent "gates" constructs (the shipped row's own ungated shape), and a named validator is rejected by resolveGateSpec's closed vocabulary, since no Webster validator exists to name.
func TestWebsterEntry_GateKeys(t *testing.T) {
	t.Run("AbsentGateConstructs", func(t *testing.T) {
		producer, err := websterEntry("Webster", Config{}, newTestEnv(t))
		if err != nil {
			t.Fatalf("websterEntry() error = %v; want nil", err)
		}
		if producer == nil {
			t.Fatalf("websterEntry() = nil producer; want non-nil")
		}
	})

	t.Run("UnknownValidatorRejected", func(t *testing.T) {
		_, err := websterEntry("Webster", gatesCfg("webster", 3), newTestEnv(t))
		if err == nil {
			t.Fatalf("websterEntry() error = nil; want non-nil for an unregistered validator name")
		}
		if !strings.Contains(err.Error(), "Webster") {
			t.Errorf("websterEntry() error = %v; want it to name the row", err)
		}
	})
}

// TestPublishEntry_LandingRejected asserts publishEntry surfaces landingshed.NewPublish's own
// rejection of a zero Deps wrapped with this package's own error-text prefix, not passed through
// raw.
func TestPublishEntry_LandingRejected(t *testing.T) {
	env := newTestEnv(t)
	_, err := publishEntry("row-name", Config{}, env)
	if err == nil {
		t.Fatalf("publishEntry() error = nil; want non-nil for a zero Env.Landing")
	}
	if !strings.HasPrefix(err.Error(), "shedrecipe: Publish: ") {
		t.Errorf("publishEntry() error = %v; want it to carry this package's own %q prefix, not a raw pass-through", err, "shedrecipe: Publish: ")
	}
}

// TestPRGateEntry covers prGateEntry's construction: it builds a *landingshed.PRGate from a full Env.Landing, and wraps the constructor's refusal of a nil TaskHead with this package's prefix.
func TestPRGateEntry(t *testing.T) {
	t.Run("BuildsPRGate", func(t *testing.T) {
		env := newTestEnv(t)
		env.Landing = validLandingDeps(t)
		armPRGate(&env.Landing)
		producer, err := prGateEntry("row-name", Config{}, env)
		if err != nil {
			t.Fatalf("prGateEntry() error = %v; want nil", err)
		}
		if _, ok := producer.(*landingshed.PRGate); !ok {
			t.Errorf("prGateEntry() = %T; want *landingshed.PRGate", producer)
		}
	})

	t.Run("NilTaskHead", func(t *testing.T) {
		env := newTestEnv(t)
		env.Landing = validLandingDeps(t)
		armPRGate(&env.Landing)
		env.Landing.TaskHead = nil
		_, err := prGateEntry("row-name", Config{}, env)
		if err == nil {
			t.Fatalf("prGateEntry() error = nil; want non-nil for a nil TaskHead")
		}
		if !strings.HasPrefix(err.Error(), "shedrecipe: PRGate: ") {
			t.Errorf("prGateEntry() error = %v; want the %q prefix", err, "shedrecipe: PRGate: ")
		}
	})
}

// TestFinalizeEntry_LandingRejected is publishEntry's failure-path twin over finalizeEntry.
func TestFinalizeEntry_LandingRejected(t *testing.T) {
	env := newTestEnv(t)
	_, err := finalizeEntry("row-name", Config{}, env)
	if err == nil {
		t.Fatalf("finalizeEntry() error = nil; want non-nil for a zero Env.Landing")
	}
	if !strings.HasPrefix(err.Error(), "shedrecipe: Finalize: ") {
		t.Errorf("finalizeEntry() error = %v; want it to carry this package's own %q prefix, not a raw pass-through", err, "shedrecipe: Finalize: ")
	}
}

// TestBatchifierEntry_RelativeAnchorPath asserts a relative Env.AnchorPath fails, as the
// representative case for every entry's requireAbsRoot validation.
func TestBatchifierEntry_RelativeAnchorPath(t *testing.T) {
	env := newTestEnv(t)
	env.AnchorPath = "relative/path"
	_, err := batchifierEntry("row-name", Config{}, env)
	if err == nil {
		t.Fatalf("batchifierEntry() error = nil; want non-nil for a relative Env.AnchorPath")
	}
	if !strings.Contains(err.Error(), "AnchorPath") {
		t.Errorf("batchifierEntry() error = %v; want it to name field %q", err, "AnchorPath")
	}
}

func TestFrictionReflectEntry_NilClosureRejected(t *testing.T) {
	env := newTestEnv(t)
	env.ReflectFriction = nil
	producer, err := frictionReflectEntry("Friction-Reflect", Config{}, env)
	if err == nil {
		t.Fatal("frictionReflectEntry(nil closure) error = nil; want non-nil")
	}
	if !strings.Contains(err.Error(), "FrictionReflect") {
		t.Errorf("frictionReflectEntry(nil closure) error = %v; want it to name %q", err, "FrictionReflect")
	}
	if producer != nil {
		t.Errorf("frictionReflectEntry(nil closure) producer = %v; want nil", producer)
	}
}
