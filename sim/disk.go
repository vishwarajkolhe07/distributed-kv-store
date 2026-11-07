package sim

// Disk is a node's simulated durable storage. It models the same thing the
// Phase 3 log had to survive: a crash can happen after bytes were written
// but before they were synced to the physical disk.
//
//   - Append adds bytes. They are visible to the running node immediately
//     but are not durable yet.
//   - Sync makes everything appended so far durable.
//   - When the node crashes, bytes that were never synced are partly lost:
//     a random-length prefix of them survives, like a torn write. Anything
//     synced is always kept.
//
// A node that wants a write to survive must call Sync before it tells anyone
// the write happened (before replying to a client, for example).
type Disk struct {
	durable []byte
	pending []byte
}

// Append adds p to the end of the disk's contents, not yet durable.
func (d *Disk) Append(p []byte) {
	d.pending = append(d.pending, p...)
}

// Sync makes all appended bytes durable.
func (d *Disk) Sync() {
	d.durable = append(d.durable, d.pending...)
	d.pending = nil
}

// Bytes returns a copy of the disk's current contents, durable and pending.
// After a crash it returns what survived.
func (d *Disk) Bytes() []byte {
	out := make([]byte, 0, len(d.durable)+len(d.pending))
	out = append(out, d.durable...)
	return append(out, d.pending...)
}

// crash applies a power cut: unsynced bytes are kept only up to a random
// point. It always draws exactly one random number.
func (d *Disk) crash(rng *Rand) {
	keep := rng.Intn(len(d.pending) + 1)
	d.durable = append(d.durable, d.pending[:keep]...)
	d.pending = nil
}
