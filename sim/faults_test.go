package sim

import (
	"reflect"
	"testing"
)

// countDeliveries sends n messages 0->1 and returns how many were delivered.
func countDeliveries(cfg Config, seed uint64, n int) int {
	s := NewSimulator(seed, cfg)
	s.AddNode(1, &recorder{})
	delivered := 0
	s.OnDeliver = func(Time, Message) { delivered++ }
	sendBurst(s, n)
	for s.Step() {
	}
	return delivered
}

func TestDropRateExtremes(t *testing.T) {
	if got := countDeliveries(Config{MinDelay: 1, MaxDelay: 5, DropRate: 0}, 1, 1000); got != 1000 {
		t.Errorf("DropRate 0 delivered %d of 1000", got)
	}
	if got := countDeliveries(Config{MinDelay: 1, MaxDelay: 5, DropRate: 1}, 1, 1000); got != 0 {
		t.Errorf("DropRate 1 delivered %d of 1000", got)
	}
}

func TestHalfDropRateLosesAboutHalf(t *testing.T) {
	got := countDeliveries(Config{MinDelay: 1, MaxDelay: 5, DropRate: 0.5}, 3, 10000)
	if got < 4700 || got > 5300 {
		t.Errorf("DropRate 0.5 delivered %d of 10000, want about 5000", got)
	}
}

func TestDupRateOneDeliversEverythingTwice(t *testing.T) {
	got := countDeliveries(Config{MinDelay: 1, MaxDelay: 5, DupRate: 1}, 1, 1000)
	if got != 2000 {
		t.Errorf("DupRate 1 delivered %d, want 2000", got)
	}
}

func TestDupRateAddsRoughlyThatManyCopies(t *testing.T) {
	got := countDeliveries(Config{MinDelay: 1, MaxDelay: 5, DupRate: 0.2}, 3, 10000)
	if got < 11700 || got > 12300 {
		t.Errorf("DupRate 0.2 delivered %d of 10000, want about 12000", got)
	}
}

func TestDuplicateCopiesCanArriveAtDifferentTimes(t *testing.T) {
	s := NewSimulator(1, Config{MinDelay: 1, MaxDelay: 50, DupRate: 1})
	s.AddNode(1, &recorder{})
	arrivals := map[int][]Time{}
	s.OnDeliver = func(at Time, m Message) {
		arrivals[m.Payload.(int)] = append(arrivals[m.Payload.(int)], at)
	}
	sendBurst(s, 100)
	for s.Step() {
	}

	apart := 0
	for _, times := range arrivals {
		if len(times) == 2 && times[0] != times[1] {
			apart++
		}
	}
	if apart == 0 {
		t.Error("no duplicate pair arrived at different times; copies share one delay")
	}
}

// Whether a message is dropped must not change the delay of the others.
func TestDropRateDoesNotShiftSurvivorsDelays(t *testing.T) {
	lossless := NewNetwork(1, 100, NewRand(5))
	lossy := NewNetwork(1, 100, NewRand(5))
	lossy.SetDropRate(0.5)

	survivors := 0
	for i := 0; i < 1000; i++ {
		a := lossless.Route(Message{})
		b := lossy.Route(Message{})
		if len(b) == 0 {
			continue
		}
		survivors++
		if a[0] != b[0] {
			t.Fatalf("message %d: delay %d lossless vs %d lossy", i, a[0], b[0])
		}
	}
	if survivors == 0 || survivors == 1000 {
		t.Fatalf("test is vacuous: %d of 1000 survived", survivors)
	}
}

func TestPartitionBlocksTrafficBetweenGroupsOnly(t *testing.T) {
	s := NewSimulator(1, Config{MinDelay: 1, MaxDelay: 3})
	r1, r2, r3 := &recorder{}, &recorder{}, &recorder{}
	s.AddNode(1, r1)
	s.AddNode(2, r2)
	s.AddNode(3, r3)
	s.Network().Partition([]NodeID{1, 2}, []NodeID{3})

	s.Send(Message{From: 1, To: 2, Payload: 0}) // same side: arrives
	s.Send(Message{From: 1, To: 3, Payload: 1}) // across: lost
	s.Send(Message{From: 3, To: 1, Payload: 2}) // across, other way: lost
	s.Send(Message{From: 3, To: 3, Payload: 3}) // to itself: arrives
	s.RunUntil(100)

	if len(r2.got) != 1 {
		t.Errorf("node 2 got %d messages, want 1", len(r2.got))
	}
	if len(r3.got) != 1 || r3.got[0].Payload.(int) != 3 {
		t.Errorf("node 3 should only get its own message, got %+v", r3.got)
	}
	if len(r1.got) != 0 {
		t.Errorf("node 1 got %+v across the partition", r1.got)
	}
}

func TestUnlistedNodesStayTogether(t *testing.T) {
	s := NewSimulator(1, Config{MinDelay: 1, MaxDelay: 1})
	r2, r3 := &recorder{}, &recorder{}
	s.AddNode(1, &recorder{})
	s.AddNode(2, r2)
	s.AddNode(3, r3)
	s.Network().Partition([]NodeID{1}) // 2 and 3 are not listed

	s.Send(Message{From: 2, To: 3, Payload: 0}) // both unlisted: arrives
	s.Send(Message{From: 1, To: 2, Payload: 1}) // 1 is isolated: lost
	s.RunUntil(10)

	if len(r3.got) != 1 {
		t.Errorf("unlisted nodes should reach each other, node 3 got %d", len(r3.got))
	}
	if len(r2.got) != 0 {
		t.Errorf("isolated node 1 reached node 2: %+v", r2.got)
	}
}

func TestHealRestoresTraffic(t *testing.T) {
	s := NewSimulator(1, Config{MinDelay: 1, MaxDelay: 1})
	r := &recorder{}
	s.AddNode(1, &recorder{})
	s.AddNode(2, r)

	s.Network().Partition([]NodeID{1}, []NodeID{2})
	s.Send(Message{From: 1, To: 2, Payload: 0}) // sent during the partition
	s.RunUntil(10)
	s.Network().Heal()
	s.Send(Message{From: 1, To: 2, Payload: 1}) // sent after healing
	s.RunUntil(20)

	// A message lost during the partition stays lost; there is no catch-up.
	if want := []int{1}; !reflect.DeepEqual(r.payloads(), want) {
		t.Errorf("node 2 got %v, want %v", r.payloads(), want)
	}
}

func TestPartitionDropsMessagesAlreadyInFlight(t *testing.T) {
	s := NewSimulator(1, Config{MinDelay: 10, MaxDelay: 10})
	r := &recorder{}
	s.AddNode(1, &recorder{})
	s.AddNode(2, r)

	var reasons []string
	s.OnDrop = func(_ Time, _ Message, reason string) { reasons = append(reasons, reason) }

	s.Send(Message{From: 1, To: 2, Payload: 0}) // would arrive at time 10
	s.RunUntil(5)
	s.Network().Partition([]NodeID{1}, []NodeID{2}) // cut while it is travelling
	s.RunUntil(20)

	if len(r.got) != 0 {
		t.Error("a message in flight when the partition started was still delivered")
	}
	if want := []string{DropPartition}; !reflect.DeepEqual(reasons, want) {
		t.Errorf("drop reasons = %v, want %v", reasons, want)
	}
}

func TestOnDropReportsReasons(t *testing.T) {
	s := NewSimulator(1, Config{MinDelay: 1, MaxDelay: 1, DropRate: 1})
	s.AddNode(1, &recorder{})
	var reasons []string
	s.OnDrop = func(_ Time, _ Message, reason string) { reasons = append(reasons, reason) }

	s.Send(Message{From: 0, To: 1}) // random loss
	s.Network().SetDropRate(0)
	s.Send(Message{From: 0, To: 99}) // nobody home
	s.RunUntil(10)

	if want := []string{DropLost, DropNoNode}; !reflect.DeepEqual(reasons, want) {
		t.Errorf("drop reasons = %v, want %v", reasons, want)
	}
}

func TestFaultyRunsReplayFromSeed(t *testing.T) {
	run := func(seed uint64) []string {
		s := NewSimulator(seed, Config{MinDelay: 1, MaxDelay: 30, DropRate: 0.2, DupRate: 0.2})
		s.AddNode(1, &recorder{})
		sendBurst(s, 300)
		return deliveryTrace(s)
	}
	a, b := run(99), run(99)
	if !reflect.DeepEqual(a, b) {
		t.Error("seed 99 with faults produced two different traces")
	}
	if c := run(100); reflect.DeepEqual(a, c) {
		t.Error("seeds 99 and 100 produced the same faulty trace")
	}
}

func TestInvalidRatesPanic(t *testing.T) {
	mustPanic(t, "DropRate > 1", func() { NewSimulator(1, Config{DropRate: 1.1}) })
	mustPanic(t, "DropRate < 0", func() { NewSimulator(1, Config{DropRate: -0.1}) })
	mustPanic(t, "DupRate > 1", func() { NewSimulator(1, Config{DupRate: 2}) })
}

func pingPair(seed uint64, cfg Config) (*Simulator, *PingNode) {
	s := NewSimulator(seed, cfg)
	a := &PingNode{ID: 1, Peer: 2, Period: 5}
	s.AddNode(1, a)
	s.AddNode(2, &PingNode{ID: 2, Peer: 1})
	return s, a
}

func TestPingNodesUnderLoss(t *testing.T) {
	base := Config{MinDelay: 1, MaxDelay: 4, TickInterval: 1}
	s0, a0 := pingPair(11, base)
	s0.RunUntil(1000)

	lossy := base
	lossy.DropRate = 0.3
	s1, a1 := pingPair(11, lossy)
	s1.RunUntil(1000)

	if len(a1.Pongs) == 0 || len(a1.Pongs) >= len(a0.Pongs) {
		t.Errorf("pongs: %d with 30%% loss vs %d without; loss should reduce but not eliminate them",
			len(a1.Pongs), len(a0.Pongs))
	}
}

func TestPingNodesAcrossAPartitionAndHeal(t *testing.T) {
	s, a := pingPair(11, Config{MinDelay: 1, MaxDelay: 4, TickInterval: 1})

	s.RunUntil(100)
	before := len(a.Pongs)
	if before == 0 {
		t.Fatal("no pongs before the partition")
	}

	s.Network().Partition([]NodeID{1}, []NodeID{2})
	s.RunUntil(300)
	if during := len(a.Pongs); during != before {
		t.Errorf("pongs grew from %d to %d during the partition", before, during)
	}

	s.Network().Heal()
	s.RunUntil(500)
	if after := len(a.Pongs); after <= before {
		t.Errorf("pongs did not resume after healing: %d, was %d", after, before)
	}
}
