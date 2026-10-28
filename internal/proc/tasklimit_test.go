//go:build tier0

// tasklimit_test.go pins the task-limit reader over fixture cgroup files: the tightest level wins, and an unreadable or limitless ancestry reads as not found.

package proc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadTaskLimitAt(t *testing.T) {
	t.Parallel()

	const leaf = "/user.slice/user-1000.slice/user@1000.service/app.slice/run.scope"
	tests := []struct {
		name string
		// selfCgroup is the cgroup file's content; empty leaves the file absent.
		selfCgroup string
		// levels maps a cgroup path to its pids.max and pids.current contents; an empty current leaves that file absent.
		levels    map[string][2]string
		want      TaskLimit
		wantFound bool
	}{
		{
			name:       "the user slice is the only numeric limit",
			selfCgroup: "0::" + leaf + "\n",
			levels: map[string][2]string{
				"/user.slice":                                   {"max", "2364"},
				"/user.slice/user-1000.slice":                   {"3000", "2997"},
				"/user.slice/user-1000.slice/user@1000.service": {"max", "2350"},
			},
			want:      TaskLimit{Current: 2997, Max: 3000, Cgroup: "/user.slice/user-1000.slice"},
			wantFound: true,
		},
		{
			name:       "the level with the fewest free tasks wins over a larger limit closer to the root",
			selfCgroup: "0::" + leaf + "\n",
			levels: map[string][2]string{
				"/user.slice/user-1000.slice": {"20000", "2400"},
				leaf:                          {"500", "450"},
			},
			want:      TaskLimit{Current: 450, Max: 500, Cgroup: leaf},
			wantFound: true,
		},
		{
			name:       "a level missing pids.current or holding a malformed number is skipped",
			selfCgroup: "12:pids:/legacy\n0::" + leaf + "\n",
			levels: map[string][2]string{
				"/user.slice":                 {"100", ""},
				"/user.slice/user-1000.slice": {"many", "1"},
				leaf:                          {"4000", "10"},
			},
			want:      TaskLimit{Current: 10, Max: 4000, Cgroup: leaf},
			wantFound: true,
		},
		{
			name:       "no numeric limit in the ancestry",
			selfCgroup: "0::" + leaf + "\n",
			levels:     map[string][2]string{"/user.slice/user-1000.slice": {"max", "2364"}},
		},
		{
			name:       "a cgroup v1 file holds no v2 path",
			selfCgroup: "12:pids:/user.slice\n",
			levels:     map[string][2]string{"/user.slice": {"3000", "2997"}},
		},
		{
			name:   "an absent cgroup file",
			levels: map[string][2]string{"/user.slice": {"3000", "2997"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			selfCgroup := filepath.Join(root, "cgroup")
			if tt.selfCgroup != "" {
				writeFixture(t, selfCgroup, tt.selfCgroup)
			}
			mount := filepath.Join(root, "mount")
			for level, files := range tt.levels {
				dir := filepath.Join(mount, filepath.FromSlash(level))
				writeFixture(t, filepath.Join(dir, "pids.max"), files[0]+"\n")
				if files[1] != "" {
					writeFixture(t, filepath.Join(dir, "pids.current"), files[1]+"\n")
				}
			}

			got, found := readTaskLimitAt(selfCgroup, mount)
			if got != tt.want || found != tt.wantFound {
				t.Errorf("readTaskLimitAt() = %+v, %v; want %+v, %v", got, found, tt.want, tt.wantFound)
			}
		})
	}
}

// writeFixture writes content to path, creating its directory.
func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}
