# ADR-0006: The graph lives in memory as CSR, replicated to every query replica

- **Status:** Accepted
- **Date:** 2026-10-27
- **Supersedes:** —
- **Superseded-by:** —
- **Issue/PR:** —

## Context

A genealogy at the scale of a large manufacturer runs to a hundred million
nodes and more, with a heavy-tailed degree: most nodes have a few edges, a
lot of a solvent or a packaging material has millions. A graph kept only as
rows in a relational database has to be rebuilt, hop by hop, for every
traversal — recursive queries explode on wide fan-out and their cost is
unpredictable. That is the known way relational genealogies fail at this
scale.

Data arrives in real time, so the graph cannot be rebuilt from scratch now
and then; and it is read by many consumers at once, so reads cannot wait on
writes.

## Decision

**The graph's skeleton is held in memory in compressed sparse row form (CSR),
in both directions, in every `query` replica.**

- **Skeleton only.** Dense `uint32` node ids, assigned in order and never
  reused; per-node kind and time as flat arrays; edge arrays and offsets for
  both directions. No pointers, so the garbage collector does not scan it,
  and no external keys — resolving a document key to a node id is an index
  lookup in PostgreSQL. Document data stays in PostgreSQL.
- **Real time on an immutable structure.** New edges go to a small delta,
  invalidated edges to a tombstone set; a read is base plus delta minus
  tombstones. When the delta passes a threshold, a new CSR is built in the
  background in linear time and swapped in atomically.
- **One writer, many readers.** Genealogy events are applied by one writer in
  sequence order; a reader takes the current version and traverses it without
  locks.
- **Snapshots between replicas.** `query` replicas run as a stateful set and
  keep the CSR as a snapshot file on their own volume, memory-mapped. A new
  replica copies a snapshot from a running one over the internal network and
  replays the events after its sequence. Rebuilding from PostgreSQL is the
  disaster path, when no replica is left.
- **Scaling reads is adding replicas.** Each holds the whole graph and follows
  the sequence. A client that has just written can ask for at least a given
  sequence and is answered once the replica has reached it. Heavy analytical
  traversals run on their own pool of replicas.
- **Every traversal has a budget** — nodes visited and time — and materials
  marked as not traced further stop it.

Rejected: **a graph database** beside PostgreSQL — another stateful system to
run in every installation, kept consistent with the events by a second
pipeline; **recursive SQL over adjacency tables** as the traversal path — the
approach that fails at this scale.

## Consequences

- `+` A traversal is a walk over contiguous memory — millions of nodes a
  second, with no database round trip in it.
- `+` A replica starts in the time it takes to copy and map a snapshot, not to
  rebuild the graph.
- `+` Memory outside the Go heap: a graph larger than RAM pages instead of
  killing the pod.
- `-` Every `query` replica holds the whole skeleton; its memory is sized from
  the installation's graph (`8·E + ~17·N` bytes for E edges and N nodes).
- `-` A graph beyond one node's memory needs partitioning, and traversals
  across partitions are a problem of their own. Not designed now; nothing
  here may rule it out.
- `-` Compaction, snapshots and replica bootstrap are Lotbook's to build and
  run; they are explicit commands of the binary (ADR-0010).
