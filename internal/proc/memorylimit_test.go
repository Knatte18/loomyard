// memorylimit_test.go pins the memory-limit reader over fixture cgroup files.
// The level with the least memory left wins, and an unreadable or limitless ancestry reads as not found.

package proc

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestReadMemoryLimitAt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const leaf = "/user.slice/user-1000.slice/user@1000.service/app.slice/run.scope"
	// level holds one cgroup's memory.high, memory.max, memory.current and memory.stat; an empty text leaves the file absent.
	type level struct{ high, max, current, stat string }
	tests := []struct {
		name string
		// selfCgroup is the cgroup file's content; empty leaves the file absent.
		selfCgroup string
		levels     map[string]level
		want       MemoryLimit
		wantFound  bool
	}{
		{
			name:       "the lower of high and max is the limit, and the file cache does not count as used",
			selfCgroup: "0::" + leaf + "\n",
			levels: map[string]level{
				"/user.slice/user-1000.slice": {
					high: "17179869184", max: "23622320128", current: "14037016576",
					stat: "anon 5658374144\nfile 7582158848\ninactive_file 4161351680\nactive_file 2899247104\n",
				},
			},
			want:      MemoryLimit{Used: 6976417792, Max: 17179869184, Cgroup: "/user.slice/user-1000.slice"},
			wantFound: true,
		},
		{
			name:       "the level with the least memory left wins over a larger limit closer to the root",
			selfCgroup: "0::" + leaf + "\n",
			levels: map[string]level{
				"/user.slice/user-1000.slice": {high: "max", max: "8000", current: "1000"},
				leaf:                          {high: "500", max: "max", current: "450"},
			},
			want:      MemoryLimit{Used: 450, Max: 500, Cgroup: leaf},
			wantFound: true,
		},
		{
			name:       "a level with no numeric limit or no memory.current is skipped",
			selfCgroup: "12:memory:/legacy\n0::" + leaf + "\n",
			levels: map[string]level{
				"/user.slice":                 {high: "max", max: "max", current: "5"},
				"/user.slice/user-1000.slice": {high: "100", max: "max"},
				leaf:                          {high: "max", max: "4000", current: "10"},
			},
			want:      MemoryLimit{Used: 10, Max: 4000, Cgroup: leaf},
			wantFound: true,
		},
		{
			name:       "no numeric limit in the ancestry",
			selfCgroup: "0::" + leaf + "\n",
			levels:     map[string]level{"/user.slice/user-1000.slice": {high: "max", max: "max", current: "2364"}},
		},
		{
			name:   "an absent cgroup file",
			levels: map[string]level{"/user.slice": {high: "3000", max: "max", current: "2997"}},
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
			for path, files := range tt.levels {
				dir := filepath.Join(mount, filepath.FromSlash(path))
				for name, text := range map[string]string{
					"memory.high": files.high, "memory.max": files.max, "memory.current": files.current, "memory.stat": files.stat,
				} {
					if text != "" {
						writeFixture(t, filepath.Join(dir, name), text+"\n")
					}
				}
			}

			got, found := readMemoryLimitAt(selfCgroup, mount)
			if got != tt.want || found != tt.wantFound {
				t.Errorf("readMemoryLimitAt() = %+v, %v; want %+v, %v", got, found, tt.want, tt.wantFound)
			}
		})
	}
}
