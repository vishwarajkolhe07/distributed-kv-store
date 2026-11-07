package sim

import "fmt"

// Time is a point in simulated time, in abstract units. It has nothing to do
// with the real clock: a simulated hour can pass in a microsecond of real
// time, and a run takes the same simulated time on every machine.
type Time int64

// Clock is the simulator's global clock. It only moves when the simulator
// moves it, and never backwards. Nodes never see it; they only receive Tick
// calls (see Node).
type Clock struct {
	now Time
}

// NewClock returns a clock set to time 0.
func NewClock() *Clock {
	return &Clock{}
}

// Now returns the current simulated time.
func (c *Clock) Now() Time {
	return c.now
}

// AdvanceTo jumps the clock forward to t. Staying at the same time is fine.
// Going backwards panics: it would mean the simulator delivered events out of
// order, which is a bug in the simulator itself, not something to recover from.
func (c *Clock) AdvanceTo(t Time) {
	if t < c.now {
		panic(fmt.Sprintf("sim: clock moved backwards from %d to %d", c.now, t))
	}
	c.now = t
}

// Advance moves the clock forward by d. A negative d panics, like AdvanceTo.
func (c *Clock) Advance(d Time) {
	c.AdvanceTo(c.now + d)
}
