package envelope

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/output"
)

// recordingTB records Fatalf instead of ending the test, by panicking with fatalSignal.
type recordingTB struct {
	testing.TB
	msg string
}

type fatalSignal struct{}

func (r *recordingTB) Helper() {}

func (r *recordingTB) Fatalf(format string, args ...any) {
	r.msg = format
	panic(fatalSignal{})
}

// fatals runs fn and reports whether it called Fatalf on the stub.
func fatals(fn func(tb testing.TB)) (fataled bool, stub *recordingTB) {
	stub = &recordingTB{}
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(fatalSignal); !ok {
				panic(r)
			}
			fataled = true
		}
	}()
	fn(stub)
	return false, stub
}

func TestDecode_OkFields(t *testing.T) {
	var buf bytes.Buffer
	output.Ok(&buf, map[string]any{"name": "x"})

	env := Decode(t, buf.String())
	if !env.OK || env.Error != "" || env.Partial != nil {
		t.Errorf("got %+v, want ok with no error and no partial", env)
	}
	if env.Raw["name"] != "x" {
		t.Errorf("Raw = %v, want name=x", env.Raw)
	}
	if _, has := env.Raw["ok"]; has {
		t.Errorf("Raw = %v, want ok lifted out", env.Raw)
	}
}

func TestDecode_ErrAndErrFields(t *testing.T) {
	var buf bytes.Buffer
	output.Err(&buf, "  boom ")
	env := Decode(t, buf.String())
	if env.OK || env.Error != "boom" {
		t.Errorf("got %+v, want not ok with error boom", env)
	}

	buf.Reset()
	output.ErrFields(&buf, "bad", map[string]any{"partial": true, "n": 2.0})
	env = Decode(t, buf.String())
	if env.Partial == nil || !*env.Partial {
		t.Errorf("Partial = %v, want pointer to true", env.Partial)
	}
	if env.Raw["n"] != 2.0 {
		t.Errorf("Raw = %v, want n=2", env.Raw)
	}
}

func TestDecode_PartialFalseIsPresent(t *testing.T) {
	env := Decode(t, `{"ok":true,"partial":false}`)
	if env.Partial == nil || *env.Partial {
		t.Errorf("Partial = %v, want pointer to false", env.Partial)
	}
}

func TestDecode_Fatals(t *testing.T) {
	for name, out := range map[string]string{
		"malformed":      `{"ok":`,
		"empty":          ``,
		"multi-document": "{\"ok\":true}\n{\"ok\":true}\n",
	} {
		t.Run(name, func(t *testing.T) {
			if fataled, _ := fatals(func(tb testing.TB) { Decode(tb, out) }); !fataled {
				t.Errorf("Decode(%q) did not fail the test", out)
			}
		})
	}
}

func TestRequireOK(t *testing.T) {
	RequireOK(t, `{"ok":true}`)
	if fataled, _ := fatals(func(tb testing.TB) { RequireOK(tb, `{"ok":false,"error":"x"}`) }); !fataled {
		t.Error("RequireOK accepted an error envelope")
	}
}

func TestRequireErr(t *testing.T) {
	env := RequireErr(t, `{"ok":false,"error":"it broke"}`, "broke")
	if env.Error != "it broke" {
		t.Errorf("Error = %q", env.Error)
	}
	if fataled, _ := fatals(func(tb testing.TB) { RequireErr(tb, `{"ok":true}`, "") }); !fataled {
		t.Error("RequireErr accepted an ok envelope")
	}
	fataled, stub := fatals(func(tb testing.TB) { RequireErr(tb, `{"ok":false,"error":"it broke"}`, "missing") })
	if !fataled || !strings.Contains(stub.msg, "does not contain") {
		t.Errorf("RequireErr missing substring: fataled=%v msg=%q", fataled, stub.msg)
	}
}
