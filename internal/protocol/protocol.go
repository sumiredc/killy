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
