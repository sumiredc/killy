// Package proc reads processes and their zombie children from a /proc file system.
package proc

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/sumiredc/killy/internal/protocol"
)

// MaxCmdBytes bounds the command line reported for a parent. /proc/<pid>/cmdline can be as long
// as the kernel's argument limit (megabytes); the app shows only the start, and every snapshot
// carries it.
const MaxCmdBytes = 4096

// Stat holds the fields of /proc/<pid>/stat that Killy uses.
type Stat struct {
	PID   int
	Comm  string
	State byte
	PPID  int
	// StartTime is field 22, in clock ticks since boot.
	StartTime int64
}

// ParseStat reads one /proc/<pid>/stat line.
func ParseStat(b []byte) (Stat, error) {
	// comm is wrapped in parentheses and may itself contain spaces and ")", so it runs from the
	// first "(" to the last ")". Splitting the whole line on spaces would shift every later field.
	open := bytes.IndexByte(b, '(')
	end := bytes.LastIndexByte(b, ')')
	if open < 0 || end < open {
		return Stat{}, errors.New("stat: no comm in parentheses")
	}
	pid, err := strconv.Atoi(string(bytes.TrimSpace(b[:open])))
	if err != nil {
		return Stat{}, fmt.Errorf("stat: pid: %w", err)
	}
	// rest[0] is field 3 (state), so field n is rest[n-3].
	rest := bytes.Fields(b[end+1:])
	if len(rest) < 22-3+1 {
		return Stat{}, fmt.Errorf("stat: %d fields after comm, need at least 20", len(rest))
	}
	if len(rest[0]) != 1 {
		return Stat{}, fmt.Errorf("stat: state %q", rest[0])
	}
	ppid, err := strconv.Atoi(string(rest[4-3]))
	if err != nil {
		return Stat{}, fmt.Errorf("stat: ppid: %w", err)
	}
	starttime, err := strconv.ParseInt(string(rest[22-3]), 10, 64)
	if err != nil {
		return Stat{}, fmt.Errorf("stat: starttime: %w", err)
	}
	return Stat{PID: pid, Comm: string(b[open+1 : end]), State: rest[0][0], PPID: ppid, StartTime: starttime}, nil
}

// Scanner reads processes from Root, which is /proc in production (os.DirFS("/proc")).
type Scanner struct {
	Root fs.FS
}

// Scan returns every process that has at least one child in state Z, ordered by PID, with its
// zombies ordered by PID. Zombies of PID 1 are left out: init reaps them, and PID 1 may not be killed.
// A process that disappears while it is being read is skipped. Scan fails only when Root cannot be listed.
func (s *Scanner) Scan() ([]protocol.Parent, error) {
	stats, err := s.readAll()
	if err != nil {
		return nil, err
	}
	zombies := map[int][]int{}
	for _, st := range stats {
		if st.State == 'Z' && st.PPID != 1 {
			zombies[st.PPID] = append(zombies[st.PPID], st.PID)
		}
	}
	parents := []protocol.Parent{}
	for ppid, children := range zombies {
		st, ok := stats[ppid]
		if !ok {
			continue
		}
		slices.Sort(children)
		parents = append(parents, protocol.Parent{
			PID:       ppid,
			StartTime: st.StartTime,
			Cmd:       s.cmd(st),
			Zombies:   children,
		})
	}
	slices.SortFunc(parents, func(a, b protocol.Parent) int { return a.PID - b.PID })
	return parents, nil
}

// Stat reads /proc/<pid>/stat. When the process does not exist the error matches fs.ErrNotExist.
func (s *Scanner) Stat(pid int) (Stat, error) {
	b, err := fs.ReadFile(s.Root, strconv.Itoa(pid)+"/stat")
	if err != nil {
		return Stat{}, err
	}
	return ParseStat(b)
}

// Children returns the processes whose parent is pid, ordered by PID, in any state.
func (s *Scanner) Children(pid int) ([]Stat, error) {
	stats, err := s.readAll()
	if err != nil {
		return nil, err
	}
	var children []Stat
	for _, st := range stats {
		if st.PPID == pid {
			children = append(children, st)
		}
	}
	slices.SortFunc(children, func(a, b Stat) int { return a.PID - b.PID })
	return children, nil
}

func (s *Scanner) readAll() (map[int]Stat, error) {
	entries, err := fs.ReadDir(s.Root, ".")
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	stats := make(map[int]Stat, len(entries))
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		b, err := fs.ReadFile(s.Root, e.Name()+"/stat")
		if err != nil {
			continue
		}
		st, err := ParseStat(b)
		if err != nil {
			continue
		}
		stats[pid] = st
	}
	return stats, nil
}

// cmd is the command line with its NUL-separated arguments joined by spaces, or comm when the
// command line is empty (kernel threads) or can no longer be read.
func (s *Scanner) cmd(st Stat) string {
	b, err := fs.ReadFile(s.Root, strconv.Itoa(st.PID)+"/cmdline")
	b = bytes.TrimRight(b, "\x00")
	if err != nil || len(b) == 0 {
		return st.Comm
	}
	cmd := string(bytes.ReplaceAll(b, []byte{0}, []byte{' '}))
	if len(cmd) > MaxCmdBytes {
		// Cutting at a byte count can split a multi-byte character; drop the broken tail.
		cmd = strings.ToValidUTF8(cmd[:MaxCmdBytes], "")
	}
	return cmd
}
