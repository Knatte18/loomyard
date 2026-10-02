//go:build integration

// codebinding_clone_integration_test.go proves CloneHub's repo-code rule end to end against real local
// bare-repo fixtures: a fresh bind needs --code, the record lands on weft:main beside .lyx-warp,
// a later clone derives it, a disagreeing code is refused, and a bound weft without a record either
// takes a code or warns.
// The fixture helpers are reused from clone_adopt_test.go and warpbinding_clone_integration_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestCloneHub_FreshBindWithoutCodeRefuses asserts a fresh weft refuses a clone with no code, naming
// the flag, before any hub directory exists.
func TestCloneHub_FreshBindWithoutCodeRefuses(t *testing.T) {
	fixtures := t.TempDir()
	warpBare := makeBareRemote(t, fixtures, "nocode-warp")
	weftBare := makeEmptyBareRemote(t, fixtures, "nocode-weft")

	cloneParent := t.TempDir()
	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: filepath.ToSlash(warpBare),
	})
	if err == nil {
		t.Fatal("CloneHub() on a fresh weft with no code = nil error; want a refusal")
	}
	if !strings.Contains(err.Error(), "--code") {
		t.Errorf("CloneHub() error = %q; want it to name --code", err.Error())
	}
	noProbeResidueInParent(t, cloneParent, "nocode-warp")
}

// TestCloneHub_CodeRecordLifecycle walks one repo through its code record: recorded at the first
// clone and committed on weft:main, derived by a second clone, kept by a --reset re-clone, and
// refused when a different code is supplied.
func TestCloneHub_CodeRecordLifecycle(t *testing.T) {
	fixtures := t.TempDir()
	warpBare := makeBareRemote(t, fixtures, "life-warp")
	weftBare := makeEmptyBareRemote(t, fixtures, "life-weft")
	weftURL := filepath.ToSlash(weftBare)

	firstParent := t.TempDir()
	first, err := fabriccli.CloneAndWire(firstParent, fabricengine.CloneOptions{
		WeftURL: weftURL,
		WarpURL: filepath.ToSlash(warpBare),
		Code:    "tst",
	})
	if err != nil {
		t.Fatalf("first CloneAndWire() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(first.HubPath) })
	if first.Code != "tst" || !first.CodeRecorded {
		t.Errorf("first clone Code = %q, CodeRecorded = %v; want %q, true", first.Code, first.CodeRecorded, "tst")
	}
	if got := gitOutput(t, weftBare, "show", "main:"+fabricengine.CodeFileName); got != "tst" {
		t.Errorf("weft:main %s = %q; want %q", fabricengine.CodeFileName, got, "tst")
	}

	t.Run("SecondCloneDerivesCode", func(t *testing.T) {
		parent := t.TempDir()
		res, err := fabricengine.CloneHub(parent, fabricengine.CloneOptions{WeftURL: weftURL})
		if err != nil {
			t.Fatalf("CloneHub() error = %v; want nil", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })
		if res.Code != "tst" || res.CodeRecorded || res.Warning != "" {
			t.Errorf("Code = %q, CodeRecorded = %v, Warning = %q; want %q, false, empty", res.Code, res.CodeRecorded, res.Warning, "tst")
		}
	})

	t.Run("DisagreeingCodeRefuses", func(t *testing.T) {
		parent := t.TempDir()
		_, err := fabricengine.CloneHub(parent, fabricengine.CloneOptions{WeftURL: weftURL, Code: "other"})
		if err == nil {
			t.Fatal("CloneHub() with a disagreeing code = nil error; want a refusal")
		}
		if !strings.Contains(err.Error(), `"tst"`) {
			t.Errorf("CloneHub() error = %q; want it to name the recorded code", err.Error())
		}
	})

	t.Run("ResetKeepsRecord", func(t *testing.T) {
		res, err := fabricengine.CloneHub(firstParent, fabricengine.CloneOptions{WeftURL: weftURL, Reset: true})
		if err != nil {
			t.Fatalf("CloneHub(Reset) error = %v; want nil", err)
		}
		if res.Code != "tst" {
			t.Errorf("res.Code = %q; want %q", res.Code, "tst")
		}
		if code, found := fabricengine.ReadCode(res.BoardDir); !found || code != "tst" {
			t.Errorf("ReadCode(board) = %q, %v; want %q, true", code, found, "tst")
		}
	})
}

// TestCloneHub_BoundWeftWithoutRecord asserts a weft bound before codes existed takes a supplied code
// and records it, and succeeds with a warning naming `lyx fabric code` when none is supplied.
func TestCloneHub_BoundWeftWithoutRecord(t *testing.T) {
	fixtures := t.TempDir()
	warpBare := makeBareRemote(t, fixtures, "bound-warp")
	weftBare := makeBareRemote(t, fixtures, "bound-weft")
	warpURL := filepath.ToSlash(warpBare)
	seedWeftBinding(t, fixtures, weftBare, warpURL)

	t.Run("WithCode", func(t *testing.T) {
		res, err := fabricengine.CloneHub(t.TempDir(), fabricengine.CloneOptions{WeftURL: filepath.ToSlash(weftBare), Code: "tst"})
		if err != nil {
			t.Fatalf("CloneHub() error = %v; want nil", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })
		if res.Code != "tst" || !res.CodeRecorded {
			t.Errorf("Code = %q, CodeRecorded = %v; want %q, true", res.Code, res.CodeRecorded, "tst")
		}
		if code, found := fabricengine.ReadCode(res.BoardDir); !found || code != "tst" {
			t.Errorf("ReadCode(board) = %q, %v; want %q, true", code, found, "tst")
		}
	})

	t.Run("WithoutCodeWarns", func(t *testing.T) {
		res, err := fabricengine.CloneHub(t.TempDir(), fabricengine.CloneOptions{WeftURL: filepath.ToSlash(weftBare)})
		if err != nil {
			t.Fatalf("CloneHub() error = %v; want nil", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })
		if res.Code != "" || res.CodeRecorded {
			t.Errorf("Code = %q, CodeRecorded = %v; want empty, false", res.Code, res.CodeRecorded)
		}
		if !strings.Contains(res.Warning, "lyx fabric code") {
			t.Errorf("Warning = %q; want it to name `lyx fabric code`", res.Warning)
		}
	})
}
