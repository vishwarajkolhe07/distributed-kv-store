# Bug log

Every bug found while building this project, with the seed that reproduces it. Seeds are only meaningful together with the code at the commit where the bug was found, because any change to the number or order of random draws changes what a seed does. Each entry therefore also records how to reproduce it.

## How to add an entry

Copy the template, fill it in the moment you find the bug (not later, when the details are fuzzy), and keep it even after the bug is fixed.

```
### BUG-N: short title

- **Found:** date, by what (simulator seed, unit test, code review, by hand)
- **Seed:** the seed, or "n/a"
- **Component:** which package or phase
- **Symptom:** what went wrong, as observed
- **Root cause:** why it happened
- **Fix:** what changed
- **Reproduce:** the exact command
- **Lesson:** what to remember or test for next time
```

## Real bugs found

None yet. Raft starts in Phase 5, which is where the real ones are expected.

## Worked example (planted, not a real bug)

This bug was planted on purpose in Phase 4 to prove the simulator can find a rare problem and replay it. It is kept here as the model for how real entries should read.

### EXAMPLE-1: acknowledged write lost after a crash

- **Found:** 2026-11-07, by searching seeds with `FirstFailure` in the simulator.
- **Seed:** 32 (the first failing seed; about 17 of the first 300 seeds fail).
- **Component:** `sim/durable.go`, `LogServer` in `SyncOnNextTick` mode.
- **Symptom:** `seed 32: write 46 was acknowledged but is missing from disk after a crash`. The client had received an `Ack` for write 46, but after the server crashed and restarted, write 46 was not on its disk.
- **Root cause:** the server appended the write to its disk and sent the ack immediately, but only synced the disk on its next `Tick`. A crash between the ack and the next tick lost bytes the server had already promised were safe. The window is narrow, which is why only about 6% of seeds hit it, and why a hand-written test would likely never have found it.
- **Fix:** sync before acknowledging (`SyncBeforeAck` mode). The same seed 32 then passes, and 300 chaotic seeds find no violation.
- **Reproduce:**
  ```
  go test -v -run PlantedBug ./sim
  ```
  The test searches for the failing seed, runs it twice and checks the failure and the full event trace are identical, then runs the fixed server on the same seed.
- **Lesson:** a reply is a promise. Anything the reply promises must already be durable (synced) when the reply is sent. The same rule applies to Raft: a node must persist its term, vote and log entries before answering a message that depends on them.
