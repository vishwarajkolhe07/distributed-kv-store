package sim

import (
	"strconv"
	"strings"
)

// Write asks a LogServer to store a number. Ack tells the sender it is stored.
type Write struct{ ID int }
type Ack struct{ ID int }

// SyncMode says when a LogServer syncs a write to its disk.
type SyncMode int

const (
	// SyncBeforeAck syncs, then acknowledges. Correct: an acknowledged write
	// survives any crash.
	SyncBeforeAck SyncMode = iota
	// SyncOnNextTick acknowledges first and syncs on the next Tick. This is a
	// planted bug: a crash in the gap loses a write that was already
	// acknowledged. It only bites when a crash lands in that narrow window,
	// which is the kind of bug a seed search exists to find.
	SyncOnNextTick
)

// LogServer is a toy durable server: it appends each Write's ID to its disk
// as a line of text and answers with an Ack. Its promise is that every Ack
// it has sent is still on disk after any crash.
type LogServer struct {
	ID    NodeID
	disk  *Disk
	mode  SyncMode
	dirty bool // appended but not yet synced
}

var _ Node = (*LogServer)(nil)

// NewLogServerFactory returns a factory for LogServers using the given mode.
func NewLogServerFactory(mode SyncMode) NodeFactory {
	return func(id NodeID, disk *Disk) Node {
		return &LogServer{ID: id, disk: disk, mode: mode}
	}
}

func (n *LogServer) HandleMessage(msg Message) []Message {
	w, ok := msg.Payload.(Write)
	if !ok {
		return nil
	}
	n.disk.Append([]byte(strconv.Itoa(w.ID) + "\n"))
	if n.mode == SyncBeforeAck {
		n.disk.Sync()
	} else {
		n.dirty = true
	}
	return []Message{{From: n.ID, To: msg.From, Payload: Ack{ID: w.ID}}}
}

func (n *LogServer) Tick() []Message {
	if n.dirty {
		n.disk.Sync()
		n.dirty = false
	}
	return nil
}

// StoredIDs returns the IDs written in a LogServer's disk contents. Only
// complete lines count; a half-written last line is ignored.
func StoredIDs(disk []byte) map[int]bool {
	out := make(map[int]bool)
	lines := strings.Split(string(disk), "\n")
	for _, line := range lines[:len(lines)-1] { // the last piece has no newline after it
		if id, err := strconv.Atoi(line); err == nil {
			out[id] = true
		}
	}
	return out
}

// Writer is a toy client: every Period ticks it sends the next Write to
// Server, and it remembers every Ack it gets.
type Writer struct {
	ID     NodeID
	Server NodeID
	Period int

	Acked []int // IDs acknowledged, in arrival order

	ticks int
	next  int
}

var _ Node = (*Writer)(nil)

func (w *Writer) Tick() []Message {
	w.ticks++
	if w.Period <= 0 || w.ticks%w.Period != 0 {
		return nil
	}
	m := Message{From: w.ID, To: w.Server, Payload: Write{ID: w.next}}
	w.next++
	return []Message{m}
}

func (w *Writer) HandleMessage(msg Message) []Message {
	if a, ok := msg.Payload.(Ack); ok {
		w.Acked = append(w.Acked, a.ID)
	}
	return nil
}
