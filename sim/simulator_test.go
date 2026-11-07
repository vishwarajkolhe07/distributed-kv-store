package sim

import (
	"fmt"
	"reflect"
	"testing"
)

// recorder is a Node that remembers what it was given.
type recorder struct {
	got   []Message
	ticks int
}

func (r *recorder) HandleMessage(m Message) []Message {
	r.got = append(r.got, m)
	return nil
}

func (r *recorder) Tick() []Message {
	r.ticks++
	return nil
}

// payloads lists the payloads a recorder received, in arrival order.
func (r *recorder) payloads() []int {
	var out []int
	for _, m := range r.got {
		out = append(out, m.Payload.(int))
	}
	return out
}

// deliveryTrace runs the simulator to completion and returns one line per
// delivery: "time from->to payload".
func deliveryTrace(s *Simulator) []string {
	var trace []string
	s.OnDeliver = func(at Time, m Message) {
		trace = append(trace, fmt.Sprintf("%d %d->%d %v", at, m.From, m.To, m.Payload))
	}
	for s.Step() {
	}
	return trace
}

func sendBurst(s *Simulator, n int) {
	for i := 0; i < n; i++ {
		s.Send(Message{From: 0, To: 1, Payload: i})
	}
}

func TestFixedDelayDeliversInSendOrder(t *testing.T) {
	s := NewSimulator(1, Config{MinDelay: 5, MaxDelay: 5})
	r := &recorder{}
	s.AddNode(1, r)

	sendBurst(s, 4)
	for s.Step() {
	}

	// Same arrival time for all four, so the tie is broken by send order.
	if want := []int{0, 1, 2, 3}; !reflect.DeepEqual(r.payloads(), want) {
		t.Errorf("arrival order = %v, want %v", r.payloads(), want)
	}
	if s.Now() != 5 {
		t.Errorf("clock = %d, want 5", s.Now())
	}
}

func TestDeliveriesAreInTimeOrderAndWithinDelayRange(t *testing.T) {
	s := NewSimulator(7, Config{MinDelay: 1, MaxDelay: 20})
	s.AddNode(1, &recorder{})

	var times []Time
	s.OnDeliver = func(at Time, _ Message) { times = append(times, at) }
	sendBurst(s, 100)
	for s.Step() {
	}

	if len(times) != 100 {
		t.Fatalf("delivered %d messages, want 100", len(times))
	}
	for i, at := range times {
		if at < 1 || at > 20 {
			t.Errorf("delivery %d at time %d, outside [1,20]", i, at)
		}
		if i > 0 && at < times[i-1] {
			t.Errorf("delivery %d at %d came after delivery at %d", i, at, times[i-1])
		}
	}
	if s.Now() != times[len(times)-1] {
		t.Errorf("clock = %d, want time of last delivery %d", s.Now(), times[len(times)-1])
	}
}

func TestRandomDelaysCanReorderMessages(t *testing.T) {
	s := NewSimulator(7, Config{MinDelay: 1, MaxDelay: 20})
	r := &recorder{}
	s.AddNode(1, r)
	sendBurst(s, 100)
	for s.Step() {
	}

	inOrder := true
	for i, p := range r.payloads() {
		if p != i {
			inOrder = false
		}
	}
	if inOrder {
		t.Error("100 messages with delays 1..20 all arrived in send order; delays look constant")
	}
	// Nothing lost or duplicated, only shuffled.
	if len(r.got) != 100 {
		t.Errorf("got %d messages, want 100", len(r.got))
	}
}

func TestSameSeedSameDeliveryOrder(t *testing.T) {
	run := func(seed uint64) []string {
		s := NewSimulator(seed, Config{MinDelay: 1, MaxDelay: 50})
		s.AddNode(1, &recorder{})
		sendBurst(s, 200)
		return deliveryTrace(s)
	}

	a, b := run(42), run(42)
	if !reflect.DeepEqual(a, b) {
		t.Error("seed 42 produced two different delivery traces")
	}
	if c := run(43); reflect.DeepEqual(a, c) {
		t.Error("seeds 42 and 43 produced the same delivery trace")
	}
}

func TestStepOnEmptyQueueReturnsFalse(t *testing.T) {
	s := NewSimulator(1, Config{})
	if s.Step() {
		t.Error("Step on an empty simulator should return false")
	}
	if s.Now() != 0 {
		t.Errorf("clock moved to %d with nothing to do", s.Now())
	}
}

func TestMessageToUnknownNodeIsLost(t *testing.T) {
	s := NewSimulator(1, Config{MinDelay: 1, MaxDelay: 1})
	s.AddNode(1, &recorder{})
	s.Send(Message{From: 1, To: 99, Payload: 0})

	delivered := 0
	s.OnDeliver = func(Time, Message) { delivered++ }
	for s.Step() {
	}
	if delivered != 0 {
		t.Errorf("a message to a missing node was delivered %d times", delivered)
	}
}

func TestTicksFireOnSchedule(t *testing.T) {
	s := NewSimulator(1, Config{TickInterval: 10})
	r := &recorder{}
	s.AddNode(1, r)

	s.RunUntil(35) // ticks at 10, 20, 30
	if r.ticks != 3 {
		t.Errorf("ticks by time 35 = %d, want 3", r.ticks)
	}
	s.RunUntil(40) // the tick at exactly 40 counts
	if r.ticks != 4 {
		t.Errorf("ticks by time 40 = %d, want 4", r.ticks)
	}
}

func TestNoTicksWhenIntervalIsZero(t *testing.T) {
	s := NewSimulator(1, Config{})
	r := &recorder{}
	s.AddNode(1, r)
	s.RunUntil(1000)
	if r.ticks != 0 {
		t.Errorf("got %d ticks with TickInterval 0", r.ticks)
	}
}

func TestRunUntilAdvancesClockEvenWithNoEvents(t *testing.T) {
	s := NewSimulator(1, Config{})
	s.RunUntil(77)
	if s.Now() != 77 {
		t.Errorf("clock = %d, want 77", s.Now())
	}
}

func TestRunUntilLeavesLaterEventsQueued(t *testing.T) {
	s := NewSimulator(1, Config{MinDelay: 10, MaxDelay: 10})
	r := &recorder{}
	s.AddNode(1, r)
	s.Send(Message{To: 1, Payload: 0}) // arrives at 10

	s.RunUntil(9)
	if len(r.got) != 0 || s.Pending() != 1 {
		t.Fatalf("at time 9: delivered=%d pending=%d, want 0 and 1", len(r.got), s.Pending())
	}
	s.RunUntil(10)
	if len(r.got) != 1 {
		t.Errorf("at time 10 the message should have arrived, got %d", len(r.got))
	}
}

func TestDuplicateNodeIDPanics(t *testing.T) {
	s := NewSimulator(1, Config{})
	s.AddNode(1, &recorder{})
	mustPanic(t, "AddNode with a used id", func() { s.AddNode(1, &recorder{}) })
}

func TestInvalidConfigPanics(t *testing.T) {
	mustPanic(t, "MaxDelay < MinDelay", func() { NewSimulator(1, Config{MinDelay: 5, MaxDelay: 1}) })
	mustPanic(t, "negative MinDelay", func() { NewSimulator(1, Config{MinDelay: -1, MaxDelay: 1}) })
	mustPanic(t, "negative TickInterval", func() { NewSimulator(1, Config{TickInterval: -1}) })
}

// The first real use of the whole stack: PingNodes talking over the
// simulated network, driven by simulated ticks.
func TestPingNodesOverSimulatedNetwork(t *testing.T) {
	run := func(seed uint64) (*PingNode, []string) {
		s := NewSimulator(seed, Config{MinDelay: 1, MaxDelay: 8, TickInterval: 1})
		a := &PingNode{ID: 1, Peer: 2, Period: 5}
		b := &PingNode{ID: 2, Peer: 1} // only answers
		s.AddNode(1, a)
		s.AddNode(2, b)

		var trace []string
		s.OnDeliver = func(at Time, m Message) {
			trace = append(trace, fmt.Sprintf("%d %d->%d %+v", at, m.From, m.To, m.Payload))
		}
		s.RunUntil(100)
		return a, trace
	}

	a, trace1 := run(5)
	// Pings go out at ticks 5, 10, ..., 100 (20 of them); the last few may
	// still be in flight at time 100, but most pongs must be back.
	if len(a.Pongs) < 15 {
		t.Errorf("only %d pongs came back, want at least 15", len(a.Pongs))
	}

	_, trace2 := run(5)
	if !reflect.DeepEqual(trace1, trace2) {
		t.Error("the same seed produced two different runs")
	}
}
