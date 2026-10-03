package websterengine

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestParseVerifyFailures(t *testing.T) {
	longTail := make([]string, 0, 50)
	for i := 1; i <= 50; i++ {
		longTail = append(longTail, fmt.Sprintf("    line %d", i))
	}
	wantLong := strings.Join(longTail[10:], "\n")

	tests := []struct {
		name   string
		output string
		passed bool
		want   []VerifyFailure
	}{
		{
			name:   "passed yields nil even with FAIL lines",
			output: "--- FAIL: TestA (0.00s)\nFAIL\tpkg/a\t0.1s\n",
			passed: true,
			want:   nil,
		},
		{
			name: "top-level failures across packages",
			output: "--- FAIL: TestA (0.00s)\n    a_test.go:5: boom\nFAIL\nFAIL\tpkg/a\t0.1s\n" +
				"ok  \tpkg/b\t0.1s\n" +
				"--- FAIL: TestC (0.00s)\n    c_test.go:9: bad\nFAIL\nFAIL\tpkg/c\t0.2s\n",
			want: []VerifyFailure{
				{ID: "pkg/a.TestA", Kind: FailureKindTest, Tail: "    a_test.go:5: boom"},
				{ID: "pkg/c.TestC", Kind: FailureKindTest, Tail: "    c_test.go:9: bad"},
			},
		},
		{
			name: "each failing subtest is its own identity",
			output: "--- FAIL: TestA (0.00s)\n    --- FAIL: TestA/one (0.00s)\n        x\n    --- FAIL: TestA/two (0.00s)\n        y\n" +
				"FAIL\nFAIL\tpkg/a\t0.1s\n",
			want: []VerifyFailure{
				{ID: "pkg/a.TestA/one", Kind: FailureKindTest, Tail: "        x"},
				{ID: "pkg/a.TestA/two", Kind: FailureKindTest, Tail: "        y"},
			},
		},
		{
			name: "nested subtests yield the deepest failing name",
			output: "--- FAIL: TestA (0.00s)\n    --- FAIL: TestA/one (0.00s)\n        --- FAIL: TestA/one/deep (0.00s)\n            z\n" +
				"--- FAIL: TestB (0.00s)\n    b_test.go:2: own failure\n" +
				"FAIL\nFAIL\tpkg/a\t0.1s\n",
			want: []VerifyFailure{
				{ID: "pkg/a.TestA/one/deep", Kind: FailureKindTest, Tail: "            z"},
				{ID: "pkg/a.TestB", Kind: FailureKindTest, Tail: "    b_test.go:2: own failure"},
			},
		},
		{
			name: "verbose output nests subtest headers the same way",
			output: "=== RUN   TestA\n=== RUN   TestA/one\n    a_test.go:5: boom\n--- FAIL: TestA (0.00s)\n    --- FAIL: TestA/one (0.00s)\n" +
				"FAIL\nFAIL\tpkg/a\t0.1s\n",
			want: []VerifyFailure{
				{ID: "pkg/a.TestA/one", Kind: FailureKindTest, Tail: ""},
			},
		},
		{
			name:   "subtest header at the wrong depth is log output",
			output: "--- FAIL: TestA (0.00s)\n    a_test.go:3: sub output:\n        --- FAIL: TestA/phantom (0.00s)\n" + "FAIL\nFAIL\tpkg/a\t0.1s\n",
			want: []VerifyFailure{
				{ID: "pkg/a.TestA", Kind: FailureKindTest, Tail: "    a_test.go:3: sub output:\n        --- FAIL: TestA/phantom (0.00s)"},
			},
		},
		{
			name:   "panic after a failing test adds a package identity",
			output: "--- FAIL: TestA (0.00s)\npanic: boom [recovered]\n\tpanic: boom\n\ngoroutine 7 [running]:\nFAIL\tpkg/a\t0.01s\n",
			want: []VerifyFailure{
				{ID: "pkg/a.TestA", Kind: FailureKindTest, Tail: ""},
				{ID: "pkg/a", Kind: FailureKindPackage, Tail: "--- FAIL: TestA (0.00s)\npanic: boom [recovered]\n\tpanic: boom\n\ngoroutine 7 [running]:\nFAIL\tpkg/a\t0.01s"},
			},
		},
		{
			name:   "timeout is a package identity",
			output: "ok  \tpkg/b\t0.1s\npanic: test timed out after 10m0s\n\trunning tests:\n\t\tTestSlow (10m0s)\nFAIL\tpkg/a\t600.0s\n",
			want: []VerifyFailure{
				{ID: "pkg/a", Kind: FailureKindPackage, Tail: "panic: test timed out after 10m0s\n\trunning tests:\n\t\tTestSlow (10m0s)\nFAIL\tpkg/a\t600.0s"},
			},
		},
		{
			name:   "failing tests without the binary's FAIL summary add a package identity",
			output: "--- FAIL: TestA (0.00s)\n    a_test.go:5: boom\nexit status 1\nFAIL\tpkg/a\t0.01s\n",
			want: []VerifyFailure{
				{ID: "pkg/a.TestA", Kind: FailureKindTest, Tail: "    a_test.go:5: boom"},
				{ID: "pkg/a", Kind: FailureKindPackage, Tail: "--- FAIL: TestA (0.00s)\n    a_test.go:5: boom\nexit status 1\nFAIL\tpkg/a\t0.01s"},
			},
		},
		{
			name:   "TestMain failure with every test passing is a package identity",
			output: "PASS\nteardown failed\nFAIL\tpkg/a\t0.01s\n",
			want: []VerifyFailure{
				{ID: "pkg/a", Kind: FailureKindPackage, Tail: "PASS\nteardown failed\nFAIL\tpkg/a\t0.01s"},
			},
		},
		{
			name: "indented FAIL header in a test's log output is not an identity",
			output: "--- FAIL: TestA (0.00s)\n    a_test.go:3: sub output:\n        --- FAIL: TestPhantom (0.00s)\n    a_test.go:3: boom\n" +
				"FAIL\nFAIL\tpkg/a\t0.1s\n",
			want: []VerifyFailure{
				{ID: "pkg/a.TestA", Kind: FailureKindTest, Tail: "    a_test.go:3: sub output:\n        --- FAIL: TestPhantom (0.00s)\n    a_test.go:3: boom"},
			},
		},
		{
			name:   "build failed is a package identity",
			output: "# pkg/a\npkg/a/a.go:3: undefined: x\nFAIL\tpkg/a [build failed]\n",
			want: []VerifyFailure{
				{ID: "pkg/a", Kind: FailureKindPackage, Tail: "# pkg/a\npkg/a/a.go:3: undefined: x\nFAIL\tpkg/a [build failed]"},
			},
		},
		{
			name:   "setup failed is a package identity",
			output: "pkg/a/a_test.go:1: no such file\nFAIL\tpkg/a [setup failed]\n",
			want: []VerifyFailure{
				{ID: "pkg/a", Kind: FailureKindPackage, Tail: "pkg/a/a_test.go:1: no such file\nFAIL\tpkg/a [setup failed]"},
			},
		},
		{
			name:   "panic without FAIL line is a package identity",
			output: "panic: init blew up\n\ngoroutine 1 [running]:\nFAIL\tpkg/a\t0.01s\n",
			want: []VerifyFailure{
				{ID: "pkg/a", Kind: FailureKindPackage, Tail: "panic: init blew up\n\ngoroutine 1 [running]:\nFAIL\tpkg/a\t0.01s"},
			},
		},
		{
			name:   "CRLF input",
			output: "--- FAIL: TestA (0.00s)\r\n    boom\r\nFAIL\r\nFAIL\tpkg/a\t0.1s\r\n",
			want: []VerifyFailure{
				{ID: "pkg/a.TestA", Kind: FailureKindTest, Tail: "    boom"},
			},
		},
		{
			name:   "tail longer than cap is truncated to last lines",
			output: "--- FAIL: TestA (0.00s)\n" + strings.Join(longTail, "\n") + "\nFAIL\nFAIL\tpkg/a\t0.1s\n",
			want: []VerifyFailure{
				{ID: "pkg/a.TestA", Kind: FailureKindTest, Tail: wantLong},
			},
		},
		{
			name:   "all ok with passed false is opaque",
			output: "ok  \tpkg/a\t0.1s\nok  \tpkg/b\t0.1s\n",
			want: []VerifyFailure{
				{ID: opaqueFailureID, Kind: FailureKindOpaque, Tail: "ok  \tpkg/a\t0.1s\nok  \tpkg/b\t0.1s"},
			},
		},
		{
			name:   "non-Go output is opaque",
			output: "make: *** [all] Error 2\n",
			want: []VerifyFailure{
				{ID: opaqueFailureID, Kind: FailureKindOpaque, Tail: "make: *** [all] Error 2"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseVerifyFailures(tc.output, tc.passed)
			// Package is pinned by TestParseVerifyFailures_Package; the table pins the rest.
			for i := range got {
				got[i].Package = ""
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseVerifyFailures() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestParseVerifyFailures_Package(t *testing.T) {
	got := parseVerifyFailures("--- FAIL: TestA (0.00s)\nFAIL\nFAIL\tpkg/a\t0.1s\nFAIL\tpkg/b [build failed]\n", false)
	want := map[string]string{"pkg/a.TestA": "pkg/a", "pkg/b": "pkg/b"}
	if len(got) != len(want) {
		t.Fatalf("parseVerifyFailures() = %#v, want identities %v", got, want)
	}
	for _, f := range got {
		if f.Package != want[f.ID] {
			t.Errorf("identity %q Package = %q, want %q", f.ID, f.Package, want[f.ID])
		}
	}

	opaque := parseVerifyFailures("make: *** [all] Error 2\n", false)
	if len(opaque) != 1 || opaque[0].Kind != FailureKindOpaque || opaque[0].Package != "" {
		t.Errorf("opaque identity = %#v, want Kind opaque and empty Package", opaque)
	}
}
