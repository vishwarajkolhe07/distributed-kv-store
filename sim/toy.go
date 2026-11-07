package sim

// Ping and Pong are the payloads PingNode exchanges.
type Ping struct{ Seq int }
type Pong struct{ Seq int }

// PingNode is a tiny example Node used to test the simulator itself, before
// we trust it with Raft.
//
//   - Every Period ticks it sends Ping{0}, Ping{1}, ... to Peer.
//   - It answers any Ping it receives with a Pong carrying the same Seq.
//   - It records the Seq of every Pong it receives in Pongs.
//
// Set Period to 0 to make a node that only answers and never starts a ping.
type PingNode struct {
	ID     NodeID
	Peer   NodeID
	Period int

	Pongs []int // Seq of each Pong received, in arrival order

	ticks   int
	nextSeq int
}

// Compile-time check that PingNode satisfies Node.
var _ Node = (*PingNode)(nil)

func (n *PingNode) Tick() []Message {
	n.ticks++
	if n.Period <= 0 || n.ticks%n.Period != 0 {
		return nil
	}
	m := Message{From: n.ID, To: n.Peer, Payload: Ping{Seq: n.nextSeq}}
	n.nextSeq++
	return []Message{m}
}

func (n *PingNode) HandleMessage(msg Message) []Message {
	switch p := msg.Payload.(type) {
	case Ping:
		return []Message{{From: n.ID, To: msg.From, Payload: Pong{Seq: p.Seq}}}
	case Pong:
		n.Pongs = append(n.Pongs, p.Seq)
	}
	return nil
}
