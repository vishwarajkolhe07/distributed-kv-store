package sim

import "testing"

func TestRouteGivesOneDelayInRange(t *testing.T) {
	n := NewNetwork(3, 9, NewRand(1))
	seen := map[Time]bool{}
	for i := 0; i < 1000; i++ {
		delays := n.Route(Message{})
		if len(delays) != 1 {
			t.Fatalf("Route returned %d copies, want 1", len(delays))
		}
		d := delays[0]
		if d < 3 || d > 9 {
			t.Fatalf("delay %d outside [3,9]", d)
		}
		seen[d] = true
	}
	if len(seen) < 7 {
		t.Errorf("only %d distinct delays in 1000 draws, want all 7 of 3..9", len(seen))
	}
}

func TestRouteWithEqualMinMaxIsConstant(t *testing.T) {
	n := NewNetwork(4, 4, NewRand(1))
	for i := 0; i < 20; i++ {
		if d := n.Route(Message{}); d[0] != 4 {
			t.Fatalf("delay = %d, want 4", d[0])
		}
	}
}

func TestNewNetworkRejectsBadRange(t *testing.T) {
	mustPanic(t, "max < min", func() { NewNetwork(5, 4, NewRand(1)) })
	mustPanic(t, "negative min", func() { NewNetwork(-1, 4, NewRand(1)) })
}
