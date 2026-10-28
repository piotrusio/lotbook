# ADR-0014: The ingest contract is one endpoint that accepts, persists and answers

- **Status:** Accepted
- **Date:** 2026-10-28
- **Supersedes:** —
- **Superseded-by:** —
- **Issue/PR:** —

## Context

Dozens to hundreds of source systems send their records to Lotbook through
connectors (ADR-0001). The ingest contract is the one inbound interface, public
and versioned from its first release.

`ingest` has one job: take what is sent, make it durable in the journal
(ADR-0009) and say so. What the records mean, in which order they are taken,
whether one arrived twice and whether a unit is complete are decided later, by
the workers, from the positions the records carry (ADR-0013). None of it is
asked of the connector or checked at the door.

Weighed and rejected:

- **An endpoint per source, or per kind of record.** It repeats in the address
  what the record already says, and a new source would need a change in
  Lotbook to be sent at all.
- **Rules on how a connector sends** — one batch at a time, sources kept
  apart, records in order. They would make the contract depend on the
  connector's behaviour for what the workers can work out from the records
  themselves.

## Decision

**One endpoint, `POST /v1/batches`, over HTTPS.** The body is JSON, and may be
sent gzip-compressed.

- **A batch is a list of records** in the envelope of ADR-0013 — source,
  position, unit with its last record marked, kind, content. It may hold
  records of any sources its sender may write to, in any order and of any
  size the endpoint accepts.
- **`ingest` checks two things:** that every record has the contract's shape,
  and that the sender may write to every record's source (ADR-0015). Then it
  writes the batch to the journal.
- **Every batch is kept with the identity that sent it** (ADR-0015) and its
  trace context (ADR-0008).
- **The answers:**
  - `202` — the batch is durable in the journal;
  - `400` — a record does not have the contract's shape; nothing is written;
  - `401` — no valid identity;
  - `403` — a record names a source its sender may not write; nothing is
    written;
  - `503` — the journal did not take the batch; send it again.
- **The version is in the path.** A new version is a new path, served beside
  the old one for as long as connectors use it.

## Consequences

- `+` Every connector sends to the same address in the same shape; a new
  source needs an authorization entry (ADR-0015) and no change to Lotbook.
- `+` `ingest` holds no state and scales by replicas.
- `+` A connector is never told how to send; whatever it does, the journal
  keeps it.
- `-` A batch sent again after a `503` or a lost answer can reach the journal
  twice; the workers drop what they have already taken by its position
  (ADR-0013).
- `-` One malformed record refuses the whole batch it came in.
- open: the record's encoding — how values keep the source's types and
  precision — written down in the contract document with its examples.
