package sim

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

var testChaos = ChaosConfig{
	Duration:      600,
	CheckInterval: 10,
	CrashProb:     0.15,
	RestartProb:   0.5,
	PartitionProb: 0.1,
	HealProb:      0.3,
}

var testNet = Config{MinDelay: 1, MaxDelay: 5, TickInterval: 1, DropRate: 0.05, DupRate: 0.05}

// runDurability is one complete simulated run: a client writes numbers to a
// server while chaos crashes and partitions things. At the end the server
// loses power one last time, and we check the server's promise: every write
// it acknowledged is on disk.
func runDurability(seed uint64, mode SyncMode) (trace []string, err error) {
	s := NewSimulator(seed, testNet)
	s.EnableTrace()
	s.AddNodeWithDisk(1, NewLogServerFactory(mode))
	client := &Writer{ID: 2, Server: 1, Period: 3}
	s.AddNode(2, client)

	s.RunChaos(testChaos)

	s.Crash(1) // worst-case final power cut
	stored := StoredIDs(s.Disk(1).Bytes())
	for _, id := range client.Acked {
		if !stored[id] {
			return s.Trace(), fmt.Errorf("write %d was acknowledged but is missing from disk after a crash", id)
		}
	}
	return s.Trace(), nil
}

func TestTraceIsOffByDefault(t *testing.T) {
	s := NewSimulator(1, Config{MinDelay: 1, MaxDelay: 1})
	s.AddNode(1, &recorder{})
	s.Send(Message{To: 1})
	s.RunUntil(10)
	if len(s.Trace()) != 0 {
		t.Errorf("trace recorded without EnableTrace: %v", s.Trace())
	}
}

func TestTraceRecordsEachKindOfEvent(t *testing.T) {
	s := NewSimulator(1, Config{MinDelay: 5, MaxDelay: 5})
	s.EnableTrace()
	s.AddNode(2, &recorder{})
	s.AddNodeWithDisk(1, newDiskNode)

	s.Send(Message{From: 2, To: 1, Payload: 42}) // arrives at 5
	s.RunUntil(5)
	s.Partition([]NodeID{1}, []NodeID{2})
	s.Send(Message{From: 2, To: 1, Payload: 43}) // blocked by the partition
	s.Heal()
	s.Crash(1)
	s.Send(Message{From: 2, To: 1, Payload: 44}) // arrives at 10, node is down
	s.RunUntil(10)
	s.Restart(1)

	want := []string{
		"t=5 deliver 2->1 42",
		"t=5 PARTITION [[1] [2]]",
		"t=5 drop(partition) 2->1 43",
		"t=5 HEAL",
		"t=5 CRASH node 1",
		"t=10 drop(node down) 2->1 44",
		"t=10 RESTART node 1",
	}
	if !reflect.DeepEqual(s.Trace(), want) {
		t.Errorf("trace:\n%s\nwant:\n%s", strings.Join(s.Trace(), "\n"), strings.Join(want, "\n"))
	}
}

func TestChaosRunReplaysExactlyFromSeed(t *testing.T) {
	a, _ := runDurability(5, SyncBeforeAck)
	b, _ := runDurability(5, SyncBeforeAck)
	if len(a) == 0 {
		t.Fatal("trace is empty")
	}
	if !reflect.DeepEqual(a, b) {
		t.Error("seed 5 produced two different traces")
	}
	if c, _ := runDurability(6, SyncBeforeAck); reflect.DeepEqual(a, c) {
		t.Error("seeds 5 and 6 produced the same trace")
	}
}

func TestChaosInjectsEveryKindOfFault(t *testing.T) {
	var all []string
	for seed := uint64(1); seed <= 20; seed++ {
		trace, _ := runDurability(seed, SyncBeforeAck)
		all = append(all, trace...)
	}
	joined := strings.Join(all, "\n")
	for _, want := range []string{"CRASH", "RESTART", "PARTITION", "HEAL", "drop(partition)", "drop(node down)", "drop(lost)", "deliver"} {
		if !strings.Contains(joined, want) {
			t.Errorf("20 chaotic runs never produced %q", want)
		}
	}
}

func TestChaosOnlyCrashesNodesThatCanRestart(t *testing.T) {
	s := NewSimulator(1, Config{TickInterval: 1})
	s.EnableTrace()
	s.AddNode(2, &recorder{}) // no disk, no factory
	s.AddNodeWithDisk(1, newDiskNode)

	s.RunChaos(ChaosConfig{Duration: 100, CheckInterval: 10, CrashProb: 1, RestartProb: 0})

	if !s.IsUp(2) {
		t.Error("chaos crashed a node that can never restart")
	}
	if s.IsUp(1) {
		t.Error("with CrashProb 1 the restartable node should have been crashed")
	}
	if got := s.Now(); got != 100 {
		t.Errorf("clock = %d, want 100", got)
	}
}

func TestChaosConfigValidation(t *testing.T) {
	s := NewSimulator(1, Config{})
	mustPanic(t, "zero CheckInterval", func() { s.RunChaos(ChaosConfig{Duration: 10}) })
	mustPanic(t, "probability above 1", func() {
		s.RunChaos(ChaosConfig{Duration: 10, CheckInterval: 1, CrashProb: 2})
	})
}

func TestCorrectServerNeverLosesAcknowledgedWrites(t *testing.T) {
	seed, err := FirstFailure(1, 300, func(seed uint64) error {
		_, err := runDurability(seed, SyncBeforeAck)
		return err
	})
	if err != nil {
		t.Fatalf("the correct server violated its promise: %v (seed %d)", err, seed)
	}
}

// The payoff of the whole simulator: find a bug by searching seeds, replay
// it exactly from the seed alone, and confirm the fix on the same seed.
func TestSeedSearchFindsPlantedBugAndReplaysIt(t *testing.T) {
	buggy := func(seed uint64) error {
		_, err := runDurability(seed, SyncOnNextTick)
		return err
	}

	// 1. Search.
	seed, err := FirstFailure(1, 1000, buggy)
	if err == nil {
		t.Fatal("1000 seeds never exposed the planted bug")
	}
	t.Logf("found: %v", err)

	failing := 0
	for s := uint64(1); s <= 300; s++ {
		if buggy(s) != nil {
			failing++
		}
	}
	t.Logf("%d of the first 300 seeds expose the bug", failing)

	// 2. Replay: the seed alone reproduces the same failure and the same trace.
	trace1, err1 := runDurability(seed, SyncOnNextTick)
	trace2, err2 := runDurability(seed, SyncOnNextTick)
	if err1 == nil || err2 == nil {
		t.Fatalf("replay of seed %d did not fail again", seed)
	}
	if err1.Error() != err2.Error() {
		t.Errorf("replay changed the failure:\n%v\n%v", err1, err2)
	}
	if !reflect.DeepEqual(trace1, trace2) {
		t.Error("replay produced a different trace")
	}

	// 3. The trace shows the acknowledged write: the failing write was acked.
	var missing int
	fmt.Sscanf(err1.Error(), "write %d", &missing)
	ack := fmt.Sprintf("deliver 1->2 {ID:%d}", missing)
	found := false
	for _, line := range trace1 {
		if strings.Contains(line, ack) {
			found = true
		}
	}
	if !found {
		t.Errorf("trace has no ack for write %d, yet the client counted it as acknowledged", missing)
	}

	// 4. The fix passes on exactly the seed that failed.
	if _, err := runDurability(seed, SyncBeforeAck); err != nil {
		t.Errorf("the fix still fails on seed %d: %v", seed, err)
	}
}

func TestFirstFailureStopsAtFirstAndReportsNilWhenAllPass(t *testing.T) {
	seed, err := FirstFailure(10, 100, func(s uint64) error {
		if s >= 15 {
			return fmt.Errorf("boom")
		}
		return nil
	})
	if err == nil || seed != 15 {
		t.Errorf("got seed %d, err %v; want seed 15 with an error", seed, err)
	}
	if _, err := FirstFailure(1, 10, func(uint64) error { return nil }); err != nil {
		t.Errorf("all-pass search returned %v", err)
	}
}
