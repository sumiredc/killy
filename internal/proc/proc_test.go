package proc

import (
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"unicode/utf8"

	"github.com/sumiredc/killy/internal/protocol"
)

// A stat line captured from Linux 6.x: pid 6, comm "sleep", state S, ppid 1, starttime 39361010.
const realStat = "6 (sleep) S 1 1 1 0 -1 4194304 56 0 0 0 0 0 0 0 20 0 1 0 39361010 1728512 179 18446744073709551615 187650554593280 187650555445252 281474111507168 0 0 0 0 6 0 1 0 0 17 5 0 0 0 0 0 187650555561768 187650555576377 187650877218816 281474111508339 281474111508348 281474111508348 281474111508461 0\n"

type fakeProc struct {
	pid       int
	comm      string
	state     byte
	ppid      int
	starttime int64
	cmdline   string
	// cgroup is the content of <pid>/cgroup; the file is left out when it is empty.
	cgroup string
}

// statLine writes a stat line in the layout of realStat with the given values in fields 1-4 and 22.
func statLine(p fakeProc) string {
	return fmt.Sprintf("%d (%s) %c %d 1 1 0 -1 4194304 56 0 0 0 0 0 0 0 20 0 1 0 %d 1728512 179 18446744073709551615 187650554593280 187650555445252 281474111507168 0 0 0 0 6 0 1 0 0 17 5 0 0 0 0 0 187650555561768 187650555576377 187650877218816 281474111508339 281474111508348 281474111508348 281474111508461 0\n",
		p.pid, p.comm, p.state, p.ppid, p.starttime)
}

// fakeRoot builds a /proc with the given processes and the non-process entries a real /proc also has.
func fakeRoot(ps ...fakeProc) fstest.MapFS {
	root := fstest.MapFS{
		"meminfo":   {Data: []byte("MemTotal: 1 kB\n")},
		"sys/x":     {Data: []byte("1\n")},
		"self/x":    {Data: []byte("1\n")},
		"uptime":    {Data: []byte("1.00 1.00\n")},
		"1/stat":    {Data: []byte(statLine(fakeProc{pid: 1, comm: "init", state: 'S', ppid: 0, starttime: 1}))},
		"1/cmdline": {Data: []byte("/sbin/init\x00")},
	}
	for _, p := range ps {
		root[fmt.Sprintf("%d/stat", p.pid)] = &fstest.MapFile{Data: []byte(statLine(p))}
		root[fmt.Sprintf("%d/cmdline", p.pid)] = &fstest.MapFile{Data: []byte(p.cmdline)}
		if p.cgroup != "" {
			root[fmt.Sprintf("%d/cgroup", p.pid)] = &fstest.MapFile{Data: []byte(p.cgroup)}
		}
	}
	return root
}

func scan(t *testing.T, root fstest.MapFS) []protocol.Parent {
	t.Helper()
	parents, err := (&Scanner{Root: root}).Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return parents
}

func TestParseStatReadsPIDStatePPIDAndStartTime(t *testing.T) {
	got, err := ParseStat([]byte(realStat))
	if err != nil {
		t.Fatalf("ParseStat: %v", err)
	}
	want := Stat{PID: 6, Comm: "sleep", State: 'S', PPID: 1, StartTime: 39361010}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseStatReadsCommWithSpacesAndClosingParen(t *testing.T) {
	line := statLine(fakeProc{pid: 42, comm: "a) Z 7 (b", state: 'S', ppid: 9, starttime: 555})
	got, err := ParseStat([]byte(line))
	if err != nil {
		t.Fatalf("ParseStat: %v", err)
	}
	want := Stat{PID: 42, Comm: "a) Z 7 (b", State: 'S', PPID: 9, StartTime: 555}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseStatRejectsTruncatedLine(t *testing.T) {
	if _, err := ParseStat([]byte("42 (sleep) S 1 1 1")); err == nil {
		t.Error("ParseStat accepted a line without field 22")
	}
}

func TestScanGroupsZombiesOfTheSameParent(t *testing.T) {
	root := fakeRoot(
		fakeProc{pid: 100, comm: "parent", state: 'S', ppid: 1, starttime: 5000, cmdline: "parent\x00"},
		fakeProc{pid: 101, comm: "child", state: 'Z', ppid: 100, starttime: 5001},
		fakeProc{pid: 102, comm: "child", state: 'Z', ppid: 100, starttime: 5002},
		fakeProc{pid: 103, comm: "child", state: 'Z', ppid: 100, starttime: 5003},
	)
	want := []protocol.Parent{{PID: 100, StartTime: 5000, Cmd: "parent", Zombies: []int{101, 102, 103}}}
	if got := scan(t, root); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestScanReportsEachParentWithItsPIDAndStartTime(t *testing.T) {
	root := fakeRoot(
		fakeProc{pid: 200, comm: "a", state: 'S', ppid: 1, starttime: 7000, cmdline: "a\x00"},
		fakeProc{pid: 201, comm: "z", state: 'Z', ppid: 200, starttime: 7001},
		fakeProc{pid: 300, comm: "b", state: 'S', ppid: 1, starttime: 9000, cmdline: "b\x00"},
		fakeProc{pid: 301, comm: "z", state: 'Z', ppid: 300, starttime: 9001},
	)
	want := []protocol.Parent{
		{PID: 200, StartTime: 7000, Cmd: "a", Zombies: []int{201}},
		{PID: 300, StartTime: 9000, Cmd: "b", Zombies: []int{301}},
	}
	if got := scan(t, root); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestScanOmitsZombiesWhoseParentIsPID1(t *testing.T) {
	root := fakeRoot(
		fakeProc{pid: 50, comm: "orphan", state: 'Z', ppid: 1, starttime: 100},
		fakeProc{pid: 100, comm: "parent", state: 'S', ppid: 1, starttime: 5000, cmdline: "parent\x00"},
		fakeProc{pid: 101, comm: "child", state: 'Z', ppid: 100, starttime: 5001},
	)
	want := []protocol.Parent{{PID: 100, StartTime: 5000, Cmd: "parent", Zombies: []int{101}}}
	if got := scan(t, root); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestScanCountsOnlyChildrenInStateZ(t *testing.T) {
	root := fakeRoot(
		fakeProc{pid: 100, comm: "parent", state: 'S', ppid: 1, starttime: 5000, cmdline: "parent\x00"},
		fakeProc{pid: 101, comm: "child", state: 'Z', ppid: 100, starttime: 5001},
		fakeProc{pid: 102, comm: "running", state: 'R', ppid: 100, starttime: 5002},
		fakeProc{pid: 103, comm: "sleeping", state: 'S', ppid: 100, starttime: 5003},
		fakeProc{pid: 400, comm: "busy", state: 'S', ppid: 1, starttime: 8000, cmdline: "busy\x00"},
		fakeProc{pid: 401, comm: "worker", state: 'S', ppid: 400, starttime: 8001},
	)
	want := []protocol.Parent{{PID: 100, StartTime: 5000, Cmd: "parent", Zombies: []int{101}}}
	if got := scan(t, root); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestScanSkipsProcessWhoseStatCannotBeRead(t *testing.T) {
	root := fakeRoot(
		fakeProc{pid: 100, comm: "parent", state: 'S', ppid: 1, starttime: 5000, cmdline: "parent\x00"},
		fakeProc{pid: 101, comm: "child", state: 'Z', ppid: 100, starttime: 5001},
	)
	// Process 77 exited between listing /proc and reading its stat: the directory is listed, the stat is gone.
	root["77/cmdline"] = &fstest.MapFile{Data: []byte("gone\x00")}
	want := []protocol.Parent{{PID: 100, StartTime: 5000, Cmd: "parent", Zombies: []int{101}}}
	if got := scan(t, root); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestScanSkipsZombiesWhoseParentStatCannotBeRead(t *testing.T) {
	root := fakeRoot(
		fakeProc{pid: 101, comm: "child", state: 'Z', ppid: 100, starttime: 5001},
		fakeProc{pid: 200, comm: "a", state: 'S', ppid: 1, starttime: 7000, cmdline: "a\x00"},
		fakeProc{pid: 201, comm: "z", state: 'Z', ppid: 200, starttime: 7001},
	)
	want := []protocol.Parent{{PID: 200, StartTime: 7000, Cmd: "a", Zombies: []int{201}}}
	if got := scan(t, root); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestScanJoinsCmdlineArgumentsWithSpaces(t *testing.T) {
	root := fakeRoot(
		fakeProc{pid: 100, comm: "node", state: 'S', ppid: 1, starttime: 5000, cmdline: "node\x00evil_daemon.js\x00--port\x003000\x00"},
		fakeProc{pid: 101, comm: "node", state: 'Z', ppid: 100, starttime: 5001},
	)
	parents := scan(t, root)
	if len(parents) != 1 {
		t.Fatalf("got %d parents, want 1", len(parents))
	}
	if want := "node evil_daemon.js --port 3000"; parents[0].Cmd != want {
		t.Errorf("cmd = %q, want %q", parents[0].Cmd, want)
	}
}

func TestScanUsesCommWhenCmdlineIsEmpty(t *testing.T) {
	root := fakeRoot(
		fakeProc{pid: 100, comm: "kworker/0:1", state: 'S', ppid: 2, starttime: 5000, cmdline: ""},
		fakeProc{pid: 101, comm: "child", state: 'Z', ppid: 100, starttime: 5001},
	)
	parents := scan(t, root)
	if len(parents) != 1 {
		t.Fatalf("got %d parents, want 1", len(parents))
	}
	if want := "kworker/0:1"; parents[0].Cmd != want {
		t.Errorf("cmd = %q, want %q", parents[0].Cmd, want)
	}
}

func TestScanReturnsEmptyListWhenThereAreNoZombies(t *testing.T) {
	root := fakeRoot(fakeProc{pid: 100, comm: "a", state: 'S', ppid: 1, starttime: 5000, cmdline: "a\x00"})
	parents := scan(t, root)
	// The snapshot schema requires an array, so "no parents" must not encode as JSON null.
	if parents == nil || len(parents) != 0 {
		t.Errorf("got %#v, want an empty non-nil slice", parents)
	}
}

type unreadableFS struct{}

func (unreadableFS) Open(string) (fs.File, error) { return nil, fs.ErrPermission }

func TestScanFailsWhenRootCannotBeListed(t *testing.T) {
	if _, err := (&Scanner{Root: unreadableFS{}}).Scan(); err == nil {
		t.Error("Scan succeeded on a root that cannot be listed")
	}
}

func TestStatReadsTheGivenProcess(t *testing.T) {
	root := fakeRoot(fakeProc{pid: 100, comm: "parent", state: 'S', ppid: 1, starttime: 5000})
	got, err := (&Scanner{Root: root}).Stat(100)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if want := (Stat{PID: 100, Comm: "parent", State: 'S', PPID: 1, StartTime: 5000}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestStatOfAMissingProcessIsNotExist(t *testing.T) {
	_, err := (&Scanner{Root: fakeRoot()}).Stat(100)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want fs.ErrNotExist", err)
	}
}

func TestChildrenReturnsOnlyDirectChildrenInAnyState(t *testing.T) {
	root := fakeRoot(
		fakeProc{pid: 100, comm: "parent", state: 'S', ppid: 1, starttime: 5000},
		fakeProc{pid: 101, comm: "zombie", state: 'Z', ppid: 100, starttime: 5001},
		fakeProc{pid: 102, comm: "running", state: 'R', ppid: 100, starttime: 5002},
		fakeProc{pid: 103, comm: "grandchild", state: 'Z', ppid: 102, starttime: 5003},
		fakeProc{pid: 200, comm: "other", state: 'Z', ppid: 1, starttime: 6000},
	)
	got, err := (&Scanner{Root: root}).Children(100)
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	want := []Stat{
		{PID: 101, Comm: "zombie", State: 'Z', PPID: 100, StartTime: 5001},
		{PID: 102, Comm: "running", State: 'R', PPID: 100, StartTime: 5002},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestScanShortensAVeryLongCommandLine(t *testing.T) {
	long := strings.Repeat("a", 3000) + "\x00" + strings.Repeat("é", 2000) + "\x00"
	root := fakeRoot(
		fakeProc{pid: 100, comm: "big", state: 'S', ppid: 1, starttime: 5000, cmdline: long},
		fakeProc{pid: 101, comm: "z", state: 'Z', ppid: 100, starttime: 5001},
	)
	parents := scan(t, root)
	if len(parents) != 1 {
		t.Fatalf("got %d parents, want 1", len(parents))
	}
	cmd := parents[0].Cmd
	if len(cmd) > MaxCmdBytes || !utf8.ValidString(cmd) || !strings.HasPrefix(cmd, strings.Repeat("a", 3000)+" é") {
		t.Errorf("cmd of %d bytes (valid UTF-8: %v), want at most %d bytes of valid UTF-8 keeping the start", len(cmd), utf8.ValidString(cmd), MaxCmdBytes)
	}
}
