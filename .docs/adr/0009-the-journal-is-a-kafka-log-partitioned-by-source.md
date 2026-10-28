# ADR-0009: The journal is a Kafka log, partitioned by source

- **Status:** Accepted
- **Date:** 2026-10-28
- **Supersedes:** ADR-0003
- **Superseded-by:** —
- **Issue/PR:** —

## Context

An installation connects hundreds of source systems — ERP, WMS and MES across
a company's sites — and every one of them sends what it says, row changes or
events, into the journal (ADR-0013). Together they write far more than one PostgreSQL primary
takes, and that primary was the limit ADR-0003 named when it put the journal
in PostgreSQL.

What the journal has to keep is the source's order **within one source**: a
document is assembled from one source's complete units, in the order the
source gave them. Nothing orders one source against another. The
one sequence that spans everything belongs to the genealogy events
(ADR-0005), and they are written at a fraction of the journal's rate — only
documents that mean something for a lot produce them.

Weighed and rejected:

- **The journal in one PostgreSQL primary** (ADR-0003). Write throughput is
  bounded by that primary.
- **PostgreSQL sharded by source** — the journal split across several
  clusters. Hundreds of sources make it a fleet of databases to run, and the
  distribution of work between them ours to build.

## Decision

**The journal is a Kafka log, partitioned by source.** All records of one
source go to one partition, in the source's order; partitions are
spread over the brokers and the workers.

- **`ingest` writes to Kafka only**, and answers `202` once Kafka holds the
  batch durably. Nothing is written twice at ingest.
- **Workers share the journal's partitions** as one consumer group: a
  partition is worked by one worker at a time and taken over when it fails.
- **PostgreSQL holds the genealogy events and everything served from them**,
  the documents, the read schema, and the journal position each partition has
  been interpreted up to.
- **Interpretation, the genealogy events it writes, the sequence it advances
  and the journal position it consumed commit in one PostgreSQL
  transaction.** A worker that fails resumes from the position PostgreSQL
  holds; nothing is interpreted twice and nothing is skipped.
- **Kafka is installed with Lotbook through a Kubernetes operator**, as
  PostgreSQL is (ADR-0010), as several brokers with replication.

## Consequences

- `+` The journal takes writes in proportion to its partitions and brokers,
  not to one primary.
- `+` Distribution of work between workers, and taking over a failed one's
  share, come from Kafka's consumer groups, not from code of ours.
- `+` Between the journal and the genealogy there is one transaction, not an
  outbox: the position consumed is committed with what was concluded from it.
- `-` A second stateful system in every installation — at least three brokers
  — operated and upgraded by the installation.
- `-` The largest single source is bound by what one partition and one worker
  take.
- `-` A Kafka client is a new dependency, added by its own ADR (ADR-0007,
  ADR-0012).
- `-` A log that grows for ever has to be kept somewhere other than the
  brokers' disks (ADR-0011).
