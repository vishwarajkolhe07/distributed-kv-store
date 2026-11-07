package sim

import (
	"fmt"
	"sort"
)

// ChaosConfig describes random trouble to inject while a simulation runs.
// Every CheckInterval of simulated time the runner rolls the dice for each
// fault below.
type ChaosConfig struct {
	// Duration is how long to run, in simulated time, counting from time 0.
	Duration Time
	// CheckInterval is how often to roll the dice. Must be positive.
	CheckInterval Time

	// CrashProb is the chance, per check, that each running node crashes.
	// Only nodes added with AddNodeWithDisk are crashed, since only they can
	// come back.
	CrashProb float64
	// RestartProb is the chance, per check, that each crashed node restarts.
	RestartProb float64
	// PartitionProb is the chance, per check, that a healthy network is split
	// into two random sides.
	PartitionProb float64
	// HealProb is the chance, per check, that an existing partition heals.
	HealProb float64
}

func (c ChaosConfig) validate() {
	if c.CheckInterval <= 0 {
		panic("sim: ChaosConfig.CheckInterval must be positive")
	}
	checkRate("CrashProb", c.CrashProb)
	checkRate("RestartProb", c.RestartProb)
	checkRate("PartitionProb", c.PartitionProb)
	checkRate("HealProb", c.HealProb)
}

// RunChaos runs the simulation up to cfg.Duration, randomly crashing and
// restarting nodes and splitting and healing the network along the way. It
// stops with the cluster in whatever state the dice left it.
//
// All its randomness comes from a fork of the simulator's seed, and it draws
// the same number of random numbers at every check no matter what happens.
// So the whole run, faults included, is fixed by the seed, and changing one
// probability does not reshuffle the other faults.
func (s *Simulator) RunChaos(cfg ChaosConfig) {
	cfg.validate()
	rng := s.rng.Fork("chaos")

	ids := make([]NodeID, 0, len(s.peers))
	for id := range s.peers {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	for t := s.Now() + cfg.CheckInterval; t <= cfg.Duration; t += cfg.CheckInterval {
		s.RunUntil(t)

		for _, id := range ids {
			p := s.peers[id]
			if p.up {
				if rng.Chance(cfg.CrashProb) && p.factory != nil {
					s.Crash(id)
				}
			} else if rng.Chance(cfg.RestartProb) && p.factory != nil {
				s.Restart(id)
			}
		}

		startSplit := rng.Chance(cfg.PartitionProb)
		heal := rng.Chance(cfg.HealProb)
		var sideA, sideB []NodeID
		for _, id := range ids {
			if rng.Chance(0.5) {
				sideA = append(sideA, id)
			} else {
				sideB = append(sideB, id)
			}
		}
		if s.net.Partitioned() {
			if heal {
				s.Heal()
			}
		} else if startSplit && len(sideA) > 0 && len(sideB) > 0 {
			s.Partition(sideA, sideB)
		}
	}
	s.RunUntil(cfg.Duration)
}

// FirstFailure runs run(seed) for seeds start, start+1, ... start+n-1 and
// returns the first seed whose run returned an error, with that error. If
// every run passed, the error is nil.
//
// The run function should build a fresh Simulator from the seed it is given,
// so that the failing seed can be handed to run again to replay the failure.
func FirstFailure(start uint64, n int, run func(seed uint64) error) (uint64, error) {
	for i := 0; i < n; i++ {
		seed := start + uint64(i)
		if err := run(seed); err != nil {
			return seed, fmt.Errorf("seed %d: %w", seed, err)
		}
	}
	return 0, nil
}
