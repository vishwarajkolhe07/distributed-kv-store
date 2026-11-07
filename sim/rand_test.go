package sim

import (
	"reflect"
	"testing"
)

func draw(r *Rand, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = r.Intn(1_000_000)
	}
	return out
}

func TestSameSeedSameSequence(t *testing.T) {
	a := draw(NewRand(42), 1000)
	b := draw(NewRand(42), 1000)
	if !reflect.DeepEqual(a, b) {
		t.Error("two generators with seed 42 produced different sequences")
	}
}

func TestDifferentSeedsDiffer(t *testing.T) {
	a := draw(NewRand(1), 100)
	b := draw(NewRand(2), 100)
	if reflect.DeepEqual(a, b) {
		t.Error("seeds 1 and 2 produced the same sequence")
	}
}

func TestSeedIsReported(t *testing.T) {
	if got := NewRand(12345).Seed(); got != 12345 {
		t.Errorf("Seed() = %d, want 12345", got)
	}
}

// Pins the exact output for one seed. Seeds saved in BUGS.md only keep
// reproducing bugs if the generator never changes, so this test fails loudly
// if anyone swaps the algorithm or the way a seed is turned into state.
func TestGoldenSequence(t *testing.T) {
	got := draw(NewRand(42), 5)
	want := []int{335835, 877900, 86591, 76147, 701353}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("seed 42 sequence = %v, want %v", got, want)
	}
}

func TestIntnStaysInRange(t *testing.T) {
	r := NewRand(7)
	for i := 0; i < 10000; i++ {
		if v := r.Intn(10); v < 0 || v >= 10 {
			t.Fatalf("Intn(10) = %d, out of [0,10)", v)
		}
	}
}

func TestRangeIncludesBothEnds(t *testing.T) {
	r := NewRand(7)
	seen := map[int]bool{}
	for i := 0; i < 1000; i++ {
		v := r.Range(3, 6)
		if v < 3 || v > 6 {
			t.Fatalf("Range(3,6) = %d, out of [3,6]", v)
		}
		seen[v] = true
	}
	for v := 3; v <= 6; v++ {
		if !seen[v] {
			t.Errorf("Range(3,6) never produced %d in 1000 draws", v)
		}
	}
	if v := r.Range(5, 5); v != 5 {
		t.Errorf("Range(5,5) = %d, want 5", v)
	}
	mustPanic(t, "Range(6,3)", func() { r.Range(6, 3) })
}

func TestChanceExtremes(t *testing.T) {
	r := NewRand(7)
	for i := 0; i < 1000; i++ {
		if r.Chance(0) {
			t.Fatal("Chance(0) returned true")
		}
		if !r.Chance(1) {
			t.Fatal("Chance(1) returned false")
		}
	}
}

func TestChanceRoughlyMatchesProbability(t *testing.T) {
	r := NewRand(7) // fixed seed, so this can never be flaky
	hits := 0
	for i := 0; i < 10000; i++ {
		if r.Chance(0.3) {
			hits++
		}
	}
	if hits < 2700 || hits > 3300 {
		t.Errorf("Chance(0.3) hit %d of 10000 times, want about 3000", hits)
	}
}

func TestChanceAlwaysUsesOneDraw(t *testing.T) {
	// After Chance(0) or Chance(1), the next draw must be the same, so
	// changing a probability never shifts the rest of the run.
	a, b := NewRand(9), NewRand(9)
	a.Chance(0)
	b.Chance(1)
	if x, y := a.Intn(1000000), b.Intn(1000000); x != y {
		t.Errorf("streams diverged after Chance(0) vs Chance(1): %d vs %d", x, y)
	}
}

func TestForkIsDeterministic(t *testing.T) {
	a := draw(NewRand(42).Fork("network"), 100)
	b := draw(NewRand(42).Fork("network"), 100)
	if !reflect.DeepEqual(a, b) {
		t.Error("same seed and name gave different forks")
	}
}

func TestForksWithDifferentNamesDiffer(t *testing.T) {
	root := NewRand(42)
	a := draw(root.Fork("network"), 100)
	b := draw(root.Fork("crashes"), 100)
	if reflect.DeepEqual(a, b) {
		t.Error("forks named network and crashes produced the same sequence")
	}
}

func TestForkDoesNotDisturbParent(t *testing.T) {
	plain := draw(NewRand(42), 50)

	root := NewRand(42)
	root.Fork("network")
	root.Fork("crashes")
	if got := draw(root, 50); !reflect.DeepEqual(got, plain) {
		t.Error("forking changed the parent's sequence")
	}
}

func TestForkOrderDoesNotMatter(t *testing.T) {
	r1 := NewRand(42)
	r1.Fork("crashes")
	net1 := draw(r1.Fork("network"), 50)

	r2 := NewRand(42)
	net2 := draw(r2.Fork("network"), 50) // forked first this time
	r2.Fork("crashes")

	if !reflect.DeepEqual(net1, net2) {
		t.Error("the network fork depends on what was forked before it")
	}
}
