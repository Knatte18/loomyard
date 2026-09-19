// ctx_test.go covers entryErr and cancelErr over a live context and a cancelled one.

package battenshed

import (
	"context"
	"strings"
	"testing"
)

func TestEntryErr(t *testing.T) {
	tests := []struct {
		name       string
		cancel     bool
		wantErr    bool
		wantSubstr string
	}{
		{name: "live context returns nil", cancel: false, wantErr: false},
		{name: "cancelled context returns wrapped error", cancel: true, wantErr: true, wantSubstr: "producer-under-test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if tt.cancel {
				cancel()
			} else {
				defer cancel()
			}

			err := entryErr(ctx, "producer-under-test")
			if tt.wantErr && err == nil {
				t.Fatalf("entryErr() = nil; want non-nil error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("entryErr() = %v; want nil", err)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("entryErr() = %q; want substring %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}

func TestCancelErr(t *testing.T) {
	tests := []struct {
		name       string
		cancel     bool
		wantErr    bool
		wantSubstr string
	}{
		{name: "live context returns nil", cancel: false, wantErr: false},
		{name: "cancelled context returns wrapped error", cancel: true, wantErr: true, wantSubstr: "producer-under-test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if tt.cancel {
				cancel()
			} else {
				defer cancel()
			}

			err := cancelErr(ctx, "producer-under-test")
			if tt.wantErr && err == nil {
				t.Fatalf("cancelErr() = nil; want non-nil error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("cancelErr() = %v; want nil", err)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("cancelErr() = %q; want substring %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}
