package sim

import (
	"container/heap"
	"fmt"
)

// Config sets up a Simulator.
type Config struct {
	// MinDelay and MaxDelay bound how long a message takes to arrive. Each
	// message's delay is chosen at random from this range, so messages can
	// overtake each other.
	MinDelay Time
	MaxDelay Time

	// TickInterval is how often every node's Tick is called, in simulated
	// time, for a node whose clock runs at normal speed. 0 means nodes are
	// never ticked.
	TickInterval Time

	// DropRate is the probability (0 to 1) that a message is lost.
	DropRate float64
	// DupRate is the probability (0 to 1) that a message that is not lost
	// is delivered twice.
	DupRate float64
}

// NodeFactory builds a node. The simulator calls it when a node is added and
// again on every restart, passing the same Disk each time. A node must rebuild
// all of its state from the disk and nothing else, exactly as a real process
// would after a crash.
type NodeFactory func(id NodeID, disk *Disk) Node

// Reasons passed to OnDrop.
const (
	DropLost      = "lost"
	DropPartition = "partition"
	DropNoNode    = "no such node"
	DropNodeDown  = "node down"
)

// peer is the simulator's bookkeeping for one node.
type peer struct {
	node    Node        // nil while crashed
	factory NodeFactory // nil if the node was added with AddNode (cannot restart)
	disk    *Disk       // nil if the node was added with AddNode
	up      bool

	// Local clock. speed is a percentage of true speed: 100 is normal, 200
	// ticks twice as often, 50 half as often.
	speed     int
	tickBase  Time  // when the current tick run began
	tickCount int64 // ticks scheduled since tickBase
	tickEpoch int   // bumped to cancel already-scheduled ticks
}

// Simulator runs a set of Nodes in a world it fully controls: one thread,
// one fake clock, one seeded random source. Given the same seed, config and
// nodes, a run unfolds identically every time.
type Simulator struct {
	cfg      Config
	clock    *Clock
	rng      *Rand
	crashRng *Rand
	net      *Network

	peers   map[NodeID]*peer
	queue   eventQueue
	nextSeq uint64

	// OnDeliver, if set, is called just before each message is handed to its
	// destination node. Use it to record what happened.
	OnDeliver func(at Time, msg Message)

	// OnDrop, if set, is called whenever a message is lost, with one of the
	// Drop* reasons.
	OnDrop func(at Time, msg Message, reason string)
}

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
		cfg:      cfg,
		clock:    NewClock(),
		rng:      rng,
		crashRng: rng.Fork("crashes"),
		net:      net,
		peers:    make(map[NodeID]*peer),
	}
}

// Seed returns the seed this run was created with. Print it when a test
// fails so the run can be replayed.
func (s *Simulator) Seed() uint64 { return s.rng.Seed() }

// Now returns the current simulated time.
func (s *Simulator) Now() Time { return s.clock.Now() }

// Network returns the simulated network, so a test can change its faults
// mid-run: SetDropRate, SetDupRate, Partition and Heal.
func (s *Simulator) Network() *Network { return s.net }

func (s *Simulator) dropped(msg Message, reason string) {
	if s.OnDrop != nil {
		s.OnDrop(s.Now(), msg, reason)
	}
}

// AddNode adds a node that has no disk. It can be crashed but not restarted,
// because there is nothing to rebuild it from. It panics on a duplicate id.
func (s *Simulator) AddNode(id NodeID, n Node) {
	s.addPeer(id, &peer{node: n, up: true, speed: 100})
}

// AddNodeWithDisk adds a node built by factory, with its own simulated disk.
// This node can be crashed and restarted. It panics on a duplicate id.
func (s *Simulator) AddNodeWithDisk(id NodeID, factory NodeFactory) {
	disk := &Disk{}
	s.addPeer(id, &peer{node: factory(id, disk), factory: factory, disk: disk, up: true, speed: 100})
}

func (s *Simulator) addPeer(id NodeID, p *peer) {
	if _, dup := s.peers[id]; dup {
		panic("sim: duplicate node id")
	}
	s.peers[id] = p
	s.startTicks(id, p)
}

func (s *Simulator) mustPeer(id NodeID) *peer {
	p, ok := s.peers[id]
	if !ok {
		panic(fmt.Sprintf("sim: unknown node %d", id))
	}
	return p
}

// Node returns the running node with this id, or nil if it is crashed or
// does not exist. After a restart it returns the new instance.
func (s *Simulator) Node(id NodeID) Node {
	if p, ok := s.peers[id]; ok {
		return p.node
	}
	return nil
}

// IsUp reports whether the node exists and is running.
func (s *Simulator) IsUp(id NodeID) bool {
	p, ok := s.peers[id]
	return ok && p.up
}

// Disk returns the node's simulated disk, or nil if it has none.
func (s *Simulator) Disk(id NodeID) *Disk {
	if p, ok := s.peers[id]; ok {
		return p.disk
	}
	return nil
}

// Crash kills a node instantly. Its in-memory state is gone, it receives no
// more messages or ticks, and any bytes it appended to its disk but never
// synced are partly lost. Messages it already sent are still on their way.
// Crashing a node that is already down does nothing.
func (s *Simulator) Crash(id NodeID) {
	p := s.mustPeer(id)
	if !p.up {
		return
	}
	p.up = false
	p.node = nil
	p.tickEpoch++ // cancel its scheduled ticks
	if p.disk != nil {
		p.disk.crash(s.crashRng)
	}
}

// Restart brings a crashed node back as a brand-new instance built by its
// factory from whatever survived on its disk. Messages that arrive from now
// on reach the new instance. It panics if the node is running or was added
// without a factory.
func (s *Simulator) Restart(id NodeID) {
	p := s.mustPeer(id)
	if p.up {
		panic(fmt.Sprintf("sim: node %d is already running", id))
	}
	if p.factory == nil {
		panic(fmt.Sprintf("sim: node %d was added with AddNode and cannot restart; use AddNodeWithDisk", id))
	}
	p.node = p.factory(id, p.disk)
	p.up = true
	s.startTicks(id, p)
}

// SetClockSpeed sets how fast a node's local clock runs, as a percentage of
// true speed: 100 is normal, 200 means it ticks twice as often, 50 half as
// often. Valid values are 1 to 10000. The change takes effect from now.
func (s *Simulator) SetClockSpeed(id NodeID, percent int) {
	if percent < 1 || percent > 10000 {
		panic(fmt.Sprintf("sim: clock speed must be between 1 and 10000 percent, got %d", percent))
	}
	p := s.mustPeer(id)
	p.speed = percent
	if p.up {
		s.startTicks(id, p)
	}
}

// startTicks cancels any scheduled ticks for the node and starts a fresh run
// of them, the first one a tick interval (scaled by the node's clock speed)
// from now.
func (s *Simulator) startTicks(id NodeID, p *peer) {
	if s.cfg.TickInterval <= 0 {
		return
	}
	p.tickEpoch++
	p.tickBase = s.Now()
	p.tickCount = 0
	s.scheduleTick(id, p)
}

// scheduleTick schedules the node's next tick. Computing each tick's time
// from the run's start (instead of adding a rounded interval each time)
// keeps the rate exact even when the interval is not a whole number.
func (s *Simulator) scheduleTick(id NodeID, p *peer) {
	p.tickCount++
	offset := p.tickCount * int64(s.cfg.TickInterval) * 100 / int64(p.speed)
	s.schedule(&event{at: p.tickBase + Time(offset), kind: evTick, node: id, epoch: p.tickEpoch})
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
		p, ok := s.peers[e.node]
		if !ok || !p.up || e.epoch != p.tickEpoch {
			break // crashed, or this tick was cancelled by a restart or speed change
		}
		s.sendAll(p.node.Tick())
		s.scheduleTick(e.node, p)
	case evDeliver:
		p, ok := s.peers[e.msg.To]
		switch {
		case !ok:
			s.dropped(e.msg, DropNoNode)
		case !p.up:
			s.dropped(e.msg, DropNodeDown)
		case s.net.Blocked(e.msg.From, e.msg.To):
			// The partition started while the message was in flight.
			s.dropped(e.msg, DropPartition)
		default:
			if s.OnDeliver != nil {
				s.OnDeliver(s.Now(), e.msg)
			}
			s.sendAll(p.node.HandleMessage(e.msg))
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
