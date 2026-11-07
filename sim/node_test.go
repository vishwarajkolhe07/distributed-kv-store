package sim

import (
	"reflect"
	"testing"
)

func TestAnswersPingWithPong(t *testing.T) {
	n := &PingNode{ID: 2}

	got := n.HandleMessage(Message{From: 1, To: 2, Payload: Ping{Seq: 7}})

	want := []Message{{From: 2, To: 1, Payload: Pong{Seq: 7}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestTickSendsPingEveryPeriod(t *testing.T) {
	n := &PingNode{ID: 1, Peer: 2, Period: 3}

	// Ticks 1 and 2 send nothing, tick 3 sends Ping 0, and so on.
	wantSeq := map[int]int{3: 0, 6: 1, 9: 2}
	for tick := 1; tick <= 9; tick++ {
		got := n.Tick()
		seq, shouldSend := wantSeq[tick]
		if !shouldSend {
			if len(got) != 0 {
				t.Errorf("tick %d: sent %+v, want nothing", tick, got)
			}
			continue
		}
		want := []Message{{From: 1, To: 2, Payload: Ping{Seq: seq}}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("tick %d: got %+v, want %+v", tick, got, want)
		}
	}
}

func TestPeriodZeroNeverPings(t *testing.T) {
	n := &PingNode{ID: 1, Peer: 2, Period: 0}
	for i := 0; i < 100; i++ {
		if got := n.Tick(); len(got) != 0 {
			t.Fatalf("tick %d: sent %+v, want nothing", i+1, got)
		}
	}
}

func TestPongsAreRecordedInOrder(t *testing.T) {
	n := &PingNode{ID: 1, Peer: 2}

	for _, seq := range []int{0, 2, 1} { // arrival order, not send order
		if out := n.HandleMessage(Message{From: 2, To: 1, Payload: Pong{Seq: seq}}); len(out) != 0 {
			t.Errorf("a Pong should not trigger any reply, got %+v", out)
		}
	}
	if want := []int{0, 2, 1}; !reflect.DeepEqual(n.Pongs, want) {
		t.Errorf("Pongs = %v, want %v", n.Pongs, want)
	}
}

func TestUnknownPayloadIsIgnored(t *testing.T) {
	n := &PingNode{ID: 1}
	if out := n.HandleMessage(Message{From: 2, To: 1, Payload: "surprise"}); len(out) != 0 {
		t.Errorf("got %+v, want nothing", out)
	}
}

// The property the whole simulator depends on: feed two fresh nodes the same
// events and they must produce the same outputs and end in the same state.
func TestSameEventsGiveSameResult(t *testing.T) {
	run := func() ([][]Message, *PingNode) {
		n := &PingNode{ID: 1, Peer: 2, Period: 2}
		var outputs [][]Message
		for i := 0; i < 6; i++ {
			outputs = append(outputs, n.Tick())
			if i%2 == 1 {
				outputs = append(outputs, n.HandleMessage(Message{From: 2, To: 1, Payload: Pong{Seq: i}}))
			}
		}
		return outputs, n
	}

	out1, n1 := run()
	out2, n2 := run()
	if !reflect.DeepEqual(out1, out2) {
		t.Errorf("outputs differ between identical runs:\n%+v\n%+v", out1, out2)
	}
	if !reflect.DeepEqual(n1, n2) {
		t.Errorf("final state differs between identical runs:\n%+v\n%+v", n1, n2)
	}
}

// Two nodes wired together by hand, standing in for the simulator we build
// in the next chunks: deliver every message instantly, in order.
func TestTwoNodesExchangePingsByHand(t *testing.T) {
	a := &PingNode{ID: 1, Peer: 2, Period: 2}
	b := &PingNode{ID: 2, Peer: 1} // only answers
	nodes := map[NodeID]Node{1: a, 2: b}

	for tick := 0; tick < 6; tick++ {
		queue := a.Tick()
		queue = append(queue, b.Tick()...)
		for len(queue) > 0 {
			msg := queue[0]
			queue = queue[1:]
			queue = append(queue, nodes[msg.To].HandleMessage(msg)...)
		}
	}

	// 6 ticks with Period 2 means pings 0, 1, 2, each answered.
	if want := []int{0, 1, 2}; !reflect.DeepEqual(a.Pongs, want) {
		t.Errorf("a.Pongs = %v, want %v", a.Pongs, want)
	}
}
