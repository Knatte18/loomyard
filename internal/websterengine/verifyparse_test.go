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
		want   []IntegrationFailure
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
			want: []IntegrationFailure{
				{ID: "pkg/a.TestA", Kind: FailureKindTest, Tail: "    a_test.go:5: boom"},
				{ID: "pkg/c.TestC", Kind: FailureKindTest, Tail: "    c_test.go:9: bad"},
			},
		},
		{
			name: "subtests collapse to top-level test",
			output: "--- FAIL: TestA (0.00s)\n    --- FAIL: TestA/one (0.00s)\n        x\n    --- FAIL: TestA/two (0.00s)\n        y\n" +
				"FAIL\nFAIL\tpkg/a\t0.1s\n",
			want: []IntegrationFailure{
				{ID: "pkg/a.TestA", Kind: FailureKindTest, Tail: "    --- FAIL: TestA/one (0.00s)\n        x\n    --- FAIL: TestA/two (0.00s)\n        y"},
			},
		},
		{
			name: "indented FAIL header in a test's log output is not an identity",
			output: "--- FAIL: TestA (0.00s)\n    a_test.go:3: sub output:\n        --- FAIL: TestPhantom (0.00s)\n    a_test.go:3: boom\n" +
				"FAIL\nFAIL\tpkg/a\t0.1s\n",
			want: []IntegrationFailure{
				{ID: "pkg/a.TestA", Kind: FailureKindTest, Tail: "    a_test.go:3: sub output:\n        --- FAIL: TestPhantom (0.00s)\n    a_test.go:3: boom"},
			},
		},
		{
			name:   "build failed is a package identity",
			output: "# pkg/a\npkg/a/a.go:3: undefined: x\nFAIL\tpkg/a [build failed]\n",
			want: []IntegrationFailure{
				{ID: "pkg/a", Kind: FailureKindPackage, Tail: "# pkg/a\npkg/a/a.go:3: undefined: x\nFAIL\tpkg/a [build failed]"},
			},
		},
		{
			name:   "setup failed is a package identity",
			output: "pkg/a/a_test.go:1: no such file\nFAIL\tpkg/a [setup failed]\n",
			want: []IntegrationFailure{
				{ID: "pkg/a", Kind: FailureKindPackage, Tail: "pkg/a/a_test.go:1: no such file\nFAIL\tpkg/a [setup failed]"},
			},
		},
		{
			name:   "panic without FAIL line is a package identity",
			output: "panic: init blew up\n\ngoroutine 1 [running]:\nFAIL\tpkg/a\t0.01s\n",
			want: []IntegrationFailure{
				{ID: "pkg/a", Kind: FailureKindPackage, Tail: "panic: init blew up\n\ngoroutine 1 [running]:\nFAIL\tpkg/a\t0.01s"},
			},
		},
		{
			name:   "CRLF input",
			output: "--- FAIL: TestA (0.00s)\r\n    boom\r\nFAIL\r\nFAIL\tpkg/a\t0.1s\r\n",
			want: []IntegrationFailure{
				{ID: "pkg/a.TestA", Kind: FailureKindTest, Tail: "    boom"},
			},
		},
		{
			name:   "tail longer than cap is truncated to last lines",
			output: "--- FAIL: TestA (0.00s)\n" + strings.Join(longTail, "\n") + "\nFAIL\nFAIL\tpkg/a\t0.1s\n",
			want: []IntegrationFailure{
				{ID: "pkg/a.TestA", Kind: FailureKindTest, Tail: wantLong},
			},
		},
		{
			name:   "all ok with passed false is opaque",
			output: "ok  \tpkg/a\t0.1s\nok  \tpkg/b\t0.1s\n",
			want: []IntegrationFailure{
				{ID: opaqueFailureID, Kind: FailureKindOpaque, Tail: "ok  \tpkg/a\t0.1s\nok  \tpkg/b\t0.1s"},
			},
		},
		{
			name:   "non-Go output is opaque",
			output: "make: *** [all] Error 2\n",
			want: []IntegrationFailure{
				{ID: opaqueFailureID, Kind: FailureKindOpaque, Tail: "make: *** [all] Error 2"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseVerifyFailures(tc.output, tc.passed)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseVerifyFailures() = %#v, want %#v", got, tc.want)
			}
		})
	}
}
