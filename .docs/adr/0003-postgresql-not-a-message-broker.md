# ADR-0003: PostgreSQL holds the journal and the genealogy, not a message broker

- **Status:** Accepted
- **Date:** 2026-10-27
- **Supersedes:** —
- **Superseded-by:** —
- **Issue/PR:** —

## Context

Accepted records have to be kept durably and interpreted in order; the
interpretation's results have to reach the graph, the change feed and the SQL
consumers in the same order. A distributed log is the textbook carrier for
that.

Weighed and rejected:

- **Apache Kafka as the journal.** Lotbook needs a relational store anyway —
  for the genealogy that SQL consumers read, for documents, users,
  configuration and audit — so a log would be a second stateful system, not a
  replacement. Keeping the two consistent takes an outbox and idempotent
  consumers on every path, and every divergence is a finding in an audit. It
  is at least three more stateful instances in every installation, and the
  volume of ERP, WMS and MES bookings does not call for it.

Events that already move through a broker still get in: a connector reads
the topics and posts them to the ingest contract.

## Decision

**PostgreSQL is Lotbook's only store of record.** The journal of raw records,
the genealogy and everything served by SQL live in it. Ordering is a
monotonically increasing sequence; work is shared between worker replicas by
key, each replica holding the keys it works on, and a key held by a failed
replica is released and taken over.

**Interpretation, the genealogy it writes and the sequence it advances commit
in one transaction.**

## Consequences

- `+` One stateful dependency; consistency between journal and genealogy is a
  transaction, not a protocol.
- `+` Brokers stay at the edge, as a connector, where an installation has
  them.
- `-` Ordering and work distribution between replicas are ours to build and
  test, where a broker would provide partitions and consumer groups.
- `-` Write throughput is bounded by one primary. At the volumes of
  bookings this is far off; it is the limit to watch.
