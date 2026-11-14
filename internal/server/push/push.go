// Package push sends push notifications of new zombie parents through Expo Push Service.
package push

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sumiredc/killy/internal/server/live"
	"github.com/sumiredc/killy/internal/server/store"
)

// MaxMessages is how many messages Expo takes in one request.
const MaxMessages = 100

// MaxAttempts is how many times one parent is sent to one device before the Killy server gives
// up and records it as notified: the first send and 3 retries (docs/ARCHITECTURE.md, Notifications).
const MaxAttempts = 4

// Message is one push (a push message in Expo's API).
type Message struct {
	To       string `json:"to"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Sound    string `json:"sound"`
	Priority string `json:"priority"`
}

// Ticket is Expo's answer for one message, in the order the messages were sent.
type Ticket struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Details struct {
		Error string `json:"error,omitempty"`
	} `json:"details"`
}

// DeviceNotRegistered is the ticket error for a push token Expo no longer accepts.
const DeviceNotRegistered = "DeviceNotRegistered"

// Sender is Expo Push Service; tests replace it.
type Sender interface {
	Send(ctx context.Context, msgs []Message) ([]Ticket, error)
}

// Store is what the notifier reads and writes; *store.Store (PostgreSQL) in production.
type Store interface {
	PushTargets(ctx context.Context, agentID string) ([]store.PushTarget, error)
	NotifiedOf(ctx context.Context, agentID string) ([]store.Notified, error)
	RecordNotified(ctx context.Context, agentID string, pairs []store.Notified) error
	ClearPushToken(ctx context.Context, deviceID, pushToken string) error
}

// Live gives the agent's latest snapshot; *live.State in production.
type Live interface {
	Agent(agentID string) (live.Agent, bool)
}

// Notifier sends each agent's new parents to its devices. Each agent has at most one send running,
// so two snapshots of one agent never send the same pair twice before it is recorded. It is safe
// for concurrent use.
type Notifier struct {
	store  Store
	live   Live
	sender Sender
	log    *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	stopped bool
	running map[string]bool // the agent has a send running
	again   map[string]bool // a snapshot came while it ran: send once more
	wg      sync.WaitGroup
	// failures counts failed sends per pair; accepted holds pairs Expo took but the store did not
	// record yet. Both are in memory only: a restart starts them again.
	failures map[pair]int
	accepted map[pair]bool
}

// pair is one parent and one device. The first-detection time is in it so a parent that left and
// came back with the same PID and start time counts as a new one.
type pair struct {
	agentID       string
	parent        store.ParentKey
	firstDetected time.Time
	deviceID      string
}

func New(st Store, lv Live, sender Sender, log *slog.Logger) *Notifier {
	ctx, cancel := context.WithCancel(context.Background())
	return &Notifier{store: st, live: lv, sender: sender, log: log, ctx: ctx, cancel: cancel,
		running: map[string]bool{}, again: map[string]bool{}, failures: map[pair]int{}, accepted: map[pair]bool{}}
}

// Wake makes the notifier look at the agent's latest snapshot, without waiting for it. A wake
// while the agent's send runs is folded into one more send: it reads the latest snapshot anyway.
func (n *Notifier) Wake(agentID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.stopped {
		return
	}
	if n.running[agentID] {
		n.again[agentID] = true
		return
	}
	n.running[agentID] = true
	n.wg.Add(1)
	go n.run(agentID)
}

func (n *Notifier) run(agentID string) {
	defer n.wg.Done()
	for {
		n.send(agentID)
		n.mu.Lock()
		if n.again[agentID] && !n.stopped {
			n.again[agentID] = false
			n.mu.Unlock()
			continue
		}
		delete(n.again, agentID)
		delete(n.running, agentID)
		n.mu.Unlock()
		return
	}
}

// Wait returns once no send is running.
func (n *Notifier) Wait() { n.wg.Wait() }

// Close stops new sends, ends the running ones and waits for them. A push Expo took just before
// may go unrecorded and be sent once more after a restart, as the PRD accepts.
func (n *Notifier) Close() {
	n.mu.Lock()
	n.stopped = true
	n.mu.Unlock()
	n.cancel()
	n.wg.Wait()
}

// send pushes every parent of the agent's latest snapshot to every device that should hear of it
// and has not yet.
func (n *Notifier) send(agentID string) {
	a, ok := n.live.Agent(agentID)
	if !ok {
		return
	}
	current := make(map[pair]bool)
	for _, p := range a.Parents {
		current[pair{agentID: agentID, parent: store.ParentKey{PID: p.PID, StartTime: p.StartTime}, firstDetected: p.FirstDetectedAt}] = true
	}
	n.forgetGone(agentID, current)

	targets, err := n.store.PushTargets(n.ctx, agentID)
	if err != nil {
		n.log.Error("list push targets", "agent", agentID, "err", err)
		return
	}
	done, err := n.store.NotifiedOf(n.ctx, agentID)
	if err != nil {
		n.log.Error("list notified parents", "agent", agentID, "err", err)
		return
	}
	notified := make(map[store.Notified]bool, len(done))
	for _, d := range done {
		notified[d] = true
	}
	// Pairs Expo took on an earlier send whose record failed: record them now, never send them.
	// Those recorded are added to notified.
	n.recordAccepted(agentID, notified)

	var msgs []Message
	var pairs []pair
	for _, p := range a.Parents {
		key := store.ParentKey{PID: p.PID, StartTime: p.StartTime}
		for _, t := range targets {
			pr := pair{agentID: agentID, parent: key, firstDetected: p.FirstDetectedAt, deviceID: t.DeviceID}
			if notified[store.Notified{Parent: key, DeviceID: t.DeviceID}] || n.isAccepted(pr) {
				continue
			}
			msgs = append(msgs, Message{
				To:       t.PushToken,
				Title:    "Killy",
				Body:     fmt.Sprintf("zombie ×%d on %s\n……！", len(p.Zombies), t.Name),
				Sound:    "default",
				Priority: "high",
			})
			pairs = append(pairs, pr)
		}
	}
	for start := 0; start < len(msgs); start += MaxMessages {
		end := min(start+MaxMessages, len(msgs))
		n.sendBatch(agentID, msgs[start:end], pairs[start:end])
	}
}

func (n *Notifier) sendBatch(agentID string, msgs []Message, pairs []pair) {
	tickets, err := n.sender.Send(n.ctx, msgs)
	if err == nil && len(tickets) != len(msgs) {
		// A ticket cannot be matched to its pair, so none of them can be said to be accepted.
		err = fmt.Errorf("%d tickets for %d messages", len(tickets), len(msgs))
	}
	var record []pair
	for i, pr := range pairs {
		switch {
		case err != nil:
			if n.failed(pr) {
				record = append(record, pr)
			}
		case tickets[i].Status == "ok":
			record = append(record, pr)
		case tickets[i].Details.Error == DeviceNotRegistered:
			// Neither notified nor a failure: the device gets nothing until the app registers again.
			if err := n.store.ClearPushToken(n.ctx, pr.deviceID, msgs[i].To); err != nil {
				n.log.Error("clear push token", "device", pr.deviceID, "err", err)
			}
		default:
			if n.failed(pr) {
				record = append(record, pr)
			}
		}
	}
	if err != nil {
		n.log.Error("send push", "agent", agentID, "messages", len(msgs), "err", err)
	}
	n.record(agentID, record)
}

// failed counts a failed send and reports whether the pair is now given up.
func (n *Notifier) failed(pr pair) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.failures[pr]++
	return n.failures[pr] >= MaxAttempts
}

// record stores the pairs as notified; if the store fails they are kept as accepted, so they are
// not sent again and are recorded on a later send.
func (n *Notifier) record(agentID string, prs []pair) {
	if len(prs) == 0 {
		return
	}
	rows := make([]store.Notified, len(prs))
	for i, pr := range prs {
		rows[i] = store.Notified{Parent: pr.parent, DeviceID: pr.deviceID}
	}
	err := n.store.RecordNotified(n.ctx, agentID, rows)
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, pr := range prs {
		delete(n.failures, pr)
		if err != nil {
			n.accepted[pr] = true
		}
	}
	if err != nil {
		n.log.Error("record notified parents", "agent", agentID, "pairs", len(prs), "err", err)
	}
}

func (n *Notifier) recordAccepted(agentID string, notified map[store.Notified]bool) {
	n.mu.Lock()
	var prs []pair
	for pr := range n.accepted {
		if pr.agentID != agentID {
			continue
		}
		if notified[store.Notified{Parent: pr.parent, DeviceID: pr.deviceID}] {
			delete(n.accepted, pr)
			continue
		}
		prs = append(prs, pr)
	}
	n.mu.Unlock()
	if len(prs) == 0 {
		return
	}
	rows := make([]store.Notified, len(prs))
	for i, pr := range prs {
		rows[i] = store.Notified{Parent: pr.parent, DeviceID: pr.deviceID}
	}
	if err := n.store.RecordNotified(n.ctx, agentID, rows); err != nil {
		n.log.Error("record notified parents", "agent", agentID, "pairs", len(prs), "err", err)
		return
	}
	n.mu.Lock()
	for _, pr := range prs {
		delete(n.accepted, pr)
		notified[store.Notified{Parent: pr.parent, DeviceID: pr.deviceID}] = true
	}
	n.mu.Unlock()
}

func (n *Notifier) isAccepted(pr pair) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.accepted[pr]
}

// forgetGone drops the counts of the agent's pairs whose parent is no longer in the snapshot.
func (n *Notifier) forgetGone(agentID string, current map[pair]bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	parentOf := func(pr pair) pair { pr.deviceID = ""; return pr }
	for pr := range n.failures {
		if pr.agentID == agentID && !current[parentOf(pr)] {
			delete(n.failures, pr)
		}
	}
	for pr := range n.accepted {
		if pr.agentID == agentID && !current[parentOf(pr)] {
			delete(n.accepted, pr)
		}
	}
}
