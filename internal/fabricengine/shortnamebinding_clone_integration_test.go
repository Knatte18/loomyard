//go:build integration

// shortnamebinding_clone_integration_test.go proves CloneHub's repo-shortname rule end to end against real local bare-repo fixtures:
// a fresh bind needs --shortname, the record lands on weft:main beside .lyx-warp, a later clone derives it, a disagreeing shortname is refused,
// and a bound weft without a record either takes a shortname or warns.
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

// TestCloneHub_FreshBindWithoutShortnameRefuses asserts a fresh weft refuses a clone with no shortname, naming the flag, before any hub directory exists.
func TestCloneHub_FreshBindWithoutShortnameRefuses(t *testing.T) {
	fixtures := t.TempDir()
	warpBare := makeBareRemote(t, fixtures, "noshortname-warp")
	weftBare := makeEmptyBareRemote(t, fixtures, "noshortname-weft")

	cloneParent := t.TempDir()
	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: filepath.ToSlash(warpBare),
	})
	if err == nil {
		t.Fatal("CloneHub() on a fresh weft with no shortname = nil error; want a refusal")
	}
	if !strings.Contains(err.Error(), "--shortname") {
		t.Errorf("CloneHub() error = %q; want it to name --shortname", err.Error())
	}
	noProbeResidueInParent(t, cloneParent, "noshortname-warp")
}

// TestCloneHub_ShortnameRecordLifecycle walks one repo through its shortname record:
// recorded at the first clone and committed on weft:main, derived by a second clone, kept by a --reset re-clone, and refused when a different shortname is supplied.
func TestCloneHub_ShortnameRecordLifecycle(t *testing.T) {
	fixtures := t.TempDir()
	warpBare := makeBareRemote(t, fixtures, "life-warp")
	weftBare := makeEmptyBareRemote(t, fixtures, "life-weft")
	weftURL := filepath.ToSlash(weftBare)

	firstParent := t.TempDir()
	first, err := fabriccli.CloneAndWire(firstParent, fabricengine.CloneOptions{
		WeftURL:   weftURL,
		WarpURL:   filepath.ToSlash(warpBare),
		Shortname: "tst",
	})
	if err != nil {
		t.Fatalf("first CloneAndWire() error = %v; want nil", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(first.HubPath) })
	if first.Shortname != "tst" || !first.ShortnameRecorded {
		t.Errorf("first clone Shortname = %q, ShortnameRecorded = %v; want %q, true", first.Shortname, first.ShortnameRecorded, "tst")
	}
	if got := gitOutput(t, weftBare, "show", "main:"+fabricengine.ShortnameFileName); got != "tst" {
		t.Errorf("weft:main %s = %q; want %q", fabricengine.ShortnameFileName, got, "tst")
	}

	t.Run("SecondCloneDerivesShortname", func(t *testing.T) {
		parent := t.TempDir()
		res, err := fabricengine.CloneHub(parent, fabricengine.CloneOptions{WeftURL: weftURL})
		if err != nil {
			t.Fatalf("CloneHub() error = %v; want nil", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })
		if res.Shortname != "tst" || res.ShortnameRecorded || res.Warning != "" {
			t.Errorf("Shortname = %q, ShortnameRecorded = %v, Warning = %q; want %q, false, empty", res.Shortname, res.ShortnameRecorded, res.Warning, "tst")
		}
	})

	t.Run("DisagreeingShortnameRefuses", func(t *testing.T) {
		parent := t.TempDir()
		_, err := fabricengine.CloneHub(parent, fabricengine.CloneOptions{WeftURL: weftURL, Shortname: "other"})
		if err == nil {
			t.Fatal("CloneHub() with a disagreeing shortname = nil error; want a refusal")
		}
		if !strings.Contains(err.Error(), `"tst"`) {
			t.Errorf("CloneHub() error = %q; want it to name the recorded shortname", err.Error())
		}
	})

	t.Run("ResetKeepsRecord", func(t *testing.T) {
		res, err := fabricengine.CloneHub(firstParent, fabricengine.CloneOptions{WeftURL: weftURL, Reset: true})
		if err != nil {
			t.Fatalf("CloneHub(Reset) error = %v; want nil", err)
		}
		if res.Shortname != "tst" {
			t.Errorf("res.Shortname = %q; want %q", res.Shortname, "tst")
		}
		if shortname, found := fabricengine.ReadShortname(res.BoardDir); !found || shortname != "tst" {
			t.Errorf("ReadShortname(board) = %q, %v; want %q, true", shortname, found, "tst")
		}
	})
}

// TestCloneHub_BoundWeftWithoutRecord asserts a weft bound before shortnames existed takes a supplied shortname and records it,
// and succeeds with a warning naming `lyx fabric shortname` when none is supplied.
func TestCloneHub_BoundWeftWithoutRecord(t *testing.T) {
	fixtures := t.TempDir()
	warpBare := makeBareRemote(t, fixtures, "bound-warp")
	weftBare := makeBareRemote(t, fixtures, "bound-weft")
	warpURL := filepath.ToSlash(warpBare)
	seedWeftBinding(t, fixtures, weftBare, warpURL)

	t.Run("WithShortname", func(t *testing.T) {
		res, err := fabricengine.CloneHub(t.TempDir(), fabricengine.CloneOptions{WeftURL: filepath.ToSlash(weftBare), Shortname: "tst"})
		if err != nil {
			t.Fatalf("CloneHub() error = %v; want nil", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })
		if res.Shortname != "tst" || !res.ShortnameRecorded {
			t.Errorf("Shortname = %q, ShortnameRecorded = %v; want %q, true", res.Shortname, res.ShortnameRecorded, "tst")
		}
		if shortname, found := fabricengine.ReadShortname(res.BoardDir); !found || shortname != "tst" {
			t.Errorf("ReadShortname(board) = %q, %v; want %q, true", shortname, found, "tst")
		}
	})

	t.Run("WithoutShortnameWarns", func(t *testing.T) {
		res, err := fabricengine.CloneHub(t.TempDir(), fabricengine.CloneOptions{WeftURL: filepath.ToSlash(weftBare)})
		if err != nil {
			t.Fatalf("CloneHub() error = %v; want nil", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })
		if res.Shortname != "" || res.ShortnameRecorded {
			t.Errorf("Shortname = %q, ShortnameRecorded = %v; want empty, false", res.Shortname, res.ShortnameRecorded)
		}
		if !strings.Contains(res.Warning, "lyx fabric shortname") {
			t.Errorf("Warning = %q; want it to name `lyx fabric shortname`", res.Warning)
		}
	})
}
