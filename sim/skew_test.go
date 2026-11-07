package sim

import "testing"

// ticksBy runs a one-node simulator with the given clock speed and returns
// how many ticks the node got by time t.
func ticksBy(speed int, interval Time, t Time) int {
	s := NewSimulator(1, Config{TickInterval: interval})
	r := &recorder{}
	s.AddNode(1, r)
	s.SetClockSpeed(1, speed)
	s.RunUntil(t)
	return r.ticks
}

func TestClockSpeedScalesTickRate(t *testing.T) {
	cases := []struct {
		speed int
		want  int
	}{
		{100, 10}, // normal: every 10, so 10 ticks by 100
		{200, 20}, // fast clock: every 5
		{50, 5},   // slow clock: every 20
		{300, 30}, // every 3.33: the fractional interval must not drift
	}
	for _, c := range cases {
		if got := ticksBy(c.speed, 10, 100); got != c.want {
			t.Errorf("speed %d%%: %d ticks by time 100, want %d", c.speed, got, c.want)
		}
	}
}

func TestNodesWithDifferentSpeedsTickAtDifferentRates(t *testing.T) {
	s := NewSimulator(1, Config{TickInterval: 10})
	fast, normal, slow := &recorder{}, &recorder{}, &recorder{}
	s.AddNode(1, fast)
	s.AddNode(2, normal)
	s.AddNode(3, slow)
	s.SetClockSpeed(1, 150)
	s.SetClockSpeed(3, 50)

	s.RunUntil(600)
	if !(fast.ticks > normal.ticks && normal.ticks > slow.ticks) {
		t.Errorf("ticks fast=%d normal=%d slow=%d; want strictly decreasing", fast.ticks, normal.ticks, slow.ticks)
	}
	if fast.ticks != 90 || normal.ticks != 60 || slow.ticks != 30 {
		t.Errorf("ticks fast=%d normal=%d slow=%d, want 90, 60, 30", fast.ticks, normal.ticks, slow.ticks)
	}
}

func TestChangingSpeedMidRunTakesEffectFromNow(t *testing.T) {
	s := NewSimulator(1, Config{TickInterval: 10})
	r := &recorder{}
	s.AddNode(1, r)

	s.RunUntil(50) // 5 ticks at normal speed
	if r.ticks != 5 {
		t.Fatalf("ticks by 50 = %d, want 5", r.ticks)
	}
	s.SetClockSpeed(1, 200) // now every 5; the old tick due at 60 must be cancelled
	s.RunUntil(100)

	if r.ticks != 15 { // 5 before, then ticks at 55, 60, ..., 100
		t.Errorf("ticks by 100 = %d, want 15", r.ticks)
	}
}

func TestSpeedSurvivesCrashAndRestart(t *testing.T) {
	s := NewSimulator(1, Config{TickInterval: 10})
	s.AddNodeWithDisk(1, func(NodeID, *Disk) Node { return &recorder{} })
	s.SetClockSpeed(1, 200)
	s.RunUntil(20)
	s.Crash(1)
	s.Restart(1)
	s.RunUntil(120) // new instance ticks every 5 from time 20

	if got := s.Node(1).(*recorder).ticks; got != 20 {
		t.Errorf("ticks after restart = %d, want 20 (clock should still be fast)", got)
	}
}

func TestSettingSpeedOnCrashedNodeAppliesOnRestart(t *testing.T) {
	s := NewSimulator(1, Config{TickInterval: 10})
	s.AddNodeWithDisk(1, func(NodeID, *Disk) Node { return &recorder{} })
	s.Crash(1)
	s.SetClockSpeed(1, 50)
	s.Restart(1)
	s.RunUntil(100)
	if got := s.Node(1).(*recorder).ticks; got != 5 {
		t.Errorf("ticks = %d, want 5", got)
	}
}

func TestInvalidClockSpeedPanics(t *testing.T) {
	s := NewSimulator(1, Config{TickInterval: 10})
	s.AddNode(1, &recorder{})
	mustPanic(t, "speed 0", func() { s.SetClockSpeed(1, 0) })
	mustPanic(t, "speed -5", func() { s.SetClockSpeed(1, -5) })
	mustPanic(t, "speed 10001", func() { s.SetClockSpeed(1, 10001) })
	mustPanic(t, "unknown node", func() { s.SetClockSpeed(9, 100) })
}
