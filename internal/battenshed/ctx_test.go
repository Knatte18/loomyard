// ctx_test.go covers entryErr and cancelErr over a live context and a cancelled one.

package battenshed

import (
	"context"
	"strings"
	"testing"
)

func TestEntryErrAndCancelErr(t *testing.T) {
	t.Parallel()

	functions := map[string]func(ctx context.Context, producer string) error{
		"entryErr":  entryErr,
		"cancelErr": cancelErr,
	}
	tests := []struct {
		name       string
		cancel     bool
		wantErr    bool
		wantSubstr string
	}{
		{name: "live context returns nil", cancel: false, wantErr: false},
		{name: "cancelled context returns wrapped error", cancel: true, wantErr: true, wantSubstr: "producer-under-test"},
	}
	for functionName, function := range functions {
		for _, tt := range tests {
			t.Run(functionName+"/"+tt.name, func(t *testing.T) {
				t.Parallel()

				ctx, cancel := context.WithCancel(context.Background())
				if tt.cancel {
					cancel()
				} else {
					defer cancel()
				}

				err := function(ctx, "producer-under-test")
				if tt.wantErr && err == nil {
					t.Fatalf("%s() = nil; want non-nil error", functionName)
				}
				if !tt.wantErr && err != nil {
					t.Fatalf("%s() = %v; want nil", functionName, err)
				}
				if tt.wantErr && !strings.Contains(err.Error(), tt.wantSubstr) {
					t.Errorf("%s() = %q; want substring %q", functionName, err.Error(), tt.wantSubstr)
				}
			})
		}
	}
}
