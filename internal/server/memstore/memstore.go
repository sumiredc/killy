// Package memstore is an in-memory stand-in for the PostgreSQL store, for unit tests. It keeps
// the same rules as the SQL in package store: tokens are looked up by their SHA-256, first
// detections are synced to the reported set of parents.
package memstore

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"time"
	"uuid"

	"github.com/sumiredc/killy/internal/server/store"
)

// Memory is safe for concurrent use.
type Memory struct {
	mu sync.Mutex
	// Err, when set, is returned by every call, as when the database cannot be reached.
	Err error
	// FailLink makes LinkDevice alone fail, as when the database goes away in the middle of a claim.
	FailLink bool
	// FailLinksOfDevice and FailDevicesOfAgent make those queries alone fail.
	FailLinksOfDevice, FailDevicesOfAgent bool
	// FailLinkChanges makes UpdateLink and UnlinkDevice alone fail.
	FailLinkChanges bool
	// FailUpdateDevice makes UpdateDevice alone fail.
	FailUpdateDevice bool
	// FailRecordNotified makes RecordNotified alone fail.
	FailRecordNotified bool
	agents             []AgentRow
	devices            []DeviceRow
	links              []LinkRow
	detections         map[string]map[store.ParentKey]time.Time
	// notified holds the notified_parents rows.
	notified []NotifiedRow
}

// NotifiedRow is what the notified_parents table would hold.
type NotifiedRow struct {
	AgentID  string
	Parent   store.ParentKey
	DeviceID string
}

// AgentRow is what the agents table would hold.
type AgentRow struct {
	ID          string
	TokenSHA256 []byte
	Hostname    string
}

func New() *Memory {
	return &Memory{detections: map[string]map[store.ParentKey]time.Time{}}
}

func (m *Memory) CreateAgent(_ context.Context, tokenSHA256 []byte, hostname string) (store.Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return store.Agent{}, m.Err
	}
	for _, a := range m.agents {
		if bytes.Equal(a.TokenSHA256, tokenSHA256) {
			return store.Agent{}, errors.New("memstore: duplicate token_sha256 (UNIQUE in the agents table)")
		}
	}
	row := AgentRow{ID: uuid.NewV4().String(), TokenSHA256: bytes.Clone(tokenSHA256), Hostname: hostname}
	m.agents = append(m.agents, row)
	return store.Agent{ID: row.ID, Hostname: row.Hostname}, nil
}

func (m *Memory) AgentByToken(_ context.Context, tokenSHA256 []byte) (store.Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return store.Agent{}, m.Err
	}
	for _, a := range m.agents {
		if bytes.Equal(a.TokenSHA256, tokenSHA256) {
			return store.Agent{ID: a.ID, Hostname: a.Hostname}, nil
		}
	}
	return store.Agent{}, store.ErrNotFound
}

// Agents returns a copy of the stored agent rows.
func (m *Memory) Agents() []AgentRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.agents)
}

func (m *Memory) SyncFirstDetections(_ context.Context, agentID string, parents []store.ParentKey, now time.Time) (map[store.ParentKey]time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	// timestamptz keeps microseconds; returning finer times would let a test pass that fails on PostgreSQL.
	now = now.Truncate(time.Microsecond)
	old := m.detections[agentID]
	next := make(map[store.ParentKey]time.Time, len(parents))
	for _, p := range parents {
		if at, ok := old[p]; ok {
			next[p] = at
		} else if _, dup := next[p]; !dup {
			next[p] = now
		}
	}
	m.detections[agentID] = next
	// ON DELETE CASCADE from first_detections.
	m.notified = slices.DeleteFunc(m.notified, func(n NotifiedRow) bool {
		_, kept := next[n.Parent]
		return n.AgentID == agentID && !kept
	})
	got := make(map[store.ParentKey]time.Time, len(next))
	for k, v := range next {
		got[k] = v
	}
	return got, nil
}

// Detections returns a copy of the agent's first-detection rows.
func (m *Memory) Detections(agentID string) map[store.ParentKey]time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	got := map[store.ParentKey]time.Time{}
	for k, v := range m.detections[agentID] {
		got[k] = v
	}
	return got
}

// DeviceRow is what the devices table would hold.
type DeviceRow struct {
	ID          string
	TokenSHA256 []byte
	PushToken   string
	// Notify is the app-wide notification switch; on for a new device, as in the devices table.
	Notify bool
}

// LinkRow is what the links table would hold.
type LinkRow struct {
	DeviceID, AgentID, Name string
	Notify                  bool
}

func (m *Memory) CreateDevice(_ context.Context, tokenSHA256 []byte, pushToken string) (store.Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return store.Device{}, m.Err
	}
	for _, d := range m.devices {
		if bytes.Equal(d.TokenSHA256, tokenSHA256) {
			return store.Device{}, errors.New("memstore: duplicate token_sha256 (UNIQUE in the devices table)")
		}
	}
	row := DeviceRow{ID: uuid.NewV4().String(), TokenSHA256: bytes.Clone(tokenSHA256), PushToken: pushToken, Notify: true}
	m.devices = append(m.devices, row)
	return store.Device{ID: row.ID}, nil
}

func (m *Memory) DeviceByToken(_ context.Context, tokenSHA256 []byte) (store.Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return store.Device{}, m.Err
	}
	for _, d := range m.devices {
		if bytes.Equal(d.TokenSHA256, tokenSHA256) {
			return store.Device{ID: d.ID}, nil
		}
	}
	return store.Device{}, store.ErrNotFound
}

func (m *Memory) AgentByID(_ context.Context, id string) (store.Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return store.Agent{}, m.Err
	}
	for _, a := range m.agents {
		if a.ID == id {
			return store.Agent{ID: a.ID, Hostname: a.Hostname}, nil
		}
	}
	return store.Agent{}, store.ErrNotFound
}

func (m *Memory) LinkDevice(_ context.Context, deviceID, agentID string, name *string, fallback string) (store.Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return store.Link{}, m.Err
	}
	if m.FailLink {
		return store.Link{}, errors.New("memstore: connection lost")
	}
	if !slices.ContainsFunc(m.devices, func(d DeviceRow) bool { return d.ID == deviceID }) ||
		!slices.ContainsFunc(m.agents, func(a AgentRow) bool { return a.ID == agentID }) {
		return store.Link{}, errors.New("memstore: foreign key violation")
	}
	for i, l := range m.links {
		if l.DeviceID == deviceID && l.AgentID == agentID {
			if name != nil {
				m.links[i].Name = *name
			}
			return store.Link{AgentID: agentID, Name: m.links[i].Name, Notify: l.Notify}, nil
		}
	}
	row := LinkRow{DeviceID: deviceID, AgentID: agentID, Name: fallback, Notify: true}
	if name != nil {
		row.Name = *name
	}
	m.links = append(m.links, row)
	return store.Link{AgentID: agentID, Name: row.Name, Notify: row.Notify}, nil
}

// Devices returns a copy of the stored device rows.
func (m *Memory) Devices() []DeviceRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.devices)
}

// Links returns a copy of the stored link rows.
func (m *Memory) Links() []LinkRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.links)
}

func (m *Memory) LinksOfDevice(_ context.Context, deviceID string) ([]store.LinkedServer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	if m.FailLinksOfDevice {
		return nil, errors.New("memstore: connection lost")
	}
	servers := []store.LinkedServer{}
	for _, l := range m.links {
		if l.DeviceID != deviceID {
			continue
		}
		for _, a := range m.agents {
			if a.ID == l.AgentID {
				servers = append(servers, store.LinkedServer{AgentID: a.ID, Name: l.Name, Notify: l.Notify, Hostname: a.Hostname})
			}
		}
	}
	return servers, nil
}

func (m *Memory) DevicesOfAgent(_ context.Context, agentID string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	if m.FailDevicesOfAgent {
		return nil, errors.New("memstore: connection lost")
	}
	var ids []string
	for _, l := range m.links {
		if l.AgentID == agentID {
			ids = append(ids, l.DeviceID)
		}
	}
	return ids, nil
}

// UpdateLink changes this device's link only; a nil name or notify leaves that field as it is.
func (m *Memory) UpdateLink(_ context.Context, deviceID, agentID string, name *string, notify *bool) (store.Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return store.Link{}, m.Err
	}
	if m.FailLinkChanges {
		return store.Link{}, errors.New("memstore: connection lost")
	}
	if err := uuidColumn(agentID); err != nil {
		return store.Link{}, err
	}
	for i, l := range m.links {
		if l.DeviceID == deviceID && l.AgentID == agentID {
			if name != nil {
				m.links[i].Name = *name
			}
			if notify != nil {
				m.links[i].Notify = *notify
			}
			return store.Link{AgentID: agentID, Name: m.links[i].Name, Notify: m.links[i].Notify}, nil
		}
	}
	return store.Link{}, store.ErrNotFound
}

// UnlinkDevice removes this device's link only.
func (m *Memory) UnlinkDevice(_ context.Context, deviceID, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if m.FailLinkChanges {
		return errors.New("memstore: connection lost")
	}
	if err := uuidColumn(agentID); err != nil {
		return err
	}
	for i, l := range m.links {
		if l.DeviceID == deviceID && l.AgentID == agentID {
			m.links = slices.Delete(m.links, i, i+1)
			// ON DELETE CASCADE from links.
			m.notified = slices.DeleteFunc(m.notified, func(n NotifiedRow) bool { return n.DeviceID == deviceID && n.AgentID == agentID })
			return nil
		}
	}
	return store.ErrNotFound
}

// uuidColumn fails as PostgreSQL does when a value that is not a UUID is compared with a uuid
// column, so a handler that passes an unchecked path id to the store fails here too.
func uuidColumn(v string) error {
	if _, err := uuid.Parse(v); err != nil {
		return errors.New(`memstore: invalid input syntax for type uuid: "` + v + `"`)
	}
	return nil
}

// SetLink changes a link's name and switch directly, for tests that set a link up without going
// through PATCH /app/v1/servers/{id}.
func (m *Memory) SetLink(deviceID, agentID, name string, notify bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, l := range m.links {
		if l.DeviceID == deviceID && l.AgentID == agentID {
			m.links[i].Name, m.links[i].Notify = name, notify
		}
	}
}

func (m *Memory) UpdateDevice(_ context.Context, deviceID string, pushToken *string, notify *bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if m.FailUpdateDevice {
		return errors.New("memstore: connection lost")
	}
	i := slices.IndexFunc(m.devices, func(d DeviceRow) bool { return d.ID == deviceID })
	if i < 0 {
		return store.ErrNotFound
	}
	if pushToken != nil {
		for j := range m.devices {
			if j != i && m.devices[j].PushToken == *pushToken {
				m.devices[j].PushToken = ""
			}
		}
		m.devices[i].PushToken = *pushToken
	}
	if notify != nil {
		m.devices[i].Notify = *notify
	}
	return nil
}

func (m *Memory) PushTargets(_ context.Context, agentID string) ([]store.PushTarget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	var targets []store.PushTarget
	for _, l := range m.links {
		if l.AgentID != agentID || !l.Notify {
			continue
		}
		for _, d := range m.devices {
			if d.ID == l.DeviceID && d.Notify && d.PushToken != "" {
				targets = append(targets, store.PushTarget{DeviceID: d.ID, PushToken: d.PushToken, Name: l.Name})
			}
		}
	}
	return targets, nil
}

func (m *Memory) NotifiedOf(_ context.Context, agentID string) ([]store.Notified, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	return m.notifiedOf(agentID), nil
}

func (m *Memory) notifiedOf(agentID string) []store.Notified {
	var out []store.Notified
	for _, n := range m.notified {
		if n.AgentID == agentID {
			out = append(out, store.Notified{Parent: n.Parent, DeviceID: n.DeviceID})
		}
	}
	return out
}

func (m *Memory) RecordNotified(_ context.Context, agentID string, pairs []store.Notified) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if m.FailRecordNotified {
		return errors.New("memstore: connection lost")
	}
	for _, p := range pairs {
		// Only pairs whose parent and link still exist, as the SQL does instead of failing on the
		// foreign keys; a pair already there is left as it is.
		if _, ok := m.detections[agentID][p.Parent]; !ok {
			continue
		}
		if !slices.ContainsFunc(m.links, func(l LinkRow) bool { return l.DeviceID == p.DeviceID && l.AgentID == agentID }) {
			continue
		}
		row := NotifiedRow{AgentID: agentID, Parent: p.Parent, DeviceID: p.DeviceID}
		if !slices.Contains(m.notified, row) {
			m.notified = append(m.notified, row)
		}
	}
	return nil
}

func (m *Memory) ClearPushToken(_ context.Context, deviceID, pushToken string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	for i, d := range m.devices {
		if d.ID == deviceID && d.PushToken == pushToken {
			m.devices[i].PushToken = ""
		}
	}
	return nil
}

// Notified returns a copy of the agent's notified rows.
func (m *Memory) Notified(agentID string) []store.Notified {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.notifiedOf(agentID)
}
