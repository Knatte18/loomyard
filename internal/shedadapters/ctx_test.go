package shedadapters

import (
	"context"
	"errors"
	"strings"
	"testing"
)

//testtiming:keep pins that the entry and cancel errors name the producer and engine, wrap the context error and read differently
func TestEntryErrAndCancelErr(t *testing.T) {
	t.Parallel()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, cancelExpired := context.WithTimeout(context.Background(), 0)
	defer cancelExpired()
	<-expired.Done()

	functions := []struct {
		name string
		call func(ctx context.Context, producer, engine string) error
	}{
		{"entryErr", entryErr},
		{"cancelErr", cancelErr},
	}
	contexts := []struct {
		name    string
		ctx     context.Context
		wantErr error
	}{
		{"healthy", context.Background(), nil},
		{"cancelled", cancelled, context.Canceled},
		{"deadline exceeded", expired, context.DeadlineExceeded},
	}
	for _, function := range functions {
		for _, tt := range contexts {
			t.Run(function.name+"/"+tt.name, func(t *testing.T) {
				t.Parallel()
				err := function.call(tt.ctx, "loom", "shuttle")
				if tt.wantErr == nil {
					if err != nil {
						t.Errorf("%s(healthy) = %v; want nil", function.name, err)
					}
					return
				}
				if err == nil {
					t.Fatalf("%s(%s) = nil; want non-nil", function.name, tt.name)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("%s(%s) = %v; want errors.Is(err, %v)", function.name, tt.name, err, tt.wantErr)
				}
				for _, part := range []string{"loom", "shuttle"} {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("%s(%s) error %q does not contain %q", function.name, tt.name, err.Error(), part)
					}
				}
			})
		}
	}

	entry := entryErr(cancelled, "loom", "shuttle")
	exit := cancelErr(cancelled, "loom", "shuttle")
	if entry.Error() == exit.Error() {
		t.Errorf("entryErr and cancelErr produced identical messages %q; want distinguishable text", entry.Error())
	}
}
