package sim

import "testing"

func TestClockStartsAtZeroAndOnlyMovesWhenTold(t *testing.T) {
	c := NewClock()
	if c.Now() != 0 {
		t.Errorf("new clock = %d, want 0", c.Now())
	}
	// Reading the clock repeatedly never changes it.
	for i := 0; i < 1000; i++ {
		c.Now()
	}
	if c.Now() != 0 {
		t.Errorf("clock moved on its own to %d", c.Now())
	}
}

func TestClockAdvance(t *testing.T) {
	c := NewClock()
	c.Advance(5)
	if c.Now() != 5 {
		t.Errorf("after Advance(5), Now = %d, want 5", c.Now())
	}
	c.AdvanceTo(20)
	if c.Now() != 20 {
		t.Errorf("after AdvanceTo(20), Now = %d, want 20", c.Now())
	}
	c.AdvanceTo(20) // same time is allowed: several events can share a moment
	c.Advance(0)
	if c.Now() != 20 {
		t.Errorf("staying put changed the clock to %d", c.Now())
	}
}

func mustPanic(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s should panic", name)
		}
	}()
	f()
}

func TestClockNeverGoesBackwards(t *testing.T) {
	c := NewClock()
	c.AdvanceTo(10)
	mustPanic(t, "AdvanceTo(9)", func() { c.AdvanceTo(9) })
	mustPanic(t, "Advance(-1)", func() { c.Advance(-1) })
	if c.Now() != 10 {
		t.Errorf("a rejected move changed the clock to %d", c.Now())
	}
}
