// processscope_parse_linux_test.go covers the parsing of the kernel journal and of a scope's memory events for OOM kills.

package proc_test

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestParseOOMKills(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const cgroup = "/user.slice/user-1000.slice/user@1000.service/app.slice/lym-pane-42.scope"
	tests := []struct {
		name    string
		journal string
		want    []proc.OOMKill
	}{
		{
			"AKillInTheCgroup",
			"tail invoked oom-killer: gfp_mask=0xcc0(GFP_KERNEL), order=0, oom_score_adj=200\n" +
				"oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),cpuset=/,mems_allowed=0,oom_memcg=" + cgroup + ",task_memcg=" + cgroup + ",task=tail,pid=116496,uid=1000\n" +
				"Memory cgroup out of memory: Killed process 116496 (tail) total-vm:79888kB\n",
			[]proc.OOMKill{{PID: 116496, Name: "tail"}},
		},
		{
			"TwoKillsOldestFirst",
			"oom-kill:constraint=CONSTRAINT_MEMCG,oom_memcg=" + cgroup + ",task_memcg=" + cgroup + ",task=script,pid=7,uid=1000\n" +
				"oom-kill:constraint=CONSTRAINT_MEMCG,oom_memcg=" + cgroup + ",task_memcg=" + cgroup + ",task=go build,pid=9,uid=1000\n",
			[]proc.OOMKill{{PID: 7, Name: "script"}, {PID: 9, Name: "go build"}},
		},
		{
			"AKillInAnotherCgroupWhoseNameStartsTheSame",
			"oom-kill:constraint=CONSTRAINT_MEMCG,oom_memcg=" + cgroup + "x,task_memcg=" + cgroup + "x,task=tail,pid=5,uid=1000\n",
			nil,
		},
		{"NoKill", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := proc.ParseOOMKills(tt.journal, cgroup); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseOOMKills(%q) = %+v; want %+v", tt.journal, got, tt.want)
			}
		})
	}
}

func TestOOMKillCount(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name   string
		events string
		want   int
	}{
		{"TwoKills", "low 0\nhigh 0\nmax 3\noom 2\noom_kill 2\noom_group_kill 0\n", 2},
		{"NoKill", "max 0\noom 0\noom_kill 0\n", 0},
		{"NoKillLine", "max 0\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := proc.OOMKillCount(tt.events); got != tt.want {
				t.Errorf("OOMKillCount(%q) = %d; want %d", tt.events, got, tt.want)
			}
		})
	}
}
