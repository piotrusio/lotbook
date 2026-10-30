# ADR-0012: Lotbook speaks Kafka through one pure-Go client

- **Status:** Accepted
- **Date:** 2026-10-28
- **Supersedes:** —
- **Superseded-by:** —
- **Issue/PR:** —

## Context

The journal is in Kafka (ADR-0009). `ingest` produces into it and answers only
once a request's records are durable; workers consume it as one group and keep
their positions in PostgreSQL, not in Kafka; the life-cycle commands create and
configure its topic (ADR-0011). That is a producer, a consumer group with
positions committed elsewhere, and an admin client — the whole of the protocol
that matters, none of which the standard library speaks.

Every dependency is a decision recorded on its own (ADR-0007).

Weighed and rejected:

- **A client built on librdkafka** (confluent-kafka-go). It needs cgo and a C
  library in every build and every image, and gives up the static binary.
- **Sarama.** Some twenty direct dependencies, among them a Kerberos
  implementation and a metrics library.
- **kafka-go.** Still before its first stable version, with releases months
  apart.
- **Implementing the protocol ourselves.** Its surface — groups, rebalancing,
  idempotent production, admin — is far beyond what a few hundred lines of
  ours could carry safely.

## Decision

**Lotbook uses one pure-Go Kafka client that covers the protocol whole —
production, consumer groups, admin — and depends on nothing but the
compression codecs Kafka itself uses.**

- **Its core and its admin package only.** None of its plugins: tracing goes
  through the client's hooks, written here, like every other instrumentation
  (ADR-0007).
- **The linter allows it** and refuses its plugins; the allowed set of
  ADR-0007 grows by this one entry.

## Consequences

- `+` The binary stays static and cgo-free.
- `+` Three modules join the binary: the client's own protocol messages and
  two compression codecs.
- `-` The client is maintained, in the main, by one author; its health is
  watched like any dependency's, and replacing it is bounded by keeping it
  behind our own producer and consumer.
- `-` Kafka instrumentation is ours to keep in step with OpenTelemetry's
  messaging conventions.
