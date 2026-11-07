package sim

import "container/heap"

// Config sets up a Simulator.
type Config struct {
	// MinDelay and MaxDelay bound how long a message takes to arrive. Each
	// message's delay is chosen at random from this range, so messages can
	// overtake each other.
	MinDelay Time
	MaxDelay Time

	// TickInterval is how often every node's Tick is called, in simulated
	// time. 0 means nodes are never ticked.
	TickInterval Time

	// DropRate is the probability (0 to 1) that a message is lost.
	DropRate float64
	// DupRate is the probability (0 to 1) that a message that is not lost
	// is delivered twice.
	DupRate float64
}

// Simulator runs a set of Nodes in a world it fully controls: one thread,
// one fake clock, one seeded random source. Given the same seed, config and
// nodes, a run unfolds identically every time.
type Simulator struct {
	cfg   Config
	clock *Clock
	rng   *Rand
	net   *Network

	nodes   map[NodeID]Node
	queue   eventQueue
	nextSeq uint64

	// OnDeliver, if set, is called just before each message is handed to its
	// destination node. Use it to record what happened.
	OnDeliver func(at Time, msg Message)

	// OnDrop, if set, is called whenever a message is lost, with the reason:
	// "lost" (random loss), "partition" or "no such node".
	OnDrop func(at Time, msg Message, reason string)
}

// Reasons passed to OnDrop.
const (
	DropLost      = "lost"
	DropPartition = "partition"
	DropNoNode    = "no such node"
)

// NewSimulator creates an empty simulation. It panics if cfg is invalid.
func NewSimulator(seed uint64, cfg Config) *Simulator {
	if cfg.TickInterval < 0 {
		panic("sim: TickInterval must not be negative")
	}
	rng := NewRand(seed)
	net := NewNetwork(cfg.MinDelay, cfg.MaxDelay, rng.Fork("network"))
	net.SetDropRate(cfg.DropRate)
	net.SetDupRate(cfg.DupRate)
	return &Simulator{
		cfg:   cfg,
		clock: NewClock(),
		rng:   rng,
		net:   net,
		nodes: make(map[NodeID]Node),
	}
}

// Network returns the simulated network, so a test can change its faults
// mid-run: SetDropRate, SetDupRate, Partition and Heal.
func (s *Simulator) Network() *Network { return s.net }

func (s *Simulator) dropped(msg Message, reason string) {
	if s.OnDrop != nil {
		s.OnDrop(s.Now(), msg, reason)
	}
}

// Seed returns the seed this run was created with. Print it when a test
// fails so the run can be replayed.
func (s *Simulator) Seed() uint64 { return s.rng.Seed() }

// Now returns the current simulated time.
func (s *Simulator) Now() Time { return s.clock.Now() }

// AddNode adds a node to the cluster and, if ticking is on, schedules its
// first Tick one TickInterval from now. It panics on a duplicate id.
func (s *Simulator) AddNode(id NodeID, n Node) {
	if _, dup := s.nodes[id]; dup {
		panic("sim: duplicate node id")
	}
	s.nodes[id] = n
	if s.cfg.TickInterval > 0 {
		s.schedule(&event{at: s.Now() + s.cfg.TickInterval, kind: evTick, node: id})
	}
}

// Send puts a message into the network, as if a node had sent it just now.
// It is how tests and clients inject messages from outside the cluster.
func (s *Simulator) Send(msg Message) {
	delays := s.net.Route(msg)
	if len(delays) == 0 {
		if s.net.Blocked(msg.From, msg.To) {
			s.dropped(msg, DropPartition)
		} else {
			s.dropped(msg, DropLost)
		}
		return
	}
	for _, delay := range delays {
		s.schedule(&event{at: s.Now() + delay, kind: evDeliver, msg: msg})
	}
}

func (s *Simulator) schedule(e *event) {
	e.seq = s.nextSeq
	s.nextSeq++
	heap.Push(&s.queue, e)
}

func (s *Simulator) sendAll(msgs []Message) {
	for _, m := range msgs {
		s.Send(m)
	}
}

// Pending returns how many events are still scheduled.
func (s *Simulator) Pending() int { return len(s.queue) }

// Step runs the single earliest scheduled event, moving the clock to its
// time. It returns false, doing nothing, if no events are scheduled.
func (s *Simulator) Step() bool {
	if len(s.queue) == 0 {
		return false
	}
	e := heap.Pop(&s.queue).(*event)
	s.clock.AdvanceTo(e.at)

	switch e.kind {
	case evTick:
		if n, ok := s.nodes[e.node]; ok {
			s.sendAll(n.Tick())
			s.schedule(&event{at: s.Now() + s.cfg.TickInterval, kind: evTick, node: e.node})
		}
	case evDeliver:
		n, ok := s.nodes[e.msg.To]
		switch {
		case !ok:
			s.dropped(e.msg, DropNoNode)
		case s.net.Blocked(e.msg.From, e.msg.To):
			// The partition started while the message was in flight.
			s.dropped(e.msg, DropPartition)
		default:
			if s.OnDeliver != nil {
				s.OnDeliver(s.Now(), e.msg)
			}
			s.sendAll(n.HandleMessage(e.msg))
		}
	}
	return true
}

// RunUntil runs every event due at or before time t, then sets the clock
// to t. Events scheduled for later stay in the queue.
func (s *Simulator) RunUntil(t Time) {
	for len(s.queue) > 0 && s.queue[0].at <= t {
		s.Step()
	}
	if t > s.Now() {
		s.clock.AdvanceTo(t)
	}
}
