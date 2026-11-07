package sim

import "fmt"

// Network decides what happens to each message a node sends: how long it
// takes to arrive, whether it is lost or duplicated, and whether a partition
// blocks it. It never delivers anything itself; the Simulator schedules
// deliveries from the plan the Network returns.
type Network struct {
	minDelay Time
	maxDelay Time
	dropRate float64
	dupRate  float64
	rng      *Rand

	// group maps a node to its side of the partition. nil means no partition.
	// Nodes missing from the map are in group 0.
	group map[NodeID]int
}

// NewNetwork returns a network whose message delays are drawn uniformly from
// [minDelay, maxDelay] using rng. It starts with no loss, no duplication and
// no partition. It panics on an invalid delay range.
func NewNetwork(minDelay, maxDelay Time, rng *Rand) *Network {
	if minDelay < 0 || maxDelay < minDelay {
		panic("sim: network delay range must satisfy 0 <= min <= max")
	}
	return &Network{minDelay: minDelay, maxDelay: maxDelay, rng: rng}
}

func checkRate(name string, p float64) {
	if p < 0 || p > 1 {
		panic(fmt.Sprintf("sim: %s must be between 0 and 1, got %v", name, p))
	}
}

// SetDropRate sets the probability that any one message is lost.
func (n *Network) SetDropRate(p float64) {
	checkRate("drop rate", p)
	n.dropRate = p
}

// SetDupRate sets the probability that a message that is not lost is
// delivered twice. Each copy gets its own random delay, so the copies can
// arrive far apart and out of order.
func (n *Network) SetDupRate(p float64) {
	checkRate("duplicate rate", p)
	n.dupRate = p
}

// Partition splits the cluster into isolated groups. Nodes in the same group
// can talk to each other; messages between different groups are lost. Nodes
// not listed in any group stay together in one extra group of their own.
// A later call replaces the previous partition.
func (n *Network) Partition(groups ...[]NodeID) {
	n.group = make(map[NodeID]int)
	for i, g := range groups {
		for _, id := range g {
			n.group[id] = i + 1 // group 0 is "everyone not listed"
		}
	}
}

// Heal removes any partition. Messages that were lost while it lasted stay lost.
func (n *Network) Heal() {
	n.group = nil
}

// Partitioned reports whether a partition is currently in effect.
func (n *Network) Partitioned() bool {
	return n.group != nil
}

// Blocked reports whether a partition currently separates from and to.
func (n *Network) Blocked(from, to NodeID) bool {
	if n.group == nil {
		return false
	}
	return n.group[from] != n.group[to]
}

func (n *Network) delay() Time {
	return Time(n.rng.Range(int(n.minDelay), int(n.maxDelay)))
}

// Route returns one delay for each copy of msg that should be delivered, in
// simulated time units from now. An empty result means the message is lost,
// either at random or because a partition blocks it.
//
// Route always draws the same four random numbers per message, whatever the
// outcome. So changing the drop rate, or adding a partition, never changes
// the delay a surviving message gets, which keeps seeds comparable across
// different fault settings.
func (n *Network) Route(msg Message) []Time {
	dropped := n.rng.Chance(n.dropRate)
	duplicated := n.rng.Chance(n.dupRate)
	first, second := n.delay(), n.delay()

	if dropped || n.Blocked(msg.From, msg.To) {
		return nil
	}
	if duplicated {
		return []Time{first, second}
	}
	return []Time{first}
}
