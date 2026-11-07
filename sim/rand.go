package sim

import (
	"hash/fnv"
	"math/rand/v2"
)

// Rand is the simulator's source of randomness. Every random choice (message
// delay, whether a message is dropped, when a node crashes) must come from a
// Rand, never from math/rand's global functions or the real clock. Then a run
// is fully determined by its seed, and a failing seed replays the exact same
// failure.
//
// A Rand is not safe for concurrent use. That is deliberate: the simulator
// runs on one thread.
type Rand struct {
	seed uint64
	r    *rand.Rand
}

// NewRand returns a generator whose output depends only on seed.
// The PCG algorithm is documented as stable, so a seed recorded in BUGS.md
// keeps reproducing the same run after Go upgrades.
func NewRand(seed uint64) *Rand {
	return &Rand{
		seed: seed,
		r:    rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15)),
	}
}

// Seed returns the seed this generator was created with, for printing in
// failure messages.
func (r *Rand) Seed() uint64 {
	return r.seed
}

// Intn returns a number in [0, n). It panics if n <= 0.
func (r *Rand) Intn(n int) int {
	return r.r.IntN(n)
}

// Range returns a number in [lo, hi], both ends included. It panics if hi < lo.
func (r *Rand) Range(lo, hi int) int {
	if hi < lo {
		panic("sim: Rand.Range with hi < lo")
	}
	return lo + r.r.IntN(hi-lo+1)
}

// Chance returns true with probability p (0 means never, 1 means always).
//
// It always draws exactly one random number, even when p is 0 or 1. That way
// changing a probability from, say, 0.1 to 0 does not shift every later
// random choice in the run.
func (r *Rand) Chance(p float64) bool {
	x := r.r.Float64()
	return x < p
}

// Fork returns an independent generator derived from this one's seed and a
// name. It does not use up any of the parent's random numbers, and the result
// does not depend on how many times or in what order you fork.
//
// Give each subsystem its own fork (for example "network" and "crashes").
// Then adding one extra random call in the network code does not change when
// the crashes happen, which keeps old seeds reproducing old runs.
func (r *Rand) Fork(name string) *Rand {
	h := fnv.New64a()
	var b [8]byte
	for i := 0; i < 8; i++ {
		b[i] = byte(r.seed >> (8 * i))
	}
	h.Write(b[:])
	h.Write([]byte(name))
	return NewRand(h.Sum64())
}
