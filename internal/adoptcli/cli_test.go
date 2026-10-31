//go:build tier0

// cli_test.go pins the fields of the adopt envelope that depend on the outcome of the adopt check's start.

package adoptcli

import "testing"

func TestAdoptFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		result      outcome
		wantMessage string
		wantWarning string
		wantSuite   bool
	}{
		{
			name:        "StartedCheckCarriesItsMessageAndSuite",
			result:      outcome{Suite: &suiteStart{PID: 42}, SuiteMessage: "the adopt check runs"},
			wantMessage: "the adopt check runs",
			wantSuite:   true,
		},
		{
			name:        "FailedStartCarriesOnlyTheWarning",
			result:      outcome{SuiteWarning: "the adopt check was not started"},
			wantWarning: "the adopt check was not started",
		},
		{name: "NoCheckCarriesNone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fields := adoptFields(tt.result)

			for key, want := range map[string]string{"message": tt.wantMessage, "suite_warning": tt.wantWarning} {
				got, present := fields[key]
				if want == "" && present {
					t.Errorf("adoptFields()[%q] = %v; want it absent", key, got)
				}
				if want != "" && got != want {
					t.Errorf("adoptFields()[%q] = %v; want %q", key, got, want)
				}
			}
			if _, present := fields["suite"]; present != tt.wantSuite {
				t.Errorf("adoptFields() holds suite = %v; want %v", present, tt.wantSuite)
			}
		})
	}
}
