package sim

// Network decides what happens to each message a node sends: how long it
// takes to arrive and, in the next chunk, whether it is lost or duplicated.
// It never delivers anything itself; the Simulator schedules deliveries from
// the plan the Network returns.
type Network struct {
	minDelay Time
	maxDelay Time
	rng      *Rand
}

// NewNetwork returns a network whose message delays are drawn uniformly from
// [minDelay, maxDelay] using rng. It panics on an invalid range.
func NewNetwork(minDelay, maxDelay Time, rng *Rand) *Network {
	if minDelay < 0 || maxDelay < minDelay {
		panic("sim: network delay range must satisfy 0 <= min <= max")
	}
	return &Network{minDelay: minDelay, maxDelay: maxDelay, rng: rng}
}

// Route returns one delay for each copy of msg that should be delivered, in
// simulated time units from now. An empty result would mean the message was
// lost. For now every message produces exactly one copy.
func (n *Network) Route(msg Message) []Time {
	delay := Time(n.rng.Range(int(n.minDelay), int(n.maxDelay)))
	return []Time{delay}
}
