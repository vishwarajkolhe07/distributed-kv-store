# kvstore

A distributed, transactional key-value database written in Go, built from scratch and tested with a deterministic simulator that deliberately breaks things to find bugs.

**Status:** Phases 1–4 done (Go basics, in-memory store, persistence to disk, deterministic simulator). Next: Phase 5, Raft.

## The idea

Data is stored as `key -> value`, copied across several machines so it survives crashes, and later split across groups of machines so it can grow. The interesting part is not the storage but proving it stays correct when messages are lost, machines crash and clocks drift.

The design rule everything follows: **each machine is a state machine that only reacts to events** (a message arrived, a timer ticked). It never reads the real clock, uses real randomness or touches the real network. That lets the simulator control all of it, so any run can be replayed exactly from a seed.

## What exists today

### The store (package `main`)

A key-value store with `Put`, `Get` and `Delete`, saved to disk as an append-only log.

- Every change is written to the log and fsynced before it is applied.
- On startup the log is replayed to rebuild the data.
- A half-written last line (a crash mid-write) is detected and cut off; corruption anywhere else is an error.
- `Compact()` rewrites the log to one line per live key, crash-safely (write a temp file, fsync, rename).

Files: `store.go` (the store), `command.go` (the log's line format).

### The simulator (package `sim`)

A single-threaded, seeded world in which nodes run.

| Piece | What it does |
|---|---|
| `Node` | The interface every machine implements: `HandleMessage(msg)` and `Tick()`, each returning the messages to send |
| `Clock`, `Rand` | A fake clock that only moves when told, and seeded randomness with named forks so adding a random call in one place does not change another |
| `Network` | Random delays, message loss, duplication, reordering and partitions |
| `Disk` | Simulated durable storage; on a crash, unsynced bytes are partly lost (torn writes) |
| Crashes | `Crash` and `Restart`: memory is lost, the node is rebuilt from its disk |
| Clock skew | Each node's local clock can run faster or slower |
| `RunChaos` | Randomly crashes, restarts, partitions and heals while a scenario runs |
| Trace | A line-by-line record of everything that happened; identical for the same seed |
| `FirstFailure` | Searches seeds for one that makes a scenario fail |

## Running it

```
go run .                  # interactive shell: put <k> <v>, get <k>, delete <k>, compact, quit
go run . -log other.log   # use a different log file
go test ./...             # all tests
go test -v ./sim          # simulator tests, one line per test
go vet ./...
```

The shell stores its data in `kv.log` in the current folder. Quit and rerun and the data is still there.

To see the simulator find a bug:

```
go test -v -run PlantedBug ./sim
```

## Replaying a failing seed

Every simulated run is fully determined by its seed. When a scenario fails, the seed is printed; running the same scenario with that seed reproduces the exact same failure and the exact same trace. See [BUGS.md](BUGS.md) for a worked example.

## Roadmap

| # | Phase | Status |
|---|---|---|
| 1 | Learn the basics (Go, core concepts) | Done |
| 2 | Simple KV store | Done |
| 3 | Saving to disk | Done |
| 4 | Deterministic simulator | Done |
| 5 | Raft: leader election, log replication, crash recovery | Next |
| 6 | Porcupine linearizability checking | Planned |
| 7 | Sharding across Raft groups | Planned |
| 8 | Percolator-style transactions across groups | Planned |
| 9 | Bug hunting and write-up | Ongoing, see [BUGS.md](BUGS.md) |

## Layout

```
store.go, command.go     the KV store and its log format
main.go                  interactive shell
sim/                     the deterministic simulator
  node.go                Node interface and the rules nodes must follow
  clock.go, rand.go      fake clock and seeded randomness
  network.go             delays, loss, duplication, partitions
  disk.go                simulated disk with torn writes
  simulator.go           the event loop, crashes, clock speed, tracing
  scenario.go            chaos runner and seed search
  toy.go, durable.go     small example nodes used to test the simulator
BUGS.md                  bug log
```

## Notes

- The simulator is single-threaded on purpose: that is what makes replay possible.
- `kv.log` and `*.log.tmp` are git-ignored.
