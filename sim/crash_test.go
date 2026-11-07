package sim

import (
	"reflect"
	"testing"
)

// diskNode is a Node that keeps one byte on its disk per "write" it handles,
// plus some volatile (memory-only) counters.
type diskNode struct {
	disk      *Disk
	onDisk    int // bytes found on the disk when this instance was built
	sinceBoot int // writes handled by this instance
	ticks     int
}

func newDiskNode(_ NodeID, disk *Disk) Node {
	return &diskNode{disk: disk, onDisk: len(disk.Bytes())}
}

const (
	writeSynced   = "write-synced"
	writeUnsynced = "write-unsynced"
)

func (n *diskNode) HandleMessage(m Message) []Message {
	switch m.Payload {
	case writeSynced:
		n.disk.Append([]byte{'x'})
		n.disk.Sync()
		n.sinceBoot++
	case writeUnsynced:
		n.disk.Append([]byte{'x'})
		n.sinceBoot++
	}
	return nil
}

func (n *diskNode) Tick() []Message {
	n.ticks++
	return nil
}

func diskSim(seed uint64, cfg Config) *Simulator {
	s := NewSimulator(seed, cfg)
	s.AddNodeWithDisk(1, newDiskNode)
	return s
}

func send(s *Simulator, payload any) {
	s.Send(Message{From: 0, To: 1, Payload: payload})
}

func TestCrashedNodeGetsNoMessagesOrTicks(t *testing.T) {
	s := diskSim(1, Config{MinDelay: 1, MaxDelay: 1, TickInterval: 10})
	n := s.Node(1).(*diskNode)
	s.RunUntil(30)
	if n.ticks != 3 {
		t.Fatalf("ticks before crash = %d, want 3", n.ticks)
	}

	var reasons []string
	s.OnDrop = func(_ Time, _ Message, r string) { reasons = append(reasons, r) }
	s.Crash(1)
	if s.IsUp(1) || s.Node(1) != nil {
		t.Error("node should be down after Crash")
	}
	send(s, writeSynced)
	s.RunUntil(200)

	if n.ticks != 3 {
		t.Errorf("crashed node kept ticking: %d ticks", n.ticks)
	}
	if want := []string{DropNodeDown}; !reflect.DeepEqual(reasons, want) {
		t.Errorf("drop reasons = %v, want %v", reasons, want)
	}
}

func TestRestartBuildsFreshInstanceWithEmptyMemory(t *testing.T) {
	s := diskSim(1, Config{MinDelay: 1, MaxDelay: 1, TickInterval: 10})
	old := s.Node(1).(*diskNode)
	send(s, writeSynced)
	s.RunUntil(25)
	if old.sinceBoot != 1 || old.ticks != 2 {
		t.Fatalf("setup: sinceBoot=%d ticks=%d", old.sinceBoot, old.ticks)
	}

	s.Crash(1)
	s.Restart(1)
	fresh := s.Node(1).(*diskNode)

	if fresh == old {
		t.Fatal("Restart reused the old instance; memory must not survive a crash")
	}
	if fresh.sinceBoot != 0 || fresh.ticks != 0 {
		t.Errorf("fresh instance has leftover memory: sinceBoot=%d ticks=%d", fresh.sinceBoot, fresh.ticks)
	}
	if !s.IsUp(1) {
		t.Error("node should be up after Restart")
	}
}

func TestRestartDoesNotDoubleTheTicks(t *testing.T) {
	s := diskSim(1, Config{TickInterval: 10})
	s.RunUntil(25)
	s.Crash(1) // old tick (due at 30) is still queued
	s.Restart(1)

	// New instance ticks at 35, 45, ..., 95: exactly 7 times by time 100.
	// A leftover tick chain from before the crash would make this 14.
	s.RunUntil(100)
	if got := s.Node(1).(*diskNode).ticks; got != 7 {
		t.Errorf("ticks after restart = %d, want 7", got)
	}
}

func TestSyncedWritesSurviveCrash(t *testing.T) {
	s := diskSim(1, Config{MinDelay: 1, MaxDelay: 1})
	for i := 0; i < 5; i++ {
		send(s, writeSynced)
	}
	s.RunUntil(10)
	s.Crash(1)
	s.Restart(1)

	if got := s.Node(1).(*diskNode).onDisk; got != 5 {
		t.Errorf("after restart found %d bytes on disk, want 5", got)
	}
}

func TestUnsyncedWritesAreTornAtCrash(t *testing.T) {
	// 3 synced bytes then 10 unsynced. Whatever the seed, the synced ones must
	// survive and no more than 13 can; across seeds the cut point must vary.
	lengths := map[int]bool{}
	for seed := uint64(1); seed <= 60; seed++ {
		s := diskSim(seed, Config{MinDelay: 1, MaxDelay: 1})
		for i := 0; i < 3; i++ {
			send(s, writeSynced)
		}
		for i := 0; i < 10; i++ {
			send(s, writeUnsynced)
		}
		s.RunUntil(10)
		s.Crash(1)

		got := len(s.Disk(1).Bytes())
		if got < 3 || got > 13 {
			t.Fatalf("seed %d: %d bytes survived, want between 3 and 13", seed, got)
		}
		lengths[got] = true
	}
	if len(lengths) < 5 {
		t.Errorf("only %d distinct survivor lengths across 60 seeds; the cut point is not random", len(lengths))
	}
	if !lengths[3] && !lengths[13] {
		t.Log("note: neither extreme (all lost / all kept) was hit; fine, just unlucky")
	}
}

func TestCrashLossIsDeterministicPerSeed(t *testing.T) {
	survivors := func(seed uint64) int {
		s := diskSim(seed, Config{MinDelay: 1, MaxDelay: 1})
		for i := 0; i < 50; i++ {
			send(s, writeUnsynced)
		}
		s.RunUntil(10)
		s.Crash(1)
		return len(s.Disk(1).Bytes())
	}
	if a, b := survivors(77), survivors(77); a != b {
		t.Errorf("same seed kept %d then %d bytes", a, b)
	}
}

func TestCrashTwiceAndBadCallsAreHandled(t *testing.T) {
	s := diskSim(1, Config{})
	s.Crash(1)
	s.Crash(1) // already down: no-op, must not panic or tear the disk twice

	mustPanic(t, "Crash of unknown node", func() { s.Crash(99) })
	mustPanic(t, "Restart of unknown node", func() { s.Restart(99) })

	s.Restart(1)
	mustPanic(t, "Restart of a running node", func() { s.Restart(1) })

	plain := NewSimulator(1, Config{})
	plain.AddNode(2, &recorder{})
	plain.Crash(2)
	mustPanic(t, "Restart of a node added with AddNode", func() { plain.Restart(2) })
}

func TestMessagesSentBeforeCrashStillArrive(t *testing.T) {
	s := NewSimulator(1, Config{MinDelay: 10, MaxDelay: 10})
	r := &recorder{}
	s.AddNode(2, r)
	s.AddNodeWithDisk(1, newDiskNode)

	s.Send(Message{From: 1, To: 2, Payload: 0}) // on the wire, due at 10
	s.RunUntil(3)
	s.Crash(1)
	s.RunUntil(20)

	if len(r.got) != 1 {
		t.Errorf("message from a node that then crashed: delivered %d times, want 1", len(r.got))
	}
}

func TestMessageInFlightReachesNewInstanceAfterRestart(t *testing.T) {
	s := diskSim(1, Config{MinDelay: 10, MaxDelay: 10})
	send(s, writeSynced) // due at 10
	s.RunUntil(2)
	s.Crash(1)
	s.RunUntil(5)
	s.Restart(1) // back before the message lands
	fresh := s.Node(1).(*diskNode)
	s.RunUntil(20)

	if fresh.sinceBoot != 1 {
		t.Errorf("new instance handled %d writes, want 1", fresh.sinceBoot)
	}
}

func TestPingerLosesMemoryButResumesAfterRestart(t *testing.T) {
	cfg := Config{MinDelay: 1, MaxDelay: 3, TickInterval: 1}
	s := NewSimulator(3, cfg)
	s.AddNodeWithDisk(1, func(id NodeID, _ *Disk) Node { return &PingNode{ID: id, Peer: 2, Period: 5} })
	s.AddNode(2, &PingNode{ID: 2, Peer: 1})

	s.RunUntil(100)
	before := len(s.Node(1).(*PingNode).Pongs)
	if before == 0 {
		t.Fatal("no pongs before the crash")
	}

	s.Crash(1)
	s.RunUntil(150)
	s.Restart(1)
	if got := len(s.Node(1).(*PingNode).Pongs); got != 0 {
		t.Errorf("restarted node remembers %d pongs; memory should be gone", got)
	}
	s.RunUntil(300)
	if got := len(s.Node(1).(*PingNode).Pongs); got == 0 {
		t.Error("restarted node never got a pong back")
	}
}
