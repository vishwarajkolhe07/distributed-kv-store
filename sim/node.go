// Package sim is a deterministic simulator for distributed systems.
//
// The one rule everything here depends on: a Node is a state machine that
// only reacts to events. It never reads the real clock, never uses real
// randomness, never starts goroutines and never touches the real network.
// Everything it needs arrives as an argument, and everything it wants to do
// leaves as a return value. That is what lets the simulator control time,
// delivery order and failures, and replay any run exactly from a seed.
package sim

// NodeID identifies a machine in the simulated cluster.
type NodeID int

// Message is something one node sends to another. The simulator, not the
// node, decides when (and whether) it is delivered.
type Message struct {
	From    NodeID
	To      NodeID
	Payload any
}

// Node is one machine, written as a pure reaction to events.
//
// A Node must be deterministic: the same sequence of calls must always
// produce the same returned messages and the same internal state. So a Node
// must not:
//   - call time.Now, time.Sleep or use timers (time arrives as Tick calls),
//   - use math/rand or crypto/rand (the simulator will hand out seeded
//     randomness in a later chunk),
//   - start goroutines or use channels, mutexes or other concurrency,
//   - open sockets or call anything else that talks to the outside world.
//
// Both methods return the messages the node wants sent as a result of the
// event. Returning nil or an empty slice means "send nothing".
type Node interface {
	// HandleMessage is called when a message is delivered to this node.
	HandleMessage(msg Message) []Message

	// Tick is called when one unit of this node's local time passes.
	// Timeouts are counted in ticks, for example "start an election after
	// 10 ticks without a heartbeat".
	Tick() []Message
}
