// Package protocol holds the request and response types shared by the agent and the Killy server.
// The JSON shapes follow docs/openapi.yaml.
package protocol

import "time"

// Parent is a process that holds at least one zombie child.
// A parent is identified by its PID together with its start time, so a reused PID is a different parent.
type Parent struct {
	PID int `json:"pid"`
	// StartTime is field 22 of /proc/<pid>/stat, in clock ticks since boot.
	StartTime int64  `json:"starttime"`
	Cmd       string `json:"cmd"`
	Zombies   []int  `json:"zombies"`
}

// Snapshot is the full set of parents with zombies on one server. It replaces the previous one; it is never a diff.
type Snapshot struct {
	Hostname  string    `json:"hostname"`
	Version   string    `json:"version"`
	ScannedAt time.Time `json:"scanned_at"`
	Parents   []Parent  `json:"parents"`
}

// KillCommand asks an agent to send SIGKILL to one parent. ID is a UUID v4 chosen by the phone.
type KillCommand struct {
	ID        string `json:"id"`
	PPID      int    `json:"ppid"`
	StartTime int64  `json:"starttime"`
}

// Outcome is how a kill ended on the agent.
type Outcome struct {
	Outcome string `json:"outcome"`
	// Reason is set for refused (one of the Reason constants) and for signal_failed (an errno name such as EPERM).
	Reason string `json:"reason,omitempty"`
	// Reaped is the number of zombies that disappeared, set for killed only.
	Reaped int `json:"reaped,omitempty"`
}

// Outcomes an agent can return. The Killy server adds "unknown" on its own.
const (
	Killed        = "killed"
	AlreadyGone   = "already_gone"
	Refused       = "refused"
	SignalFailed  = "signal_failed"
	ZombiesRemain = "zombies_remain"
)

// Reasons for a refused kill.
const (
	ReasonPidfdUnavailable = "pidfd_unavailable"
	ReasonPIDReused        = "pid_reused"
	ReasonNotAZombieParent = "not_a_zombie_parent"
	ReasonProtected        = "protected"
)
