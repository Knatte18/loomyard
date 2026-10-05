package configengine

import (
	"errors"
	"slices"
	"testing"
)

func TestResolveEditor(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		onPath   []string
		goos     string
		wantBin  string
		wantArgs []string
	}{
		{
			name:    "VISUAL wins over EDITOR and code",
			env:     map[string]string{"VISUAL": "emacs", "EDITOR": "ed"},
			onPath:  []string{"code", "nano"},
			goos:    "linux",
			wantBin: "emacs",
		},
		{
			name:    "EDITOR used when VISUAL is empty",
			env:     map[string]string{"VISUAL": "", "EDITOR": "ed"},
			onPath:  []string{"code"},
			goos:    "linux",
			wantBin: "ed",
		},
		{
			name:     "code on linux",
			onPath:   []string{"code", "nano"},
			goos:     "linux",
			wantBin:  "code",
			wantArgs: []string{"--wait"},
		},
		{
			name:     "code on windows",
			onPath:   []string{"code"},
			goos:     "windows",
			wantBin:  "code",
			wantArgs: []string{"--wait"},
		},
		{
			name:    "notepad on windows without code even with nano",
			onPath:  []string{"nano"},
			goos:    "windows",
			wantBin: "notepad",
		},
		{
			name:    "nano on linux without code",
			onPath:  []string{"nano"},
			goos:    "linux",
			wantBin: "nano",
		},
		{
			name:    "vi on linux with neither",
			goos:    "linux",
			wantBin: "vi",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(k string) string { return tt.env[k] }
			lookPath := func(name string) (string, error) {
				if slices.Contains(tt.onPath, name) {
					return "/bin/" + name, nil
				}
				return "", errors.New("not found")
			}
			bin, args := resolveEditor(getenv, lookPath, tt.goos)
			if bin != tt.wantBin {
				t.Errorf("binary = %q; want %q", bin, tt.wantBin)
			}
			if !slices.Equal(args, tt.wantArgs) {
				t.Errorf("args = %q; want %q", args, tt.wantArgs)
			}
		})
	}
}
