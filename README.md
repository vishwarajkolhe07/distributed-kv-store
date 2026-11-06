# distributed-kv-store

A distributed, transactional key-value database written in Go: replicated with
Raft, sharded across Raft groups, with Percolator-style cross-shard
transactions. It is tested by a deterministic simulator that injects message
delays and loss, crashes and clock skew (replayable from a seed), and checked
for linearizability with Porcupine.

**Status:** Phase 2 (simple in-memory KV store) in progress.

## Design rule

Each node is a state machine that only reacts to events (messages, timer
ticks). The simulator controls every event, so every run can be replayed.

## Roadmap

| # | Phase | Status |
|---|---|---|
| 1 | Learn the basics (Go + core concepts) | in progress |
| 2 | Simple KV store: `Put` / `Get` / `Delete` with tests | in progress |
| 3 | Persistence: data survives a restart | not started |
| 4 | Simulator: fake network, crashes, replayable seeds | not started |
| 5 | Raft: leader election, log replication, crash recovery | not started |
| 6 | Porcupine linearizability checks | not started |
| 7 | Sharding across Raft groups | not started |
| 8 | Percolator transactions | not started |
| 9 | Bug hunting and write-up (see [BUGS.md](BUGS.md)) | ongoing |

## Running

```bash
go run .
go test ./...
```
