# ADR-0011: The journal is kept for ever, its cold segments in object storage

- **Status:** Accepted
- **Date:** 2026-10-28
- **Supersedes:** —
- **Superseded-by:** —
- **Issue/PR:** —

## Context

The journal is a record and is never dropped (ADR-0004); it lives in Kafka
(ADR-0009). With hundreds of sources writing into it, its history reaches
tens of terabytes a year, and every broker holds three copies of whatever it
keeps.

The history is read rarely and in one way: from an offset onwards, in order —
when a defect is repaired by rebuilding the derived levels or by
interpreting again (ADR-0004), and when an audit follows a provenance down to
the rows. What is read every moment is the head of each partition.

Weighed and rejected:

- **Unlimited retention on the brokers' own disks.** Three copies of the
  whole history on the most expensive storage in the installation, and every
  broker replaced or added copies all of it.
- **An archive written by Lotbook into PostgreSQL.** It brings back into one
  primary the volume the journal was moved to Kafka to escape.
- **An archive written by Lotbook as files on persistent volumes.** A
  replicated store of our own to build and run.
- **An archive written by Lotbook into object storage.** It splits the
  journal in two, and every reader of old history has a second path to
  follow and a seam to get right.

## Decision

**The journal's topic is kept for ever and never compacted, and Kafka's
tiered storage moves its closed segments to object storage** (ADR-0010).

- **Retention is unlimited**, in time and in size. Compaction is off: it keeps
  only the latest value of a key, and would erase the history the record
  is.
- **The brokers keep a hot window on their own disks** — the head of each
  partition, what the workers read in the ordinary course. Older segments
  live in object storage only.
- **Reading old history is reading the topic.** A consumer that seeks to an
  old offset gets it from object storage through Kafka; a rebuild and an
  audit read the journal from offset zero the way a worker reads its head.

## Consequences

- `+` The journal is one log from its first offset to its last, with one way
  to read it.
- `+` The brokers' disks are sized by the hot window, not by the history.
- `+` Adding or replacing a broker copies the hot window only.
- `-` Reading cold history is slower than reading the head, bound by object
  storage; a full rebuild is a long, sequential read.
- `-` The brokers carry a remote storage plugin for the S3 API, which the
  Kafka distribution does not ship; it is part of the broker image the
  installation runs.
- `-` The record's durability past the hot window is the object store's
  (ADR-0010).
