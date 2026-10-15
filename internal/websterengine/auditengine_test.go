// auditengine_test.go covers AuditEngineFrom: the engine set's default comes back as an AuditEngine when it audits forks, and is refused with the way forward when it does not.

package websterengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

func TestAuditEngineFrom(t *testing.T) {
	t.Parallel()

	const refusal = `webster: shuttle.yaml's engine: "fake" cannot audit forks, which webster needs; way forward: set engine: in shuttle.yaml to an engine with fork support, then re-run the verb, or under the Webster row re-step the row`
	fake := &shuttlefake.Engine{}
	tests := []struct {
		name    string
		engine  shuttleengine.Engine
		wantErr string
	}{
		{name: "an engine that audits forks", engine: fake},
		{name: "an engine hiding its fork audit", engine: struct{ shuttleengine.Engine }{fake}, wantErr: refusal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := websterengine.AuditEngineFrom(shuttlefake.EngineSet(tt.engine))
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("AuditEngineFrom() error = %v; want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("AuditEngineFrom() error: %v", err)
			}
			if got != websterengine.AuditEngine(fake) {
				t.Errorf("AuditEngineFrom() = %v; want the set's default engine", got)
			}
		})
	}
}
