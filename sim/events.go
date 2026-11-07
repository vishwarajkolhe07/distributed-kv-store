package sim

// eventKind says what happens when an event comes due.
type eventKind int

const (
	evDeliver eventKind = iota // hand msg to its destination node
	evTick                     // call Tick on node
)

// event is something scheduled to happen at a simulated time.
type event struct {
	at    Time
	seq   uint64 // order the event was scheduled in; breaks ties between equal times
	kind  eventKind
	msg   Message // for evDeliver
	node  NodeID  // for evTick
	epoch int     // for evTick: ignored if the node's tick epoch has moved on
}

// eventQueue is a min-heap of events ordered by (at, seq). It implements
// container/heap.Interface; use heap.Push and heap.Pop on it.
//
// Ordering by seq as well as time is what makes runs deterministic: two
// events due at the same instant always fire in the order they were
// scheduled, never in an order that depends on the heap's internals.
type eventQueue []*event

func (q eventQueue) Len() int { return len(q) }

func (q eventQueue) Less(i, j int) bool {
	if q[i].at != q[j].at {
		return q[i].at < q[j].at
	}
	return q[i].seq < q[j].seq
}

func (q eventQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }

func (q *eventQueue) Push(x any) { *q = append(*q, x.(*event)) }

func (q *eventQueue) Pop() any {
	old := *q
	n := len(old)
	e := old[n-1]
	old[n-1] = nil // let the garbage collector have it
	*q = old[:n-1]
	return e
}
