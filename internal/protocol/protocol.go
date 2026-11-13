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
	// Unknown is made by the Killy server only, when no outcome came back.
	Unknown = "unknown"
)

// Reasons for an unknown outcome.
const (
	ReasonAgentDisconnected = "agent_disconnected"
	ReasonTimeout           = "timeout"
)

// IsAgentOutcome reports whether o is an outcome an agent may return.
func IsAgentOutcome(o string) bool {
	switch o {
	case Killed, AlreadyGone, Refused, SignalFailed, ZombiesRemain:
		return true
	}
	return false
}

// CommandResult is what an agent posts back for a command (CommandResult in docs/openapi.yaml).
type CommandResult struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Outcome string `json:"outcome,omitempty"`
	Reaped  int    `json:"reaped,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Version string `json:"version,omitempty"`
}

// Result types.
const (
	ResultKill = "kill"
	ResultPong = "pong"
)

// Reasons for a refused kill.
const (
	ReasonPidfdUnavailable = "pidfd_unavailable"
	ReasonPIDReused        = "pid_reused"
	ReasonNotAZombieParent = "not_a_zombie_parent"
	ReasonProtected        = "protected"
)

// IsUUIDv4 reports whether s is a UUID v4 in lower case: 8-4-4-4-12 hex digits, version digit 4,
// variant bits 10. The agent ignores kill commands with any other ID and the Killy server rejects them.
func IsUUIDv4(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		case 14:
			if c != '4' {
				return false
			}
		case 19:
			if c != '8' && c != '9' && c != 'a' && c != 'b' {
				return false
			}
		default:
			if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
				return false
			}
		}
	}
	return true
}
