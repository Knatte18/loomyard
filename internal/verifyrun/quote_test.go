// quote_test.go pins QuoteArg's quoting per platform shell.

package verifyrun

import (
	"errors"
	"testing"
)

func TestQuoteArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		goos    string
		value   string
		want    string
		wantErr error
	}{
		{name: "linux plain", goos: "linux", value: "plain", want: `'plain'`},
		{name: "linux spaces", goos: "linux", value: "two words", want: `'two words'`},
		{name: "linux single quote", goos: "linux", value: "it's", want: `'it'\''s'`},
		{name: "linux percent", goos: "linux", value: "100%", want: `'100%'`},
		{name: "windows spaces", goos: "windows", value: "two words", want: `"two words"`},
		{name: "windows double quote", goos: "windows", value: `say "hi"`, want: `"say ""hi"""`},
		{name: "windows percent", goos: "windows", value: "100%", wantErr: ErrPercentUnderCmd},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := QuoteArg(tt.goos, tt.value)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("QuoteArg(%q, %q) error = %v; want %v", tt.goos, tt.value, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("QuoteArg(%q, %q) = %q; want %q", tt.goos, tt.value, got, tt.want)
			}
		})
	}
}
